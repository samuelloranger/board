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

<style>
  .run-split {
    position: relative; display: inline-flex; align-items: stretch;
  }
  .run-split > :global(.btn-run) {
    border-radius: 9px 0 0 9px;
  }
  .btn-run-chevron {
    display: grid; place-items: center;
    min-width: 28px; border: none; border-left: 1px solid color-mix(in srgb, #fff 28%, transparent);
    border-radius: 0 9px 9px 0; background: var(--prog); color: #fff;
    cursor: pointer; padding: 0 4px;
    transition: filter .12s ease;
  }
  .btn-run-chevron :global(svg) { width: 14px; height: 14px; }
  .btn-run-chevron:hover { filter: brightness(1.08); }
  .btn-run-chevron:global(.sm) { min-width: 26px; }
  .run-split:global(.sm) > :global(.btn-run) { min-height: 32px; padding: 0 10px; font-size: 12px; }
  .run-menu {
    position: absolute; right: 0; bottom: calc(100% + 6px); z-index: 5;
    min-width: 120px; padding: 4px; border-radius: 10px;
    background: var(--surface); border: 1px solid var(--border);
    box-shadow: 0 8px 24px color-mix(in srgb, #000 28%, transparent);
    display: grid; gap: 2px;
  }
  .run-menu button {
    text-align: left; border: none; border-radius: 7px; background: transparent;
    color: var(--text); font-family: inherit; font-size: 13px; font-weight: 550;
    padding: 8px 10px; cursor: pointer;
  }
  .run-menu button:hover { background: color-mix(in srgb, var(--prog) 14%, transparent); }
  .run-menu button:global(.active) { color: var(--prog); font-weight: 700; }
</style>
