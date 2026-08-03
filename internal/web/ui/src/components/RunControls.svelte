<script>
  import { AGENTS } from "../lib/constants.js";
  import { isWorking as checkWorking } from "../lib/agent.js";
  import Icon from "./Icon.svelte";

  let {
    agentMenuOpen = false,
    selectedAgent = "cursor",
    sm = false,
    run = null,
    starting = false,
    onRun,
    onCancel,
    onToggleMenu,
    onSelectAgent,
  } = $props();

  const working = $derived(checkWorking({ isStarting: starting, run }));
</script>

{#if working}
  <button
    class="btn-run cancel"
    class:sm
    disabled={starting && run?.status !== "running"}
    onclick={(e) => { e.stopPropagation(); onCancel(); }}
  >Cancel</button>
{:else}
  <div class="run-split" class:sm>
    <button
      class="btn-run"
      class:sm
      onclick={(e) => { e.stopPropagation(); onRun(); }}
    ><Icon name="play" /><span>Run</span></button>
    <button
      class="btn-run-chevron"
      class:sm
      aria-label="Choose agent"
      aria-expanded={agentMenuOpen}
      onclick={(e) => { e.stopPropagation(); onToggleMenu(); }}
    ><Icon name="chevron" /></button>
    {#if agentMenuOpen}
      <div class="run-menu" role="menu">
        {#each AGENTS as a}
          <button
            role="menuitem"
            class:active={selectedAgent === a.key}
            onclick={(e) => { e.stopPropagation(); onSelectAgent(a.key); }}
          >{a.label}</button>
        {/each}
      </div>
    {/if}
  </div>
{/if}
