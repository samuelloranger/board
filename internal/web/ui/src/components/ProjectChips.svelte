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
