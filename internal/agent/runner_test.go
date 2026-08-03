package agent

import (
	"errors"
	"reflect"
	"strings"
	"testing"
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
