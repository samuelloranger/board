<script>
  import { renderMd } from "../md.js";

  let {
    question,
    answerDraft = "",
    onAnswerDraft,
    onSubmit,
  } = $props();
</script>

<div class="ask-card" role="form" aria-label="Agent question">
  <div class="ask-head">
    <span class="ask-label">Agent asks</span>
  </div>
  <div class="md ask-q">{@html renderMd(question.question)}</div>
  <textarea
    rows="3"
    value={answerDraft}
    placeholder="Your answer…"
    oninput={(e) => onAnswerDraft(e.currentTarget.value)}
    onkeydown={(e) => {
      if (e.key === "Enter" && (e.metaKey || e.ctrlKey) && answerDraft.trim()) onSubmit(question.id);
    }}
  ></textarea>
  <div class="ask-actions">
    <button class="btn-primary sm" disabled={!answerDraft.trim()} onclick={() => onSubmit(question.id)}>Submit answer</button>
  </div>
</div>

<style>
  .ask-card {
    margin-bottom: 12px; padding: 12px; border-radius: 12px;
    border: 1px solid color-mix(in srgb, var(--amber) 45%, var(--border));
    background: color-mix(in srgb, var(--amber) 10%, var(--surface-2));
    display: flex; flex-direction: column; gap: 10px;
  }
  .ask-head { display: flex; align-items: center; gap: 8px; }
  .ask-label {
    font-size: 11px; font-weight: 800; text-transform: uppercase; letter-spacing: .04em; color: var(--amber);
  }
  .ask-q { margin: 0; font-size: 14px; line-height: 1.45; color: var(--text); }
  .ask-card textarea {
    width: 100%; padding: 10px 12px; border-radius: 8px; border: 1px solid var(--border);
    background: var(--surface); color: var(--text); resize: vertical; font-family: inherit; font-size: 16px;
  }
  .ask-card textarea:focus {
    outline: none; border-color: var(--amber);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--amber) 25%, transparent);
  }
  .ask-actions { display: flex; justify-content: flex-end; }
</style>
