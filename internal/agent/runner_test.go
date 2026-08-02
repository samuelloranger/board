package agent

import (
	"errors"
	"testing"
)

func TestFakeRunnerRecordsPrompt(t *testing.T) {
	f := &FakeRunner{ExitCode: 0, Output: "hello out"}
	res, err := f.Start(StartOpts{Cwd: "/tmp/x", Prompt: "hello"})
	if err != nil || f.LastCwd != "/tmp/x" || f.LastPrompt != "hello" || res.PID != 4242 {
		t.Fatalf("%v %+v %+v", err, f, res)
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
