<script>
  import { COLUMNS, PRIORITIES } from "../lib/constants.js";
  import { fmtDateTime, noteAuthorLabel, projectLabel, statusLabel } from "../lib/format.js";
  import {
    agentStatusText,
    isWorking as checkWorking,
    waitStatusClass,
  } from "../lib/agent.js";
  import { boardUI, isStarting, latestRun, pendingAskFor } from "../lib/board-ui.svelte.js";
  import { detailUI, runUI } from "../lib/detail-ui.svelte.js";
  import { renderMd } from "../md.js";
  import Icon from "./Icon.svelte";
  import RunControls from "./RunControls.svelte";
  import QuestionPrompt from "./QuestionPrompt.svelte";

  let {
    projectPaths = [],
    onClose,
    onStartEdit,
    onCancelEdit,
    onSaveEdit,
    onEditChange,
    onMove,
    onClearHandoff,
    onRun,
    onCancelRun,
    onToggleAgentMenu,
    onSelectAgent,
    onSavePathAndRun,
    onCancelPath,
    onPathInput,
    onSubmitAnswer,
    onAnswerDraft,
    onAddNote,
    onNoteBody,
  } = $props();

  const detail = $derived(detailUI.detail);
  const editing = $derived(detailUI.editing);
  const edit = $derived(detailUI.edit);
  const noteBody = $derived(detailUI.noteBody);
  const answerDraft = $derived(detailUI.answerDraft);
  const pathInput = $derived(runUI.pathInput);
  const animNoteIds = $derived(detailUI.animNoteIds);
  const runFiles = $derived(detailUI.runFiles);
  const needPath = $derived(runUI.needPath);
  const runError = $derived(runUI.runError);
  const run = $derived(detail ? latestRun(detail.id) : null);
  const starting = $derived(detail ? isStarting(detail.id) : false);
  const pendingAsk = $derived(detail ? pendingAskFor(detail.id) : null);
  const working = $derived(checkWorking({ isStarting: starting, run }));
  const statusClass = $derived(waitStatusClass({ isStarting: starting, run, hasPendingAsk: !!pendingAsk }));
  const statusText = $derived(agentStatusText({
    isStarting: starting,
    run,
    hasPendingAsk: !!pendingAsk,
    selectedAgent: boardUI.selectedAgent,
  }));
  const showStatus = $derived(working || pendingAsk || (run && ["exited", "failed", "killed"].includes(run.status)));

  function patchEdit(patch) {
    onEditChange({ ...edit, ...patch });
  }
</script>

{#if detail}
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
          <input
            value={edit.title}
            oninput={(e) => patchEdit({ title: e.currentTarget.value })}
            onkeydown={(e) => { if (e.key === "Escape") onCancelEdit(); }}
          />
        </label>
        <label class="field">
          <span>Project</span>
          <select
            value={edit.project}
            onchange={(e) => patchEdit({ project: e.currentTarget.value })}
          >
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
          <textarea
            rows="5"
            value={edit.description}
            oninput={(e) => patchEdit({ description: e.currentTarget.value })}
          ></textarea>
        </label>
        <div class="field">
          <span>Priority</span>
          <div class="pri-seg">
            {#each PRIORITIES as p}
              <button class:sel={edit.priority === p.key} onclick={() => patchEdit({ priority: p.key })}>{p.label}</button>
            {/each}
          </div>
        </div>
        <label class="field">
          <span>Due date</span>
          <input
            value={edit.due_date}
            placeholder="YYYY-MM-DD"
            oninput={(e) => patchEdit({ due_date: e.currentTarget.value })}
          />
        </label>
        <label class="field">
          <span>Tags (comma-separated)</span>
          <input
            value={edit.tags}
            placeholder="bug, frontend"
            oninput={(e) => patchEdit({ tags: e.currentTarget.value })}
          />
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
              selectedAgent={boardUI.selectedAgent}
              agentMenuOpen={boardUI.openAgentMenu === "detail"}
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
            <input
              value={pathInput}
              placeholder="~/sites/…"
              oninput={(e) => onPathInput(e.currentTarget.value)}
              onkeydown={(e) => { if (e.key === "Enter") onSavePathAndRun(); }}
            />
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
            <QuestionPrompt
              question={pendingAsk}
              {answerDraft}
              onAnswerDraft={onAnswerDraft}
              onSubmit={onSubmitAnswer}
            />
          {/if}
          <div class="d-note-add">
            <input
              value={noteBody}
              placeholder="Add to thread (markdown ok)…"
              oninput={(e) => onNoteBody(e.currentTarget.value)}
              onkeydown={(e) => { if (e.key === "Enter") onAddNote(); }}
            />
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
{/if}

<style>
  .detail {
    position: fixed; z-index: 50; right: 0; top: var(--topbar-h); bottom: 0; width: min(440px, 100vw);
    background: var(--surface); border-left: 1px solid var(--border);
    display: flex; flex-direction: column; animation: slide .2s ease;
  }
  .detail:global(.is-working) { border-left-color: color-mix(in srgb, var(--prog) 55%, var(--border)); }
  .d-id {
    font-variant-numeric: tabular-nums; font-size: 12px; font-weight: 700;
    color: var(--muted); letter-spacing: .02em;
  }
  .d-working {
    display: inline-flex; align-items: center; gap: 6px;
    margin-left: 4px; padding: 2px 8px; border-radius: 999px;
    font-size: 11px; font-weight: 700; letter-spacing: .02em; text-transform: uppercase;
  }
  .d-working:global(.s-starting), .d-working:global(.s-running) {
    color: var(--prog); background: color-mix(in srgb, var(--prog) 16%, transparent);
  }
  .d-working:global(.s-waiting-you) {
    color: var(--amber); background: color-mix(in srgb, var(--amber) 16%, transparent);
  }
  .d-working:global(.s-waiting-ci) {
    color: var(--prog); background: color-mix(in srgb, var(--prog) 12%, transparent);
  }
  .d-working:global(.s-exited) {
    color: var(--done); background: color-mix(in srgb, var(--done) 15%, transparent);
  }
  .d-working:global(.s-failed), .d-working:global(.s-killed) {
    color: var(--danger); background: color-mix(in srgb, var(--danger) 14%, transparent);
  }
  .agent-pulse {
    width: 8px; height: 8px; border-radius: 50%; background: var(--prog);
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--prog) 60%, transparent);
    animation: pulse 1.4s ease-out infinite;
  }
  .detail-body { flex: 1; overflow-y: auto; padding: 16px 18px calc(24px + env(safe-area-inset-bottom)); }
  .detail-body textarea {
    width: 100%; padding: 10px 12px; border-radius: 10px; border: 1px solid var(--border);
    background: var(--surface-2); color: var(--text); font-family: inherit; font-size: 16px; line-height: 1.5; resize: vertical;
  }
  .detail-body textarea:focus { outline: none; border-color: var(--accent); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 25%, transparent); }
  .d-title { margin: 0 0 10px; font-size: 18px; font-weight: 700; line-height: 1.3; letter-spacing: -.01em; word-break: break-word; }
  .d-chips { display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 10px; }
  .d-status { font-size: 11px; font-weight: 700; padding: 3px 9px; border-radius: 999px; text-transform: uppercase; letter-spacing: .03em; }
  .d-status:global(.s-todo) { color: var(--todo); background: color-mix(in srgb, var(--todo) 18%, transparent); }
  .d-status:global(.s-in_progress) { color: var(--prog); background: color-mix(in srgb, var(--prog) 15%, transparent); }
  .d-status:global(.s-done) { color: var(--done); background: color-mix(in srgb, var(--done) 15%, transparent); }
  .d-facts {
    display: flex; flex-wrap: wrap; gap: 6px 14px; margin-bottom: 12px;
    font-size: 12px; color: var(--text); border-top: 1px solid var(--border); padding-top: 10px;
  }
  .d-facts em { font-style: normal; font-weight: 600; color: var(--muted); margin-right: 4px; }
  .d-handoff { display: inline-flex; align-items: center; gap: 2px; }
  .d-desc { font-size: 14px; line-height: 1.55; color: var(--text); word-break: break-word; margin: 0 0 8px; }
  .d-desc:global(.is-muted) { color: var(--muted); font-style: italic; white-space: pre-wrap; }
  .d-meta { display: flex; flex-direction: column; gap: 8px; margin: 18px 0 0; padding: 14px; background: var(--surface-2); border: 1px solid var(--border); border-radius: 12px; }
  .d-meta > div { display: flex; gap: 10px; font-size: 13px; }
  .d-meta dt { flex: 0 0 74px; color: var(--muted); font-weight: 600; margin: 0; }
  .d-meta dd { margin: 0; color: var(--text); word-break: break-word; }
  .d-move { display: flex; flex-wrap: wrap; gap: 8px; margin: 0 0 16px; }
  .d-section { margin-bottom: 18px; }
  .d-section h4 { margin: 0 0 10px; font-size: 13px; font-weight: 700; text-transform: uppercase; letter-spacing: .03em; color: var(--muted); }
  .d-note { padding: 10px 12px; border-radius: 10px; background: var(--surface-2); border: 1px solid var(--border); margin-bottom: 8px; }
  .d-note:global(.is-new) {
    border-color: color-mix(in srgb, var(--prog) 45%, var(--border));
    background: color-mix(in srgb, var(--prog) 10%, var(--surface-2));
    animation: note-in .38s ease;
  }
  .note-meta { display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 6px; }
  .note-author {
    font-size: 10px; font-weight: 800; text-transform: uppercase; letter-spacing: .04em;
    padding: 2px 7px; border-radius: 999px;
  }
  .note-author:global(.a-human) { color: var(--accent); background: color-mix(in srgb, var(--accent) 14%, transparent); }
  .note-author:global(.a-agent) { color: var(--prog); background: color-mix(in srgb, var(--prog) 14%, transparent); }
  .note-author:global(.a-unknown), .note-author:global(.a-system) { color: var(--muted); background: var(--surface-3); }
  .d-note :global(.md) { font-size: 13px; line-height: 1.5; color: var(--text); word-break: break-word; }
  .d-note-add { display: flex; gap: 8px; margin: 0 0 12px; }
  .d-note-add input {
    flex: 1; min-width: 0; height: 44px; padding: 0 12px; border-radius: 10px; border: 1px solid var(--border);
    background: var(--surface-2); color: var(--text); font-family: inherit; font-size: 16px;
  }
  .d-note-add input:focus { outline: none; border-color: var(--accent); box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 25%, transparent); }
  .path-prompt {
    margin-top: 12px; padding: 12px; border-radius: 10px; border: 1px solid var(--border);
    background: var(--surface-2); display: flex; flex-direction: column; gap: 8px;
  }
  .path-prompt input { padding: 10px 12px; border-radius: 8px; border: 1px solid var(--border); background: var(--bg); color: var(--text); }
  .run-err { color: var(--danger); font-size: 13px; margin: 8px 0 0; }
  .files-list {
    list-style: none; margin: 0; padding: 0;
    display: flex; flex-direction: column; gap: 4px;
  }
  .files-list code { font-size: 12px; color: var(--muted); word-break: break-all; }
</style>
