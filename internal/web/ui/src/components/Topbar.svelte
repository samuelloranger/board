<script>
  import Icon from "./Icon.svelte";

  let {
    theme,
    unseen = 0,
    onToggleProjects,
    onToggleActivity,
    onToggleTheme,
    onAddTask,
  } = $props();
</script>

<header class="topbar">
  <div class="brand">
    <span class="logo" aria-hidden="true"></span>
    <h1>board</h1>
  </div>
  <div class="actions">
    <button class="icon-btn" onclick={onToggleProjects} aria-label="Project paths" title="Project paths">
      <Icon name="folder" />
    </button>
    <button class="icon-btn" onclick={onToggleActivity} aria-label="Toggle activity">
      <Icon name="activity" />
      {#if unseen > 0}<span class="badge">{unseen}</span>{/if}
    </button>
    <button class="icon-btn" onclick={onToggleTheme} aria-label="Toggle theme">
      {#if theme === "dark"}<Icon name="sun" />{:else}<Icon name="moon" />{/if}
    </button>
    <button class="btn-primary" onclick={onAddTask}>
      <Icon name="plus" /><span>Task</span>
    </button>
  </div>
</header>
