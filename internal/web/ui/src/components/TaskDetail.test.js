import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { boardUI, resetBoardUI } from "../lib/board-ui.svelte.js";
import { detailUI, resetDetailUI, runUI } from "../lib/detail-ui.svelte.js";
import TaskDetail from "./TaskDetail.svelte";

const noop = () => {};

const detail = {
  id: 651,
  title: "FE split follow-up",
  status: "in_progress",
  project: "board",
  priority: "medium",
  description: "Move CSS into components",
  tags: ["ui"],
  notes: [{ id: 1, body: "started", author: "agent", created_at: "2026-08-02T12:00:00Z" }],
  created_at: "2026-08-02T10:00:00Z",
  updated_at: "2026-08-02T12:00:00Z",
};

afterEach(() => {
  resetBoardUI();
  resetDetailUI();
});

function renderDetail(overrides = {}) {
  detailUI.detail = detail;
  return render(TaskDetail, {
    props: {
      projectPaths: [],
      onClose: noop,
      onStartEdit: noop,
      onCancelEdit: noop,
      onSaveEdit: noop,
      onEditChange: noop,
      onMove: noop,
      onClearHandoff: noop,
      onRun: noop,
      onCancelRun: noop,
      onToggleAgentMenu: noop,
      onSelectAgent: noop,
      onSavePathAndRun: noop,
      onCancelPath: noop,
      onPathInput: (v) => { runUI.pathInput = v; },
      onSubmitAnswer: noop,
      onAnswerDraft: (v) => { detailUI.answerDraft = v; },
      onAddNote: noop,
      onNoteBody: (v) => { detailUI.noteBody = v; },
      ...overrides,
    },
  });
}

describe("TaskDetail", () => {
  it("renders task id, title, description, and thread note", () => {
    renderDetail();
    expect(screen.getByText("#651")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "FE split follow-up" })).toBeInTheDocument();
    expect(screen.getByText("Move CSS into components")).toBeInTheDocument();
    expect(screen.getByText("started")).toBeInTheDocument();
    expect(screen.getByText("Agent")).toBeInTheDocument();
  });

  it("exposes move actions and run controls when idle", () => {
    renderDetail();
    expect(screen.getByRole("button", { name: "Move to To-Do" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Move to Done" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Run/ })).toBeInTheDocument();
  });

  it("calls onClose from the close button", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    renderDetail({ onClose });
    await user.click(screen.getByRole("button", { name: "Close" }));
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("shows path prompt and wires Save & Run", async () => {
    const user = userEvent.setup();
    const onSavePathAndRun = vi.fn();
    runUI.needPath = { project: "board", taskId: 651 };
    runUI.pathInput = "~/sites/board";
    renderDetail({ onSavePathAndRun });
    expect(screen.getByText(/Where is project/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Save & Run/ }));
    expect(onSavePathAndRun).toHaveBeenCalledOnce();
  });

  it("shows pending question and submits an answer", async () => {
    const user = userEvent.setup();
    const onSubmitAnswer = vi.fn();
    boardUI.pendingQuestions = [{ id: 9, task_id: 651, question: "Prefer context or store?" }];
    detailUI.answerDraft = "context";
    renderDetail({ onSubmitAnswer });
    expect(screen.getByText("Prefer context or store?")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Submit answer" }));
    expect(onSubmitAnswer).toHaveBeenCalledWith(9);
  });
});
