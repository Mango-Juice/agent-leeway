package drift

// Regression tests for docs/20-independent-review.md. Each one failed on the behavior the review reported.
// No model calls: judges are the test binary or a local script.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/proc"
)

// M1: a judge tier given by full model id is priced, so --budget stops the run; a tier with no price row is
// estimated as unknown (and the CLI refuses --budget for it, see internal/cli).
func TestBudgetAppliesToFullModelIDs(t *testing.T) {
	tb := fixtureTables(t)
	rb, err := LoadRubric("")
	if err != nil {
		t.Fatal(err)
	}
	sel := Select(tb, fxSince, fxNow.Add(time.Minute), nil, "", true, 0)
	tiers := map[string]string{"claude": "claude:claude-opus-5-5", "codex": "codex:gpt-6.2-astra"}
	items := Prepare(NewRenderer(tb), sel, rb, tiers, false)
	for _, it := range items {
		if it.EstUSD <= 0 {
			t.Fatalf("%s estimated at %v", it.Tier, it.EstUSD)
		}
	}
	res, recs := runWith(t, "ok", items, rb, Options{Concurrency: 1, Budget: 0.01})
	if !strings.Contains(res.StoppedReason, "budget") || len(recs) != 0 {
		t.Fatalf("budget not enforced: stopped %q, %d judged", res.StoppedReason, len(recs))
	}
	if _, ok := config.JudgeUSD("codex:gpt-6.2-astra-high", config.Usage{}); ok {
		t.Error("an unlisted model must not get a price silently")
	}
	lines, _, _ := EstimateLines([]Item{{Key: "k", Tier: "codex:gpt-6.2-astra-high"}}, nil, rb.Version, rb.Settings, 0)
	if !strings.Contains(strings.Join(lines, "\n"), "price unknown") {
		t.Errorf("estimate for an unpriced tier: %q", lines)
	}
}

// M1 (second part): a failed call without usage counts at its estimate against the budget.
func TestFailedCallsCountAgainstBudget(t *testing.T) {
	items, rb := fixtureItems(t)
	for i := range items {
		items[i].EstUSD = 0.05
	}
	res, recs := runWith(t, "fail", items[:1], rb, Options{Concurrency: 1})
	if len(recs) != 1 || res.Spent != 0.05 {
		t.Fatalf("spent %v after a failed call with no usage", res.Spent)
	}
}

func standIn(t *testing.T, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell script as the stand-in judge")
	}
	script := filepath.Join(t.TempDir(), "judge.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LEEWAY_JUDGE_CMD", script)
}

// M2: an argument error or a word such as "accurate" is not a usage limit; a real limit message still is.
func TestErrorsAreNotMisreadAsUsageLimits(t *testing.T) {
	cases := map[string]bool{
		"echo \"error: unexpected argument '--ignore-rules' found\" >&2\necho \"Usage: codex exec [OPTIONS] [PROMPT]\" >&2\nexit 2\n": false,
		"echo 'could not generate an accurate, separate answer' >&2\nexit 1\n":                                                        false,
		"echo 'Error: rate limit reached (429)' >&2\nexit 1\n":                                                                        true,
		"echo 'You have hit your usage limit. Try again later.' >&2\nexit 1\n":                                                        true,
	}
	for body, limit := range cases {
		standIn(t, body)
		_, _, _, err := Call(context.Background(), "codex:gpt-6.2-astra", "system", "prompt", 10*time.Second)
		var lim *ErrLimit
		if errors.As(err, &lim) != limit {
			t.Errorf("%q: limit=%v, err=%v", body, !limit, err)
		}
	}
	// a real CLI that rejects its arguments is a configuration error, not a limit
	err := argError(procResult(2, "error: unexpected argument '--restricted' found\n\nUsage: claude [options]"), "claude")
	var cfg *ErrConfig
	if !errors.As(err, &cfg) {
		t.Errorf("argument error: %v", err)
	}
}

// H1: each judge call runs in a fresh private folder under the user's cache folder, never a shared or
// predictable one, and refuses a cache folder that other accounts can write.
func TestJudgeWorkDirIsPrivateAndFresh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	cache := filepath.Join(t.TempDir(), "cache")
	t.Setenv("LEEWAY_CACHE", cache)
	a, err := privateDir()
	if err != nil {
		t.Fatal(err)
	}
	b, err := privateDir()
	if err != nil {
		t.Fatal(err)
	}
	if a == b || !strings.HasPrefix(a, cache) || strings.HasPrefix(a, os.TempDir()) && !strings.HasPrefix(cache, os.TempDir()) {
		t.Errorf("folders %s %s", a, b)
	}
	if fi, _ := os.Stat(a); fi.Mode().Perm() != 0o700 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	if err := os.Chmod(filepath.Join(cache, "judge"), 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := privateDir(); err == nil {
		t.Error("a judge folder that others can write must be refused")
	}
}

// H1/H2: the command lines switch off settings, hooks, instruction files, MCP and tools.
func TestJudgeCommandLinesLockDownTheCLIs(t *testing.T) {
	c := strings.Join(ClaudeArgs("opus", "/x/system.md"), " ")
	for _, f := range []string{"--tools  ", "--restricted", "--safe-mode", "--strict-mcp-config", "--disable-slash-commands", "--permission-prompts none", "--no-session-persistence"} {
		if !strings.Contains(c, f) {
			t.Errorf("claude args lack %q: %s", f, c)
		}
	}
	if strings.Contains(c, "--setting-sources") {
		t.Error("claude must not load project settings from its working folder")
	}
	x := strings.Join(CodexArgs("gpt-6.2-astra", "medium", "/x/s.md", "/x/run"), " ")
	for _, f := range []string{"--disable shell_tool", "--disable unified_exec", "--disable view_image", "--disable apps", "--disable hooks",
		"web_search=disabled", "approval_policy=never", "project_root_markers=[]", "--ignore-user-config", "--ignore-rules", "-s read-only"} {
		if !strings.Contains(x, f) {
			t.Errorf("codex args lack %q", f)
		}
	}
}

// H2: a Codex judge that runs a command anyway is stopped and nothing is recorded.
func TestCodexToolUseFailsClosed(t *testing.T) {
	out := `{"type":"thread.started"}` + "\n" + `{"type":"item.completed","item":{"type":"command_execution","command":"cat ~/.ssh/id_ed25519"}}` + "\n" +
		`{"type":"item.completed","item":{"type":"agent_message","text":"{}"}}` + "\n" + `{"type":"turn.completed","usage":{"input_tokens":1}}` + "\n"
	_, _, err := parseCodex(procResultOut(0, out), "gpt-6.2-astra")
	var cfg *ErrConfig
	if !errors.As(err, &cfg) {
		t.Fatalf("tool use not refused: %v", err)
	}
	if _, _, err := parseClaude(procResultOut(0, `{"result":"{}","num_turns":3,"usage":{}}`), "claude-opus-5-5"); !errors.As(err, &cfg) {
		t.Errorf("claude with several turns: %v", err)
	}
}

func procResult(code int, stderr string) proc.Result {
	return proc.Result{ExitCode: code, Stderr: []byte(stderr)}
}

func procResultOut(code int, stdout string) proc.Result {
	return proc.Result{ExitCode: code, Stdout: []byte(stdout)}
}

// docs/20 L10: a project CLAUDE.md that links outside the project folder is not read.
func TestInstructionSymlinkOutsideProjectIsSkipped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	secret := filepath.Join(home, "secret.txt")
	_ = os.WriteFile(secret, []byte("aws_secret_access_key = do-not-send"), 0o600)
	proj := filepath.Join(home, "proj")
	_ = os.MkdirAll(proj, 0o755)
	if err := os.Symlink(secret, filepath.Join(proj, "CLAUDE.md")); err != nil {
		t.Skip(err)
	}
	_ = os.WriteFile(filepath.Join(proj, "AGENTS.md"), []byte("use tabs in this project, always."), 0o600)
	got := CurrentInstructionFiles("claude", proj)
	for _, in := range got {
		if strings.Contains(in.Text, "do-not-send") {
			t.Fatal("a symlink to a file outside the project was sent")
		}
	}
	if len(got) != 1 {
		t.Errorf("want only AGENTS.md, got %d files", len(got))
	}
}
