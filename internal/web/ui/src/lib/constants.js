export const COLUMNS = [
  { key: "todo", label: "To-Do", dot: "var(--todo)" },
  { key: "in_progress", label: "In Progress", dot: "var(--prog)" },
  { key: "done", label: "Done", dot: "var(--done)" },
];

export const PRIORITIES = [
  { key: "", label: "None" },
  { key: "low", label: "Low" },
  { key: "medium", label: "Med" },
  { key: "high", label: "High" },
];

export const AGENTS = [
  { key: "cursor", label: "Cursor" },
  { key: "claude", label: "Claude" },
  { key: "codex", label: "Codex" },
];

export const AGENT_LABEL = Object.fromEntries(AGENTS.map((a) => [a.key, a.label]));

export const eventKindLabel = {
  created: "created", moved: "moved", note: "note", handoff: "handoff",
  archived: "archived", unarchived: "restored", updated: "updated", deleted: "deleted",
  tool: "tool", session: "session", run: "run", run_done: "run done",
  run_progress: "progress", run_wait: "wait", run_file: "file",
  question: "question", answered: "answered",
};
