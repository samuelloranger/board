package agent

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"unicode/utf8"
)

// StartResult is a running agent process handle.
type StartResult struct {
	PID int
	// Wait blocks until exit and returns exit code, captured output (trimmed), and error.
	Wait func() (exitCode int, output string, err error)
	Kill func() error
}

// Runner launches an agent CLI in a project directory.
type Runner interface {
	Start(cwd, prompt string) (*StartResult, error)
}

// CursorRunner spawns `cursor-agent -p --force`.
type CursorRunner struct{}

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

func (CursorRunner) Start(cwd, prompt string) (*StartResult, error) {
	bin, err := lookPath("cursor-agent")
	if err != nil {
		return nil, errors.New("cursor-agent not found on PATH — install Cursor Agent CLI")
	}
	cmd := exec.Command(bin, "-p", "--force", "--output-format", "text", prompt)
	cmd.Dir = cwd
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pid := cmd.Process.Pid
	return &StartResult{
		PID: pid,
		Wait: func() (int, string, error) {
			err := cmd.Wait()
			out := trimOutput(buf.String())
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

// FakeRunner records Start args for tests.
type FakeRunner struct {
	LastCwd, LastPrompt string
	StartErr            error
	ExitCode            int
	Output              string
	KillErr             error
}

func (f *FakeRunner) Start(cwd, prompt string) (*StartResult, error) {
	f.LastCwd, f.LastPrompt = cwd, prompt
	if f.StartErr != nil {
		return nil, f.StartErr
	}
	out := f.Output
	code := f.ExitCode
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
