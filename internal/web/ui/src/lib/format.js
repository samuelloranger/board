import { COLUMNS } from "./constants.js";

export function projectLabel(p) {
  return p && String(p).trim() ? p : "global";
}

export function statusLabel(s) {
  return (COLUMNS.find((c) => c.key === s) || {}).label ?? s;
}

export function noteAuthorLabel(author) {
  if (author === "human") return "You";
  if (author === "agent") return "Agent";
  return "Note";
}

export function fmtTime(iso) {
  try {
    return new Date(iso).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  } catch {
    return "";
  }
}

export function fmtDateTime(iso, { seconds = false } = {}) {
  try {
    return new Date(iso).toLocaleString([], {
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
      ...(seconds ? { second: "2-digit" } : {}),
    });
  } catch {
    return "";
  }
}

/** Relative "3m", "2h", "4d" for compact activity timestamps. */
export function fmtAgo(iso, now = Date.now()) {
  try {
    const s = Math.max(0, (now - new Date(iso).getTime()) / 1000);
    if (s < 60) return "now";
    if (s < 3600) return Math.floor(s / 60) + "m";
    if (s < 86400) return Math.floor(s / 3600) + "h";
    return Math.floor(s / 86400) + "d";
  } catch {
    return "";
  }
}
