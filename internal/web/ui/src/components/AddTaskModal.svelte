<script>
  import { PRIORITIES } from "../lib/constants.js";
  import Icon from "./Icon.svelte";

  let {
    title = $bindable(""),
    description = $bindable(""),
    priority = $bindable(""),
    project = $bindable(""),
    projectPaths = [],
    onClose,
    onCreate,
  } = $props();
</script>

<div
  class="scrim"
  role="button"
  tabindex="-1"
  aria-label="Dismiss"
  onclick={onClose}
  onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onClose(); } }}
></div>
<div class="modal" role="dialog" aria-modal="true" aria-label="New task">
  <div class="modal-head">
    <h3>New task</h3>
    <button class="icon-btn sm" aria-label="Close" onclick={onClose}><Icon name="close" /></button>
  </div>
  <label class="field">
    <span>Title</span>
    <!-- svelte-ignore a11y_autofocus -->
    <input
      autofocus
      bind:value={title}
      placeholder="What needs doing?"
      onkeydown={(e) => { if (e.key === "Enter") onCreate(); if (e.key === "Escape") onClose(); }}
    />
  </label>
  <label class="field">
    <span>Project</span>
    <select bind:value={project}>
      <option value="">global</option>
      {#each projectPaths.filter((p) => p.project !== "_") as pp (pp.project)}
        <option value={pp.project}>{pp.project}</option>
      {/each}
    </select>
    {#if projectPaths.length === 0}
      <span class="field-hint">Add a project path (folder icon) to assign tasks to a project.</span>
    {/if}
  </label>
  <label class="field">
    <span>Description (markdown)</span>
    <textarea
      rows="4"
      bind:value={description}
      placeholder="Optional context…"
      onkeydown={(e) => { if (e.key === "Escape") onClose(); }}
    ></textarea>
  </label>
  <div class="field">
    <span>Priority</span>
    <div class="pri-seg">
      {#each PRIORITIES as p}
        <button class:sel={priority === p.key} onclick={() => (priority = p.key)}>{p.label}</button>
      {/each}
    </div>
  </div>
  <div class="modal-foot">
    <button class="btn-ghost" onclick={onClose}>Cancel</button>
    <button class="btn-primary" disabled={!title.trim()} onclick={onCreate}><Icon name="check" /><span>Create</span></button>
  </div>
</div>
