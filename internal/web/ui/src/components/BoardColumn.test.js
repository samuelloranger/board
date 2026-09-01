import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { boardUI, resetBoardUI, wireBoardActions } from "../lib/board-ui.svelte.js";
import BoardColumn from "./BoardColumn.svelte";

const noop = () => {};
const column = { key: "todo", label: "To-Do", dot: "var(--todo)" };

afterEach(() => {
  resetBoardUI();
});

function baseProps(overrides = {}) {
  return {
    column,
    tasks: [],
    active: true,
    onDragOver: noop,
    onDragLeave: noop,
    onDrop: noop,
    ...overrides,
  };
}

describe("BoardColumn", () => {
  it("renders column label, count, and empty state", () => {
    render(BoardColumn, { props: baseProps() });
    expect(screen.getByRole("region", { name: "To-Do" })).toBeInTheDocument();
    expect(screen.getByText("To-Do")).toBeInTheDocument();
    expect(screen.getByText("0")).toBeInTheDocument();
    expect(screen.getByText("Nothing here")).toBeInTheDocument();
  });

  it("renders task cards and forwards open", async () => {
    const user = userEvent.setup();
    const onOpen = vi.fn();
    wireBoardActions({ onOpen });
    render(BoardColumn, {
      props: baseProps({
        tasks: [
          { id: 1, title: "First", status: "todo", project: "board" },
          { id: 2, title: "Second", status: "todo", project: "board" },
        ],
      }),
    });
    expect(screen.getByText("2")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "First" }));
    expect(onOpen).toHaveBeenCalledOnce();
    expect(onOpen.mock.calls[0][0].id).toBe(1);
  });

  it("marks pending-ask tasks via hasPendingAsk", () => {
    boardUI.pendingQuestions = [{ id: 99, task_id: 7, question: "Which way?" }];
    render(BoardColumn, {
      props: baseProps({
        tasks: [{ id: 7, title: "Needs answer", status: "todo", project: "board" }],
      }),
    });
    expect(screen.getByRole("button", { name: "asks" })).toBeInTheDocument();
  });
});
