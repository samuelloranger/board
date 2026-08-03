<script>
  import TaskCard from "./TaskCard.svelte";

  let {
    column,
    tasks = [],
    active = false,
    drop = false,
    openMenu = null,
    openAgentMenu = null,
    selectedAgent = "cursor",
    latestByTask = {},
    startingIds = {},
    pendingQuestions = [],
    onOpen,
    onToggleMenu,
    onMove,
    onArchive,
    onRun,
    onCancel,
    onToggleAgentMenu,
    onSelectAgent,
    onDragStart,
    onDragOver,
    onDragLeave,
    onDrop,
  } = $props();

  function hasPending(id) {
    return pendingQuestions.some((q) => q.task_id === id);
  }
</script>

<div
  class="col"
  class:active
  class:drop
  role="region"
  aria-label={column.label}
  ondragover={(e) => onDragOver(e, column.key)}
  ondragleave={() => onDragLeave(column.key)}
  ondrop={(e) => onDrop(e, column.key)}
>
  <div class="col-head">
    <span class="col-dot" style="background:{column.dot}"></span>
    <h2>{column.label}</h2>
    <span class="count">{tasks.length}</span>
  </div>
  <div class="cards">
    {#each tasks as t (t.id)}
      <TaskCard
        task={t}
        menuOpen={openMenu === t.id}
        agentMenuOpen={openAgentMenu === t.id}
        {selectedAgent}
        run={latestByTask[t.id] ?? null}
        starting={!!startingIds[t.id]}
        hasPendingAsk={hasPending(t.id)}
        {onOpen}
        onToggleMenu={() => onToggleMenu(t.id)}
        {onMove}
        {onArchive}
        {onRun}
        {onCancel}
        onToggleAgentMenu={() => onToggleAgentMenu(t.id)}
        {onSelectAgent}
        {onDragStart}
      />
    {/each}
    {#if tasks.length === 0}
      <div class="empty">Nothing here</div>
    {/if}
  </div>
</div>
