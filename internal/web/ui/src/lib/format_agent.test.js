import { describe, expect, it } from "vitest";
import { fmtAgo, noteAuthorLabel, projectLabel, statusLabel } from "./format.js";
import {
  agentStatusText,
  isWorking,
  runStatusLabel,
  waitStatusClass,
  waitStatusLabel,
} from "./agent.js";

describe("format", () => {
  it("projectLabel falls back to global", () => {
    expect(projectLabel(null)).toBe("global");
    expect(projectLabel("")).toBe("global");
    expect(projectLabel("board")).toBe("board");
  });

  it("statusLabel maps known columns", () => {
    expect(statusLabel("todo")).toBe("To-Do");
    expect(statusLabel("in_progress")).toBe("In Progress");
    expect(statusLabel("weird")).toBe("weird");
  });

  it("noteAuthorLabel maps authors", () => {
    expect(noteAuthorLabel("human")).toBe("You");
    expect(noteAuthorLabel("agent")).toBe("Agent");
    expect(noteAuthorLabel("x")).toBe("Note");
  });

  it("fmtAgo returns compact relative times", () => {
    const now = Date.parse("2026-08-02T12:00:00Z");
    expect(fmtAgo("2026-08-02T11:59:30Z", now)).toBe("now");
    expect(fmtAgo("2026-08-02T11:50:00Z", now)).toBe("10m");
    expect(fmtAgo("2026-08-02T09:00:00Z", now)).toBe("3h");
    expect(fmtAgo("2026-07-30T12:00:00Z", now)).toBe("3d");
  });
});

describe("agent status", () => {
  it("runStatusLabel covers lifecycle", () => {
    expect(runStatusLabel({ isStarting: true, run: null })).toBe("Starting…");
    expect(runStatusLabel({ isStarting: false, run: { status: "running" } })).toBe("Working…");
    expect(runStatusLabel({ isStarting: false, run: { status: "exited" } })).toBe("Done");
    expect(runStatusLabel({ isStarting: false, run: { status: "failed" } })).toBe("Failed");
    expect(runStatusLabel({ isStarting: false, run: { status: "killed" } })).toBe("Cancelled");
  });

  it("waitStatusLabel prioritizes ask and CI wait", () => {
    expect(waitStatusLabel({
      isStarting: false,
      run: { status: "running" },
      hasPendingAsk: true,
    })).toBe("Waiting on you");
    expect(waitStatusLabel({
      isStarting: false,
      run: { status: "running", wait: "ci" },
      hasPendingAsk: false,
    })).toBe("Waiting on CI");
  });

  it("waitStatusClass and isWorking", () => {
    expect(waitStatusClass({
      isStarting: false,
      run: { status: "running", wait: "ci" },
      hasPendingAsk: false,
    })).toBe("waiting-ci");
    expect(isWorking({ isStarting: true, run: null })).toBe(true);
    expect(isWorking({ isStarting: false, run: { status: "running" } })).toBe(true);
    expect(isWorking({ isStarting: false, run: { status: "exited" } })).toBe(false);
  });

  it("agentStatusText prefixes agent name", () => {
    expect(agentStatusText({
      isStarting: false,
      run: { status: "running", agent: "claude" },
      hasPendingAsk: false,
      selectedAgent: "cursor",
    })).toBe("Claude · Working…");
  });
});
