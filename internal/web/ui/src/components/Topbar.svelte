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

<style>
  .topbar {
    position: sticky; top: 0; z-index: 60;
    display: flex; align-items: center; justify-content: space-between;
    gap: 12px; padding: 12px 16px; padding-top: calc(12px + env(safe-area-inset-top));
    background: color-mix(in srgb, var(--bg) 85%, transparent);
    backdrop-filter: blur(12px); border-bottom: 1px solid var(--border);
  }
  .brand { display: flex; align-items: center; gap: 10px; }
  .logo {
    width: 22px; height: 22px; border-radius: 7px;
    background: linear-gradient(135deg, var(--accent), var(--prog));
    box-shadow: 0 0 0 1px color-mix(in srgb, var(--accent) 40%, transparent);
  }
  .brand h1 { font-size: 18px; font-weight: 700; margin: 0; letter-spacing: -.02em; }
  .actions { display: flex; align-items: center; gap: 8px; }
  @media (min-width: 768px) {
    .topbar { padding-left: 24px; padding-right: 24px; }
  }
</style>
