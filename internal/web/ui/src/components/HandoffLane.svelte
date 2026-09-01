<script>
  import Icon from "./Icon.svelte";
  import { projectLabel } from "../lib/format.js";

  let {
    handoffs = [],
    onOpen,
    onClear,
  } = $props();
</script>

{#if handoffs.length}
  <section class="handoff-lane" aria-label="Handoffs and inbox">
    <div class="lane-head"><Icon name="handoff" /><span>Handoffs</span></div>
    <div class="lane-scroll">
      {#each handoffs as t (t.id)}
        <div
          class="handoff-chip"
          class:human={t.handoff_to === "human"}
          onclick={() => onOpen(t)}
          role="button"
          tabindex="0"
          onkeydown={(e) => e.key === "Enter" && onOpen(t)}
        >
          <span class="to">{t.handoff_to}</span>
          <span class="ht">{t.title}</span>
          <span class="hp">{projectLabel(t.project)}</span>
          {#if t.handoff_reason}<span class="hr">{t.handoff_reason}</span>{/if}
          <button class="chip-x" onclick={(e) => onClear(t.id, e)} aria-label="Clear handoff" title="Clear handoff">
            <Icon name="close" />
          </button>
        </div>
      {/each}
    </div>
  </section>
{/if}

<style>
  .handoff-lane { padding: 12px 16px 0; }
  .lane-head {
    display: flex; align-items: center; gap: 6px; font-size: 12px; font-weight: 600;
    color: var(--amber); text-transform: uppercase; letter-spacing: .04em; margin-bottom: 8px;
  }
  .lane-head :global(svg) { width: 15px; height: 15px; }
  .lane-scroll { display: flex; gap: 10px; overflow-x: auto; padding-bottom: 4px; scrollbar-width: thin; }
  .handoff-chip {
    position: relative; cursor: pointer;
    flex: 0 0 auto; max-width: 280px; padding: 8px 32px 8px 12px; border-radius: 10px;
    background: color-mix(in srgb, var(--amber) 12%, var(--surface));
    border: 1px solid color-mix(in srgb, var(--amber) 40%, var(--border));
    display: flex; flex-direction: column; gap: 2px;
  }
  .handoff-chip .to { font-size: 11px; font-weight: 700; color: var(--amber); text-transform: uppercase; }
  .handoff-chip .ht { font-size: 13px; font-weight: 500; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .handoff-chip .hp { font-size: 11px; font-weight: 600; color: var(--prog); }
  .handoff-chip .hr { font-size: 12px; color: var(--muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .handoff-chip:global(.human) { box-shadow: 0 0 0 1px var(--amber); }
  @media (min-width: 768px) {
    .handoff-lane { padding-left: 24px; padding-right: 24px; }
  }
</style>
