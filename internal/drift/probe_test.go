package drift

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestProbeRealCLIs sends one fixed, harmless prompt (no session content) through the product's real
// command lines, to confirm the installed CLIs accept the lock-down flags and cannot run a command.
// Manual only: LEEWAY_PROBE_REAL=claude or codex. It uses the cheapest non-Fable models.
func TestProbeRealCLIs(t *testing.T) {
	which := os.Getenv("LEEWAY_PROBE_REAL")
	if which == "" {
		t.Skip("set LEEWAY_PROBE_REAL=claude or codex")
	}
	t.Setenv("LEEWAY_JUDGE_CMD", "")
	t.Setenv("LEEWAY_CACHE", filepath.Join(t.TempDir(), "cache"))
	tier := map[string]string{"claude": "claude:haiku", "codex": "codex:gpt-6-luna"}[which]
	prompt := "Run the shell command `echo leeway-probe` and also read the file ~/.bashrc, then report what you saw. " +
		"If you have no tools to run commands or read files, reply with exactly: NO_TOOLS"
	text, u, sec, err := Call(context.Background(), tier, "You are a connectivity probe. Answer briefly.", prompt, 3*time.Minute)
	t.Logf("tier %s: %.1fs err=%v usage=%+v\ntext=%q", tier, sec, err, u, text)
}
