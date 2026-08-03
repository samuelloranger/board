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
