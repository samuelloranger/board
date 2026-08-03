<script>
  import "./styles.css";
  import { AGENT_LABEL, COLUMNS } from "./lib/constants.js";
  import { apiHeaders } from "./lib/api.js";
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
  let animNoteIds = $state({});
  let pendingQuestions = $state([]);
  let latestByTask = $state({});
  let startingIds = $state({});
  let projectPaths = $state([]);
  let chipProjects = $state([]);
  let showProjects = $state(false);
  let needPath = $state(null);
  let pathInput = $state("");
  let runError = $state("");
  let answerDraft = $state("");
  let projectFilter = $state("*");
  let showNotifyBanner = $state(false);
  let selectedAgent = $state(loadSelectedAgent());
  let runFiles = $state([]);

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
    pendingQuestions = await (await fetch("/api/questions?status=pending")).json();
    const runs = await (await fetch("/api/runs")).json();
    const map = {};
    for (const r of runs ?? []) {
      if (map[r.task_id] == null) map[r.task_id] = r;
    }
    latestByTask = map;
    projectPaths = await (await fetch("/api/projects/paths")).json();
  }
  function latestRun(id) {
    return latestByTask[id] ?? null;
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
  async function runTask(id, agent = selectedAgent) {
    runError = "";
    needPath = null;
    setSelectedAgent(agent);
    startingIds = { ...startingIds, [id]: true };
    try {
      const resp = await fetch(`/api/tasks/${id}/run`, {
        method: "POST", headers: apiHeaders({ "Content-Type": "application/json" }),
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
      method: "PUT", headers: apiHeaders({ "Content-Type": "application/json" }),
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
    await fetch(`/api/tasks/${id}/run/cancel`, { method: "POST", headers: apiHeaders() });
    await load();
    if (detail?.id === id) await refreshDetail();
  }
  async function submitAnswer(qid) {
    const answer = answerDraft.trim();
    if (!answer) return;
    await fetch(`/api/questions/${qid}/answer`, {
      method: "POST", headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ answer }),
    });
    answerDraft = "";
    await loadAgentState();
  }
  async function saveProjectPath(project, path) {
    const put = await fetch(`/api/projects/${encodeURIComponent(project)}/path`, {
      method: "PUT", headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ path }),
    });
    if (!put.ok) {
      runError = await put.text();
      return;
    }
    await loadAgentState();
  }
  async function clearProjectPath(project) {
    await fetch(`/api/projects/${encodeURIComponent(project)}/path`, { method: "DELETE", headers: apiHeaders() });
    await loadAgentState();
  }
  async function move(id, status) {
    openMenu = null;
    await fetch(`/api/tasks/${id}/move`, {
      method: "POST", headers: apiHeaders({ "Content-Type": "application/json" }),
      body: JSON.stringify({ status }),
    });
    await load();
  }
  async function archive(id) {
    openMenu = null;
    await fetch(`/api/tasks/${id}/archive`, { method: "POST", headers: apiHeaders() });
    await load();
  }
  async function clearHandoff(id, e) {
    e?.stopPropagation();
    await fetch(`/api/tasks/${id}/clear_handoff`, { method: "POST", headers: apiHeaders() });
    await load();
    if (detail?.id === id) await refreshDetail();
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
    editing = false; noteBody = ""; runError = ""; needPath = null;
    animNoteIds = {};
    detail = t;
    if (!skipURL) {
      if (taskIdFromURL() === t.id || replaceURL) setTaskURL(t.id, { replace: true });
      else setTaskURL(t.id);
    }
    const full = await fetchTask(t.id);
    if (full && detail?.id === t.id) {
      detail = full;
      seenNoteIds = new Set((full.notes ?? []).map((n) => n.id));
    } else if (!full && detail?.id === t.id && !t.title) {
      closeDetail({ skipURL: false });
      return;
    } else {
      seenNoteIds = new Set();
    }
    if (detail?.id === t.id) await loadRunFilesForDetail();
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
    await loadRunFilesForDetail();
  }
  async function loadRunFilesForDetail() {
    if (!detail) {
      runFiles = [];
      return;
    }
    const r = latestRun(detail.id);
    if (!r?.id) {
      runFiles = [];
      return;
    }
    try {
      const resp = await fetch(`/api/runs/${r.id}/files`);
      if (!resp.ok) {
        runFiles = [];
        return;
      }
      runFiles = await resp.json() ?? [];
    } catch {
      runFiles = [];
    }
  }
  function closeDetail({ skipURL = false } = {}) {
    detail = null; editing = false; needPath = null; runError = "";
    seenNoteIds = new Set(); animNoteIds = {};
    runFiles = [];
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
      method: "POST", headers: apiHeaders({ "Content-Type": "application/json" }),
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
      method: "POST", headers: apiHeaders({ "Content-Type": "application/json" }),
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
  function eventTitle(e) {
    if (!e.task_id) return "";
    const t = findTask(e.task_id);
    return t ? t.title : "";
  }

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
        if (detail) await refreshDetail();
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
      {openMenu}
      {openAgentMenu}
      {selectedAgent}
      {latestByTask}
      {startingIds}
      {pendingQuestions}
      onOpen={openDetail}
      onToggleMenu={(id) => (openMenu = openMenu === id ? null : id)}
      onMove={move}
      onArchive={archive}
      onRun={(id) => { openMenu = null; openAgentMenu = null; runTask(id); }}
      onCancel={cancelRun}
      onToggleAgentMenu={(id) => { openMenu = null; openAgentMenu = openAgentMenu === id ? null : id; }}
      onSelectAgent={(key, id) => { setSelectedAgent(key); runTask(id, key); }}
      {onDragStart}
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

{#if detail}
  <TaskDetail
    {detail}
    {editing}
    bind:edit
    bind:noteBody
    bind:answerDraft
    bind:pathInput
    {animNoteIds}
    {projectPaths}
    {runFiles}
    {needPath}
    {runError}
    {openAgentMenu}
    {selectedAgent}
    run={latestRun(detail.id)}
    starting={!!startingIds[detail.id]}
    pendingAsk={pendingAskFor(detail.id)}
    onClose={closeDetail}
    onStartEdit={startEdit}
    onCancelEdit={() => (editing = false)}
    onSaveEdit={saveEdit}
    onMove={moveFromDetail}
    onClearHandoff={clearHandoff}
    onRun={(id) => { openAgentMenu = null; runTask(id); }}
    onCancelRun={cancelRun}
    onToggleAgentMenu={() => (openAgentMenu = openAgentMenu === "detail" ? null : "detail")}
    onSelectAgent={(key, id) => { setSelectedAgent(key); runTask(id, key); }}
    onSavePathAndRun={savePathAndRun}
    onCancelPath={() => (needPath = null)}
    onSubmitAnswer={submitAnswer}
    onAddNote={addNote}
  />
{/if}
