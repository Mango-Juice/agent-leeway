package cli

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/drift"
	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/Mango-Juice/agent-leeway/internal/testfake"
)

func TestMain(m *testing.M) {
	for _, a := range os.Args[1:] {
		if a == "-fake-judge" { // a judge started by a CLI under test
			os.Exit(testfake.Main(os.Getenv("LEEWAY_FAKE_MODE")))
		}
	}
	if args := os.Getenv("LEEWAY_CLI_ARGS"); args != "" { // the CLI itself, run as a child process
		os.Exit(Main(strings.Fields(args), os.Stdout, os.Stderr))
	}
	if mode := os.Getenv("LEEWAY_FAKE_MODE"); mode != "" {
		os.Exit(testfake.Main(mode))
	}
	os.Exit(m.Run())
}

func repo(p ...string) string {
	a, _ := filepath.Abs(filepath.Join(append([]string{"..", ".."}, p...)...))
	return a
}

// fixtureEnv points every location at the fixtures and temporary folders, with the reference's clock and zone.
func fixtureEnv(t *testing.T) string {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Fatal(err)
	}
	old := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = old })
	tmp := t.TempDir()
	for k, v := range map[string]string{"LEEWAY_NOW": "2026-01-20T00:00:00Z", "LEEWAY_CLAUDE_DIR": repo("testdata", "logs", "claude"),
		"LEEWAY_CODEX_DIR": repo("testdata", "logs", "codex"), "LEEWAY_CONFIG": filepath.Join(tmp, "config"), "LEEWAY_CACHE": filepath.Join(tmp, "cache"),
		"CLAUDE_CONFIG_DIR": filepath.Join(tmp, "claude-home"), "LEEWAY_RUBRIC": repo("testdata", "rubric-v2.1-b636272b.md"), "FIXTURE_JUDGE_DIR": repo("testdata", "judge"), "LEEWAY_FAKE_MODE": "ok"} {
		t.Setenv(k, v)
	}
	exe, _ := os.Executable()
	t.Setenv("LEEWAY_JUDGE_CMD", exe)
	return tmp
}

func run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := Main(args, &out, &errb)
	return out.String(), errb.String(), rc
}

func expectedFile(t *testing.T, name string) string {
	b, err := os.ReadFile(repo("testdata", "expected", name))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if strings.HasSuffix(name, ".html") {
		return messages.FromReferenceHTML(text, config.Prices().Read, htmlEscape)
	}
	return messages.FromReference(text, config.Prices().Read)
}

func TestLeakCommandMatchesReference(t *testing.T) {
	legacy(t) // the Python reference's prices (docs/17 section 4)
	fixtureEnv(t)
	out, errs, rc := run(t, "leak")
	if rc != 0 || out != expectedFile(t, "leak.txt") {
		t.Fatalf("rc %d, stderr %s\n%s", rc, errs, out)
	}
	// the cached second run gives the same text
	if out2, _, _ := run(t, "leak"); out2 != out {
		t.Error("cached run differs")
	}
	js, _, rc := run(t, "leak", "--json")
	if rc != 0 || !strings.Contains(js, `"currency": "USD (API list-price equivalent)"`) {
		t.Fatalf("json: rc %d", rc)
	}
}

func TestDriftAndReportCommands(t *testing.T) {
	legacy(t) // the Python reference's prices (docs/17 section 4)
	tmp := fixtureEnv(t)
	_, errs, rc := run(t, "drift", "--include-auto", "--no-current-instructions", "--dry-run")
	if rc != 0 || !strings.Contains(errs, expectedFile(t, "drift_estimate.txt")[:40]) || !strings.Contains(errs, "--dry-run") {
		t.Fatalf("dry run: rc %d\n%s", rc, errs)
	}
	out, errs, rc := run(t, "drift", "--include-auto", "--no-current-instructions", "--yes", "--concurrency", "1")
	if rc != 0 || !strings.Contains(errs, "9 judged") || !strings.Contains(out, "Drift check: 9 sessions") {
		t.Fatalf("drift: rc %d\n%s\n%s", rc, errs, out)
	}
	_, errs, _ = run(t, "drift", "--include-auto", "--no-current-instructions", "--dry-run")
	if !strings.Contains(errs, "9 sessions already judged are skipped") {
		t.Errorf("resume: %s", errs)
	}
	// The report text is checked against the Go golden files (testdata/expected-go/report.*), in both
	// languages, with the product's own defaults; it is no longer compared with the reference's text (docs/17
	// section 4). The judged records above stay as they are.
	config.Legacy = false
	for _, lang := range []string{"en", "ko"} {
		// (rc is 1: the two sessions whose judgment did not parse are judged again and fail again)
		out, _, _ := run(t, "drift", "--include-auto", "--no-current-instructions", "--yes", "--refresh", "--lang", lang)
		if i := strings.Index(out, "\n\n"); i < 0 {
			t.Fatalf("drift summary: %s", out)
		} else {
			golden(t, "drift-terminal."+lang+".txt", out[i+2:])
		}
		out, _, rc := run(t, "leak", "--lang", lang)
		if i := strings.LastIndex(out, "\n\n"); rc != 0 || i < 0 {
			t.Fatalf("leak: %d %s", rc, out)
		} else {
			golden(t, "leak-cards."+lang+".txt", out[i+2:])
		}
		dir := filepath.Join(tmp, "reports-"+lang)
		out, errs, rc := run(t, "report", "--include-auto", "--lang", lang, "--out", dir)
		if rc != 0 {
			t.Fatalf("report: %d %s", rc, errs)
		}
		files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
		if len(files) != 1 {
			t.Fatalf("report files %v", files)
		}
		b, _ := os.ReadFile(files[0])
		golden(t, "report."+lang+".md", string(b))
		h, _ := os.ReadFile(strings.TrimSuffix(files[0], ".md") + ".html")
		golden(t, "report."+lang+".html", string(h))
		term := strings.ReplaceAll(out, dir+string(filepath.Separator), "<reports>/")
		golden(t, "report-terminal."+lang+".txt", term)
		if !strings.Contains(term, "<reports>/report-") {
			t.Errorf("terminal summary lacks the file paths:\n%s", out)
		}
	}
	// the themed state: a stored grouping (hand-written, the shape drift.GroupAll stores) over the fixture sessions
	themed := `{"tool": "claude", "tier": "claude:opus", "group_version": "` + drift.GroupVersion + `", "input_hash": "fixture", "n_events": 5, "error": null, "ts": 9e9,` +
		` "themes": [{"tool": "claude", "label": "Keep existing tables as they are", "events": ["claude:00000000-0000-4000-8000-000000000101#0",` +
		` "claude:00000000-0000-4000-8000-000000000102#0", "claude:00000000-0000-4000-8000-000000000103#0"], "sessions": [], "n_sessions": 3,` +
		` "draft_rule": "Do not change or remove existing tables unless I ask you to edit them."}], "dropped": []}` + "\n"
	gp := drift.GroupsPath(filepath.Join(tmp, "cache"))
	f, err := os.OpenFile(gp, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(themed)
	f.Close()
	for _, lang := range []string{"en", "ko"} {
		dir := filepath.Join(tmp, "themed-"+lang)
		// a tool whose grouping is turned off: its stored themes are not shown
		offDir := filepath.Join(tmp, "themed-off-"+lang)
		run(t, "report", "--include-auto", "--lang", lang, "--out", offDir, "--grouping", "claude=off")
		if offs, _ := filepath.Glob(filepath.Join(offDir, "*.html")); len(offs) == 1 {
			if b, _ := os.ReadFile(offs[0]); strings.Contains(string(b), `class="bq theme"`) {
				t.Error("themes shown from a tool whose grouping is off")
			}
		}
		out, errs, rc := run(t, "report", "--include-auto", "--lang", lang, "--out", dir)
		if rc != 0 {
			t.Fatalf("themed report: %d %s", rc, errs)
		}
		files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
		b, _ := os.ReadFile(files[0])
		h, _ := os.ReadFile(strings.TrimSuffix(files[0], ".md") + ".html")
		golden(t, "report-themed."+lang+".md", string(b))
		golden(t, "report-themed."+lang+".html", string(h))
		golden(t, "report-themed-terminal."+lang+".txt", strings.ReplaceAll(out, dir+string(filepath.Separator), "<reports>/"))
		if !strings.Contains(string(h), `class="bq theme"`) {
			t.Error("the themed report shows no theme")
		}
	}
}

var update = flag.Bool("update", false, "rewrite testdata/expected-go (outputs of the Go product's own defaults)")

// golden compares got with testdata/expected-go/name (rewritten with -update).
func golden(t *testing.T, name, got string) {
	t.Helper()
	p := repo("testdata", "expected-go", name)
	if *update {
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != got {
		t.Errorf("%s differs (run go test -update to accept):\n%s", name, firstLineDiff(string(b), got))
	}
}

func firstLineDiff(a, b string) string {
	la, lb := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(la) || i < len(lb); i++ {
		x, y := "", ""
		if i < len(la) {
			x = la[i]
		}
		if i < len(lb) {
			y = lb[i]
		}
		if x != y {
			return fmt.Sprintf("line %d\nwant: %s\ngot:  %s", i+1, x, y)
		}
	}
	return ""
}

func TestSinceAndModels(t *testing.T) {
	now := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
	for in, want := range map[string]time.Duration{"30d": 720 * time.Hour, " 2w ": 336 * time.Hour, "12h": 12 * time.Hour} {
		got, err := parseSince(in, now)
		if err != nil || now.Sub(got) != want {
			t.Errorf("parseSince(%q) = %v %v", in, got, err)
		}
	}
	if _, err := parseSince("3m", now); err == nil {
		t.Error("3m should be rejected")
	}
	m := parseModels("claude=sonnet, codex=codex:gpt-6-luna")
	if m["claude"] != "claude:sonnet" || m["codex"] != "codex:gpt-6-luna" {
		t.Errorf("parseModels = %v", m)
	}
}

// htmlEscape is Python's html.escape, as the report uses it.
func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#x27;").Replace(s)
}

// docs/19 B2: missing, mistyped or unreadable folders are errors; files outside the window get a hint.
func TestNothingFoundIsNotNothingLeaked(t *testing.T) {
	tmp := fixtureEnv(t)
	_, errs, rc := run(t, "leak", "--claude-dir", filepath.Join(tmp, "typo"), "--codex-dir", filepath.Join(tmp, "typo2"))
	if rc == 0 || !strings.Contains(errs, "does not exist") {
		t.Errorf("mistyped folders: rc %d\n%s", rc, errs)
	}
	empty := filepath.Join(tmp, "empty")
	_ = os.MkdirAll(empty, 0o755)
	if _, errs, rc = run(t, "leak", "--claude-dir", empty, "--codex-dir", empty); rc == 0 || !strings.Contains(errs, "No Claude Code or Codex session files") {
		t.Errorf("empty folders: rc %d\n%s", rc, errs)
	}
	t.Setenv("LEEWAY_NOW", "2026-11-20T00:00:00Z") // the fixtures' sessions are older than this window
	out, errs, rc := run(t, "leak")
	if rc != 0 || !strings.Contains(errs, "Read Claude Code logs:") || !strings.Contains(errs, "session files") {
		t.Errorf("folder summary missing: %s", errs)
	}
	if !strings.Contains(out, "no responses in this window") {
		t.Errorf("empty window should say so:\n%s", out)
	}
	if runtime.GOOS != "windows" {
		locked := filepath.Join(tmp, "locked")
		_ = os.MkdirAll(locked, 0o000)
		defer os.Chmod(locked, 0o755)
		if _, errs, rc = run(t, "leak", "--claude-dir", locked, "--tool", "claude"); rc == 0 || !strings.Contains(errs, "cannot be read") {
			t.Errorf("unreadable folder: rc %d\n%s", rc, errs)
		}
	}
}

// docs/19 B3: rubric --copy-to never overwrites without --force, and --force keeps a backup.
func TestRubricCopyDoesNotOverwrite(t *testing.T) {
	fixtureEnv(t)
	p := filepath.Join(t.TempDir(), "my.md")
	if _, _, rc := run(t, "rubric", "--copy-to", p); rc != 0 {
		t.Fatal("first copy failed")
	}
	_ = os.WriteFile(p, []byte("my edits"), 0o644)
	if _, errs, rc := run(t, "rubric", "--copy-to", p); rc == 0 || !strings.Contains(errs, "already exists") {
		t.Errorf("second copy: rc %d %s", rc, errs)
	}
	if b, _ := os.ReadFile(p); string(b) != "my edits" {
		t.Fatal("edits lost")
	}
	if _, _, rc := run(t, "rubric", "--copy-to", p, "--force"); rc != 0 {
		t.Fatal("--force failed")
	}
	baks, _ := filepath.Glob(p + ".bak-*")
	if len(baks) != 1 {
		t.Fatalf("no backup: %v", baks)
	}
	if b, _ := os.ReadFile(baks[0]); string(b) != "my edits" {
		t.Error("backup does not hold the edits")
	}
}

// docs/19 C6, C7: no judge CLI is an error; a run stopped by failures exits non-zero without an empty summary.
func TestDriftExitsNonZeroWhenNothingCanBeJudged(t *testing.T) {
	fixtureEnv(t)
	t.Setenv("LEEWAY_JUDGE_CMD", "")
	t.Setenv("PATH", t.TempDir())
	if _, errs, rc := run(t, "drift", "--include-auto", "--dry-run"); rc == 0 || !strings.Contains(errs, "No judge CLI is available") {
		t.Errorf("no CLI: rc %d\n%s", rc, errs)
	}
	fixtureEnv(t)
	t.Setenv("LEEWAY_FAKE_MODE", "fail")
	out, errs, rc := run(t, "drift", "--include-auto", "--no-current-instructions", "--yes", "--concurrency", "1")
	if rc == 0 || strings.Contains(out, "Drift check: 0 sessions") || !strings.Contains(errs, "two judge calls failed") {
		t.Errorf("failed run: rc %d\nout %s\nerr %s", rc, out, errs)
	}
}

// docs/19 C5: the dry run lists the sessions, says what is included and how it is billed, and can write the
// exact judge input to a folder.
func TestDryRunShowsWhatWouldBeSent(t *testing.T) {
	fixtureEnv(t)
	dir := filepath.Join(t.TempDir(), "inputs")
	_, errs, rc := run(t, "drift", "--include-auto", "--no-current-instructions", "--dry-run", "--write-inputs", dir)
	if rc != 0 {
		t.Fatalf("rc %d %s", rc, errs)
	}
	for _, want := range []string{"Claude Code", "k tokens", "instruction files", "plan quota on a subscription, money on an API key", "Wrote the exact judge input of 11 sessions", "LEEWAY_JUDGE_CMD is set"} {
		if !strings.Contains(errs, want) {
			t.Errorf("dry run lacks %q:\n%s", want, errs)
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.txt"))
	if len(files) != 11 {
		t.Fatalf("%d input files", len(files))
	}
	sys, _ := os.ReadFile(filepath.Join(dir, "system-prompt.md"))
	// the reference's prompt (validated text) with the t2 summary-language change
	if string(sys) != drift.SummaryLanguageT2(expectedFile(t, "system_prompt.txt")) {
		t.Error("system prompt written differs from the judge's")
	}
}

// The drift check leads with session counts, judges the 50 most recent sessions unless --max-sessions says
// otherwise, has no dollar limit unless --budget is given, and names the limit that cuts the run.
func TestDriftDefaultsAndPlanMessage(t *testing.T) {
	legacy(t)
	fixtureEnv(t)
	_, errs, rc := run(t, "drift", "--include-auto", "--no-current-instructions", "--dry-run")
	if rc != 0 || !strings.Contains(errs, "11 sessions in the window; the 11 most recent will be judged (one judge call per session)") ||
		strings.Contains(errs, "--budget $") || strings.Contains(errs, "Only the") {
		t.Fatalf("default run: rc %d\n%s", rc, errs)
	}
	if !strings.Contains(errs, "upper estimates for the input") {
		t.Errorf("the estimate is not labelled as an upper estimate:\n%s", errs)
	}
	_, errs, _ = run(t, "drift", "--include-auto", "--no-current-instructions", "--dry-run", "--max-sessions", "3")
	if !strings.Contains(errs, "the 3 most recent will be judged") || !strings.Contains(errs, "Only the 3 most recent of the 11 sessions are taken") || !strings.Contains(errs, "--max-sessions 0 for all 11") {
		t.Errorf("--max-sessions line missing:\n%s", errs)
	}
	for _, n := range []string{"100", "0"} {
		_, errs, _ = run(t, "drift", "--include-auto", "--no-current-instructions", "--dry-run", "--max-sessions", n)
		if !strings.Contains(errs, "the 11 most recent will be judged") || strings.Contains(errs, "Only the") {
			t.Errorf("--max-sessions %s must take all 11:\n%s", n, errs)
		}
	}
	_, errs, _ = run(t, "drift", "--include-auto", "--no-current-instructions", "--dry-run", "--budget", "0.5")
	if !strings.Contains(errs, "--budget $0.50 (API list-price equivalent) stops the run after about ") {
		t.Errorf("budget line missing:\n%s", errs)
	}
	_, errs, _ = run(t, "drift", "-h")
	if !strings.Contains(errs, "(default 50)") || strings.Contains(errs, "(default 30)") {
		t.Errorf("help defaults:\n%s", errs)
	}
}

// --group-only judges nothing and runs the grouping step on stored results; --no-grouping makes no grouping call.
func TestGroupingFlags(t *testing.T) {
	tmp := fixtureEnv(t)
	if _, errs, rc := run(t, "drift", "--include-auto", "--no-current-instructions", "--yes", "--no-grouping"); rc != 1 && rc != 0 {
		t.Fatalf("drift: %d %s", rc, errs)
	}
	gp := drift.GroupsPath(filepath.Join(tmp, "cache"))
	if _, err := os.Stat(gp); err == nil {
		t.Fatal("--no-grouping made a grouping call")
	}
	n := len(drift.LoadResults(filepath.Join(tmp, "cache")))
	_, errs, _ := run(t, "drift", "--include-auto", "--no-current-instructions", "--yes", "--group-only")
	if len(drift.LoadResults(filepath.Join(tmp, "cache"))) != n {
		t.Error("--group-only judged sessions")
	}
	// defaults: on for both tools, and the pre-run text says so
	if g := drift.LoadGroups(filepath.Join(tmp, "cache")); len(g) != 2 || !strings.Contains(errs, "Claude Code on, Codex on") {
		t.Errorf("--group-only with defaults: %d grouping records\n%s", len(g), errs)
	}
	before := len(drift.LoadGroups(filepath.Join(tmp, "cache")))
	_, errs, _ = run(t, "drift", "--include-auto", "--no-current-instructions", "--yes", "--group-only", "--grouping", "claude=off")
	claude := 0
	for _, g := range drift.LoadGroups(filepath.Join(tmp, "cache"))[before:] {
		if g.Get("tool").S == "claude" {
			claude++
		}
	}
	if claude != 0 || !strings.Contains(errs, "Claude Code off, Codex on") {
		t.Errorf("--grouping claude=off: %d new Claude grouping records\n%s", claude, errs)
	}
	if _, errs, rc := run(t, "drift", "--group-only", "--yes", "--grouping", "all=off"); rc != 0 || !strings.Contains(errs, "Grouping is off for every judged tool") {
		t.Errorf("all off: %d %s", rc, errs)
	}
	if _, errs, rc := run(t, "drift", "--dry-run", "--grouping", "claude=maybe"); rc != 2 || !strings.Contains(errs, "--grouping") {
		t.Errorf("bad --grouping value: %d %s", rc, errs)
	}
	// unchanged input: the next run makes no call and asks nothing
	_, errs, _ = run(t, "drift", "--include-auto", "--no-current-instructions", "--group-only")
	if strings.Contains(errs, "need --yes") && len(drift.LoadGroups(filepath.Join(tmp, "cache"))) != 2 {
		t.Errorf("second run: %s", errs)
	}
	// with --tool claude only Claude Code's problems are grouped, and the left-out Codex sessions are named
	_, errs, _ = run(t, "drift", "--include-auto", "--no-current-instructions", "--yes", "--tool", "claude", "--no-grouping", "--dry-run")
	if !strings.Contains(errs, "Codex session(s) in the window are not judged") || !strings.Contains(errs, "--tool all") || !strings.Contains(errs, "Sessions in the window: Claude Code") {
		t.Errorf("--tool claude pre-run text:\n%s", errs)
	}
	if _, errs, rc := run(t, "drift", "--tool", "all", "--dry-run", "--include-auto", "--no-current-instructions"); rc != 0 || strings.Contains(errs, "are not judged in this run") {
		t.Errorf("--tool all: %d\n%s", rc, errs)
	}
}

// Grouping runs only for the tools that were judged.
func TestGroupingOnlyForJudgedTools(t *testing.T) {
	tmp := fixtureEnv(t)
	run(t, "drift", "--include-auto", "--no-current-instructions", "--yes", "--no-grouping")
	run(t, "drift", "--include-auto", "--no-current-instructions", "--yes", "--group-only", "--tool", "claude", "--grouping", "claude=on")
	g := drift.LoadGroups(filepath.Join(tmp, "cache"))
	if len(g) != 1 || g[0].Get("tool").S != "claude" {
		t.Errorf("grouping records: %d", len(g))
	}
}
