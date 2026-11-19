package agent

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestFakeRunnerRecordsPrompt(t *testing.T) {
	f := &FakeRunner{ExitCode: 0, Output: "hello out"}
	res, err := f.Start(StartOpts{Cwd: "/tmp/x", Prompt: "hello", Env: []string{"BOARD_RUN_ID=9"}})
	if err != nil || f.LastCwd != "/tmp/x" || f.LastPrompt != "hello" || res.PID != 4242 {
		t.Fatalf("%v %+v %+v", err, f, res)
	}
	if len(f.LastEnv) != 1 || f.LastEnv[0] != "BOARD_RUN_ID=9" {
		t.Fatalf("LastEnv=%v", f.LastEnv)
	}
	code, out, err := res.Wait()
	if err != nil || code != 0 || out != "hello out" {
		t.Fatalf("code=%d out=%q err=%v", code, out, err)
	}
}

func TestCursorRunnerMissingBinary(t *testing.T) {
	old := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	defer func() { lookPath = old }()
	_, err := (CursorRunner{}).Start(StartOpts{Cwd: t.TempDir(), Prompt: "hi"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClaudeRunnerMissingBinary(t *testing.T) {
	old := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	defer func() { lookPath = old }()
	_, err := (ClaudeRunner{}).Start(StartOpts{Cwd: t.TempDir(), Prompt: "hi"})
	if err == nil || !strings.Contains(err.Error(), "claude") {
		t.Fatalf("got %v", err)
	}
}

func TestCodexRunnerMissingBinary(t *testing.T) {
	old := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	defer func() { lookPath = old }()
	_, err := (CodexRunner{}).Start(StartOpts{Cwd: t.TempDir(), Prompt: "hi"})
	if err == nil || !strings.Contains(err.Error(), "codex") {
		t.Fatalf("got %v", err)
	}
}

func TestDefaultRunner(t *testing.T) {
	cases := []struct {
		name string
		want any
	}{
		{"", CursorRunner{}},
		{AgentCursor, CursorRunner{}},
		{AgentClaude, ClaudeRunner{}},
		{AgentCodex, CodexRunner{}},
	}
	for _, tc := range cases {
		r, err := DefaultRunner(tc.name)
		if err != nil || reflect.TypeOf(r) != reflect.TypeOf(tc.want) {
			t.Fatalf("%q: got %T %v", tc.name, r, err)
		}
	}
	if _, err := DefaultRunner("nope"); err == nil {
		t.Fatal("expected error")
	}
}

func TestAgentArgs(t *testing.T) {
	if got := cursorArgs("p"); !reflect.DeepEqual(got, []string{"-p", "--force", "--output-format", "text", "p"}) {
		t.Fatalf("cursor: %v", got)
	}
	if got := claudeArgs("p"); !reflect.DeepEqual(got, []string{"-p", "--dangerously-skip-permissions", "--output-format", "text", "p"}) {
		t.Fatalf("claude: %v", got)
	}
	if got := codexArgs("p"); !reflect.DeepEqual(got, []string{"exec", "--sandbox", "workspace-write", "p"}) {
		t.Fatalf("codex: %v", got)
	}
}

func TestWithoutEnvKeysStripsAPIKey(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"ANTHROPIC_API_KEY=sk-secret",
		"HOME=/home/u",
		"ANTHROPIC_API_KEY_EXTRA=keep",
	}
	got := withoutEnvKeys(env, "ANTHROPIC_API_KEY")
	want := []string{"PATH=/usr/bin", "HOME=/home/u", "ANTHROPIC_API_KEY_EXTRA=keep"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// TestKillTerminatesProcessGroup verifies Kill takes down the whole process
// tree, not just the direct child: agent CLIs spawn shells whose children
// would otherwise outlive a cancel.
func TestKillTerminatesProcessGroup(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("/proc-based liveness probe not supported on %s", runtime.GOOS)
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "grandchild.pid")
	res, err := startCLI("sh", "sh not found on PATH",
		[]string{"-c", `sleep 30 & echo $! > "$GP"; wait`},
		StartOpts{Cwd: dir}, append(os.Environ(), "GP="+pidFile))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Kill() })

	// The child must lead its own process group, or a group kill cannot
	// reach the tree at all.
	if err := syscall.Kill(-res.PID, 0); err != nil {
		t.Fatalf("child %d is not a process-group leader: %v", res.PID, err)
	}

	grand := waitForGrandchild(t, pidFile)
	if !procAlive(grand) {
		t.Fatalf("grandchild %d is not running before Kill", grand)
	}
	if err := res.Kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for procAlive(grand) {
		if time.Now().After(deadline) {
			_ = syscall.Kill(grand, syscall.SIGKILL) // do not leak sleep on failure
			t.Fatalf("grandchild %d survived Kill", grand)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, _, err := res.Wait(); err == nil {
		t.Fatal("expected the killed child to report an error exit")
	}
}

func waitForGrandchild(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if b, err := os.ReadFile(pidFile); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && pid > 0 {
				return pid
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("grandchild pid never appeared in %s", pidFile)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// procAlive reports whether pid is a live process, via /proc: a zombie or a
// vanished entry counts as dead.
func procAlive(pid int) bool {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	// Fields: pid (comm) state ... — the state letter follows the last ')'.
	i := bytes.LastIndexByte(b, ')')
	if i < 0 || i+2 >= len(b) {
		return false
	}
	switch b[i+2] {
	case 'Z', 'X':
		return false
	}
	return true
}
