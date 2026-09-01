import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { boardUI, resetBoardUI, wireBoardActions } from "../lib/board-ui.svelte.js";
import TaskCard from "./TaskCard.svelte";

afterEach(() => {
  resetBoardUI();
});

function renderCard(taskOverrides = {}, actionOverrides = {}) {
  const actions = {
    onOpen: vi.fn(),
    onToggleMenu: vi.fn(),
    onMove: vi.fn(),
    onArchive: vi.fn(),
    onRun: vi.fn(),
    onCancel: vi.fn(),
    onToggleAgentMenu: vi.fn(),
    onSelectAgent: vi.fn(),
    onDragStart: vi.fn(),
    ...actionOverrides,
  };
  wireBoardActions(actions);
  render(TaskCard, {
    props: {
      task: {
        id: 42,
        title: "Ship scoped styles",
        status: "todo",
        project: "board",
        priority: "high",
        tags: ["ui"],
        ...taskOverrides,
      },
    },
  });
  return actions;
}

describe("TaskCard", () => {
  it("renders title, project, priority, and tags", () => {
    renderCard();
    expect(screen.getByRole("button", { name: "Ship scoped styles" })).toBeInTheDocument();
    expect(screen.getByText("board")).toBeInTheDocument();
    expect(screen.getByText("high")).toBeInTheDocument();
    expect(screen.getByText("ui")).toBeInTheDocument();
  });

  it("opens task on title click", async () => {
    const user = userEvent.setup();
    const actions = renderCard();
    await user.click(screen.getByRole("button", { name: "Ship scoped styles" }));
    expect(actions.onOpen).toHaveBeenCalledOnce();
    expect(actions.onOpen.mock.calls[0][0].id).toBe(42);
  });

  it("shows working agent status and Cancel when run is active", () => {
    boardUI.latestByTask = {
      42: { status: "running", agent: "claude", message: "editing styles" },
    };
    renderCard();
    expect(screen.getByText(/Claude · Working/)).toBeInTheDocument();
    expect(screen.getByText("editing styles")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
  });

  it("shows asks badge when a question is pending", async () => {
    const user = userEvent.setup();
    boardUI.pendingQuestions = [{ id: 1, task_id: 42, question: "ok?" }];
    const actions = renderCard();
    const badge = screen.getByRole("button", { name: "asks" });
    expect(badge).toBeInTheDocument();
    await user.click(badge);
    expect(actions.onOpen).toHaveBeenCalledOnce();
  });

  it("lists move targets and archive when menu is open", async () => {
    const user = userEvent.setup();
    boardUI.openMenu = 42;
    const actions = renderCard();
    await user.click(screen.getByRole("menuitem", { name: "Move to In Progress" }));
    expect(actions.onMove).toHaveBeenCalledWith(42, "in_progress");
    await user.click(screen.getByRole("menuitem", { name: "Archive" }));
    expect(actions.onArchive).toHaveBeenCalledWith(42);
  });
});
