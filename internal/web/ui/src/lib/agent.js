import { AGENT_LABEL } from "./constants.js";

export function agentLabel(key) {
  return AGENT_LABEL[key] || key || "Agent";
}

export function runStatusLabel({ isStarting, run }) {
  if (isStarting) return "Starting…";
  if (!run) return "";
  switch (run.status) {
    case "running":
      return "Working…";
    case "exited":
      return "Done";
    case "failed":
      return "Failed";
    case "killed":
      return "Cancelled";
    default:
      return run.status;
  }
}

export function waitStatusLabel({ isStarting, run, hasPendingAsk }) {
  if (hasPendingAsk) return "Waiting on you";
  if (run?.status === "running" && run.wait === "ci") return "Waiting on CI";
  return runStatusLabel({ isStarting, run });
}

export function agentStatusText({ isStarting, run, hasPendingAsk, selectedAgent }) {
  const label = waitStatusLabel({ isStarting, run, hasPendingAsk });
  if (!label) return "";
  const whoKey = run?.agent || (isStarting ? selectedAgent : "");
  if (!whoKey && !hasPendingAsk) return label;
  const who = agentLabel(whoKey || selectedAgent);
  return `${who} · ${label}`;
}

export function waitStatusClass({ isStarting, run, hasPendingAsk }) {
  if (hasPendingAsk) return "waiting-you";
  if (run?.status === "running" && run.wait === "ci") return "waiting-ci";
  if (isStarting) return "starting";
  return run?.status || "";
}

export function isWorking({ isStarting, run }) {
  return !!isStarting || run?.status === "running";
}
