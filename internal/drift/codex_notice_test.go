package drift

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// Beta report (Leeway 0.1.3, codex-cli 0.169.0): Codex's "error" item is a non-fatal notice, not a tool call.
// Shapes recorded from codex-cli 0.169.0 `codex exec --json` (see internal/testfake): the notice plus a completed
// turn is a judgment; the notice plus a failed turn fails that session with the turn's message (a normal failure,
// not a lock-down stop); a real tool item still stops the run.
func TestCodexNoticeIsNotToolUse(t *testing.T) {
	fakeCLI(t, "codex-cli")
	resetPreflight()

	t.Setenv("LEEWAY_FAKE_CODEX", "notice-ok")
	notes := &Notes{}
	text, u, _, err := Call(WithNotes(context.Background(), notes), "codex:gpt-6.2-astra", "s", "p", time.Minute)
	if err != nil || !strings.HasPrefix(text, "PROMPT=p") || u.Output != 10 {
		t.Fatalf("notice + completed turn must be a judgment: %q %+v %v", text, u, err)
	}
	if len(notes.List) != 1 || !strings.Contains(notes.List[0], "Model metadata for `gpt-6.2-astra` not found") {
		t.Errorf("notice not kept: %q", notes.List)
	}

	t.Setenv("LEEWAY_FAKE_CODEX", "notice-fail")
	_, _, _, err = Call(context.Background(), "codex:gpt-6.2-astra", "s", "p", time.Minute)
	var cfg *ErrConfig
	if err == nil || errors.As(err, &cfg) || !strings.Contains(err.Error(), "codex failed: unexpected status 401 Unauthorized") {
		t.Errorf("notice + failed turn must fail the session with the turn's message: %v", err)
	}

	t.Setenv("LEEWAY_FAKE_CODEX", "tool")
	_, _, _, err = Call(context.Background(), "codex:gpt-6.2-astra", "s", "p", time.Minute)
	if !errors.As(err, &cfg) || !strings.Contains(err.Error(), "command_execution item (a tool call)") {
		t.Errorf("a tool item must stop the run: %v", err)
	}
}

// In a run: the notice is logged once (not per session), the sessions are recorded, and nothing stops.
func TestCodexNoticeInARun(t *testing.T) {
	items, rb := fixtureItems(t)
	var codex []Item
	for _, it := range items {
		if it.Tool == "codex" {
			codex = append(codex, it)
		}
	}
	if len(codex) < 2 {
		t.Skip("fixture has fewer than 2 Codex sessions")
	}
	fakeCLI(t, "codex-cli")
	resetPreflight()
	t.Setenv("LEEWAY_FAKE_CODEX", "notice-ok")
	t.Setenv("LEEWAY_FAKE_TEXT", `{"verdict": "as_intended", "events": [], "summary": "ok"}`)
	var log []string
	res := JudgeAll(context.Background(), t.TempDir(), codex, rb, Options{Concurrency: 1, Log: func(s string) { log = append(log, s) }})
	joined := strings.Join(log, "\n")
	if res.StoppedReason != "" || res.Done != len(codex) || strings.Count(joined, "note from the Codex CLI: Model metadata") != 1 {
		t.Errorf("stopped %q, log:\n%s", res.StoppedReason, joined)
	}
}

// The preflight notes a CLI older than the tested one; the comparison reads both version spellings.
func TestOlderThanTested(t *testing.T) {
	for v, want := range map[string]bool{"codex-cli 0.169.0": true, "codex-cli 0.171.0": false, "codex-cli 0.172.1": false, "codex-cli": false} {
		if OlderThanTested("codex", v) != want {
			t.Errorf("codex %q: want %v", v, want)
		}
	}
	if !OlderThanTested("claude", "2.1.300 (Claude Code)") || OlderThanTested("claude", "2.1.329 (Claude Code)") {
		t.Error("claude version comparison")
	}
}

// Notices are masked like quotes and capped; the diagnostics file holds the call's facts, never the prompt.
func TestNoticeMaskedAndDiagnostics(t *testing.T) {
	tok := "sk-proj-" + strings.Repeat("Ab3dE", 8)
	if s := NoteText("bad key " + tok + strings.Repeat(" x", 400)); strings.Contains(s, tok) || len([]rune(s)) > 300 {
		t.Errorf("notice not masked or capped: %q", s)
	}
	fakeCLI(t, "codex-cli")
	resetPreflight()
	p, err := EnableDebug(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { debugMu.Lock(); debugPath = ""; debugMu.Unlock() }()
	t.Setenv("LEEWAY_FAKE_CODEX", "notice-fail")
	_, _, _, _ = Call(context.Background(), "codex:gpt-6.2-astra", "SYSTEM-SECRET-TEXT", "SESSION-TEXT", time.Minute)
	b, _ := os.ReadFile(p)
	s := string(b)
	for _, want := range []string{"cli: ", "codex-cli 0.171.0", "args: ", "--json", "exit: 1", "event: item.completed error: Model metadata", "event: turn.failed: unexpected status 401", "result: error: codex failed"} {
		if !strings.Contains(s, want) {
			t.Errorf("diagnostics lack %q:\n%s", want, s)
		}
	}
	home, _ := os.UserHomeDir()
	if !strings.Contains(s, "<email>") || strings.Contains(s, "someone@example.com") || strings.Contains(s, home) || !strings.Contains(s, "earlier lines left out") {
		t.Errorf("diagnostics not redacted or not capped:\n%s", s)
	}
	for _, bad := range []string{"SESSION-TEXT", "SYSTEM-SECRET-TEXT", "PROMPT="} {
		if strings.Contains(s, bad) {
			t.Errorf("diagnostics hold %q", bad)
		}
	}
}
