<script>
  import Icon from "./Icon.svelte";

  let {
    projectFilter = "*",
    chipProjects = [],
    showProjects = false,
    projectPaths = [],
    onSetFilter,
    onCloseProjects,
    onSavePath,
    onClearPath,
  } = $props();
</script>

<nav class="proj-filter" aria-label="Filter by project">
  <button class:active={projectFilter === "*"} onclick={() => onSetFilter("*")}>All</button>
  {#each chipProjects as p (p)}
    <button
      class:active={projectFilter === p}
      onclick={() => onSetFilter(p)}
    >{p === "_" ? "global" : p}</button>
  {/each}
</nav>

{#if showProjects}
  <section class="projects-panel" aria-label="Project paths">
    <div class="lane-head"><span>Project paths</span>
      <button class="icon-btn sm" onclick={onCloseProjects} aria-label="Close"><Icon name="close" /></button>
    </div>
    {#if projectPaths.length === 0}
      <div class="empty sm">No paths yet — set one when you Run a task.</div>
    {/if}
    {#each projectPaths as pp (pp.project)}
      <div class="proj-row">
        <code class="proj-name">{pp.project === "_" ? "(global)" : pp.project}</code>
        <input class="proj-path" value={pp.path} onchange={(e) => onSavePath(pp.project, e.currentTarget.value)} />
        <button class="btn-ghost sm danger" onclick={() => onClearPath(pp.project)}>Clear</button>
      </div>
    {/each}
  </section>
{/if}

<style>
  .proj-filter {
    display: flex; gap: 6px; flex-wrap: wrap; padding: 10px 16px 0;
  }
  .proj-filter button {
    font-family: inherit; font-size: 12px; font-weight: 600; padding: 5px 10px;
    border-radius: 999px; border: 1px solid var(--border); background: var(--surface);
    color: var(--muted); cursor: pointer;
  }
  .proj-filter button:global(.active) {
    color: var(--prog); border-color: color-mix(in srgb, var(--prog) 45%, var(--border));
    background: color-mix(in srgb, var(--prog) 12%, var(--surface));
  }
  .projects-panel {
    margin: 0 16px; padding: 12px; border: 1px solid var(--border); border-radius: var(--radius);
    background: var(--surface); display: flex; flex-direction: column; gap: 8px;
  }
  .projects-panel .lane-head {
    display: flex; align-items: center; justify-content: space-between;
    font-size: 13px; font-weight: 600; color: var(--muted);
  }
  .proj-row { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
  .proj-name { font-size: 12px; color: var(--amber); min-width: 72px; }
  .proj-path {
    flex: 1; min-width: 180px; padding: 8px 10px; border-radius: 8px;
    border: 1px solid var(--border); background: var(--surface-2); color: var(--text);
  }
</style>
