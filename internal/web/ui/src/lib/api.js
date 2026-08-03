/** Attach CSRF header when the server injects `window.__BOARD_CSRF__`. */
export function apiHeaders(extra = {}) {
  const t = typeof window !== "undefined" ? window.__BOARD_CSRF__ : undefined;
  return t ? { ...extra, "X-CSRF-Token": t } : { ...extra };
}
