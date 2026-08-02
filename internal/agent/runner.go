package agent

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// StartResult is a running agent process handle.
type StartResult struct {
	PID  int
	Wait func() (exitCode int, err error)
	Kill func() error
}

// Runner launches an agent CLI in a project directory.
type Runner interface {
	Start(cwd, prompt string) (*StartResult, error)
}

// CursorRunner spawns `cursor-agent -p --force`.
type CursorRunner struct{}

var lookPath = exec.LookPath

func (CursorRunner) Start(cwd, prompt string) (*StartResult, error) {
	bin, err := lookPath("cursor-agent")
	if err != nil {
		return nil, errors.New("cursor-agent not found on PATH — install Cursor Agent CLI")
	}
	cmd := exec.Command(bin, "-p", "--force", "--output-format", "text", prompt)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	pid := cmd.Process.Pid
	return &StartResult{
		PID: pid,
		Wait: func() (int, error) {
			err := cmd.Wait()
			if err == nil {
				return 0, nil
			}
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				return ee.ExitCode(), err
			}
			return -1, err
		},
		Kill: func() error {
			if cmd.Process == nil {
				return nil
			}
			// Kill the process group if possible; otherwise the process itself.
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
	KillErr             error
}

func (f *FakeRunner) Start(cwd, prompt string) (*StartResult, error) {
	f.LastCwd, f.LastPrompt = cwd, prompt
	if f.StartErr != nil {
		return nil, f.StartErr
	}
	return &StartResult{
		PID:  4242,
		Wait: func() (int, error) { return f.ExitCode, nil },
		Kill: func() error { return f.KillErr },
	}, nil
}
