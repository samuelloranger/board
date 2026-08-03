<script>
  import { renderMd } from "./md.js";

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
  const AGENTS = [
    { key: "cursor", label: "Cursor" },
    { key: "claude", label: "Claude" },
    { key: "codex", label: "Codex" },
  ];
  const AGENT_LABEL = Object.fromEntries(AGENTS.map((a) => [a.key, a.label]));

  let board = $state({ todo: [], in_progress: [], done: [] });
  let events = $state([]);
  let handoffs = $state([]);
  let theme = $state("dark");
  let activeCol = $state("todo");
  let showActivity = $state(false);
  let showAdd = $state(false);
  let openMenu = $state(null);
  let openAgentMenu = $state(null); // task id | "detail" | null
  let dragOver = $state(null);
  let addTitle = $state("");
  let addDescription = $state("");
  let addPriority = $state("");
  let addProject = $state("");
  let unseen = $state(0);
  let detail = $state(null);
  let editing = $state(false);
  let edit = $state({ title: "", description: "", priority: "", due_date: "", tags: "", project: "" });
  let noteBody = $state("");
  let seenNoteIds = $state(new Set());
  let animNoteIds = $state({}); // id -> true while entrance animation plays
  let pendingQuestions = $state([]);
  let latestByTask = $state({}); // task_id -> latest run
  let startingIds = $state({}); // task_id -> true while POST /run in flight
  let projectPaths = $state([]);
  let chipProjects = $state([]); // names from board ∪ paths; "_" = global
  let showProjects = $state(false);
  let needPath = $state(null); // { project, taskId }
  let pathInput = $state("");
  let runError = $state("");
  let answerDraft = $state("");
  let projectFilter = $state("*"); // "*" = All
  let showNotifyBanner = $state(false);
  let selectedAgent = $state(loadSelectedAgent());

  function loadSelectedAgent() {
    try {
      const v = localStorage.getItem("board-default-agent");
      if (v && AGENT_LABEL[v]) return v;
    } catch {}
    return "cursor";
  }
  function setSelectedAgent(key) {
    if (!AGENT_LABEL[key]) return;
    selectedAgent = key;
    openAgentMenu = null;
    try { localStorage.setItem("board-default-agent", key); } catch {}
  }
  function agentLabel(key) {
    return AGENT_LABEL[key] || key || "Agent";
  }

  function applyTheme(t) {
    theme = t;
    document.documentElement.setAttribute("data-theme", t);
    try { localStorage.setItem("board-theme", t); } catch {}
  }
  function toggleTheme() { applyTheme(theme === "dark" ? "light" : "dark"); }

  function loadProjectFilter() {
    try {
      const v = localStorage.getItem("board-project-filter");
      if (v) projectFilter = v;
    } catch {}
  }
  function setProjectFilter(v) {
    projectFilter = v;
    try { localStorage.setItem("board-project-filter", v); } catch {}
    load();
  }

  function deriveChipProjects(b, paths) {
    const named = new Set();
    let hasGlobal = false;
    for (const col of COLUMNS) {
      for (const t of b?.[col.key] ?? []) {
        if (t.project) named.add(t.project);
        else hasGlobal = true;
      }
    }
    for (const pp of paths ?? []) {
      if (pp.project && pp.project !== "_") named.add(pp.project);
    }
    const list = [...named].sort((a, b) => a.localeCompare(b));
    if (hasGlobal) list.unshift("_");
    return list;
  }

  async function load() {
    // Full board first so chips cover every project with tasks (not just paths).
    const allBoard = await (await fetch("/api/board?project=*")).json();
    await loadAgentState();
    chipProjects = deriveChipProjects(allBoard, projectPaths);

    if (projectFilter !== "*" && !chipProjects.includes(projectFilter)) {
      projectFilter = "*";
      try { localStorage.setItem("board-project-filter", "*"); } catch {}
    }

    const q = projectFilter === "*" ? "*" : projectFilter;
    board = q === "*" ? allBoard : await (await fetch(`/api/board?project=${encodeURIComponent(q)}`)).json();
    const res = await (await fetch(`/api/resume?project=${encodeURIComponent(q)}`)).json();
    handoffs = res.handoffs ?? [];
  }
  async function loadAgentState() {
    pendingQuestions = await (await fetch("/api/questions?status=pending")).json();
    const runs = await (await fetch("/api/runs")).json();
    const map = {};
    for (const r of runs ?? []) {
      if (map[r.task_id] == null) map[r.task_id] = r; // ListRuns is id DESC
    }
    latestByTask = map;
    projectPaths = await (await fetch("/api/projects/paths")).json();
  }
  function latestRun(id) {
    return latestByTask[id] ?? null;
  }
  function taskHasPending(id) {
    return pendingQuestions.some((q) => q.task_id === id);
  }
  function isStarting(id) {
    return !!startingIds[id];
  }
  function isWorking(id) {
    return isStarting(id) || latestRun(id)?.status === "running";
  }
  function runStatusLabel(id) {
    if (isStarting(id)) return "Starting…";
    const r = latestRun(id);
    if (!r) return "";
    switch (r.status) {
      case "running": return "Working…";
      case "exited": return "Done";
      case "failed": return "Failed";
      case "killed": return "Cancelled";
      default: return r.status;
    }
  }
  function waitStatusLabel(id) {
    if (pendingAskFor(id)) return "Waiting on you";
    const r = latestRun(id);
    if (r?.status === "running" && r.wait === "ci") return "Waiting on CI";
    return runStatusLabel(id);
  }
  function waitStatusClass(id) {
    if (pendingAskFor(id)) return "waiting-you";
    const r = latestRun(id);
    if (r?.status === "running" && r.wait === "ci") return "waiting-ci";
    if (isStarting(id)) return "starting";
    return r?.status || "";
  }
  function noteAuthorLabel(author) {
    if (author === "human") return "You";
    if (author === "agent") return "Agent";
    return "Note";
  }
  function pendingAskFor(id) {
    if (id == null) return null;
    return pendingQuestions.find((q) => q.task_id === id) ?? null;
  }
  function maybeShowNotifyBanner() {
    if (typeof Notification === "undefined") return;
    if (Notification.permission !== "default") return;
    try {
      if (localStorage.getItem("board-notify-ask-dismissed") === "1") return;
    } catch {}
    showNotifyBanner = true;
  }
  async function enableAskNotify() {
    showNotifyBanner = false;
    if (typeof Notification === "undefined") return;
    await Notification.requestPermission();
  }
  function dismissNotifyBanner() {
    showNotifyBanner = false;
    try { localStorage.setItem("board-notify-ask-dismissed", "1"); } catch {}
  }
  function notifyAsk(taskId, question) {
    if (typeof Notification === "undefined") return;
    if (Notification.permission !== "granted") return;
    if (!document.hidden && detail?.id === taskId) return;
    const n = new Notification("Agent asks", {
      body: String(question || "").slice(0, 80),
      tag: "board-ask-" + taskId,
    });
    n.onclick = () => {
      window.focus();
      openDetail(findTask(taskId) ?? { id: taskId });
      n.close();
    };
  }
  function eventVisible(e) {
    if (projectFilter === "*") return true;
    if (!e.task_id) return true;
    const t = findTask(e.task_id);
    if (!t) return true;
    return (t.project || "") === projectFilter || (projectFilter === "_" && !t.project);
  }
  async function runTask(id, agent = selectedAgent) {
    runError = "";
    needPath = null;
    setSelectedAgent(agent);
    startingIds = { ...startingIds, [id]: true };
    try {
      const resp = await fetch(`/api/tasks/${id}/run`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ agent }),
      });
      if (resp.status === 409) {
        const body = await resp.json();
        if (body.need_path) {
          needPath = { project: body.project, taskId: id };
          pathInput = "";
          if (!detail || detail.id !== id) {
            const t = findTask(id);
            if (t) openDetail(t);
          }
          return;
        }
        runError = body.error || "Conflict";
        return;
      }
      if (!resp.ok) {
        runError = await resp.text();
        return;
      }
      await load();
      if (detail?.id === id) await refreshDetail();
    } finally {
      const next = { ...startingIds };
      delete next[id];
      startingIds = next;
    }
  }
  async function savePathAndRun() {
    if (!needPath || !pathInput.trim()) return;
    const put = await fetch(`/api/projects/${encodeURIComponent(needPath.project)}/path`, {
      method: "PUT", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path: pathInput.trim() }),
    });
    if (!put.ok) {
      runError = await put.text();
      return;
    }
    const tid = needPath.taskId;
    needPath = null;
    await runTask(tid);
  }
  async function cancelRun(id) {
    await fetch(`/api/tasks/${id}/run/cancel`, { method: "POST" });
    await load();
    if (detail?.id === id) await refreshDetail();
  }
  async function submitAnswer(qid) {
    const answer = answerDraft.trim();
    if (!answer) return;
    await fetch(`/api/questions/${qid}/answer`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ answer }),
    });
    answerDraft = "";
    await loadAgentState();
  }
  async function saveProjectPath(project, path) {
    const put = await fetch(`/api/projects/${encodeURIComponent(project)}/path`, {
      method: "PUT", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ path }),
    });
    if (!put.ok) {
      runError = await put.text();
      return;
    }
    await loadAgentState();
  }
  async function clearProjectPath(project) {
    await fetch(`/api/projects/${encodeURIComponent(project)}/path`, { method: "DELETE" });
    await loadAgentState();
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
  async function clearHandoff(id, e) {
    e?.stopPropagation();
    await fetch(`/api/tasks/${id}/clear_handoff`, { method: "POST" });
    await load();
    if (detail?.id === id) await refreshDetail();
  }
  async function createTask() {
    const title = addTitle.trim();
    if (!title) return;
    const project = addProject.trim();
    const description = addDescription.trim();
    await fetch("/api/tasks", {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        title, status: "todo", priority: addPriority,
        ...(description ? { description } : {}),
        ...(project ? { project } : {}),
      }),
    });
    addTitle = ""; addDescription = ""; addPriority = ""; addProject = ""; showAdd = false;
    await load();
  }

  function projectLabel(p) {
    return p && String(p).trim() ? p : "global";
  }

  function findTask(id) {
    for (const c of COLUMNS) {
      const f = (board[c.key] ?? []).find((t) => t.id === id);
      if (f) return f;
    }
    return null;
  }
  function taskIdFromURL() {
    const id = Number(new URLSearchParams(location.search).get("task"));
    return Number.isFinite(id) && id > 0 ? id : null;
  }
  function setTaskURL(id, { replace = false } = {}) {
    const url = new URL(location.href);
    if (id == null) url.searchParams.delete("task");
    else url.searchParams.set("task", String(id));
    const next = url.pathname + url.search + url.hash;
    const cur = location.pathname + location.search + location.hash;
    if (next === cur) return;
    if (replace) history.replaceState({ task: id }, "", next);
    else history.pushState({ task: id }, "", next);
  }
  function statusLabel(s) { return (COLUMNS.find((c) => c.key === s) || {}).label ?? s; }
  async function fetchTask(id) {
    const resp = await fetch(`/api/tasks/${id}`);
    if (!resp.ok) return null;
    return await resp.json();
  }
  async function openDetail(t, { skipURL = false, replaceURL = false } = {}) {
    editing = false; noteBody = ""; runError = ""; needPath = null;
    animNoteIds = {};
    detail = t; // show immediately from board card
    if (!skipURL) {
      if (taskIdFromURL() === t.id || replaceURL) setTaskURL(t.id, { replace: true });
      else setTaskURL(t.id);
    }
    const full = await fetchTask(t.id);
    if (full && detail?.id === t.id) {
      detail = full;
      seenNoteIds = new Set((full.notes ?? []).map((n) => n.id));
    } else if (!full && detail?.id === t.id && !t.title) {
      // Deep-linked to a missing/archived task — clear URL, close.
      closeDetail({ skipURL: false });
      return;
    } else {
      seenNoteIds = new Set();
    }
  }
  function applyNoteEntrance(notes) {
    const ids = (notes ?? []).map((n) => n.id);
    const fresh = ids.filter((id) => !seenNoteIds.has(id));
    seenNoteIds = new Set([...seenNoteIds, ...ids]);
    if (!fresh.length) return;
    const next = { ...animNoteIds };
    for (const id of fresh) next[id] = true;
    animNoteIds = next;
    setTimeout(() => {
      const cleared = { ...animNoteIds };
      for (const id of fresh) delete cleared[id];
      animNoteIds = cleared;
    }, 420);
  }
  async function refreshDetail() {
    if (!detail) return;
    const full = await fetchTask(detail.id);
    if (full) {
      applyNoteEntrance(full.notes);
      detail = full;
    } else {
      const t = findTask(detail.id);
      if (t) detail = t;
    }
  }
  function closeDetail({ skipURL = false } = {}) {
    detail = null; editing = false; needPath = null; runError = "";
    seenNoteIds = new Set(); animNoteIds = {};
    if (!skipURL && taskIdFromURL() != null) setTaskURL(null);
  }
  function startEdit() {
    edit = {
      title: detail.title ?? "",
      description: detail.description ?? "",
      priority: detail.priority ?? "",
      due_date: detail.due_date ?? "",
      tags: (detail.tags ?? []).join(", "),
      project: detail.project ?? "",
    };
    editing = true;
  }
  async function saveEdit() {
    const title = edit.title.trim();
    if (!title) return;
    const tags = edit.tags.split(",").map((s) => s.trim()).filter(Boolean);
    await fetch(`/api/tasks/${detail.id}/update`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        title, description: edit.description,
        priority: edit.priority, due_date: edit.due_date, tags,
        project: edit.project.trim(),
      }),
    });
    await load();
    if (detail) await refreshDetail();
    editing = false;
  }
  async function addNote() {
    const body = noteBody.trim();
    if (!body) return;
    await fetch(`/api/tasks/${detail.id}/note`, {
      method: "POST", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ body }),
    });
    noteBody = "";
    await load();
    if (detail) await refreshDetail();
  }
  async function moveFromDetail(status) {
    await move(detail.id, status);
    if (detail) await refreshDetail();
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
  function fmtDateTime(iso, { seconds = false } = {}) {
    try {
      return new Date(iso).toLocaleString([], {
        month: "short", day: "numeric",
        hour: "2-digit", minute: "2-digit",
        ...(seconds ? { second: "2-digit" } : {}),
      });
    }
    catch { return ""; }
  }
  // Relative "3m", "2h", "4d" for compact activity timestamps.
  function fmtAgo(iso) {
    try {
      const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
      if (s < 60) return "now";
      if (s < 3600) return Math.floor(s / 60) + "m";
      if (s < 86400) return Math.floor(s / 3600) + "h";
      return Math.floor(s / 86400) + "d";
    } catch { return ""; }
  }
  // Resolve an event's task title from the current board, when it has one.
  function eventTitle(e) {
    if (!e.task_id) return "";
    const t = findTask(e.task_id);
    return t ? t.title : "";
  }
  const eventKindLabel = { created: "created", moved: "moved", note: "note", handoff: "handoff", archived: "archived", unarchived: "restored", updated: "updated", deleted: "deleted", tool: "tool", session: "session", run: "run", run_done: "run done", run_progress: "progress", run_wait: "wait", question: "question", answered: "answered" };
  const eventKindVerb = {
    created: "Created", moved: "Moved", note: "Note on", handoff: "Handed off",
    archived: "Archived", unarchived: "Restored", updated: "Updated", deleted: "Deleted",
    tool: "Tool", session: "Session", run: "Started agent on", run_done: "Agent finished",
    run_progress: "Progress on", run_wait: "Wait on", question: "Asked on", answered: "Answered on",
  };

  $effect(() => {
    let saved = "dark";
    try { saved = localStorage.getItem("board-theme") || (matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark"); } catch {}
    applyTheme(saved);
    loadProjectFilter();

    let cancelled = false;
    (async () => {
      await load();
      if (cancelled) return;
      maybeShowNotifyBanner();
      const id = taskIdFromURL();
      if (id) {
        const t = findTask(id) ?? { id };
        await openDetail(t, { replaceURL: true });
      }
    })();

    // While the server replays its backlog, events are historical: fill the
    // feed but don't bump the unseen badge. The `synced` sentinel flips us live.
    let syncing = true;
    // Coalesce the per-event refetch — a 200-event backlog triggers one load,
    // not 200.
    let loadTimer = null;
    const scheduleLoad = () => {
      clearTimeout(loadTimer);
      loadTimer = setTimeout(async () => {
        await load();
        if (detail) await refreshDetail();
      }, 120);
    };

    const es = new EventSource("/api/events?since=0");
    es.onmessage = (m) => {
      const ev = JSON.parse(m.data);
      events = [ev, ...events].slice(0, 60);
      if (!syncing) unseen = Math.min(unseen + 1, 99);
      if (!syncing && ev.kind === "question" && ev.task_id) {
        notifyAsk(ev.task_id, ev.detail);
        if (!detail) openDetail(findTask(ev.task_id) ?? { id: ev.task_id });
      }
      scheduleLoad();
    };
    es.addEventListener("synced", () => { syncing = false; });

    const onPop = () => {
      const id = taskIdFromURL();
      if (id) {
        if (detail?.id !== id) openDetail(findTask(id) ?? { id }, { skipURL: true });
      } else if (detail) {
        closeDetail({ skipURL: true });
      }
    };
    window.addEventListener("popstate", onPop);

    return () => {
      cancelled = true;
      clearTimeout(loadTimer);
      es.close();
      window.removeEventListener("popstate", onPop);
    };
  });

  function toggleActivity() { showActivity = !showActivity; }
  function markAllSeen() { unseen = 0; }

  // Close the open card menu on outside click (defer so the opening click doesn't close it).
  $effect(() => {
    if (openMenu === null) return;
    let remove = () => {};
    const timer = setTimeout(() => {
      const onDocClick = (e) => {
        if (!e.target.closest(".menu") && !e.target.closest(".menu-btn")) openMenu = null;
      };
      window.addEventListener("click", onDocClick);
      remove = () => window.removeEventListener("click", onDocClick);
    }, 0);
    return () => { clearTimeout(timer); remove(); };
  });

  $effect(() => {
    if (openAgentMenu === null) return;
    let remove = () => {};
    const timer = setTimeout(() => {
      const onDocClick = (e) => {
        if (!e.target.closest(".run-split")) openAgentMenu = null;
      };
      window.addEventListener("click", onDocClick);
      remove = () => window.removeEventListener("click", onDocClick);
    }, 0);
    return () => { clearTimeout(timer); remove(); };
  });
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
{#snippet iconEdit()}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z"/></svg>{/snippet}
{#snippet iconFolder()}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>{/snippet}
{#snippet iconPlay()}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M8 5v14l11-7z"/></svg>{/snippet}
{#snippet iconChevron()}<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M6 9l6 6 6-6"/></svg>{/snippet}

<header class="topbar">
  <div class="brand">
    <span class="logo" aria-hidden="true"></span>
    <h1>board</h1>
  </div>
  <div class="actions">
    <button class="icon-btn" onclick={() => (showProjects = !showProjects)} aria-label="Project paths" title="Project paths">
      {@render iconFolder()}
    </button>
    <button class="icon-btn" onclick={toggleActivity} aria-label="Toggle activity">
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

{#if showNotifyBanner}
  <div class="notify-banner" role="status">
    <span>Notify me when an agent asks</span>
    <div class="notify-actions">
      <button class="btn-primary sm" onclick={enableAskNotify}>Enable</button>
      <button class="btn-ghost sm" onclick={dismissNotifyBanner}>Not now</button>
    </div>
  </div>
{/if}

<nav class="proj-filter" aria-label="Filter by project">
  <button class:active={projectFilter === "*"} onclick={() => setProjectFilter("*")}>All</button>
  {#each chipProjects as p (p)}
    <button
      class:active={projectFilter === p}
      onclick={() => setProjectFilter(p)}
    >{p === "_" ? "global" : p}</button>
  {/each}
</nav>

{#if showProjects}
  <section class="projects-panel" aria-label="Project paths">
    <div class="lane-head"><span>Project paths</span>
      <button class="icon-btn sm" onclick={() => (showProjects = false)} aria-label="Close">{@render iconClose()}</button>
    </div>
    {#if projectPaths.length === 0}
      <div class="empty sm">No paths yet — set one when you Run a task.</div>
    {/if}
    {#each projectPaths as pp (pp.project)}
      <div class="proj-row">
        <code class="proj-name">{pp.project === "_" ? "(global)" : pp.project}</code>
        <input class="proj-path" value={pp.path} onchange={(e) => saveProjectPath(pp.project, e.currentTarget.value)} />
        <button class="btn-ghost sm danger" onclick={() => clearProjectPath(pp.project)}>Clear</button>
      </div>
    {/each}
  </section>
{/if}

{#if handoffs.length}
  <section class="handoff-lane" aria-label="Handoffs and inbox">
    <div class="lane-head">{@render iconHandoff()}<span>Handoffs</span></div>
    <div class="lane-scroll">
      {#each handoffs as t (t.id)}
        <div class="handoff-chip" class:human={t.handoff_to === "human"} onclick={() => openDetail(t)} role="button" tabindex="0" onkeydown={(e) => e.key === "Enter" && openDetail(t)}>
          <span class="to">{t.handoff_to}</span>
          <span class="ht">{t.title}</span>
          <span class="hp">{projectLabel(t.project)}</span>
          {#if t.handoff_reason}<span class="hr">{t.handoff_reason}</span>{/if}
          <button class="chip-x" onclick={(e) => clearHandoff(t.id, e)} aria-label="Clear handoff" title="Clear handoff">
            {@render iconClose()}
          </button>
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
    <div
      class="col"
      class:active={activeCol === c.key}
      class:drop={dragOver === c.key}
      role="region"
      aria-label={c.label}
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
            class:menu-open={openMenu === t.id}
            class:working={isWorking(t.id)}
            class:run-failed={latestRun(t.id)?.status === "failed"}
            class:run-done={latestRun(t.id)?.status === "exited"}
            draggable="true"
            ondragstart={(e) => onDragStart(e, t.id)}
          >
            <div class="card-top">
              <button class="title" onclick={() => openDetail(t)}>{t.title}</button>
              <button
                class="menu-btn"
                aria-label="Task actions"
                aria-expanded={openMenu === t.id}
                onclick={(e) => { e.stopPropagation(); openMenu = openMenu === t.id ? null : t.id; }}
              >
                {@render iconMore()}
              </button>
            </div>
            <div class="meta">
              <span class="proj" class:is-global={!t.project}>{projectLabel(t.project)}</span>
              {#if t.priority}<span class="pri pri-{t.priority}">{t.priority}</span>{/if}
              {#each t.tags ?? [] as tag}<span class="tag">{tag}</span>{/each}
              {#if t.handoff_to}<span class="hbadge">{@render iconHandoff()}{t.handoff_to}</span>{/if}
              {#if taskHasPending(t.id)}
                <button
                  type="button"
                  class="askbadge"
                  onclick={(e) => { e.stopPropagation(); openDetail(t); }}
                >asks</button>
              {/if}
            </div>
            {#if latestRun(t.id) || isStarting(t.id) || taskHasPending(t.id)}
              <div class="agent-status s-{waitStatusClass(t.id)}">
                <span class="agent-label">{waitStatusLabel(t.id)}</span>
                {#if latestRun(t.id)?.message}
                  <p class="agent-msg">{latestRun(t.id).message}</p>
                {:else if isWorking(t.id) && !pendingAskFor(t.id)}
                  <p class="agent-msg muted">{agentLabel(latestRun(t.id)?.agent || selectedAgent)} is on it…</p>
                {/if}
              </div>
            {/if}
            <div class="card-actions">
              {#if isWorking(t.id)}
                <button
                  class="btn-run cancel"
                  disabled={isStarting(t.id) && latestRun(t.id)?.status !== "running"}
                  onclick={(e) => { e.stopPropagation(); cancelRun(t.id); }}
                >Cancel</button>
              {:else}
                <div class="run-split">
                  <button
                    class="btn-run"
                    onclick={(e) => { e.stopPropagation(); openMenu = null; openAgentMenu = null; runTask(t.id); }}
                  >{@render iconPlay()}<span>Run</span></button>
                  <button
                    class="btn-run-chevron"
                    aria-label="Choose agent"
                    aria-expanded={openAgentMenu === t.id}
                    onclick={(e) => { e.stopPropagation(); openMenu = null; openAgentMenu = openAgentMenu === t.id ? null : t.id; }}
                  >{@render iconChevron()}</button>
                  {#if openAgentMenu === t.id}
                    <div class="run-menu" role="menu">
                      {#each AGENTS as a}
                        <button
                          role="menuitem"
                          class:active={selectedAgent === a.key}
                          onclick={(e) => { e.stopPropagation(); setSelectedAgent(a.key); runTask(t.id, a.key); }}
                        >{a.label}</button>
                      {/each}
                    </div>
                  {/if}
                </div>
              {/if}
            </div>
            {#if openMenu === t.id}
              <div class="menu" role="menu" tabindex="-1">
                {#each COLUMNS.filter((x) => x.key !== t.status) as m}
                  <button role="menuitem" onclick={() => { openMenu = null; move(t.id, m.key); }}>Move to {m.label}</button>
                {/each}
                <button role="menuitem" class="danger" onclick={() => { openMenu = null; archive(t.id); }}>Archive</button>
              </div>
            {/if}
          </article>
        {/each}
        {#if (board[c.key]?.length ?? 0) === 0}
          <div class="empty">Nothing here</div>
        {/if}
      </div>
    </div>
  {/each}
</main>

<!-- Add task modal -->
{#if showAdd}
  <div
    class="scrim"
    role="button"
    tabindex="-1"
    aria-label="Dismiss"
    onclick={() => (showAdd = false)}
    onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); showAdd = false; } }}
  ></div>
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
    <label class="field">
      <span>Project</span>
      <select bind:value={addProject}>
        <option value="">global</option>
        {#each projectPaths.filter((p) => p.project !== "_") as pp (pp.project)}
          <option value={pp.project}>{pp.project}</option>
        {/each}
      </select>
      {#if projectPaths.length === 0}
        <span class="field-hint">Add a project path (folder icon) to assign tasks to a project.</span>
      {/if}
    </label>
    <label class="field">
      <span>Description (markdown)</span>
      <textarea
        rows="4"
        bind:value={addDescription}
        placeholder="Optional context…"
        onkeydown={(e) => { if (e.key === "Escape") showAdd = false; }}
      ></textarea>
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
  <div
    class="scrim"
    role="button"
    tabindex="-1"
    aria-label="Dismiss activity"
    onclick={() => (showActivity = false)}
    onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); showActivity = false; } }}
  ></div>
  <div class="activity" role="dialog" aria-label="Activity feed">
    <div class="activity-head">
      <div class="live">{@render iconActivity()}<span>Activity</span></div>
      <div class="head-actions">
        {#if unseen > 0}
          <button class="btn-ghost sm" onclick={markAllSeen}>Seen all</button>
        {/if}
        <button class="icon-btn sm" aria-label="Close activity" onclick={() => (showActivity = false)}>{@render iconClose()}</button>
      </div>
    </div>
    <div class="feed">
      {#if events.length === 0}<div class="empty">No activity yet</div>{/if}
      {#each events.filter(eventVisible) as e (e.id)}
        {@const et = eventTitle(e)}
        <div class="ev">
          <span class="ev-dot k-{e.kind}" aria-hidden="true"></span>
          <div class="ev-body">
            <div class="ev-line">
              <span class="ev-kind k-{e.kind}">{eventKindLabel[e.kind] ?? e.kind}</span>
              {#if et}
                <button class="ev-task" onclick={() => { const t = findTask(e.task_id); if (t) { showActivity = false; openDetail(t); } }}>{et}</button>
              {/if}
              <span class="ev-time" title={fmtDateTime(e.created_at)}>{fmtAgo(e.created_at)}</span>
            </div>
            {#if e.detail}<div class="ev-detail">{e.detail}</div>{/if}
          </div>
        </div>
      {/each}
    </div>
  </div>
{/if}

<!-- Task detail drawer -->
{#if detail}
  <div
    class="scrim"
    role="button"
    tabindex="-1"
    aria-label="Dismiss task detail"
    onclick={closeDetail}
    onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); closeDetail(); } }}
  ></div>
  <div class="detail" class:is-working={isWorking(detail.id)} role="dialog" aria-modal="true" aria-label="Task detail">
    <div class="activity-head">
      <div class="live">
        <span class="d-id">#{detail.id}</span>
        <span>{editing ? "Edit" : "Task"}</span>
        {#if isWorking(detail.id) || pendingAskFor(detail.id) || (latestRun(detail.id) && ["exited","failed","killed"].includes(latestRun(detail.id).status))}
          <span class="d-working s-{waitStatusClass(detail.id)}">
            {#if isWorking(detail.id) || pendingAskFor(detail.id)}<span class="agent-pulse" aria-hidden="true"></span>{/if}
            {waitStatusLabel(detail.id)}
          </span>
        {/if}
      </div>
      <div class="head-actions">
        {#if !editing && isWorking(detail.id)}
          <button class="btn-run cancel sm" onclick={() => cancelRun(detail.id)}>Cancel</button>
        {/if}
        {#if !editing}
          <button class="icon-btn sm" aria-label="Edit task" onclick={startEdit}>{@render iconEdit()}</button>
        {/if}
        <button class="icon-btn sm" aria-label="Close" onclick={closeDetail}>{@render iconClose()}</button>
      </div>
    </div>
    <div class="detail-body">
      {#if editing}
        <label class="field">
          <span>Title</span>
          <input bind:value={edit.title} onkeydown={(e) => { if (e.key === "Escape") editing = false; }} />
        </label>
        <label class="field">
          <span>Project</span>
          <select bind:value={edit.project}>
            <option value="">global</option>
            {#each projectPaths.filter((p) => p.project !== "_") as pp (pp.project)}
              <option value={pp.project}>{pp.project}</option>
            {/each}
            {#if edit.project && !projectPaths.some((p) => p.project === edit.project)}
              <option value={edit.project}>{edit.project} (unmapped)</option>
            {/if}
          </select>
          {#if projectPaths.length === 0}
            <span class="field-hint">Add a project path (folder icon) to assign tasks to a project.</span>
          {/if}
        </label>
        <label class="field">
          <span>Description (markdown)</span>
          <textarea rows="5" bind:value={edit.description}></textarea>
        </label>
        <div class="field">
          <span>Priority</span>
          <div class="pri-seg">
            {#each PRIORITIES as p}
              <button class:sel={edit.priority === p.key} onclick={() => (edit.priority = p.key)}>{p.label}</button>
            {/each}
          </div>
        </div>
        <label class="field">
          <span>Due date</span>
          <input bind:value={edit.due_date} placeholder="YYYY-MM-DD" />
        </label>
        <label class="field">
          <span>Tags (comma-separated)</span>
          <input bind:value={edit.tags} placeholder="bug, frontend" />
        </label>
        <div class="modal-foot">
          <button class="btn-ghost" onclick={() => (editing = false)}>Cancel</button>
          <button class="btn-primary" disabled={!edit.title.trim()} onclick={saveEdit}>{@render iconCheck()}<span>Save</span></button>
        </div>
      {:else}
        <h3 class="d-title">{detail.title}</h3>
        <div class="d-chips">
          <span class="d-status s-{detail.status}">{statusLabel(detail.status)}</span>
          <span class="proj" class:is-global={!detail.project}>{projectLabel(detail.project)}</span>
          {#if detail.priority}<span class="pri pri-{detail.priority}">{detail.priority}</span>{/if}
          {#each detail.tags ?? [] as tag}<span class="tag">{tag}</span>{/each}
        </div>
        <div class="d-facts">
          <span><em>Project</em> {projectLabel(detail.project)}</span>
          {#if detail.due_date}<span><em>Due</em> {detail.due_date}</span>{/if}
          {#if detail.handoff_to}
            <span class="d-handoff">
              <em>Handoff</em> → {detail.handoff_to}
              <button class="chip-x inline" onclick={(e) => clearHandoff(detail.id, e)} aria-label="Clear handoff" title="Clear handoff">{@render iconClose()}</button>
            </span>
          {/if}
          <span><em>Updated</em> {fmtDateTime(detail.updated_at)}</span>
        </div>
        <div class="d-move">
          {#each COLUMNS.filter((x) => x.key !== detail.status) as m}
            <button class="btn-ghost sm" onclick={() => moveFromDetail(m.key)}>Move to {m.label}</button>
          {/each}
          {#if !isWorking(detail.id)}
            <div class="run-split sm">
              <button class="btn-run sm" onclick={() => { openAgentMenu = null; runTask(detail.id); }}>{@render iconPlay()}<span>Run</span></button>
              <button
                class="btn-run-chevron sm"
                aria-label="Choose agent"
                aria-expanded={openAgentMenu === "detail"}
                onclick={() => (openAgentMenu = openAgentMenu === "detail" ? null : "detail")}
              >{@render iconChevron()}</button>
              {#if openAgentMenu === "detail"}
                <div class="run-menu" role="menu">
                  {#each AGENTS as a}
                    <button
                      role="menuitem"
                      class:active={selectedAgent === a.key}
                      onclick={() => { setSelectedAgent(a.key); runTask(detail.id, a.key); }}
                    >{a.label}</button>
                  {/each}
                </div>
              {/if}
            </div>
          {/if}
        </div>
        {#if needPath && needPath.taskId === detail.id}
          <div class="path-prompt">
            <p>Where is project <strong>{needPath.project === "_" ? "(global)" : needPath.project}</strong> on disk?</p>
            <input bind:value={pathInput} placeholder="/home/you/sites/…" onkeydown={(e) => { if (e.key === "Enter") savePathAndRun(); }} />
            <div class="modal-foot">
              <button class="btn-ghost" onclick={() => (needPath = null)}>Cancel</button>
              <button class="btn-primary" disabled={!pathInput.trim()} onclick={savePathAndRun}>Save &amp; Run</button>
            </div>
          </div>
        {/if}
        {#if runError}<p class="run-err">{runError}</p>{/if}
        <div class="d-section">
          <h4>Description</h4>
          {#if detail.description}
            <div class="md d-desc">{@html renderMd(detail.description)}</div>
          {:else}
            <p class="d-desc is-muted">No description</p>
          {/if}
        </div>
        <div class="d-section">
          <h4>Thread {(detail.notes ?? []).length ? `(${detail.notes.length})` : ""}</h4>
          {#if pendingAskFor(detail.id)}
            {@const ask = pendingAskFor(detail.id)}
            <div class="ask-card" role="form" aria-label="Agent question">
              <div class="ask-head">
                <span class="ask-label">Agent asks</span>
              </div>
              <div class="md ask-q">{@html renderMd(ask.question)}</div>
              <textarea
                rows="3"
                bind:value={answerDraft}
                placeholder="Your answer…"
                onkeydown={(e) => {
                  if (e.key === "Enter" && (e.metaKey || e.ctrlKey) && answerDraft.trim()) submitAnswer(ask.id);
                }}
              ></textarea>
              <div class="ask-actions">
                <button class="btn-primary sm" disabled={!answerDraft.trim()} onclick={() => submitAnswer(ask.id)}>Submit answer</button>
              </div>
            </div>
          {/if}
          <div class="d-note-add">
            <input bind:value={noteBody} placeholder="Add to thread (markdown ok)…" onkeydown={(e) => { if (e.key === "Enter") addNote(); }} />
            <button class="btn-primary sm" disabled={!noteBody.trim()} onclick={addNote}>Add</button>
          </div>
          {#if (detail.notes ?? []).length === 0 && !pendingAskFor(detail.id)}
            <div class="empty sm">Nothing in the thread yet</div>
          {/if}
          {#each [...(detail.notes ?? [])].reverse() as n (n.id)}
            <div class="d-note" class:is-new={!!animNoteIds[n.id]}>
              <div class="note-meta">
                <span class="note-author a-{n.author || 'unknown'}">{noteAuthorLabel(n.author)}</span>
                <span class="ev-time">{fmtDateTime(n.created_at, { seconds: true })}</span>
              </div>
              <div class="md">{@html renderMd(n.body)}</div>
            </div>
          {/each}
        </div>
        <dl class="d-meta">
          <div><dt>Created</dt><dd>{fmtDateTime(detail.created_at)}</dd></div>
          <div><dt>Updated</dt><dd>{fmtDateTime(detail.updated_at)}</dd></div>
          {#if detail.handoff_reason}<div><dt>Handoff</dt><dd>{detail.handoff_reason}</dd></div>{/if}
        </dl>
      {/if}
    </div>
  </div>
{/if}

<style>
  :global(:root) {
    --bg: #0f172a; --surface: #1e293b; --surface-2: #172033; --surface-3: #232f45;
    --text: #f8fafc; --muted: #94a3b8; --border: #334155;
    --accent: #22c55e; --accent-fg: #052e13; --danger: #ef4444; --amber: #f59e0b;
    --todo: #64748b; --prog: #3b82f6; --done: #22c55e;
    --radius: 12px; --shadow: 0 1px 2px rgba(0,0,0,.4), 0 4px 16px rgba(0,0,0,.25);
    --topbar-h: calc(64px + env(safe-area-inset-top));
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
  /* Never let a focused input trigger iOS auto-zoom: 16px floor everywhere. */
  :global(input), :global(textarea), :global(select) { font-size: 16px; }
  :global(#app) {
    font-family: var(--font); color: var(--text); background: var(--bg);
    min-height: 100dvh; -webkit-font-smoothing: antialiased;
    padding-bottom: env(safe-area-inset-bottom);
  }

  .topbar {
    position: sticky; top: 0; z-index: 60;
    display: flex; align-items: center; justify-content: space-between;
    gap: 12px; padding: 12px 16px; padding-top: calc(12px + env(safe-area-inset-top));
    background: color-mix(in srgb, var(--bg) 85%, transparent);
    backdrop-filter: blur(12px); border-bottom: 1px solid var(--border);
  }
  .brand { display: flex; align-items: center; gap: 10px; }
  .logo { width: 22px; height: 22px; border-radius: 7px; background: linear-gradient(135deg, var(--accent), var(--prog)); box-shadow: 0 0 0 1px color-mix(in srgb, var(--accent) 40%, transparent); }
  .brand h1 { font-size: 18px; font-weight: 700; margin: 0; letter-spacing: -.02em; }
  .actions { display: flex; align-items: center; gap: 8px; }

  .notify-banner {
    display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap;
    margin: 8px 16px 0; padding: 10px 14px; border-radius: 10px;
    border: 1px solid color-mix(in srgb, var(--amber) 40%, var(--border));
    background: color-mix(in srgb, var(--amber) 10%, var(--surface));
    font-size: 13px; color: var(--text);
  }
  .notify-actions { display: flex; gap: 8px; }
  .proj-filter {
    display: flex; gap: 6px; flex-wrap: wrap; padding: 10px 16px 0;
  }
  .proj-filter button {
    font-family: inherit; font-size: 12px; font-weight: 600; padding: 5px 10px;
    border-radius: 999px; border: 1px solid var(--border); background: var(--surface);
    color: var(--muted); cursor: pointer;
  }
  .proj-filter button.active {
    color: var(--prog); border-color: color-mix(in srgb, var(--prog) 45%, var(--border));
    background: color-mix(in srgb, var(--prog) 12%, var(--surface));
  }
  .field-hint { display: block; margin-top: 6px; font-size: 12px; color: var(--muted); }
  .field select {
    width: 100%; height: 44px; padding: 0 12px; border-radius: 10px; border: 1px solid var(--border);
    background: var(--surface-2); color: var(--text); font-family: inherit; font-size: 16px;
  }

  .projects-panel {
    margin: 0 16px; padding: 12px; border: 1px solid var(--border); border-radius: var(--radius);
    background: var(--surface); display: flex; flex-direction: column; gap: 8px;
  }
  .projects-panel .lane-head { display: flex; align-items: center; justify-content: space-between; font-size: 13px; font-weight: 600; color: var(--muted); }
  .proj-row { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
  .proj-name { font-size: 12px; color: var(--amber); min-width: 72px; }
  .proj-path { flex: 1; min-width: 180px; padding: 8px 10px; border-radius: 8px; border: 1px solid var(--border); background: var(--surface-2); color: var(--text); }
  .askbadge {
    font-size: 11px; font-weight: 700; text-transform: uppercase; letter-spacing: .02em;
    padding: 2px 7px; border-radius: 999px; border: none; cursor: pointer;
    color: var(--amber); background: color-mix(in srgb, var(--amber) 18%, transparent);
    font-family: inherit;
  }
  .askbadge:hover { background: color-mix(in srgb, var(--amber) 28%, transparent); }
  .ask-card {
    margin-bottom: 12px; padding: 12px; border-radius: 12px;
    border: 1px solid color-mix(in srgb, var(--amber) 45%, var(--border));
    background: color-mix(in srgb, var(--amber) 10%, var(--surface-2));
    display: flex; flex-direction: column; gap: 10px;
  }
  .ask-head { display: flex; align-items: center; gap: 8px; }
  .ask-label {
    font-size: 11px; font-weight: 800; text-transform: uppercase; letter-spacing: .04em; color: var(--amber);
  }
  .ask-q { margin: 0; font-size: 14px; line-height: 1.45; color: var(--text); }
  .ask-card textarea {
    width: 100%; padding: 10px 12px; border-radius: 8px; border: 1px solid var(--border);
    background: var(--surface); color: var(--text); resize: vertical; font-family: inherit; font-size: 16px;
  }
  .ask-card textarea:focus {
    outline: none; border-color: var(--amber);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--amber) 25%, transparent);
  }
  .ask-actions { display: flex; justify-content: flex-end; }
  .card-actions { display: flex; gap: 8px; margin-top: 10px; }
  .btn-run {
    display: inline-flex; align-items: center; justify-content: center; gap: 6px;
    min-height: 34px; padding: 0 12px; border-radius: 9px; border: none;
    background: var(--prog); color: #fff; font-family: inherit; font-size: 13px; font-weight: 700;
    cursor: pointer; transition: filter .12s ease, transform .08s ease, opacity .12s ease;
  }
  .btn-run svg { width: 14px; height: 14px; }
  .btn-run:hover { filter: brightness(1.08); }
  .btn-run:active { transform: scale(.97); }
  .btn-run:disabled { opacity: .55; cursor: wait; }
  .btn-run.cancel { background: color-mix(in srgb, var(--danger) 85%, #000); }
  .btn-run.sm { min-height: 32px; padding: 0 10px; font-size: 12px; }
  .run-split {
    position: relative; display: inline-flex; align-items: stretch;
  }
  .run-split > .btn-run {
    border-radius: 9px 0 0 9px;
  }
  .btn-run-chevron {
    display: grid; place-items: center;
    min-width: 28px; border: none; border-left: 1px solid color-mix(in srgb, #fff 28%, transparent);
    border-radius: 0 9px 9px 0; background: var(--prog); color: #fff;
    cursor: pointer; padding: 0 4px;
    transition: filter .12s ease;
  }
  .btn-run-chevron svg { width: 14px; height: 14px; }
  .btn-run-chevron:hover { filter: brightness(1.08); }
  .btn-run-chevron.sm { min-width: 26px; }
  .run-split.sm > .btn-run { min-height: 32px; padding: 0 10px; font-size: 12px; }
  .run-menu {
    position: absolute; right: 0; bottom: calc(100% + 6px); z-index: 5;
    min-width: 120px; padding: 4px; border-radius: 10px;
    background: var(--surface); border: 1px solid var(--border);
    box-shadow: 0 8px 24px color-mix(in srgb, #000 28%, transparent);
    display: grid; gap: 2px;
  }
  .run-menu button {
    text-align: left; border: none; border-radius: 7px; background: transparent;
    color: var(--text); font-family: inherit; font-size: 13px; font-weight: 550;
    padding: 8px 10px; cursor: pointer;
  }
  .run-menu button:hover { background: color-mix(in srgb, var(--prog) 14%, transparent); }
  .run-menu button.active { color: var(--prog); font-weight: 700; }
  .agent-status {
    margin-top: 10px; padding: 8px 10px; border-radius: 9px;
    border: 1px solid var(--border); background: var(--surface-2);
    display: flex; flex-direction: column; gap: 4px;
  }
  .agent-status.s-starting, .agent-status.s-running {
    border-color: color-mix(in srgb, var(--prog) 45%, var(--border));
    background: color-mix(in srgb, var(--prog) 10%, var(--surface-2));
  }
  .agent-status.s-waiting-you {
    border-color: color-mix(in srgb, var(--amber) 45%, var(--border));
    background: color-mix(in srgb, var(--amber) 12%, var(--surface-2));
  }
  .agent-status.s-waiting-ci {
    border-color: color-mix(in srgb, var(--prog) 35%, var(--border));
    background: color-mix(in srgb, var(--prog) 8%, var(--surface-2));
  }
  .agent-status.s-waiting-you .agent-label { color: var(--amber); }
  .agent-status.s-waiting-ci .agent-label { color: var(--prog); }
  .agent-status.s-exited {
    border-color: color-mix(in srgb, var(--done) 45%, var(--border));
    background: color-mix(in srgb, var(--done) 10%, var(--surface-2));
  }
  .agent-status.s-failed, .agent-status.s-killed {
    border-color: color-mix(in srgb, var(--danger) 45%, var(--border));
    background: color-mix(in srgb, var(--danger) 10%, var(--surface-2));
  }
  .agent-label {
    font-size: 11px; font-weight: 800; text-transform: uppercase; letter-spacing: .04em;
    color: var(--prog);
  }
  .agent-status.s-exited .agent-label { color: var(--done); }
  .agent-status.s-failed .agent-label, .agent-status.s-killed .agent-label { color: var(--danger); }
  .agent-msg {
    margin: 0; font-size: 12px; line-height: 1.4; color: var(--text);
    white-space: pre-wrap; word-break: break-word;
    display: -webkit-box; -webkit-line-clamp: 4; -webkit-box-orient: vertical; overflow: hidden;
  }
  .agent-msg.muted { color: var(--muted); -webkit-line-clamp: 2; }
  .card.working { box-shadow: 0 0 0 1px color-mix(in srgb, var(--prog) 50%, transparent), var(--shadow); }
  .card.run-failed { box-shadow: 0 0 0 1px color-mix(in srgb, var(--danger) 40%, transparent), var(--shadow); }
  .card.run-done { box-shadow: 0 0 0 1px color-mix(in srgb, var(--done) 35%, transparent), var(--shadow); }
  .path-prompt {
    margin-top: 12px; padding: 12px; border-radius: 10px; border: 1px solid var(--border);
    background: var(--surface-2); display: flex; flex-direction: column; gap: 8px;
  }
  .path-prompt input { padding: 10px 12px; border-radius: 8px; border: 1px solid var(--border); background: var(--bg); color: var(--text); }
  .run-err { color: var(--danger); font-size: 13px; margin: 8px 0 0; }
  .btn-ghost.danger, .btn-ghost.sm.danger { color: var(--danger); }

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
    position: relative;
    cursor: pointer;
    flex: 0 0 auto; max-width: 280px; padding: 8px 32px 8px 12px; border-radius: 10px;
    background: color-mix(in srgb, var(--amber) 12%, var(--surface));
    border: 1px solid color-mix(in srgb, var(--amber) 40%, var(--border));
    display: flex; flex-direction: column; gap: 2px;
  }
  .handoff-chip .to { font-size: 11px; font-weight: 700; color: var(--amber); text-transform: uppercase; }
  .handoff-chip .ht { font-size: 13px; font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .handoff-chip .hp { font-size: 11px; font-weight: 600; color: var(--prog); }
  .handoff-chip .hr { font-size: 12px; color: var(--muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .handoff-chip.human { box-shadow: 0 0 0 1px var(--amber); }
  .chip-x {
    position: absolute; top: 4px; right: 4px;
    display: inline-flex; align-items: center; justify-content: center;
    width: 24px; height: 24px; padding: 0; border: none; border-radius: 6px;
    background: transparent; color: var(--muted); cursor: pointer;
  }
  .chip-x:hover { background: var(--surface-2); color: var(--text); }
  .chip-x svg { width: 14px; height: 14px; }
  .chip-x.inline { position: static; width: 18px; height: 18px; vertical-align: middle; margin-left: 4px; }
  .d-handoff { display: inline-flex; align-items: center; gap: 2px; }

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
  .card.menu-open { z-index: 50; } /* lift above sibling cards so the action menu isn't covered */
  .card-top { display: flex; align-items: flex-start; gap: 8px; }
  .title {
    margin: 0; padding: 0; flex: 1; text-align: left; border: none; background: transparent;
    font-family: inherit; font-size: 14px; font-weight: 500; line-height: 1.45; color: var(--text);
    word-break: break-word; cursor: pointer;
  }
  .title:hover { color: var(--accent); }
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
  .proj {
    font-size: 11px; font-weight: 700; padding: 2px 8px; border-radius: 999px;
    color: var(--prog); background: color-mix(in srgb, var(--prog) 12%, transparent);
    letter-spacing: .01em;
  }
  .proj.is-global { color: var(--muted); background: var(--surface-2); border: 1px solid var(--border); font-weight: 600; }
  .hbadge { display: inline-flex; align-items: center; gap: 4px; font-size: 11px; font-weight: 600; padding: 2px 8px; border-radius: 999px; color: var(--amber); background: color-mix(in srgb, var(--amber) 13%, transparent); }
  .hbadge svg { width: 12px; height: 12px; }

  .menu {
    position: absolute; top: 40px; right: 10px; z-index: 80; min-width: 168px; padding: 6px;
    background: var(--surface); border: 1px solid var(--border); border-radius: 10px;
    box-shadow: 0 8px 28px rgba(0,0,0,.35); display: flex; flex-direction: column; gap: 2px;
    animation: popmenu .12s ease;
  }
  .menu button {
    text-align: left; padding: 9px 10px; min-height: 40px; border: none; border-radius: 7px;
    background: transparent; color: var(--text); font-family: inherit; font-size: 13px; cursor: pointer;
  }
  .menu button:hover { background: var(--surface-2); }
  .menu button.danger { color: var(--danger); }

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
    background: var(--surface-2); color: var(--text); font-family: inherit; font-size: 16px;
    transition: border-color .12s ease, box-shadow .12s ease;
  }
  .field input:focus { outline: none; border-color: var(--accent); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 25%, transparent); }
  .pri-seg { display: flex; gap: 4px; padding: 4px; background: var(--surface-2); border: 1px solid var(--border); border-radius: 10px; }
  .pri-seg button { flex: 1; min-height: 36px; border: none; border-radius: 7px; background: transparent; color: var(--muted); font-family: inherit; font-size: 13px; font-weight: 600; cursor: pointer; transition: background .12s ease, color .12s ease; }
  .pri-seg button.sel { background: var(--surface); color: var(--text); box-shadow: var(--shadow); }
  .modal-foot { display: flex; justify-content: flex-end; gap: 8px; margin-top: 4px; }

  /* Activity drawer */
  .activity {
    position: fixed; z-index: 50; right: 0; top: var(--topbar-h); bottom: 0; width: min(360px, 100vw);
    background: var(--surface); border-left: 1px solid var(--border);
    display: flex; flex-direction: column; animation: slide .2s ease;
  }
  .activity-head { display: flex; align-items: center; justify-content: space-between; padding: 14px 16px; border-bottom: 1px solid var(--border); }
  .live { display: flex; align-items: center; gap: 8px; font-size: 14px; font-weight: 700; }
  .live svg { width: 17px; height: 17px; color: var(--accent); }
  .feed { flex: 1; overflow-y: auto; padding: 10px 12px; display: flex; flex-direction: column; gap: 1px; }
  .ev { display: grid; grid-template-columns: auto 1fr; gap: 10px; padding: 9px 8px; border-radius: 8px; }
  .ev:hover { background: var(--surface-2); }
  .ev-dot { width: 8px; height: 8px; margin-top: 6px; border-radius: 50%; background: var(--muted); }
  .ev-body { min-width: 0; display: flex; flex-direction: column; gap: 3px; }
  .ev-line { display: flex; align-items: center; gap: 7px; }
  .ev-kind { font-size: 10px; font-weight: 700; text-transform: uppercase; letter-spacing: .03em; padding: 2px 7px; border-radius: 6px; background: var(--surface-3); color: var(--muted); flex: 0 0 auto; }
  .ev-task { flex: 1; min-width: 0; text-align: left; border: none; background: transparent; padding: 0; font-family: inherit; font-size: 13px; font-weight: 600; color: var(--text); cursor: pointer; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .ev-task:hover { color: var(--accent); }
  .k-created { color: var(--done); background: color-mix(in srgb, var(--done) 15%, transparent); }
  .k-moved { color: var(--prog); background: color-mix(in srgb, var(--prog) 15%, transparent); }
  .k-handoff { color: var(--amber); background: color-mix(in srgb, var(--amber) 15%, transparent); }
  .k-note, .k-updated { color: var(--prog); background: color-mix(in srgb, var(--prog) 12%, transparent); }
  .k-deleted, .k-archived { color: var(--danger); background: color-mix(in srgb, var(--danger) 13%, transparent); }
  .ev-dot.k-created { background: var(--done); }
  .ev-dot.k-moved, .ev-dot.k-note, .ev-dot.k-updated { background: var(--prog); }
  .ev-dot.k-handoff { background: var(--amber); }
  .ev-dot.k-deleted, .ev-dot.k-archived { background: var(--danger); }
  .ev-detail { font-size: 12.5px; color: var(--muted); line-height: 1.4; word-break: break-word; }
  .ev-time { font-size: 11px; color: var(--muted); font-variant-numeric: tabular-nums; flex: 0 0 auto; margin-left: auto; }

  /* Task detail drawer */
  .detail {
    position: fixed; z-index: 50; right: 0; top: var(--topbar-h); bottom: 0; width: min(440px, 100vw);
    background: var(--surface); border-left: 1px solid var(--border);
    display: flex; flex-direction: column; animation: slide .2s ease;
  }
  .detail.is-working { border-left-color: color-mix(in srgb, var(--prog) 55%, var(--border)); }
  .d-id {
    font-variant-numeric: tabular-nums; font-size: 12px; font-weight: 700;
    color: var(--muted); letter-spacing: .02em;
  }
  .head-actions { display: flex; align-items: center; gap: 6px; }
  .d-working {
    display: inline-flex; align-items: center; gap: 6px;
    margin-left: 4px; padding: 2px 8px; border-radius: 999px;
    font-size: 11px; font-weight: 700; letter-spacing: .02em; text-transform: uppercase;
  }
  .d-working.s-starting, .d-working.s-running {
    color: var(--prog); background: color-mix(in srgb, var(--prog) 16%, transparent);
  }
  .d-working.s-waiting-you {
    color: var(--amber); background: color-mix(in srgb, var(--amber) 16%, transparent);
  }
  .d-working.s-waiting-ci {
    color: var(--prog); background: color-mix(in srgb, var(--prog) 12%, transparent);
  }
  .d-working.s-exited {
    color: var(--done); background: color-mix(in srgb, var(--done) 15%, transparent);
  }
  .d-working.s-failed, .d-working.s-killed {
    color: var(--danger); background: color-mix(in srgb, var(--danger) 14%, transparent);
  }
  .detail-body { flex: 1; overflow-y: auto; padding: 16px 18px calc(24px + env(safe-area-inset-bottom)); }
  .d-title { margin: 0 0 10px; font-size: 18px; font-weight: 700; line-height: 1.3; letter-spacing: -.01em; word-break: break-word; }
  .d-chips { display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 10px; }
  .d-status { font-size: 11px; font-weight: 700; padding: 3px 9px; border-radius: 999px; text-transform: uppercase; letter-spacing: .03em; }
  .s-todo { color: var(--todo); background: color-mix(in srgb, var(--todo) 18%, transparent); }
  .s-in_progress { color: var(--prog); background: color-mix(in srgb, var(--prog) 15%, transparent); }
  .s-done { color: var(--done); background: color-mix(in srgb, var(--done) 15%, transparent); }
  .d-facts {
    display: flex; flex-wrap: wrap; gap: 6px 14px; margin-bottom: 12px;
    font-size: 12px; color: var(--text); border-top: 1px solid var(--border); padding-top: 10px;
  }
  .d-facts em { font-style: normal; font-weight: 600; color: var(--muted); margin-right: 4px; }
  .d-desc { font-size: 14px; line-height: 1.55; color: var(--text); word-break: break-word; margin: 0 0 8px; }
  .d-desc.is-muted { color: var(--muted); font-style: italic; white-space: pre-wrap; }
  .md :global(p) { margin: 0 0 .65em; }
  .md :global(p:last-child) { margin-bottom: 0; }
  .md :global(ul), .md :global(ol) { margin: 0 0 .65em; padding-left: 1.25em; }
  .md :global(li) { margin: .15em 0; }
  .md :global(code) {
    font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
    font-size: .9em; padding: .1em .35em; border-radius: 4px;
    background: color-mix(in srgb, var(--surface-3) 80%, transparent);
  }
  .md :global(pre) {
    margin: 0 0 .65em; padding: 10px 12px; border-radius: 8px; overflow-x: auto;
    background: var(--surface-3); border: 1px solid var(--border);
  }
  .md :global(pre code) { padding: 0; background: transparent; font-size: 12px; }
  .md :global(a) { color: var(--prog); }
  .md :global(h1), .md :global(h2), .md :global(h3), .md :global(h4) {
    margin: .8em 0 .35em; font-size: 1em; font-weight: 700; line-height: 1.3;
  }
  .md :global(blockquote) {
    margin: 0 0 .65em; padding: 2px 0 2px 10px; border-left: 3px solid var(--border); color: var(--muted);
  }
  .md :global(hr) { border: none; border-top: 1px solid var(--border); margin: .8em 0; }
  .md :global(strong) { font-weight: 700; }
  .d-meta { display: flex; flex-direction: column; gap: 8px; margin: 18px 0 0; padding: 14px; background: var(--surface-2); border: 1px solid var(--border); border-radius: 12px; }
  .d-meta > div { display: flex; gap: 10px; font-size: 13px; }
  .d-meta dt { flex: 0 0 74px; color: var(--muted); font-weight: 600; margin: 0; }
  .d-meta dd { margin: 0; color: var(--text); word-break: break-word; }
  .d-move { display: flex; flex-wrap: wrap; gap: 8px; margin: 0 0 16px; }
  .d-section { margin-bottom: 18px; }
  .d-section h4 { margin: 0 0 10px; font-size: 13px; font-weight: 700; text-transform: uppercase; letter-spacing: .03em; color: var(--muted); }
  .d-note { padding: 10px 12px; border-radius: 10px; background: var(--surface-2); border: 1px solid var(--border); margin-bottom: 8px; }
  .d-note.is-new {
    border-color: color-mix(in srgb, var(--prog) 45%, var(--border));
    background: color-mix(in srgb, var(--prog) 10%, var(--surface-2));
    animation: note-in .38s ease;
  }
  .note-meta { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 6px; }
  .note-author {
    font-size: 10px; font-weight: 800; text-transform: uppercase; letter-spacing: .04em;
    padding: 2px 7px; border-radius: 999px;
  }
  .note-author.a-human { color: var(--accent); background: color-mix(in srgb, var(--accent) 14%, transparent); }
  .note-author.a-agent { color: var(--prog); background: color-mix(in srgb, var(--prog) 14%, transparent); }
  .note-author.a-unknown, .note-author.a-system { color: var(--muted); background: var(--surface-3); }
  .d-note .md { font-size: 13px; line-height: 1.5; color: var(--text); word-break: break-word; }
  .d-note-add { display: flex; gap: 8px; margin: 0 0 12px; }
  .d-note-add input {
    flex: 1; min-width: 0; height: 44px; padding: 0 12px; border-radius: 10px; border: 1px solid var(--border);
    background: var(--surface-2); color: var(--text); font-family: inherit; font-size: 16px;
  }
  .d-note-add input:focus { outline: none; border-color: var(--accent); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 25%, transparent); }
  .agent-pulse {
    width: 8px; height: 8px; border-radius: 50%; background: var(--prog);
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--prog) 60%, transparent);
    animation: pulse 1.4s ease-out infinite;
  }
  @keyframes pulse {
    0% { box-shadow: 0 0 0 0 color-mix(in srgb, var(--prog) 55%, transparent); }
    70% { box-shadow: 0 0 0 8px transparent; }
    100% { box-shadow: 0 0 0 0 transparent; }
  }
  @keyframes note-in {
    from { opacity: 0; transform: translateY(-8px); }
    to { opacity: 1; transform: none; }
  }
  .detail-body textarea {
    width: 100%; padding: 10px 12px; border-radius: 10px; border: 1px solid var(--border);
    background: var(--surface-2); color: var(--text); font-family: inherit; font-size: 16px; line-height: 1.5; resize: vertical;
  }
  .detail-body textarea:focus { outline: none; border-color: var(--accent); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 25%, transparent); }
  .btn-primary.sm, .btn-ghost.sm { height: 36px; padding: 0 12px; font-size: 13px; flex: 0 0 auto; }
  .empty.sm { padding: 12px; font-size: 12px; }

  @keyframes pop { from { opacity: 0; transform: translate(-50%, -50%) scale(.96); } }
  @keyframes fade { from { opacity: 0; } }
  @keyframes slide { from { transform: translateX(100%); } }
  .menu { transform-origin: top right; }
  @keyframes popmenu { from { opacity: 0; transform: scale(.95); } }

  /* Desktop */
  @media (min-width: 768px) {
    .segmented { display: none; }
    .board { display: grid; grid-template-columns: repeat(3, 1fr); gap: 16px; padding: 8px 24px 32px; }
    .col { display: flex; background: var(--surface-2); border: 1px solid var(--border); border-radius: 14px; padding: 12px; }
    .col.drop { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 8%, var(--surface-2)); }
    .col-head { padding: 4px 4px 12px; }
    .handoff-lane, .topbar { padding-left: 24px; padding-right: 24px; }
    .modal { animation: pop .16s ease; }
  }

  @media (prefers-reduced-motion: reduce) {
    * { animation: none !important; transition: none !important; }
  }
</style>
