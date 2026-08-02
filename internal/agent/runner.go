package agent

import (
	"bufio"
	"bytes"
	"errors"
	"io"
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

func (CursorRunner) Start(opts StartOpts) (*StartResult, error) {
	bin, err := lookPath("cursor-agent")
	if err != nil {
		return nil, errors.New("cursor-agent not found on PATH — install Cursor Agent CLI")
	}
	cmd := exec.Command(bin, "-p", "--force", "--output-format", "text", opts.Prompt)
	cmd.Dir = opts.Cwd

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

// FakeRunner records Start args for tests.
type FakeRunner struct {
	LastCwd, LastPrompt string
	StartErr            error
	ExitCode            int
	Output              string
	KillErr             error
}

func (f *FakeRunner) Start(opts StartOpts) (*StartResult, error) {
	f.LastCwd, f.LastPrompt = opts.Cwd, opts.Prompt
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
