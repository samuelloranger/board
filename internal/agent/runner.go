package agent

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

// StartOpts configures an agent launch.
type StartOpts struct {
	Cwd    string
	Prompt string
	// OnProgress is called with the trimmed output so far (throttled). Optional.
	OnProgress func(output string)
	// Env is the child process environment. nil means inherit the default
	// (os.Environ, possibly filtered by the Runner).
	Env []string
}

// StartResult is a running agent process handle.
type StartResult struct {
	PID int
	// Wait blocks until exit and returns exit code, captured output (trimmed), and error.
	Wait func() (exitCode int, output string, err error)
	Kill func() error
}

// Runner launches an agent CLI in a project directory.
type Runner interface {
	Start(opts StartOpts) (*StartResult, error)
}

// Known agent identifiers accepted by the web API / UI.
const (
	AgentCursor = "cursor"
	AgentClaude = "claude"
	AgentCodex  = "codex"
)

// DefaultRunner returns the built-in Runner for a named agent.
func DefaultRunner(name string) (Runner, error) {
	switch name {
	case "", AgentCursor:
		return CursorRunner{}, nil
	case AgentClaude:
		return ClaudeRunner{}, nil
	case AgentCodex:
		return CodexRunner{}, nil
	default:
		return nil, fmt.Errorf("unknown agent %q (supported: cursor, claude, codex)", name)
	}
}

// CursorRunner spawns `cursor-agent -p --force`.
type CursorRunner struct{}

// ClaudeRunner spawns `claude -p` with permissions bypassed.
// ANTHROPIC_API_KEY is stripped from the child env so Claude Code uses
// the logged-in subscription (Pro/Max/Team) instead of API billing.
type ClaudeRunner struct{}

// CodexRunner spawns `codex exec` with workspace-write sandbox.
// codex exec defaults to approval_policy=never (headless).
type CodexRunner struct{}

var lookPath = exec.LookPath

const maxOutputRunes = 2000

func trimOutput(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxOutputRunes {
		return s
	}
	runes := []rune(s)
	return "…" + string(runes[len(runes)-maxOutputRunes:])
}

func cursorArgs(prompt string) []string {
	return []string{"-p", "--force", "--output-format", "text", prompt}
}

func claudeArgs(prompt string) []string {
	return []string{"-p", "--dangerously-skip-permissions", "--output-format", "text", prompt}
}

func codexArgs(prompt string) []string {
	return []string{"exec", "--sandbox", "workspace-write", prompt}
}

// withoutEnvKeys returns env without the named keys (case-sensitive match on KEY=).
func withoutEnvKeys(env []string, keys ...string) []string {
	drop := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		drop[k] = struct{}{}
	}
	out := make([]string, 0, len(env))
	for _, e := range env {
		k, _, _ := strings.Cut(e, "=")
		if _, ok := drop[k]; ok {
			continue
		}
		out = append(out, e)
	}
	return out
}

func startCLI(binName, missingHint string, args []string, opts StartOpts, env []string) (*StartResult, error) {
	bin, err := lookPath(binName)
	if err != nil {
		return nil, errors.New(missingHint)
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = opts.Cwd
	if env != nil {
		cmd.Env = env
	}

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	var (
		mu  sync.Mutex
		buf bytes.Buffer
	)
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		var lastFlush time.Time
		flush := func(force bool) {
			if opts.OnProgress == nil {
				return
			}
			now := time.Now()
			if !force && now.Sub(lastFlush) < 1500*time.Millisecond {
				return
			}
			lastFlush = now
			mu.Lock()
			out := trimOutput(buf.String())
			mu.Unlock()
			if out != "" {
				opts.OnProgress(out)
			}
		}
		for sc.Scan() {
			line := sc.Text()
			mu.Lock()
			buf.WriteString(line)
			buf.WriteByte('\n')
			mu.Unlock()
			flush(false)
		}
		flush(true)
	}()

	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		return nil, err
	}
	pid := cmd.Process.Pid
	return &StartResult{
		PID: pid,
		Wait: func() (int, string, error) {
			err := cmd.Wait()
			_ = pw.Close()
			<-done
			mu.Lock()
			out := trimOutput(buf.String())
			mu.Unlock()
			if err == nil {
				return 0, out, nil
			}
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				return ee.ExitCode(), out, err
			}
			return -1, out, err
		},
		Kill: func() error {
			if cmd.Process == nil {
				return nil
			}
			if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
				return cmd.Process.Kill()
			}
			return nil
		},
	}, nil
}

func (CursorRunner) Start(opts StartOpts) (*StartResult, error) {
	return startCLI("cursor-agent", "cursor-agent not found on PATH — install Cursor Agent CLI",
		cursorArgs(opts.Prompt), opts, opts.Env)
}

func (ClaudeRunner) Start(opts StartOpts) (*StartResult, error) {
	// Strip API key so Claude Code falls back to subscription OAuth credentials.
	env := opts.Env
	if env == nil {
		env = withoutEnvKeys(os.Environ(), "ANTHROPIC_API_KEY")
	} else {
		env = withoutEnvKeys(env, "ANTHROPIC_API_KEY")
	}
	return startCLI("claude", "claude not found on PATH — install Claude Code CLI",
		claudeArgs(opts.Prompt), opts, env)
}

func (CodexRunner) Start(opts StartOpts) (*StartResult, error) {
	return startCLI("codex", "codex not found on PATH — install OpenAI Codex CLI",
		codexArgs(opts.Prompt), opts, opts.Env)
}

// FakeRunner records Start args for tests.
type FakeRunner struct {
	LastCwd, LastPrompt string
	LastEnv             []string
	StartErr            error
	ExitCode            int
	Output              string
	KillErr             error
}

func (f *FakeRunner) Start(opts StartOpts) (*StartResult, error) {
	f.LastCwd, f.LastPrompt, f.LastEnv = opts.Cwd, opts.Prompt, opts.Env
	if f.StartErr != nil {
		return nil, f.StartErr
	}
	out := f.Output
	code := f.ExitCode
	if opts.OnProgress != nil && out != "" {
		opts.OnProgress(out)
	}
	return &StartResult{
		PID: 4242,
		Wait: func() (int, string, error) {
			if code != 0 {
				return code, out, errors.New("exit")
			}
			return code, out, nil
		},
		Kill: func() error { return f.KillErr },
	}, nil
}
