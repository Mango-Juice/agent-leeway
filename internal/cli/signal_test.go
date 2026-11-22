//go:build !windows

package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// docs/20 H3: SIGTERM (what a tool timeout sends) and SIGHUP end the judge process trees, and nothing
// half-done is recorded, the same as Ctrl-C.
func TestTerminationEndsJudgesAndRecordsNothing(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGHUP} {
		tmp := fixtureEnv(t)
		exe, _ := os.Executable()
		pid := filepath.Join(tmp, "grandchild.pid")
		cmd := exec.Command(exe)
		cmd.Env = append(os.Environ(), "LEEWAY_CLI_ARGS=drift --include-auto --no-current-instructions --yes --concurrency 2",
			"LEEWAY_JUDGE_CMD="+exe+" -fake-judge", "LEEWAY_FAKE_MODE=grandchild", "LEEWAY_FAKE_PID="+pid)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			if b, err := os.ReadFile(pid); err == nil && len(b) > 0 {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		b, err := os.ReadFile(pid)
		if err != nil {
			_ = cmd.Process.Kill()
			t.Fatal("the judge never started")
		}
		gpid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		_ = cmd.Process.Signal(sig)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			_ = cmd.Process.Kill()
			t.Fatalf("%v: agent-leeway did not stop", sig)
		}
		time.Sleep(300 * time.Millisecond)
		if syscall.Kill(gpid, 0) == nil {
			_ = syscall.Kill(gpid, syscall.SIGKILL)
			t.Errorf("%v: the judge's child %d is still running", sig, gpid)
		}
		if res, err := os.ReadFile(filepath.Join(tmp, "cache", "drift", "results.jsonl")); err == nil && len(res) > 0 {
			t.Errorf("%v: a half-done judgment was recorded", sig)
		}
		judge, _ := filepath.Glob(filepath.Join(tmp, "cache", "judge", "run-*"))
		if len(judge) > 0 {
			t.Errorf("%v: judge folders left behind: %v", sig, judge)
		}
	}
}
