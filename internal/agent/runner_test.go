package agent

import (
	"errors"
	"testing"
)

func TestCursorRunnerMissingBinary(t *testing.T) {
	old := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	defer func() { lookPath = old }()
	_, err := (CursorRunner{}).Start(t.TempDir(), "hi")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFakeRunnerRecordsPrompt(t *testing.T) {
	f := &FakeRunner{ExitCode: 0}
	res, err := f.Start("/tmp/x", "hello")
	if err != nil || f.LastCwd != "/tmp/x" || f.LastPrompt != "hello" || res.PID != 4242 {
		t.Fatalf("%v %+v %+v", err, f, res)
	}
	code, out, err := res.Wait()
	if err != nil || code != 0 || out != "" {
		t.Fatalf("code=%d out=%q err=%v", code, out, err)
	}
}
