<script>
  import { renderMd } from "../md.js";

  let {
    question,
    answerDraft = $bindable(""),
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
    bind:value={answerDraft}
    placeholder="Your answer…"
    onkeydown={(e) => {
      if (e.key === "Enter" && (e.metaKey || e.ctrlKey) && answerDraft.trim()) onSubmit(question.id);
    }}
  ></textarea>
  <div class="ask-actions">
    <button class="btn-primary sm" disabled={!answerDraft.trim()} onclick={() => onSubmit(question.id)}>Submit answer</button>
  </div>
</div>
