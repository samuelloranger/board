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
