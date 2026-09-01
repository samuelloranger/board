<script>
  import { eventKindLabel } from "../lib/constants.js";
  import { fmtAgo, fmtDateTime } from "../lib/format.js";
  import Icon from "./Icon.svelte";

  let {
    events = [],
    unseen = 0,
    eventVisible,
    eventTitle,
    onClose,
    onMarkSeen,
    onOpenTask,
  } = $props();
</script>

<div
  class="scrim"
  role="button"
  tabindex="-1"
  aria-label="Dismiss activity"
  onclick={onClose}
  onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onClose(); } }}
></div>
<div class="activity" role="dialog" aria-label="Activity feed">
  <div class="activity-head">
    <div class="live"><Icon name="activity" /><span>Activity</span></div>
    <div class="head-actions">
      {#if unseen > 0}
        <button class="btn-ghost sm" onclick={onMarkSeen}>Seen all</button>
      {/if}
      <button class="icon-btn sm" aria-label="Close activity" onclick={onClose}><Icon name="close" /></button>
    </div>
  </div>
  <div class="feed">
    {#if events.length === 0}<div class="empty">No activity yet</div>{/if}
    {#each events.filter(eventVisible) as e (e.id)}
      {@const et = eventTitle(e)}
      <div class="ev">
        <span class="ev-dot k-{e.kind}" aria-hidden="true"></span>
        <div class="ev-body">
          <div class="ev-line">
            <span class="ev-kind k-{e.kind}">{eventKindLabel[e.kind] ?? e.kind}</span>
            {#if et}
              <button class="ev-task" onclick={() => onOpenTask(e.task_id)}>{et}</button>
            {/if}
            <span class="ev-time" title={fmtDateTime(e.created_at)}>{fmtAgo(e.created_at)}</span>
          </div>
          {#if e.detail}<div class="ev-detail">{e.detail}</div>{/if}
        </div>
      </div>
    {/each}
  </div>
</div>

<style>
  .activity {
    position: fixed; z-index: 50; right: 0; top: var(--topbar-h); bottom: 0; width: min(360px, 100vw);
    background: var(--surface); border-left: 1px solid var(--border);
    display: flex; flex-direction: column; animation: slide .2s ease;
  }
  .feed { flex: 1; overflow-y: auto; padding: 10px 12px; display: flex; flex-direction: column; gap: 1px; }
  .ev { display: grid; grid-template-columns: auto 1fr; gap: 10px; padding: 9px 8px; border-radius: 8px; }
  .ev:hover { background: var(--surface-2); }
  .ev-dot { width: 8px; height: 8px; margin-top: 6px; border-radius: 50%; background: var(--muted); }
  .ev-body { min-width: 0; display: flex; flex-direction: column; gap: 3px; }
  .ev-line { display: flex; align-items: center; gap: 7px; }
  .ev-kind {
    font-size: 10px; font-weight: 700; text-transform: uppercase; letter-spacing: .03em;
    padding: 2px 7px; border-radius: 6px; background: var(--surface-3); color: var(--muted); flex: 0 0 auto;
  }
  .ev-task {
    flex: 1; min-width: 0; text-align: left; border: none; background: transparent; padding: 0;
    font-family: inherit; font-size: 13px; font-weight: 600; color: var(--text); cursor: pointer;
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .ev-task:hover { color: var(--accent); }
  .ev-kind:global(.k-created) { color: var(--done); background: color-mix(in srgb, var(--done) 15%, transparent); }
  .ev-kind:global(.k-moved) { color: var(--prog); background: color-mix(in srgb, var(--prog) 15%, transparent); }
  .ev-kind:global(.k-handoff) { color: var(--amber); background: color-mix(in srgb, var(--amber) 15%, transparent); }
  .ev-kind:global(.k-note), .ev-kind:global(.k-updated) { color: var(--prog); background: color-mix(in srgb, var(--prog) 12%, transparent); }
  .ev-kind:global(.k-deleted), .ev-kind:global(.k-archived) { color: var(--danger); background: color-mix(in srgb, var(--danger) 13%, transparent); }
  .ev-dot:global(.k-created) { background: var(--done); }
  .ev-dot:global(.k-moved), .ev-dot:global(.k-note), .ev-dot:global(.k-updated) { background: var(--prog); }
  .ev-dot:global(.k-handoff) { background: var(--amber); }
  .ev-dot:global(.k-deleted), .ev-dot:global(.k-archived) { background: var(--danger); }
  .ev-detail { font-size: 12.5px; color: var(--muted); line-height: 1.4; word-break: break-word; }
</style>
