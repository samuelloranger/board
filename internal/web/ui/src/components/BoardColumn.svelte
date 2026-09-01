<script>
  import TaskCard from "./TaskCard.svelte";

  let {
    column,
    tasks = [],
    active = false,
    drop = false,
    onDragOver,
    onDragLeave,
    onDrop,
  } = $props();
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
      <TaskCard task={t} />
    {/each}
    {#if tasks.length === 0}
      <div class="empty">Nothing here</div>
    {/if}
  </div>
</div>

<style>
  .col { display: none; flex-direction: column; }
  .col:global(.active) { display: flex; }
  .col-head { display: flex; align-items: center; gap: 8px; padding: 6px 2px 12px; }
  .col-dot { width: 9px; height: 9px; border-radius: 50%; }
  .col-head h2 { margin: 0; font-size: 14px; font-weight: 600; letter-spacing: -.01em; }
  .col-head .count {
    margin-left: auto; font-size: 12px; font-weight: 600; color: var(--muted);
    font-variant-numeric: tabular-nums;
  }
  .cards { display: flex; flex-direction: column; gap: 10px; min-height: 60px; }
  @media (min-width: 768px) {
    .col {
      display: flex; background: var(--surface-2); border: 1px solid var(--border);
      border-radius: 14px; padding: 12px;
    }
    .col:global(.drop) {
      border-color: var(--accent);
      background: color-mix(in srgb, var(--accent) 8%, var(--surface-2));
    }
    .col-head { padding: 4px 4px 12px; }
  }
</style>
