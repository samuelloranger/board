<script>
  let board = $state({ todo: [], in_progress: [], done: [] });
  let events = $state([]);
  let handoffs = $state([]);
  const columns = [["todo", "To-Do"], ["in_progress", "In Progress"], ["done", "Done"]];

  async function load() {
    board = await (await fetch("/api/board?project=*")).json();
    const res = await (await fetch("/api/resume?project=*")).json();
    handoffs = res.handoffs ?? [];
  }
  async function move(id, status) {
    await fetch(`/api/tasks/${id}/move`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ status }),
    });
    await load();
  }
  async function add() {
    const title = prompt("Task title");
    if (!title) return;
    await fetch("/api/tasks", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ title, status: "todo" }),
    });
    await load();
  }
  function onDrop(e, status) {
    const id = e.dataTransfer.getData("id");
    if (id) move(id, status);
  }
  $effect(() => {
    load();
    const es = new EventSource("/api/events?since=0");
    es.onmessage = (m) => {
      events = [JSON.parse(m.data), ...events].slice(0, 50);
      load(); // refresh board + handoffs on any activity
    };
    return () => es.close();
  });
</script>

<header><h1>board</h1><button onclick={add}>+ Task</button></header>

{#if handoffs.length}
  <section class="handoffs">
    <h2>⇄ Handoffs / inbox</h2>
    {#each handoffs as t}
      <div class="handoff" class:human={t.handoff_to === "human"}>
        <b>{t.title}</b> → {t.handoff_to}
        {#if t.handoff_reason}<span>· {t.handoff_reason}</span>{/if}
      </div>
    {/each}
  </section>
{/if}

<div class="cols">
  {#each columns as [key, label]}
    <div class="col" ondragover={(e) => e.preventDefault()} ondrop={(e) => onDrop(e, key)}>
      <h2>{label} ({board[key]?.length ?? 0})</h2>
      {#each board[key] ?? [] as t}
        <div class="card" draggable="true" ondragstart={(e) => e.dataTransfer.setData("id", t.id)}>
          {t.title}
          {#if t.handoff_to}<em class="tag">⇄ {t.handoff_to}</em>{/if}
        </div>
      {/each}
    </div>
  {/each}
</div>

<aside class="activity">
  <h2>Activity</h2>
  {#each events as e}
    <div class="ev"><code>{e.kind}</code> {e.detail}</div>
  {/each}
</aside>

<style>
  header { display: flex; gap: 1rem; align-items: center; padding: 1rem; font-family: system-ui; }
  .cols { display: grid; grid-template-columns: repeat(3, 1fr); gap: 1rem; padding: 1rem; }
  .col { background: #f4f4f5; border-radius: 8px; padding: 0.5rem; min-height: 300px; }
  .card { background: white; border-radius: 6px; padding: 0.6rem; margin: 0.4rem 0; box-shadow: 0 1px 2px rgba(0,0,0,.1); cursor: grab; }
  .tag { font-size: 0.7rem; color: #6b7280; margin-left: 0.3rem; }
  .handoffs { margin: 0 1rem; padding: 0.6rem; background: #fef3c7; border-radius: 8px; }
  .handoff.human { font-weight: 600; }
  .activity { position: fixed; right: 0; top: 0; width: 240px; height: 100vh; overflow: auto; background: #fafafa; border-left: 1px solid #e5e7eb; padding: 0.6rem; font: 0.75rem system-ui; }
  .ev { padding: 0.2rem 0; border-bottom: 1px solid #eee; }
  h2 { font-size: 0.9rem; font-family: system-ui; }
</style>
