package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/samuelloranger/board/internal/mcpserver"
	"github.com/samuelloranger/board/internal/setup"
	"github.com/samuelloranger/board/internal/store"
)

func dbPath() string {
	if p := os.Getenv("BOARD_DB"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".board", "board.db")
}

func openStore() (*store.Store, error) { return store.Open(dbPath()) }

// currentProject detects the project from the process working directory.
func currentProject() *string {
	wd, err := os.Getwd()
	if err != nil {
		return nil
	}
	return store.DetectProject(wd)
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: board <mcp|serve|setup|list|board|add|move|archive|note> ...")
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "mcp":
		return runMCP()
	case "serve":
		return runServe(rest, stdout)
	case "setup":
		return runSetup(rest, stdout)
	case "add":
		return cmdAdd(rest, stdout)
	case "list":
		return cmdList(rest, stdout)
	case "board":
		return cmdBoard(rest, stdout)
	case "move":
		return cmdMove(rest, stdout)
	case "archive":
		return cmdArchive(rest, stdout)
	case "note":
		return cmdNote(rest, stdout)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func runMCP() error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	srv := mcpserver.BuildServer(st, currentProject())
	return srv.Run(context.Background(), &mcp.StdioTransport{})
}

func cmdAdd(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: board add <title>")
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	tk, err := st.CreateTask(store.CreateTaskParams{
		Title:   strings.Join(args, " "),
		Project: currentProject(),
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "created #%d: %s\n", tk.ID, tk.Title)
	return nil
}

func cmdList(args []string, stdout io.Writer) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	f := store.ListFilter{Project: currentProject()}
	if len(args) == 1 && args[0] == "--all" {
		f.Project = nil
	}
	tasks, err := st.ListTasks(f)
	if err != nil {
		return err
	}
	for _, tk := range tasks {
		fmt.Fprintf(stdout, "#%d [%s] %s\n", tk.ID, tk.Status, tk.Title)
	}
	return nil
}

func cmdBoard(args []string, stdout io.Writer) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	b, err := st.GetBoard(currentProject())
	if err != nil {
		return err
	}
	printColumn(stdout, "TODO", b.Todo)
	printColumn(stdout, "IN PROGRESS", b.InProgress)
	printColumn(stdout, "DONE", b.Done)
	return nil
}

func printColumn(w io.Writer, title string, tasks []*store.Task) {
	fmt.Fprintf(w, "\n%s\n", title)
	for _, tk := range tasks {
		fmt.Fprintf(w, "  #%d %s\n", tk.ID, tk.Title)
	}
}

func cmdMove(args []string, stdout io.Writer) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: board move <id> <todo|in_progress|done>")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid id: %v", err)
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	tk, err := st.MoveTask(id, args[1])
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "#%d -> %s\n", tk.ID, tk.Status)
	return nil
}

func cmdArchive(args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: board archive <id>")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid id: %v", err)
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	if _, err := st.SetArchived(id, true); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "#%d archived\n", id)
	return nil
}

func cmdNote(args []string, stdout io.Writer) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: board note <id> <text>")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid id: %v", err)
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	defer st.Close()
	if _, err := st.AddNote(id, strings.Join(args[1:], " ")); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "note added to #%d\n", id)
	return nil
}

// Temporary stub — replaced in Task 12 (serve).
func runServe(args []string, stdout io.Writer) error {
	return fmt.Errorf("serve: not yet implemented")
}

func runSetup(args []string, stdout io.Writer) error {
	binPath, err := os.Executable()
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	clients := []setup.Client{
		{Name: "Claude Code", Kind: "json", Path: filepath.Join(home, ".claude.json")},
		{Name: "Codex CLI", Kind: "toml", Path: filepath.Join(home, ".codex", "config.toml")},
		{Name: "Cursor", Kind: "json", Path: filepath.Join(home, ".cursor", "mcp.json")},
		{Name: "Antigravity", Kind: "json", Path: filepath.Join(home, ".antigravity", "mcp_config.json")},
	}
	yes := len(args) == 1 && args[0] == "--yes"
	for _, c := range clients {
		if !yes {
			fmt.Fprintf(stdout, "Register board in %s (%s)? [y/N] ", c.Name, c.Path)
			var resp string
			fmt.Scanln(&resp)
			if !strings.HasPrefix(strings.ToLower(resp), "y") {
				fmt.Fprintf(stdout, "  skipped %s\n", c.Name)
				continue
			}
		}
		if err := setup.Register(binPath, c); err != nil {
			fmt.Fprintf(stdout, "  FAILED %s: %v\n", c.Name, err)
			continue
		}
		fmt.Fprintf(stdout, "  configured %s -> %s\n", c.Name, c.Path)
	}
	return nil
}
