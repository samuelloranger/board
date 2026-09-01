<script>
  import { COLUMNS } from "../lib/constants.js";
  import { projectLabel } from "../lib/format.js";
  import {
    agentLabel,
    agentStatusText,
    isWorking as checkWorking,
    waitStatusClass,
  } from "../lib/agent.js";
  import {
    boardActions,
    boardUI,
    hasPendingAsk as checkPendingAsk,
    isStarting,
    latestRun,
  } from "../lib/board-ui.svelte.js";
  import Icon from "./Icon.svelte";
  import RunControls from "./RunControls.svelte";

  let { task } = $props();

  const run = $derived(latestRun(task.id));
  const starting = $derived(isStarting(task.id));
  const hasPendingAsk = $derived(checkPendingAsk(task.id));
  const menuOpen = $derived(boardUI.openMenu === task.id);
  const agentMenuOpen = $derived(boardUI.openAgentMenu === task.id);
  const working = $derived(checkWorking({ isStarting: starting, run }));
  const statusClass = $derived(waitStatusClass({ isStarting: starting, run, hasPendingAsk }));
  const statusText = $derived(agentStatusText({
    isStarting: starting,
    run,
    hasPendingAsk,
    selectedAgent: boardUI.selectedAgent,
  }));
</script>

<article
  class="card"
  class:menu-open={menuOpen}
  class:working
  class:run-failed={run?.status === "failed"}
  class:run-done={run?.status === "exited"}
  draggable="true"
  ondragstart={(e) => boardActions.onDragStart(e, task.id)}
>
  <div class="card-top">
    <button class="title" onclick={() => boardActions.onOpen(task)}>{task.title}</button>
    <button
      class="menu-btn"
      aria-label="Task actions"
      aria-expanded={menuOpen}
      onclick={(e) => { e.stopPropagation(); boardActions.onToggleMenu(task.id); }}
    >
      <Icon name="more" />
    </button>
  </div>
  <div class="meta">
    <span class="proj" class:is-global={!task.project}>{projectLabel(task.project)}</span>
    {#if task.priority}<span class="pri pri-{task.priority}">{task.priority}</span>{/if}
    {#each task.tags ?? [] as tag}<span class="tag">{tag}</span>{/each}
    {#if task.handoff_to}<span class="hbadge"><Icon name="handoff" />{task.handoff_to}</span>{/if}
    {#if hasPendingAsk}
      <button
        type="button"
        class="askbadge"
        onclick={(e) => { e.stopPropagation(); boardActions.onOpen(task); }}
      >asks</button>
    {/if}
  </div>
  {#if run || starting || hasPendingAsk}
    <div class="agent-status s-{statusClass}">
      <span class="agent-label">{statusText}</span>
      {#if (task.recent_agent_notes ?? []).length}
        <ul class="agent-thread">
          {#each task.recent_agent_notes as n (n.id)}
            <li class="agent-thread-item">{n.body}</li>
          {/each}
        </ul>
      {:else if run?.message}
        <p class="agent-msg">{run.message}</p>
      {:else if working && !hasPendingAsk}
        <p class="agent-msg muted">{agentLabel(run?.agent || boardUI.selectedAgent)} is on it…</p>
      {/if}
    </div>
  {/if}
  <div class="card-actions">
    <RunControls
      {run}
      {starting}
      selectedAgent={boardUI.selectedAgent}
      agentMenuOpen={agentMenuOpen}
      onRun={() => boardActions.onRun(task.id)}
      onCancel={() => boardActions.onCancel(task.id)}
      onToggleMenu={() => boardActions.onToggleAgentMenu(task.id)}
      onSelectAgent={(key) => boardActions.onSelectAgent(key, task.id)}
    />
  </div>
  {#if menuOpen}
    <div class="menu" role="menu" tabindex="-1">
      {#each COLUMNS.filter((x) => x.key !== task.status) as m}
        <button role="menuitem" onclick={() => boardActions.onMove(task.id, m.key)}>Move to {m.label}</button>
      {/each}
      <button role="menuitem" class="danger" onclick={() => boardActions.onArchive(task.id)}>Archive</button>
    </div>
  {/if}
</article>

<style>
  .card {
    position: relative; padding: 12px 12px 12px 14px; border-radius: var(--radius);
    background: var(--surface); border: 1px solid var(--border); box-shadow: var(--shadow);
    cursor: grab; transition: transform .1s ease, border-color .12s ease, box-shadow .12s ease;
  }
  .card:hover { border-color: color-mix(in srgb, var(--accent) 45%, var(--border)); transform: translateY(-1px); }
  .card:active { cursor: grabbing; }
  .card:global(.menu-open) { z-index: 50; }
  .card:global(.working) { box-shadow: 0 0 0 1px color-mix(in srgb, var(--prog) 50%, transparent), var(--shadow); }
  .card:global(.run-failed) { box-shadow: 0 0 0 1px color-mix(in srgb, var(--danger) 40%, transparent), var(--shadow); }
  .card:global(.run-done) { box-shadow: 0 0 0 1px color-mix(in srgb, var(--done) 35%, transparent), var(--shadow); }
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
  .menu-btn :global(svg) { width: 16px; height: 16px; }
  .meta { display: flex; flex-wrap: wrap; gap: 6px; margin-top: 10px; }
  .hbadge {
    display: inline-flex; align-items: center; gap: 4px; font-size: 11px; font-weight: 600;
    padding: 2px 8px; border-radius: 999px; color: var(--amber);
    background: color-mix(in srgb, var(--amber) 13%, transparent);
  }
  .hbadge :global(svg) { width: 12px; height: 12px; }
  .askbadge {
    font-size: 11px; font-weight: 700; text-transform: uppercase; letter-spacing: .02em;
    padding: 2px 7px; border-radius: 999px; border: none; cursor: pointer;
    color: var(--amber); background: color-mix(in srgb, var(--amber) 18%, transparent);
    font-family: inherit;
  }
  .askbadge:hover { background: color-mix(in srgb, var(--amber) 28%, transparent); }
  .card-actions { display: flex; gap: 8px; margin-top: 10px; }
  .agent-status {
    margin-top: 10px; padding: 8px 10px; border-radius: 9px;
    border: 1px solid var(--border); background: var(--surface-2);
    display: flex; flex-direction: column; gap: 4px;
  }
  .agent-status:global(.s-starting), .agent-status:global(.s-running) {
    border-color: color-mix(in srgb, var(--prog) 45%, var(--border));
    background: color-mix(in srgb, var(--prog) 10%, var(--surface-2));
  }
  .agent-status:global(.s-waiting-you) {
    border-color: color-mix(in srgb, var(--amber) 45%, var(--border));
    background: color-mix(in srgb, var(--amber) 12%, var(--surface-2));
  }
  .agent-status:global(.s-waiting-ci) {
    border-color: color-mix(in srgb, var(--prog) 35%, var(--border));
    background: color-mix(in srgb, var(--prog) 8%, var(--surface-2));
  }
  .agent-status:global(.s-waiting-you) .agent-label { color: var(--amber); }
  .agent-status:global(.s-waiting-ci) .agent-label { color: var(--prog); }
  .agent-status:global(.s-exited) {
    border-color: color-mix(in srgb, var(--done) 45%, var(--border));
    background: color-mix(in srgb, var(--done) 10%, var(--surface-2));
  }
  .agent-status:global(.s-failed), .agent-status:global(.s-killed) {
    border-color: color-mix(in srgb, var(--danger) 45%, var(--border));
    background: color-mix(in srgb, var(--danger) 10%, var(--surface-2));
  }
  .agent-label {
    font-size: 11px; font-weight: 800; text-transform: uppercase; letter-spacing: .04em;
    color: var(--prog);
  }
  .agent-status:global(.s-exited) .agent-label { color: var(--done); }
  .agent-status:global(.s-failed) .agent-label,
  .agent-status:global(.s-killed) .agent-label { color: var(--danger); }
  .agent-msg {
    margin: 0; font-size: 12px; line-height: 1.4; color: var(--text);
    white-space: pre-wrap; word-break: break-word;
    display: -webkit-box; -webkit-line-clamp: 4; -webkit-box-orient: vertical; overflow: hidden;
  }
  .agent-msg:global(.muted) { color: var(--muted); -webkit-line-clamp: 2; }
  .agent-thread {
    list-style: none; margin: 6px 0 0; padding: 0;
    display: flex; flex-direction: column; gap: 4px;
  }
  .agent-thread-item {
    margin: 0; font-size: 11px; line-height: 1.35; color: var(--text);
    display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden;
    opacity: 0.9;
  }
  .menu {
    position: absolute; top: 40px; right: 10px; z-index: 80; min-width: 168px; padding: 6px;
    background: var(--surface); border: 1px solid var(--border); border-radius: 10px;
    box-shadow: 0 8px 28px rgba(0,0,0,.35); display: flex; flex-direction: column; gap: 2px;
    animation: popmenu .12s ease; transform-origin: top right;
  }
  .menu button {
    text-align: left; padding: 9px 10px; min-height: 40px; border: none; border-radius: 7px;
    background: transparent; color: var(--text); font-family: inherit; font-size: 13px; cursor: pointer;
  }
  .menu button:hover { background: var(--surface-2); }
  .menu button:global(.danger) { color: var(--danger); }
</style>
