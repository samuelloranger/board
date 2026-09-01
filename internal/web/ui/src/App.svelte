<script>
  import "./styles.css";
  import { COLUMNS } from "./lib/constants.js";
  import { apiHeaders } from "./lib/api.js";
  import {
    boardUI,
    latestRun,
    setSelectedAgent,
    wireBoardActions,
  } from "./lib/board-ui.svelte.js";
  import { detailUI, runUI } from "./lib/detail-ui.svelte.js";
  import Topbar from "./components/Topbar.svelte";
  import ProjectChips from "./components/ProjectChips.svelte";
  import HandoffLane from "./components/HandoffLane.svelte";
  import BoardColumn from "./components/BoardColumn.svelte";
  import AddTaskModal from "./components/AddTaskModal.svelte";
  import ActivityDrawer from "./components/ActivityDrawer.svelte";
  import TaskDetail from "./components/TaskDetail.svelte";

  let board = $state({ todo: [], in_progress: [], done: [] });
  let events = $state([]);
  let handoffs = $state([]);
  let theme = $state("dark");
  let activeCol = $state("todo");
  let showActivity = $state(false);
  let showAdd = $state(false);
  let dragOver = $state(null);
  let addTitle = $state("");
  let addDescription = $state("");
  let addPriority = $state("");
  let addProject = $state("");
  let unseen = $state(0);
  let projectPaths = $state([]);
  let chipProjects = $state([]);
  let showProjects = $state(false);
  let projectFilter = $state("*");
  let showNotifyBanner = $state(false);

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
    boardUI.pendingQuestions = await (await fetch("/api/questions?status=pending")).json();
    const runs = await (await fetch("/api/runs")).json();
    const map = {};
    for (const r of runs ?? []) {
      if (map[r.task_id] == null) map[r.task_id] = r;
    }
    boardUI.latestByTask = map;
    projectPaths = await (await fetch("/api/projects/paths")).json();
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
    if (!document.hidden && detailUI.detail?.id === taskId) return;
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
    if (e.kind === "tool" || e.kind === "run_progress") return false;
    if (projectFilter === "*") return true;
    if (!e.task_id) return true;
    const t = findTask(e.task_id);
    if (!t) return true;
    return (t.project || "") === projectFilter || (projectFilter === "_" && !t.project);
  }
  function isActivityEvent(e) {
    return e.kind !== "tool" && e.kind !== "run_progress";
  }
  async function runTask(id, agent = boardUI.selectedAgent) {
    runUI.runError = "";
    runUI.needPath = null;
    setSelectedAgent(agent);
    boardUI.startingIds = { ...boardUI.startingIds, [id]: true };
    try {
      const resp = await fetch(`/api/tasks/${id}/run`, {
        method: "POST", headers: apiHeaders({ "Content-Type": "application/json" }),
        body: JSON.stringify({ agent }),
      });
      if (resp.status === 409) {
        const body = await resp.json();
        if (body.need_path) {
          runUI.needPath = { project: body.project, taskId: id };
          runUI.pathInput = "";
          if (!detailUI.detail || detailUI.detail.id !== id) {
            const t = findTask(id);
            if (t) openDetail(t);
          }
          return;
        }
        runUI.runError = body.error || "Conflict";
        return;
      }
      if (!resp.ok) {
        runUI.runError = await resp.text();
        return;
      }
      await load();
      if (detailUI.detail?.id === id) await refreshDetail();
    } finally {
      const next = { ...boardUI.startingIds };
      delete next[id];
      boardUI.startingIds = next;
    }
  }
  async function savePathAndRun() {
    if (!runUI.needPath || !runUI.pathInput.trim()) return;
    const put = await fetch(`/api/projects/${encodeURIComponent(runUI.needPath.project)}/path`, {
      method: "PUT", headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ path: runUI.pathInput.trim() }),
    });
    if (!put.ok) {
      runUI.runError = await put.text();
      return;
    }
    const tid = runUI.needPath.taskId;
    runUI.needPath = null;
    await runTask(tid);
  }
  async function cancelRun(id) {
    await fetch(`/api/tasks/${id}/run/cancel`, { method: "POST", headers: apiHeaders() });
    await load();
    if (detailUI.detail?.id === id) await refreshDetail();
  }
  async function submitAnswer(qid) {
    const answer = detailUI.answerDraft.trim();
    if (!answer) return;
    await fetch(`/api/questions/${qid}/answer`, {
      method: "POST", headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ answer }),
    });
    detailUI.answerDraft = "";
    await loadAgentState();
  }
  async function saveProjectPath(project, path) {
    const put = await fetch(`/api/projects/${encodeURIComponent(project)}/path`, {
      method: "PUT", headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ path }),
    });
    if (!put.ok) {
      runUI.runError = await put.text();
      return;
    }
    await loadAgentState();
  }
  async function clearProjectPath(project) {
    await fetch(`/api/projects/${encodeURIComponent(project)}/path`, { method: "DELETE", headers: apiHeaders() });
    await loadAgentState();
  }
  async function move(id, status) {
    boardUI.openMenu = null;
    await fetch(`/api/tasks/${id}/move`, {
      method: "POST", headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ status }),
    });
    await load();
  }
  async function archive(id) {
    boardUI.openMenu = null;
    await fetch(`/api/tasks/${id}/archive`, { method: "POST", headers: apiHeaders() });
    await load();
  }
  async function clearHandoff(id, e) {
    e?.stopPropagation();
    await fetch(`/api/tasks/${id}/clear_handoff`, { method: "POST", headers: apiHeaders() });
    await load();
    if (detailUI.detail?.id === id) await refreshDetail();
  }
  async function createTask() {
    const title = addTitle.trim();
    if (!title) return;
    const project = addProject.trim();
    const description = addDescription.trim();
    await fetch("/api/tasks", {
      method: "POST", headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({
        title, status: "todo", priority: addPriority,
        ...(description ? { description } : {}),
        ...(project ? { project } : {}),
      }),
    });
    addTitle = ""; addDescription = ""; addPriority = ""; addProject = ""; showAdd = false;
    await load();
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
  async function fetchTask(id) {
    const resp = await fetch(`/api/tasks/${id}`);
    if (!resp.ok) return null;
    return await resp.json();
  }
  async function openDetail(t, { skipURL = false, replaceURL = false } = {}) {
    detailUI.editing = false;
    detailUI.noteBody = "";
    runUI.runError = "";
    runUI.needPath = null;
    detailUI.animNoteIds = {};
    detailUI.detail = t;
    if (!skipURL) {
      if (taskIdFromURL() === t.id || replaceURL) setTaskURL(t.id, { replace: true });
      else setTaskURL(t.id);
    }
    const full = await fetchTask(t.id);
    if (full && detailUI.detail?.id === t.id) {
      detailUI.detail = full;
      detailUI.seenNoteIds = new Set((full.notes ?? []).map((n) => n.id));
    } else if (!full && detailUI.detail?.id === t.id && !t.title) {
      closeDetail({ skipURL: false });
      return;
    } else {
      detailUI.seenNoteIds = new Set();
    }
    if (detailUI.detail?.id === t.id) await loadRunFilesForDetail();
  }
  function applyNoteEntrance(notes) {
    const ids = (notes ?? []).map((n) => n.id);
    const fresh = ids.filter((id) => !detailUI.seenNoteIds.has(id));
    detailUI.seenNoteIds = new Set([...detailUI.seenNoteIds, ...ids]);
    if (!fresh.length) return;
    const next = { ...detailUI.animNoteIds };
    for (const id of fresh) next[id] = true;
    detailUI.animNoteIds = next;
    setTimeout(() => {
      const cleared = { ...detailUI.animNoteIds };
      for (const id of fresh) delete cleared[id];
      detailUI.animNoteIds = cleared;
    }, 420);
  }
  async function refreshDetail() {
    if (!detailUI.detail) return;
    const full = await fetchTask(detailUI.detail.id);
    if (full) {
      applyNoteEntrance(full.notes);
      detailUI.detail = full;
    } else {
      const t = findTask(detailUI.detail.id);
      if (t) detailUI.detail = t;
    }
    await loadRunFilesForDetail();
  }
  async function loadRunFilesForDetail() {
    if (!detailUI.detail) {
      detailUI.runFiles = [];
      return;
    }
    const r = latestRun(detailUI.detail.id);
    if (!r?.id) {
      detailUI.runFiles = [];
      return;
    }
    try {
      const resp = await fetch(`/api/runs/${r.id}/files`);
      if (!resp.ok) {
        detailUI.runFiles = [];
        return;
      }
      detailUI.runFiles = await resp.json() ?? [];
    } catch {
      detailUI.runFiles = [];
    }
  }
  function closeDetail({ skipURL = false } = {}) {
    detailUI.detail = null;
    detailUI.editing = false;
    runUI.needPath = null;
    runUI.runError = "";
    detailUI.seenNoteIds = new Set();
    detailUI.animNoteIds = {};
    detailUI.runFiles = [];
    if (!skipURL && taskIdFromURL() != null) setTaskURL(null);
  }
  function startEdit() {
    const d = detailUI.detail;
    detailUI.edit = {
      title: d.title ?? "",
      description: d.description ?? "",
      priority: d.priority ?? "",
      due_date: d.due_date ?? "",
      tags: (d.tags ?? []).join(", "),
      project: d.project ?? "",
    };
    detailUI.editing = true;
  }
  async function saveEdit() {
    const title = detailUI.edit.title.trim();
    if (!title) return;
    const tags = detailUI.edit.tags.split(",").map((s) => s.trim()).filter(Boolean);
    await fetch(`/api/tasks/${detailUI.detail.id}/update`, {
      method: "POST", headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({
        title, description: detailUI.edit.description,
        priority: detailUI.edit.priority, due_date: detailUI.edit.due_date, tags,
        project: detailUI.edit.project.trim(),
      }),
    });
    await load();
    if (detailUI.detail) await refreshDetail();
    detailUI.editing = false;
  }
  async function addNote() {
    const body = detailUI.noteBody.trim();
    if (!body) return;
    await fetch(`/api/tasks/${detailUI.detail.id}/note`, {
      method: "POST", headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ body }),
    });
    detailUI.noteBody = "";
    await load();
    if (detailUI.detail) await refreshDetail();
  }
  async function moveFromDetail(status) {
    await move(detailUI.detail.id, status);
    if (detailUI.detail) await refreshDetail();
  }

  function onDragStart(e, id) { e.dataTransfer.setData("id", String(id)); e.dataTransfer.effectAllowed = "move"; }
  function onDrop(e, status) {
    e.preventDefault(); dragOver = null;
    const id = e.dataTransfer.getData("id");
    if (id) move(Number(id), status);
  }
  function eventTitle(e) {
    if (!e.task_id) return "";
    const t = findTask(e.task_id);
    return t ? t.title : "";
  }

  wireBoardActions({
    onOpen: openDetail,
    onToggleMenu: (id) => { boardUI.openMenu = boardUI.openMenu === id ? null : id; },
    onMove: move,
    onArchive: archive,
    onRun: (id) => { boardUI.openMenu = null; boardUI.openAgentMenu = null; runTask(id); },
    onCancel: cancelRun,
    onToggleAgentMenu: (id) => {
      boardUI.openMenu = null;
      boardUI.openAgentMenu = boardUI.openAgentMenu === id ? null : id;
    },
    onSelectAgent: (key, id) => { setSelectedAgent(key); runTask(id, key); },
    onDragStart,
  });

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

    let syncing = true;
    let loadTimer = null;
    const scheduleLoad = () => {
      clearTimeout(loadTimer);
      loadTimer = setTimeout(async () => {
        await load();
        if (detailUI.detail) await refreshDetail();
      }, 120);
    };

    const es = new EventSource("/api/events");
    es.onmessage = (m) => {
      const ev = JSON.parse(m.data);
      if (isActivityEvent(ev)) {
        events = [ev, ...events].slice(0, 60);
        if (!syncing) unseen = Math.min(unseen + 1, 99);
      }
      if (!syncing && ev.kind === "question" && ev.task_id) {
        notifyAsk(ev.task_id, ev.detail);
        if (!detailUI.detail) openDetail(findTask(ev.task_id) ?? { id: ev.task_id });
      }
      scheduleLoad();
    };
    es.addEventListener("synced", () => { syncing = false; });

    const onPop = () => {
      const id = taskIdFromURL();
      if (id) {
        if (detailUI.detail?.id !== id) openDetail(findTask(id) ?? { id }, { skipURL: true });
      } else if (detailUI.detail) {
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

  $effect(() => {
    if (boardUI.openMenu === null) return;
    let remove = () => {};
    const timer = setTimeout(() => {
      const onDocClick = (e) => {
        if (!e.target.closest(".menu") && !e.target.closest(".menu-btn")) boardUI.openMenu = null;
      };
      window.addEventListener("click", onDocClick);
      remove = () => window.removeEventListener("click", onDocClick);
    }, 0);
    return () => { clearTimeout(timer); remove(); };
  });

  $effect(() => {
    if (boardUI.openAgentMenu === null) return;
    let remove = () => {};
    const timer = setTimeout(() => {
      const onDocClick = (e) => {
        if (!e.target.closest(".run-split")) boardUI.openAgentMenu = null;
      };
      window.addEventListener("click", onDocClick);
      remove = () => window.removeEventListener("click", onDocClick);
    }, 0);
    return () => { clearTimeout(timer); remove(); };
  });
</script>

<Topbar
  {theme}
  {unseen}
  onToggleProjects={() => (showProjects = !showProjects)}
  onToggleActivity={toggleActivity}
  onToggleTheme={toggleTheme}
  onAddTask={() => (showAdd = true)}
/>

{#if showNotifyBanner}
  <div class="notify-banner" role="status">
    <span>Notify me when an agent asks</span>
    <div class="notify-actions">
      <button class="btn-primary sm" onclick={enableAskNotify}>Enable</button>
      <button class="btn-ghost sm" onclick={dismissNotifyBanner}>Not now</button>
    </div>
  </div>
{/if}

<ProjectChips
  {projectFilter}
  {chipProjects}
  {showProjects}
  {projectPaths}
  onSetFilter={setProjectFilter}
  onCloseProjects={() => (showProjects = false)}
  onSavePath={saveProjectPath}
  onClearPath={clearProjectPath}
/>

<HandoffLane
  {handoffs}
  onOpen={openDetail}
  onClear={clearHandoff}
/>

<nav class="segmented" aria-label="Select column">
  {#each COLUMNS as c (c.key)}
    <button class:active={activeCol === c.key} onclick={() => (activeCol = c.key)}>
      {c.label}<span class="seg-count">{board[c.key]?.length ?? 0}</span>
    </button>
  {/each}
</nav>

<main class="board">
  {#each COLUMNS as c (c.key)}
    <BoardColumn
      column={c}
      tasks={board[c.key] ?? []}
      active={activeCol === c.key}
      drop={dragOver === c.key}
      onDragOver={(e, key) => { e.preventDefault(); dragOver = key; }}
      onDragLeave={(key) => { if (dragOver === key) dragOver = null; }}
      {onDrop}
    />
  {/each}
</main>

{#if showAdd}
  <AddTaskModal
    bind:title={addTitle}
    bind:description={addDescription}
    bind:priority={addPriority}
    bind:project={addProject}
    {projectPaths}
    onClose={() => (showAdd = false)}
    onCreate={createTask}
  />
{/if}

{#if showActivity}
  <ActivityDrawer
    {events}
    {unseen}
    {eventVisible}
    {eventTitle}
    onClose={() => (showActivity = false)}
    onMarkSeen={markAllSeen}
    onOpenTask={(taskId) => {
      const t = findTask(taskId);
      if (t) { showActivity = false; openDetail(t); }
    }}
  />
{/if}

{#if detailUI.detail}
  <TaskDetail
    {projectPaths}
    onClose={closeDetail}
    onStartEdit={startEdit}
    onCancelEdit={() => (detailUI.editing = false)}
    onSaveEdit={saveEdit}
    onEditChange={(next) => (detailUI.edit = next)}
    onMove={moveFromDetail}
    onClearHandoff={clearHandoff}
    onRun={(id) => { boardUI.openAgentMenu = null; runTask(id); }}
    onCancelRun={cancelRun}
    onToggleAgentMenu={() => {
      boardUI.openAgentMenu = boardUI.openAgentMenu === "detail" ? null : "detail";
    }}
    onSelectAgent={(key, id) => { setSelectedAgent(key); runTask(id, key); }}
    onSavePathAndRun={savePathAndRun}
    onCancelPath={() => (runUI.needPath = null)}
    onPathInput={(v) => (runUI.pathInput = v)}
    onSubmitAnswer={submitAnswer}
    onAnswerDraft={(v) => (detailUI.answerDraft = v)}
    onAddNote={addNote}
    onNoteBody={(v) => (detailUI.noteBody = v)}
  />
{/if}

<style>
  .notify-banner {
    display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap;
    margin: 8px 16px 0; padding: 10px 14px; border-radius: 10px;
    border: 1px solid color-mix(in srgb, var(--amber) 40%, var(--border));
    background: color-mix(in srgb, var(--amber) 10%, var(--surface));
    font-size: 13px; color: var(--text);
  }
  .notify-actions { display: flex; gap: 8px; }
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
  .segmented button:global(.active) { background: var(--surface); color: var(--text); box-shadow: var(--shadow); }
  .seg-count {
    font-size: 11px; font-weight: 700; padding: 1px 6px; border-radius: 999px;
    background: var(--surface-3); font-variant-numeric: tabular-nums;
  }
  .board { padding: 0 16px 24px; }
  @media (min-width: 768px) {
    .segmented { display: none; }
    .board { display: grid; grid-template-columns: repeat(3, 1fr); gap: 16px; padding: 8px 24px 32px; }
  }
</style>
