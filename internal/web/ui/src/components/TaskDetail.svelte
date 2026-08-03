<script>
  import { COLUMNS, PRIORITIES } from "../lib/constants.js";
  import { fmtDateTime, noteAuthorLabel, projectLabel, statusLabel } from "../lib/format.js";
  import {
    agentStatusText,
    isWorking as checkWorking,
    waitStatusClass,
  } from "../lib/agent.js";
  import { renderMd } from "../md.js";
  import Icon from "./Icon.svelte";
  import RunControls from "./RunControls.svelte";
  import QuestionPrompt from "./QuestionPrompt.svelte";

  let {
    detail,
    editing = false,
    edit = $bindable({ title: "", description: "", priority: "", due_date: "", tags: "", project: "" }),
    noteBody = $bindable(""),
    answerDraft = $bindable(""),
    pathInput = $bindable(""),
    animNoteIds = {},
    projectPaths = [],
    runFiles = [],
    needPath = null,
    runError = "",
    openAgentMenu = null,
    selectedAgent = "cursor",
    run = null,
    starting = false,
    pendingAsk = null,
    onClose,
    onStartEdit,
    onCancelEdit,
    onSaveEdit,
    onMove,
    onClearHandoff,
    onRun,
    onCancelRun,
    onToggleAgentMenu,
    onSelectAgent,
    onSavePathAndRun,
    onCancelPath,
    onSubmitAnswer,
    onAddNote,
  } = $props();

  const working = $derived(checkWorking({ isStarting: starting, run }));
  const statusClass = $derived(waitStatusClass({ isStarting: starting, run, hasPendingAsk: !!pendingAsk }));
  const statusText = $derived(agentStatusText({ isStarting: starting, run, hasPendingAsk: !!pendingAsk, selectedAgent }));
  const showStatus = $derived(working || pendingAsk || (run && ["exited", "failed", "killed"].includes(run.status)));
</script>

<div
  class="scrim"
  role="button"
  tabindex="-1"
  aria-label="Dismiss task detail"
  onclick={onClose}
  onkeydown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onClose(); } }}
></div>
<div class="detail" class:is-working={working} role="dialog" aria-modal="true" aria-label="Task detail">
  <div class="activity-head">
    <div class="live">
      <span class="d-id">#{detail.id}</span>
      <span>{editing ? "Edit" : "Task"}</span>
      {#if showStatus}
        <span class="d-working s-{statusClass}">
          {#if working || pendingAsk}<span class="agent-pulse" aria-hidden="true"></span>{/if}
          {statusText}
        </span>
      {/if}
    </div>
    <div class="head-actions">
      {#if !editing && working}
        <button class="btn-run cancel sm" onclick={() => onCancelRun(detail.id)}>Cancel</button>
      {/if}
      {#if !editing}
        <button class="icon-btn sm" aria-label="Edit task" onclick={onStartEdit}><Icon name="edit" /></button>
      {/if}
      <button class="icon-btn sm" aria-label="Close" onclick={onClose}><Icon name="close" /></button>
    </div>
  </div>
  <div class="detail-body">
    {#if editing}
      <label class="field">
        <span>Title</span>
        <input bind:value={edit.title} onkeydown={(e) => { if (e.key === "Escape") onCancelEdit(); }} />
      </label>
      <label class="field">
        <span>Project</span>
        <select bind:value={edit.project}>
          <option value="">global</option>
          {#each projectPaths.filter((p) => p.project !== "_") as pp (pp.project)}
            <option value={pp.project}>{pp.project}</option>
          {/each}
          {#if edit.project && !projectPaths.some((p) => p.project === edit.project)}
            <option value={edit.project}>{edit.project} (unmapped)</option>
          {/if}
        </select>
        {#if projectPaths.length === 0}
          <span class="field-hint">Add a project path (folder icon) to assign tasks to a project.</span>
        {/if}
      </label>
      <label class="field">
        <span>Description (markdown)</span>
        <textarea rows="5" bind:value={edit.description}></textarea>
      </label>
      <div class="field">
        <span>Priority</span>
        <div class="pri-seg">
          {#each PRIORITIES as p}
            <button class:sel={edit.priority === p.key} onclick={() => (edit.priority = p.key)}>{p.label}</button>
          {/each}
        </div>
      </div>
      <label class="field">
        <span>Due date</span>
        <input bind:value={edit.due_date} placeholder="YYYY-MM-DD" />
      </label>
      <label class="field">
        <span>Tags (comma-separated)</span>
        <input bind:value={edit.tags} placeholder="bug, frontend" />
      </label>
      <div class="modal-foot">
        <button class="btn-ghost" onclick={onCancelEdit}>Cancel</button>
        <button class="btn-primary" disabled={!edit.title.trim()} onclick={onSaveEdit}><Icon name="check" /><span>Save</span></button>
      </div>
    {:else}
      <h3 class="d-title">{detail.title}</h3>
      <div class="d-chips">
        <span class="d-status s-{detail.status}">{statusLabel(detail.status)}</span>
        <span class="proj" class:is-global={!detail.project}>{projectLabel(detail.project)}</span>
        {#if detail.priority}<span class="pri pri-{detail.priority}">{detail.priority}</span>{/if}
        {#each detail.tags ?? [] as tag}<span class="tag">{tag}</span>{/each}
      </div>
      <div class="d-facts">
        <span><em>Project</em> {projectLabel(detail.project)}</span>
        {#if detail.due_date}<span><em>Due</em> {detail.due_date}</span>{/if}
        {#if detail.handoff_to}
          <span class="d-handoff">
            <em>Handoff</em> → {detail.handoff_to}
            <button class="chip-x inline" onclick={(e) => onClearHandoff(detail.id, e)} aria-label="Clear handoff" title="Clear handoff"><Icon name="close" /></button>
          </span>
        {/if}
        <span><em>Updated</em> {fmtDateTime(detail.updated_at)}</span>
      </div>
      <div class="d-move">
        {#each COLUMNS.filter((x) => x.key !== detail.status) as m}
          <button class="btn-ghost sm" onclick={() => onMove(m.key)}>Move to {m.label}</button>
        {/each}
        {#if !working}
          <RunControls
            sm
            {run}
            {starting}
            {selectedAgent}
            agentMenuOpen={openAgentMenu === "detail"}
            onRun={() => onRun(detail.id)}
            onCancel={() => onCancelRun(detail.id)}
            onToggleMenu={onToggleAgentMenu}
            onSelectAgent={(key) => onSelectAgent(key, detail.id)}
          />
        {/if}
      </div>
      {#if needPath && needPath.taskId === detail.id}
        <div class="path-prompt">
          <p>Where is project <strong>{needPath.project === "_" ? "(global)" : needPath.project}</strong> on disk?</p>
          <input bind:value={pathInput} placeholder="~/sites/…" onkeydown={(e) => { if (e.key === "Enter") onSavePathAndRun(); }} />
          <div class="modal-foot">
            <button class="btn-ghost" onclick={onCancelPath}>Cancel</button>
            <button class="btn-primary" disabled={!pathInput.trim()} onclick={onSavePathAndRun}>Save &amp; Run</button>
          </div>
        </div>
      {/if}
      {#if runError}<p class="run-err">{runError}</p>{/if}
      <div class="d-section">
        <h4>Description</h4>
        {#if detail.description}
          <div class="md d-desc">{@html renderMd(detail.description)}</div>
        {:else}
          <p class="d-desc is-muted">No description</p>
        {/if}
      </div>
      {#if run}
        <div class="d-section files-touched">
          <h4>Files touched</h4>
          {#if runFiles.length === 0}
            <p class="d-desc is-muted">No files recorded yet</p>
          {:else}
            <ul class="files-list">
              {#each runFiles.slice(0, 40) as f (f.path)}
                <li><code>{f.path}</code></li>
              {/each}
            </ul>
          {/if}
        </div>
      {/if}
      <div class="d-section">
        <h4>Thread {(detail.notes ?? []).length ? `(${detail.notes.length})` : ""}</h4>
        {#if pendingAsk}
          <QuestionPrompt question={pendingAsk} bind:answerDraft onSubmit={onSubmitAnswer} />
        {/if}
        <div class="d-note-add">
          <input bind:value={noteBody} placeholder="Add to thread (markdown ok)…" onkeydown={(e) => { if (e.key === "Enter") onAddNote(); }} />
          <button class="btn-primary sm" disabled={!noteBody.trim()} onclick={onAddNote}>Add</button>
        </div>
        {#if (detail.notes ?? []).length === 0 && !pendingAsk}
          <div class="empty sm">Nothing in the thread yet</div>
        {/if}
        {#each [...(detail.notes ?? [])].reverse() as n (n.id)}
          <div class="d-note" class:is-new={!!animNoteIds[n.id]}>
            <div class="note-meta">
              <span class="note-author a-{n.author || 'unknown'}">{noteAuthorLabel(n.author)}</span>
              <span class="ev-time">{fmtDateTime(n.created_at, { seconds: true })}</span>
            </div>
            <div class="md">{@html renderMd(n.body)}</div>
          </div>
        {/each}
      </div>
      <dl class="d-meta">
        <div><dt>Created</dt><dd>{fmtDateTime(detail.created_at)}</dd></div>
        <div><dt>Updated</dt><dd>{fmtDateTime(detail.updated_at)}</dd></div>
        {#if detail.handoff_reason}<div><dt>Handoff</dt><dd>{detail.handoff_reason}</dd></div>{/if}
      </dl>
    {/if}
  </div>
</div>
