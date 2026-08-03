<script>
  import { COLUMNS } from "../lib/constants.js";
  import { projectLabel } from "../lib/format.js";
  import {
    agentLabel,
    agentStatusText,
    isWorking as checkWorking,
    waitStatusClass,
  } from "../lib/agent.js";
  import Icon from "./Icon.svelte";
  import RunControls from "./RunControls.svelte";

  let {
    task,
    menuOpen = false,
    agentMenuOpen = false,
    selectedAgent = "cursor",
    run = null,
    starting = false,
    hasPendingAsk = false,
    onOpen,
    onToggleMenu,
    onMove,
    onArchive,
    onRun,
    onCancel,
    onToggleAgentMenu,
    onSelectAgent,
    onDragStart,
  } = $props();

  const working = $derived(checkWorking({ isStarting: starting, run }));
  const statusClass = $derived(waitStatusClass({ isStarting: starting, run, hasPendingAsk }));
  const statusText = $derived(agentStatusText({ isStarting: starting, run, hasPendingAsk, selectedAgent }));
</script>

<article
  class="card"
  class:menu-open={menuOpen}
  class:working
  class:run-failed={run?.status === "failed"}
  class:run-done={run?.status === "exited"}
  draggable="true"
  ondragstart={(e) => onDragStart(e, task.id)}
>
  <div class="card-top">
    <button class="title" onclick={() => onOpen(task)}>{task.title}</button>
    <button
      class="menu-btn"
      aria-label="Task actions"
      aria-expanded={menuOpen}
      onclick={(e) => { e.stopPropagation(); onToggleMenu(); }}
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
        onclick={(e) => { e.stopPropagation(); onOpen(task); }}
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
        <p class="agent-msg muted">{agentLabel(run?.agent || selectedAgent)} is on it…</p>
      {/if}
    </div>
  {/if}
  <div class="card-actions">
    <RunControls
      {run}
      {starting}
      {selectedAgent}
      agentMenuOpen={agentMenuOpen}
      onRun={() => { onRun(task.id); }}
      onCancel={() => onCancel(task.id)}
      onToggleMenu={onToggleAgentMenu}
      onSelectAgent={(key) => onSelectAgent(key, task.id)}
    />
  </div>
  {#if menuOpen}
    <div class="menu" role="menu" tabindex="-1">
      {#each COLUMNS.filter((x) => x.key !== task.status) as m}
        <button role="menuitem" onclick={() => onMove(task.id, m.key)}>Move to {m.label}</button>
      {/each}
      <button role="menuitem" class="danger" onclick={() => onArchive(task.id)}>Archive</button>
    </div>
  {/if}
</article>
