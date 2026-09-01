import { AGENT_LABEL } from "./constants.js";

function loadSelectedAgent() {
  try {
    const v = localStorage.getItem("board-default-agent");
    if (v && AGENT_LABEL[v]) return v;
  } catch {}
  return "cursor";
}

/** Shared board/card UI state — replaces App → BoardColumn → TaskCard prop drilling. */
export const boardUI = $state({
  openMenu: null,
  openAgentMenu: null,
  selectedAgent: typeof localStorage !== "undefined" ? loadSelectedAgent() : "cursor",
  latestByTask: {},
  startingIds: {},
  pendingQuestions: [],
});

/** Action hooks App wires once; components call through these. */
export const boardActions = {
  onOpen: () => {},
  onToggleMenu: () => {},
  onMove: () => {},
  onArchive: () => {},
  onRun: () => {},
  onCancel: () => {},
  onToggleAgentMenu: () => {},
  onSelectAgent: () => {},
  onDragStart: () => {},
};

export function wireBoardActions(next) {
  Object.assign(boardActions, next);
}

export function setSelectedAgent(key) {
  if (!AGENT_LABEL[key]) return;
  boardUI.selectedAgent = key;
  boardUI.openAgentMenu = null;
  try {
    localStorage.setItem("board-default-agent", key);
  } catch {}
}

export function latestRun(id) {
  return boardUI.latestByTask[id] ?? null;
}

export function isStarting(id) {
  return !!boardUI.startingIds[id];
}

export function pendingAskFor(id) {
  if (id == null) return null;
  return boardUI.pendingQuestions.find((q) => q.task_id === id) ?? null;
}

export function hasPendingAsk(id) {
  return !!pendingAskFor(id);
}

/** Reset mutable fields — used by tests between cases. */
export function resetBoardUI() {
  boardUI.openMenu = null;
  boardUI.openAgentMenu = null;
  boardUI.selectedAgent = "cursor";
  boardUI.latestByTask = {};
  boardUI.startingIds = {};
  boardUI.pendingQuestions = [];
  wireBoardActions({
    onOpen: () => {},
    onToggleMenu: () => {},
    onMove: () => {},
    onArchive: () => {},
    onRun: () => {},
    onCancel: () => {},
    onToggleAgentMenu: () => {},
    onSelectAgent: () => {},
    onDragStart: () => {},
  });
}
