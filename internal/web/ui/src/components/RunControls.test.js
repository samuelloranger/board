import { render, screen } from "@testing-library/svelte";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import RunControls from "./RunControls.svelte";

const noop = () => {};

describe("RunControls", () => {
  it("shows Run and agent picker when idle", async () => {
    const user = userEvent.setup();
    const onRun = vi.fn();
    const onToggleMenu = vi.fn();
    render(RunControls, {
      props: {
        onRun,
        onCancel: noop,
        onToggleMenu,
        onSelectAgent: noop,
      },
    });
    await user.click(screen.getByRole("button", { name: /Run/ }));
    expect(onRun).toHaveBeenCalledOnce();
    await user.click(screen.getByRole("button", { name: "Choose agent" }));
    expect(onToggleMenu).toHaveBeenCalledOnce();
  });

  it("lists agents when menu is open and selects one", async () => {
    const user = userEvent.setup();
    const onSelectAgent = vi.fn();
    render(RunControls, {
      props: {
        agentMenuOpen: true,
        selectedAgent: "cursor",
        onRun: noop,
        onCancel: noop,
        onToggleMenu: noop,
        onSelectAgent,
      },
    });
    await user.click(screen.getByRole("menuitem", { name: "Claude" }));
    expect(onSelectAgent).toHaveBeenCalledWith("claude");
  });

  it("shows Cancel while a run is working", async () => {
    const user = userEvent.setup();
    const onCancel = vi.fn();
    render(RunControls, {
      props: {
        run: { status: "running" },
        onRun: noop,
        onCancel,
        onToggleMenu: noop,
        onSelectAgent: noop,
      },
    });
    expect(screen.queryByRole("button", { name: /Run/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onCancel).toHaveBeenCalledOnce();
  });
});
