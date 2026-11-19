/** Attach CSRF header when the server injects `window.__BOARD_CSRF__`. */
export function apiHeaders(extra = {}) {
  const t = typeof window !== "undefined" ? window.__BOARD_CSRF__ : undefined;
  return t ? { ...extra, "X-CSRF-Token": t } : { ...extra };
}

/** The run token is never served by the board — the human enters it once. */
export function runTokenHeaders(extra = {}) {
  try {
    const t = localStorage.getItem("board-run-token");
    return t ? { ...extra, "X-Run-Token": t } : { ...extra };
  } catch {
    return { ...extra };
  }
}

export function setStoredRunToken(token) {
  try { localStorage.setItem("board-run-token", token); } catch {}
}
