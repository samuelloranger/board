<script>
  const COLUMNS = [
    { key: "todo", label: "To-Do", dot: "var(--todo)" },
    { key: "in_progress", label: "In Progress", dot: "var(--prog)" },
    { key: "done", label: "Done", dot: "var(--done)" },
  ];
  const PRIORITIES = [
    { key: "", label: "None" },
    { key: "low", label: "Low" },
    { key: "medium", label: "Med" },
    { key: "high", label: "High" },
  ];

  let board = $state({ todo: [], in_progress: [], done: [] });
  let events = $state([]);
  let handoffs = $state([]);
  let theme = $state("dark");
  let activeCol = $state("todo");
  let showActivity = $state(false);
  let showAdd = $state(false);
  let openMenu = $state(null);
  let dragOver = $state(null);
  let addTitle = $state("");
  let addPriority = $state("");
  let unseen = $state(0);

  function applyTheme(t) {
    theme = t;
    document.documentElement.setAttribute("data-theme", t);
    try { localStorage.setItem("board-theme", t); } catch {}
  }
  function toggleTheme() { applyTheme(theme === "dark" ? "light" : "dark"); }

  async function load() {
    board = await (await fetch("/api/board?project=*")).json();
    const res = await (await fetch("/api/resume?project=*")).json();
    handoffs = res.handoffs ?? [];
  }
  async function move(id, status) {
    openMenu = null;
    await fetch(`/api/tasks/${id}/move`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status }),
    });
    await load();
  }
  async function archive(id) {
    openMenu = null;
    await fetch(`/api/tasks/${id}/archive`, { method: "POST" });
    await load();
  }
  async function createTask() {
    const title = addTitle.trim();
    if (!title) return;
    await fetch("/api/tasks", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title, status: "todo", priority: addPriority }),
    });
    addTitle = ""; addPriority = ""; showAdd = false;
    await load();
  }

  function onDragStart(e, id) { e.dataTransfer.setData("id", String(id)); e.dataTransfer.effectAllowed = "move"; }
  function onDrop(e, status) {
    e.preventDefault(); dragOver = null;
    const id = e.dataTransfer.getData("id");
    if (id) move(Number(id), status);
  }

  function fmtTime(iso) {
    try { return new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }); }
    catch { return ""; }
  }
  const eventKindLabel = { created: "created", moved: "moved", note: "note", handoff: "handoff", archived: "archived", unarchived: "restored", updated: "updated", deleted: "deleted", tool: "tool", session: "session" };

  $effect(() => {
    let saved = "dark";
    try { saved = localStorage.getItem("board-theme") || (matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark"); } catch {}
    applyTheme(saved);
    load();
    const es = new EventSource("/api/events?since=0");
    es.onmessage = (m) => {
      events = [JSON.parse(m.data), ...events].slice(0, 60);
      if (!showActivity) unseen = Math.min(unseen + 1, 99);
      load();
    };
    return () => es.close();
  });

  function openActivity() { showActivity = true; unseen = 0; }
</script>

<!-- icon snippets -->
{#snippet iconPlus()}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg>{/snippet}
{#snippet iconActivity()}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 12h-4l-3 9L9 3l-3 9H2"/></svg>{/snippet}
{#snippet iconSun()}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>{/snippet}
{#snippet iconMoon()}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z"/></svg>{/snippet}
{#snippet iconMore()}<svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="5" r="2"/><circle cx="12" cy="12" r="2"/><circle cx="12" cy="19" r="2"/></svg>{/snippet}
{#snippet iconHandoff()}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M7 8l-4 4 4 4M3 12h13M17 16l4-4-4-4M21 12H8"/></svg>{/snippet}
{#snippet iconClose()}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M18 6 6 18M6 6l12 12"/></svg>{/snippet}
{#snippet iconCheck()}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>{/snippet}

<header class="topbar">
  <div class="brand">
    <span class="logo" aria-hidden="true"></span>
    <h1>board</h1>
  </div>
  <div class="actions">
    <button class="icon-btn" onclick={openActivity} aria-label="Show activity">
      {@render iconActivity()}
      {#if unseen > 0}<span class="badge">{unseen}</span>{/if}
    </button>
    <button class="icon-btn" onclick={toggleTheme} aria-label="Toggle theme">
      {#if theme === "dark"}{@render iconSun()}{:else}{@render iconMoon()}{/if}
    </button>
    <button class="btn-primary" onclick={() => (showAdd = true)}>
      {@render iconPlus()}<span>Task</span>
    </button>
  </div>
</header>

{#if handoffs.length}
  <section class="handoff-lane" aria-label="Handoffs and inbox">
    <div class="lane-head">{@render iconHandoff()}<span>Handoffs</span></div>
    <div class="lane-scroll">
      {#each handoffs as t (t.id)}
        <div class="handoff-chip" class:human={t.handoff_to === "human"}>
          <span class="to">{t.handoff_to}</span>
          <span class="ht">{t.title}</span>
          {#if t.handoff_reason}<span class="hr">{t.handoff_reason}</span>{/if}
        </div>
      {/each}
    </div>
  </section>
{/if}

<nav class="segmented" aria-label="Select column">
  {#each COLUMNS as c (c.key)}
    <button class:active={activeCol === c.key} onclick={() => (activeCol = c.key)}>
      {c.label}<span class="seg-count">{board[c.key]?.length ?? 0}</span>
    </button>
  {/each}
</nav>

<main class="board">
  {#each COLUMNS as c (c.key)}
    <section
      class="col"
      class:active={activeCol === c.key}
      class:drop={dragOver === c.key}
      ondragover={(e) => { e.preventDefault(); dragOver = c.key; }}
      ondragleave={() => { if (dragOver === c.key) dragOver = null; }}
      ondrop={(e) => onDrop(e, c.key)}
    >
      <div class="col-head">
        <span class="col-dot" style="background:{c.dot}"></span>
        <h2>{c.label}</h2>
        <span class="count">{board[c.key]?.length ?? 0}</span>
      </div>
      <div class="cards">
        {#each board[c.key] ?? [] as t (t.id)}
          <article
            class="card"
            draggable="true"
            ondragstart={(e) => onDragStart(e, t.id)}
          >
            <div class="card-top">
              <p class="title">{t.title}</p>
              <button class="menu-btn" aria-label="Task actions" onclick={() => (openMenu = openMenu === t.id ? null : t.id)}>
                {@render iconMore()}
              </button>
            </div>
            {#if t.priority || (t.tags && t.tags.length) || t.handoff_to}
              <div class="meta">
                {#if t.priority}<span class="pri pri-{t.priority}">{t.priority}</span>{/if}
                {#each t.tags ?? [] as tag}<span class="tag">{tag}</span>{/each}
                {#if t.handoff_to}<span class="hbadge">{@render iconHandoff()}{t.handoff_to}</span>{/if}
              </div>
            {/if}
            {#if openMenu === t.id}
              <div class="menu" role="menu">
                {#each COLUMNS.filter((x) => x.key !== t.status) as m}
                  <button role="menuitem" onclick={() => move(t.id, m.key)}>Move to {m.label}</button>
                {/each}
                <button role="menuitem" class="danger" onclick={() => archive(t.id)}>Archive</button>
              </div>
            {/if}
          </article>
        {/each}
        {#if (board[c.key]?.length ?? 0) === 0}
          <div class="empty">Nothing here</div>
        {/if}
      </div>
    </section>
  {/each}
</main>

{#if openMenu !== null}
  <button class="backdrop-invisible" aria-label="Close menu" onclick={() => (openMenu = null)}></button>
{/if}

<!-- Add task modal -->
{#if showAdd}
  <div class="scrim" onclick={() => (showAdd = false)}></div>
  <div class="modal" role="dialog" aria-modal="true" aria-label="New task">
    <div class="modal-head">
      <h3>New task</h3>
      <button class="icon-btn sm" aria-label="Close" onclick={() => (showAdd = false)}>{@render iconClose()}</button>
    </div>
    <label class="field">
      <span>Title</span>
      <!-- svelte-ignore a11y_autofocus -->
      <input
        autofocus
        bind:value={addTitle}
        placeholder="What needs doing?"
        onkeydown={(e) => { if (e.key === "Enter") createTask(); if (e.key === "Escape") showAdd = false; }}
      />
    </label>
    <div class="field">
      <span>Priority</span>
      <div class="pri-seg">
        {#each PRIORITIES as p}
          <button class:sel={addPriority === p.key} onclick={() => (addPriority = p.key)}>{p.label}</button>
        {/each}
      </div>
    </div>
    <div class="modal-foot">
      <button class="btn-ghost" onclick={() => (showAdd = false)}>Cancel</button>
      <button class="btn-primary" disabled={!addTitle.trim()} onclick={createTask}>{@render iconCheck()}<span>Create</span></button>
    </div>
  </div>
{/if}

<!-- Activity drawer -->
{#if showActivity}
  <div class="scrim" onclick={() => (showActivity = false)}></div>
  <aside class="activity" role="dialog" aria-label="Activity feed">
    <div class="activity-head">
      <div class="live">{@render iconActivity()}<span>Activity</span></div>
      <button class="icon-btn sm" aria-label="Close activity" onclick={() => (showActivity = false)}>{@render iconClose()}</button>
    </div>
    <div class="feed">
      {#if events.length === 0}<div class="empty">No activity yet</div>{/if}
      {#each events as e (e.id)}
        <div class="ev">
          <span class="ev-kind k-{e.kind}">{eventKindLabel[e.kind] ?? e.kind}</span>
          <span class="ev-detail">{e.detail}</span>
          <span class="ev-time">{fmtTime(e.created_at)}</span>
        </div>
      {/each}
    </div>
  </aside>
{/if}

<style>
  :global(:root) {
    --bg: #0f172a; --surface: #1e293b; --surface-2: #172033; --surface-3: #232f45;
    --text: #f8fafc; --muted: #94a3b8; --border: #334155;
    --accent: #22c55e; --accent-fg: #052e13; --danger: #ef4444; --amber: #f59e0b;
    --todo: #64748b; --prog: #3b82f6; --done: #22c55e;
    --radius: 12px; --shadow: 0 1px 2px rgba(0,0,0,.4), 0 4px 16px rgba(0,0,0,.25);
    --font: "Inter", ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, sans-serif;
  }
  :global(:root[data-theme="light"]) {
    --bg: #f1f5f9; --surface: #ffffff; --surface-2: #f8fafc; --surface-3: #eef2f7;
    --text: #0f172a; --muted: #64748b; --border: #e2e8f0;
    --accent: #16a34a; --accent-fg: #ffffff; --danger: #dc2626; --amber: #d97706;
    --todo: #94a3b8; --prog: #3b82f6; --done: #16a34a;
    --shadow: 0 1px 2px rgba(15,23,42,.06), 0 4px 16px rgba(15,23,42,.08);
  }
  :global(html), :global(body) { margin: 0; background: var(--bg); }
  :global(*) { box-sizing: border-box; }
  :global(#app) {
    font-family: var(--font); color: var(--text); background: var(--bg);
    min-height: 100dvh; -webkit-font-smoothing: antialiased;
    padding-bottom: env(safe-area-inset-bottom);
  }

  .topbar {
    position: sticky; top: 0; z-index: 20;
    display: flex; align-items: center; justify-content: space-between;
    gap: 12px; padding: 12px 16px; padding-top: calc(12px + env(safe-area-inset-top));
    background: color-mix(in srgb, var(--bg) 85%, transparent);
    backdrop-filter: blur(12px); border-bottom: 1px solid var(--border);
  }
  .brand { display: flex; align-items: center; gap: 10px; }
  .logo { width: 22px; height: 22px; border-radius: 7px; background: linear-gradient(135deg, var(--accent), var(--prog)); box-shadow: 0 0 0 1px color-mix(in srgb, var(--accent) 40%, transparent); }
  .brand h1 { font-size: 18px; font-weight: 700; margin: 0; letter-spacing: -.02em; }
  .actions { display: flex; align-items: center; gap: 8px; }

  .icon-btn {
    position: relative; display: inline-grid; place-items: center;
    width: 40px; height: 40px; border-radius: 10px; border: 1px solid var(--border);
    background: var(--surface); color: var(--text); cursor: pointer;
    transition: background .12s ease, transform .08s ease, border-color .12s ease;
  }
  .icon-btn.sm { width: 34px; height: 34px; }
  .icon-btn:hover { background: var(--surface-3); }
  .icon-btn:active { transform: scale(.94); }
  .icon-btn svg { width: 19px; height: 19px; }
  .badge {
    position: absolute; top: -4px; right: -4px; min-width: 17px; height: 17px; padding: 0 4px;
    display: grid; place-items: center; font-size: 10px; font-weight: 700;
    background: var(--accent); color: var(--accent-fg); border-radius: 9px;
  }

  .btn-primary {
    display: inline-flex; align-items: center; gap: 6px; height: 40px; padding: 0 14px;
    border: none; border-radius: 10px; background: var(--accent); color: var(--accent-fg);
    font-family: inherit; font-size: 14px; font-weight: 600; cursor: pointer;
    transition: filter .12s ease, transform .08s ease;
  }
  .btn-primary svg { width: 17px; height: 17px; }
  .btn-primary:hover { filter: brightness(1.06); }
  .btn-primary:active { transform: scale(.97); }
  .btn-primary:disabled { opacity: .5; cursor: not-allowed; }
  .btn-ghost {
    height: 40px; padding: 0 14px; border: 1px solid var(--border); border-radius: 10px;
    background: transparent; color: var(--text); font-family: inherit; font-size: 14px; font-weight: 500; cursor: pointer;
    transition: background .12s ease;
  }
  .btn-ghost:hover { background: var(--surface-2); }

  /* Handoff lane */
  .handoff-lane { padding: 12px 16px 0; }
  .lane-head { display: flex; align-items: center; gap: 6px; font-size: 12px; font-weight: 600; color: var(--amber); text-transform: uppercase; letter-spacing: .04em; margin-bottom: 8px; }
  .lane-head svg { width: 15px; height: 15px; }
  .lane-scroll { display: flex; gap: 10px; overflow-x: auto; padding-bottom: 4px; scrollbar-width: thin; }
  .handoff-chip {
    flex: 0 0 auto; max-width: 260px; padding: 8px 12px; border-radius: 10px;
    background: color-mix(in srgb, var(--amber) 12%, var(--surface));
    border: 1px solid color-mix(in srgb, var(--amber) 40%, var(--border));
    display: flex; flex-direction: column; gap: 2px;
  }
  .handoff-chip .to { font-size: 11px; font-weight: 700; color: var(--amber); text-transform: uppercase; }
  .handoff-chip .ht { font-size: 13px; font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .handoff-chip .hr { font-size: 12px; color: var(--muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .handoff-chip.human { box-shadow: 0 0 0 1px var(--amber); }

  /* Segmented (mobile) */
  .segmented {
    display: flex; gap: 4px; margin: 12px 16px; padding: 4px;
    background: var(--surface-2); border: 1px solid var(--border); border-radius: 12px;
  }
  .segmented button {
    flex: 1; display: inline-flex; align-items: center; justify-content: center; gap: 6px;
    min-height: 40px; border: none; border-radius: 9px; background: transparent;
    color: var(--muted); font-family: inherit; font-size: 13px; font-weight: 600; cursor: pointer;
    transition: background .15s ease, color .15s ease;
  }
  .segmented button.active { background: var(--surface); color: var(--text); box-shadow: var(--shadow); }
  .seg-count { font-size: 11px; font-weight: 700; padding: 1px 6px; border-radius: 999px; background: var(--surface-3); font-variant-numeric: tabular-nums; }

  /* Board */
  .board { padding: 0 16px 24px; }
  .col { display: none; flex-direction: column; }
  .col.active { display: flex; }
  .col-head { display: flex; align-items: center; gap: 8px; padding: 6px 2px 12px; }
  .col-dot { width: 9px; height: 9px; border-radius: 50%; }
  .col-head h2 { margin: 0; font-size: 14px; font-weight: 600; letter-spacing: -.01em; }
  .col-head .count { margin-left: auto; font-size: 12px; font-weight: 600; color: var(--muted); font-variant-numeric: tabular-nums; }
  .cards { display: flex; flex-direction: column; gap: 10px; min-height: 60px; }

  .card {
    position: relative; padding: 12px 12px 12px 14px; border-radius: var(--radius);
    background: var(--surface); border: 1px solid var(--border); box-shadow: var(--shadow);
    cursor: grab; transition: transform .1s ease, border-color .12s ease, box-shadow .12s ease;
  }
  .card:hover { border-color: color-mix(in srgb, var(--accent) 45%, var(--border)); transform: translateY(-1px); }
  .card:active { cursor: grabbing; }
  .card-top { display: flex; align-items: flex-start; gap: 8px; }
  .title { margin: 0; font-size: 14px; font-weight: 500; line-height: 1.45; flex: 1; word-break: break-word; }
  .menu-btn {
    flex: 0 0 auto; width: 30px; height: 30px; margin: -4px -4px 0 0; border: none; border-radius: 8px;
    background: transparent; color: var(--muted); cursor: pointer; display: grid; place-items: center;
    transition: background .12s ease, color .12s ease;
  }
  .menu-btn:hover { background: var(--surface-3); color: var(--text); }
  .menu-btn svg { width: 16px; height: 16px; }

  .meta { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 10px; }
  .pri { font-size: 11px; font-weight: 700; padding: 2px 8px; border-radius: 999px; text-transform: capitalize; }
  .pri-high { color: var(--danger); background: color-mix(in srgb, var(--danger) 15%, transparent); }
  .pri-medium { color: var(--amber); background: color-mix(in srgb, var(--amber) 15%, transparent); }
  .pri-low { color: var(--muted); background: color-mix(in srgb, var(--muted) 18%, transparent); }
  .tag { font-size: 11px; font-weight: 500; padding: 2px 8px; border-radius: 999px; color: var(--muted); background: var(--surface-2); border: 1px solid var(--border); }
  .hbadge { display: inline-flex; align-items: center; gap: 4px; font-size: 11px; font-weight: 600; padding: 2px 8px; border-radius: 999px; color: var(--amber); background: color-mix(in srgb, var(--amber) 13%, transparent); }
  .hbadge svg { width: 12px; height: 12px; }

  .menu {
    position: absolute; top: 40px; right: 10px; z-index: 30; min-width: 168px; padding: 6px;
    background: var(--surface); border: 1px solid var(--border); border-radius: 10px;
    box-shadow: 0 8px 28px rgba(0,0,0,.35); display: flex; flex-direction: column; gap: 2px;
    animation: pop .12s ease;
  }
  .menu button {
    text-align: left; padding: 9px 10px; min-height: 40px; border: none; border-radius: 7px;
    background: transparent; color: var(--text); font-family: inherit; font-size: 13px; cursor: pointer;
  }
  .menu button:hover { background: var(--surface-2); }
  .menu button.danger { color: var(--danger); }
  .backdrop-invisible { position: fixed; inset: 0; z-index: 25; background: transparent; border: none; cursor: default; }

  .empty { padding: 20px; text-align: center; font-size: 13px; color: var(--muted); border: 1px dashed var(--border); border-radius: var(--radius); }

  /* Modal */
  .scrim { position: fixed; inset: 0; z-index: 40; background: rgba(2,6,23,.55); backdrop-filter: blur(2px); animation: fade .15s ease; }
  .modal {
    position: fixed; z-index: 50; left: 50%; top: 50%; transform: translate(-50%, -50%);
    width: min(440px, calc(100vw - 32px)); padding: 20px;
    background: var(--surface); border: 1px solid var(--border); border-radius: 16px; box-shadow: 0 20px 60px rgba(0,0,0,.5);
    animation: pop .16s ease;
  }
  .modal-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 16px; }
  .modal-head h3 { margin: 0; font-size: 17px; font-weight: 700; }
  .field { display: block; margin-bottom: 16px; }
  .field > span { display: block; font-size: 12px; font-weight: 600; color: var(--muted); margin-bottom: 6px; }
  .field input {
    width: 100%; height: 44px; padding: 0 12px; border-radius: 10px; border: 1px solid var(--border);
    background: var(--surface-2); color: var(--text); font-family: inherit; font-size: 15px;
    transition: border-color .12s ease, box-shadow .12s ease;
  }
  .field input:focus { outline: none; border-color: var(--accent); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 25%, transparent); }
  .pri-seg { display: flex; gap: 4px; padding: 4px; background: var(--surface-2); border: 1px solid var(--border); border-radius: 10px; }
  .pri-seg button { flex: 1; min-height: 36px; border: none; border-radius: 7px; background: transparent; color: var(--muted); font-family: inherit; font-size: 13px; font-weight: 600; cursor: pointer; transition: background .12s ease, color .12s ease; }
  .pri-seg button.sel { background: var(--surface); color: var(--text); box-shadow: var(--shadow); }
  .modal-foot { display: flex; justify-content: flex-end; gap: 8px; margin-top: 4px; }

  /* Activity drawer */
  .activity {
    position: fixed; z-index: 50; right: 0; top: 0; bottom: 0; width: min(360px, 100vw);
    background: var(--surface); border-left: 1px solid var(--border);
    display: flex; flex-direction: column; animation: slide .2s ease;
    padding-top: env(safe-area-inset-top);
  }
  .activity-head { display: flex; align-items: center; justify-content: space-between; padding: 14px 16px; border-bottom: 1px solid var(--border); }
  .live { display: flex; align-items: center; gap: 8px; font-size: 14px; font-weight: 700; }
  .live svg { width: 17px; height: 17px; color: var(--accent); }
  .feed { flex: 1; overflow-y: auto; padding: 10px 12px; display: flex; flex-direction: column; gap: 2px; }
  .ev { display: grid; grid-template-columns: auto 1fr auto; align-items: center; gap: 8px; padding: 8px 8px; border-radius: 8px; }
  .ev:hover { background: var(--surface-2); }
  .ev-kind { font-size: 10px; font-weight: 700; text-transform: uppercase; letter-spacing: .03em; padding: 2px 7px; border-radius: 6px; background: var(--surface-3); color: var(--muted); }
  .k-created { color: var(--done); background: color-mix(in srgb, var(--done) 15%, transparent); }
  .k-moved { color: var(--prog); background: color-mix(in srgb, var(--prog) 15%, transparent); }
  .k-handoff { color: var(--amber); background: color-mix(in srgb, var(--amber) 15%, transparent); }
  .k-deleted, .k-archived { color: var(--danger); background: color-mix(in srgb, var(--danger) 13%, transparent); }
  .ev-detail { font-size: 13px; color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .ev-time { font-size: 11px; color: var(--muted); font-variant-numeric: tabular-nums; }

  @keyframes pop { from { opacity: 0; transform: translate(-50%, -50%) scale(.96); } }
  @keyframes fade { from { opacity: 0; } }
  @keyframes slide { from { transform: translateX(100%); } }
  .menu { transform-origin: top right; }
  @keyframes popmenu { from { opacity: 0; transform: scale(.95); } }

  /* Desktop */
  @media (min-width: 768px) {
    .segmented { display: none; }
    .board { display: grid; grid-template-columns: repeat(3, 1fr); gap: 16px; max-width: 1200px; margin: 0 auto; padding: 8px 24px 32px; }
    .col { display: flex; background: var(--surface-2); border: 1px solid var(--border); border-radius: 14px; padding: 12px; }
    .col.drop { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 8%, var(--surface-2)); }
    .col-head { padding: 4px 4px 12px; }
    .handoff-lane, .topbar { padding-left: 24px; padding-right: 24px; }
    .modal { animation: pop .16s ease; }
  }
  @media (min-width: 1200px) {
    .topbar { padding-left: max(24px, calc((100vw - 1200px) / 2)); padding-right: max(24px, calc((100vw - 1200px) / 2)); }
  }

  @media (prefers-reduced-motion: reduce) {
    * { animation: none !important; transition: none !important; }
  }
</style>
