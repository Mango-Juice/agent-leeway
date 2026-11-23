package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/drift"
	"github.com/Mango-Juice/agent-leeway/internal/leak"
	"github.com/Mango-Juice/agent-leeway/internal/py"
	"github.com/Mango-Juice/agent-leeway/internal/report"
)

var cmpNow = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

// cmpEnv is an isolated home with made-up Claude Code sessions, one per day offset given (days before cmpNow),
// each file modified at its last turn, as Claude Code leaves it. Codex has an empty folder.
func cmpEnv(t *testing.T, days ...int) string {
	t.Helper()
	tmp := t.TempDir()
	cl := filepath.Join(tmp, "claude", "-tmp-cmp")
	_ = os.MkdirAll(cl, 0o700)
	_ = os.MkdirAll(filepath.Join(tmp, "codex"), 0o700)
	for i, d := range days {
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", i+1)
		ts := cmpNow.AddDate(0, 0, -d)
		line := func(kind string, at time.Time, body string) string {
			return fmt.Sprintf(`{"type":%q,"sessionId":%q,"cwd":"/tmp/cmp","version":"1.0.80","timestamp":%q,"uuid":"%s-%s","message":%s,"origin":{"kind":"human"}}`,
				kind, id, at.Format("2006-01-02T15:04:05.000Z"), id, kind, body)
		}
		body := line("user", ts, `{"role":"user","content":"Rename the helper."}`) + "\n" +
			line("assistant", ts.Add(10*time.Second), fmt.Sprintf(`{"id":"msg_%d","model":"claude-opus-5-5","role":"assistant","content":[{"type":"text","text":"Done."}],"usage":{"input_tokens":10,"cache_read_input_tokens":0,"cache_creation_input_tokens":4000,"output_tokens":100000}}`, i)) + "\n"
		p := filepath.Join(cl, id+".jsonl")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(p, ts.Add(time.Minute), ts.Add(time.Minute))
	}
	for k, v := range map[string]string{"LEEWAY_NOW": cmpNow.Format(time.RFC3339), "LEEWAY_CLAUDE_DIR": filepath.Join(tmp, "claude"),
		"LEEWAY_CODEX_DIR": filepath.Join(tmp, "codex"), "LEEWAY_CONFIG": filepath.Join(tmp, "config"), "LEEWAY_CACHE": filepath.Join(tmp, "cache"),
		"CLAUDE_CONFIG_DIR": filepath.Join(tmp, "claude-home")} {
		t.Setenv(k, v)
	}
	_ = os.MkdirAll(filepath.Join(tmp, "claude-home"), 0o700)
	return tmp
}

// claudeSettings writes Claude Code's settings.json and dates it.
func claudeSettings(t *testing.T, tmp, body string, changed time.Time) {
	p := filepath.Join(tmp, "claude-home", "settings.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(p, changed, changed)
}

// judgedAll is one judged record (no problems) for every session in the logs.
func judgedAll(t *testing.T, o *driftOpts, since time.Time) []py.Value {
	_, tb, err := leakReports(&o.logOpts, since, cmpNow, true, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	var out []py.Value
	for _, s := range drift.Select(tb, since.Add(-cmpNow.Sub(since)), cmpNow.Add(time.Minute), nil, "", false, 0) {
		v, err := py.Loads(fmt.Sprintf(`{"key":%q,"tool":%q,"tier":"claude:opus","rubric_version":"v2.1-b636272b+t2","usage":{"model":"claude-opus-5-5"},"derived":"as_intended","out":{"events":[]},"ts":1}`, s.Key, s.Tool))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

// runCompare is what `report` does for the comparison, on the made-up logs.
func runCompare(t *testing.T, sinceDays int, all []py.Value) *report.Compare {
	t.Helper()
	o := &driftOpts{}
	since := cmpNow.AddDate(0, 0, -sinceDays)
	x, tb, err := leakReports(&o.logOpts, since, cmpNow, true, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	keys, inWin := map[string]bool{}, map[string]int{}
	for _, s := range drift.Select(tb, since, cmpNow.Add(time.Minute), nil, "", false, 0) {
		keys[s.Key] = true
		inWin[s.Tool]++
	}
	var raw []py.Value
	for _, r := range all {
		if keys[r.Get("key").S] {
			raw = append(raw, r)
		}
	}
	recs, _ := report.Records(raw, 0)
	return compareWithPrevious(o, tb, x, &report.Model{Records: recs}, since, cmpNow, nil, inWin, keys, all)
}

func snapFiles(tmp, tool string) []string {
	f, _ := filepath.Glob(filepath.Join(tmp, "cache", "compare", tool+"-*.json"))
	return f
}

// A1: a window the Claude Code logs do not fully cover is never saved as a snapshot (it would later be a confident
// but half-empty baseline); a covered one is.
func TestCompareSnapshotOnlyForCoveredWindows(t *testing.T) {
	tmp := cmpEnv(t, 1, 3, 5, 7, 9, 11, 13, 15, 17, 19, 40, 42, 44) // the oldest left is 44 days old
	if c := runCompare(t, 60, nil); c == nil {
		t.Fatal("no comparison")
	}
	if f := snapFiles(tmp, "claude"); len(f) != 0 {
		t.Errorf("60-day window with logs from 44 days back saved: %v", f)
	}
	runCompare(t, 30, nil)
	if f := snapFiles(tmp, "claude"); len(f) != 1 {
		t.Errorf("covered 30-day window not saved: %v", f)
	}
	if f := snapFiles(tmp, "codex"); len(f) != 0 {
		t.Errorf("Codex had no spend, nothing to save: %v", f)
	}
}

// A11 and the coverage rule: logs that start inside the previous window cover it only when Claude Code's
// retention setting reaches back past it and was not changed since before it began.
func TestCompareCoverageRule(t *testing.T) {
	tmp := cmpEnv(t, 1, 3, 5, 7, 9, 11, 13, 15, 17, 19, 40, 42, 44)
	if c := runCompare(t, 30, nil); c.Cost != nil || c.CostWhy != "cmp_first" {
		t.Errorf("default 30 days, logs from 44 days back, no earlier report: %+v", c)
	}
	claudeSettings(t, tmp, `{"cleanupPeriodDays": 90}`, cmpNow.AddDate(0, 0, -1))
	if c := runCompare(t, 30, nil); c.Cost != nil || c.CostWhy != "cmp_why_cost_gone" {
		t.Errorf("retention raised yesterday must not count: %+v", c)
	}
	claudeSettings(t, tmp, `{"cleanupPeriodDays": 90}`, cmpNow.AddDate(0, 0, -100))
	// the logs cover the previous window, which has 3 sessions: compared from the logs, too little use
	if c := runCompare(t, 30, nil); c.Cost != nil || c.CostWhy != "cmp_why_cost_few" {
		t.Errorf("retention 90 days, set 100 days ago: the logs cover the previous window: %+v", c)
	}
}

// The folder hash: other folders or a fixed clock give other names.
func TestCompareSnapshotSources(t *testing.T) {
	t.Setenv("LEEWAY_NOW", "")
	a := snapSources(&logOpts{claudeDir: "/x/claude", codexDir: "/x/codex"})
	b := snapSources(&logOpts{claudeDir: "/y/claude", codexDir: "/x/codex"})
	if a["claude"] == b["claude"] || a["codex"] != b["codex"] {
		t.Errorf("per-tool folder hash: %v %v", a, b)
	}
	// without a fixed clock the name is the one 0.1.5 and 0.1.6 used, so their saved numbers still count
	abs, _ := filepath.Abs("/x/claude")
	sum := sha256.Sum256([]byte(abs + "\x00now="))
	if a["claude"] != hex.EncodeToString(sum[:])[:10] {
		t.Error("the name changed for real runs")
	}
	t.Setenv("LEEWAY_NOW", "2026-01-20T00:00:00Z")
	c := snapSources(&logOpts{claudeDir: "/x/claude", codexDir: "/x/codex"})
	if c["claude"] == a["claude"] {
		t.Error("a fixed clock must not share records with real runs")
	}
	t.Setenv("LEEWAY_NOW", "2026-03-01T00:00:00Z")
	if d := snapSources(&logOpts{claudeDir: "/x/claude", codexDir: "/x/codex"}); d["claude"] != c["claude"] {
		t.Error("runs at different made-up times share records (the example site and simulations need that)")
	}
}

// ClaudeCleanupDays: missing file, malformed, missing key, 0, negative, a value.
func TestClaudeCleanupDays(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmp)
	if d, ch := config.ClaudeCleanupDays(); d != 30 || !ch.IsZero() {
		t.Errorf("no file: %d %v", d, ch)
	}
	for body, want := range map[string]int{`{`: 30, `{}`: 30, `{"cleanupPeriodDays": "x"}`: 30, `{"cleanupPeriodDays": 0}`: 0, `{"cleanupPeriodDays": -5}`: 30, `{"cleanupPeriodDays": 90}`: 90} {
		_ = os.WriteFile(filepath.Join(tmp, "settings.json"), []byte(body), 0o600)
		if d, ch := config.ClaudeCleanupDays(); d != want || ch.IsZero() {
			t.Errorf("%s: %d (want %d), changed %v", body, d, want, ch)
		}
	}
}

// A9: windows under a day are not compared.
func TestCompareNotUnderADay(t *testing.T) {
	cmpEnv(t, 1)
	o := &driftOpts{}
	since := cmpNow.Add(-12 * time.Hour)
	x, tb, err := leakReports(&o.logOpts, since, cmpNow, true, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c := compareWithPrevious(o, tb, x, &report.Model{}, since, cmpNow, nil, nil, nil, nil); c != nil {
		t.Errorf("12-hour window compared: %+v", c)
	}
}

// A15: per-tool sums from the events already loaded equal those from reading the tables per tool.
func TestEventsForToolMatchesLoadEvents(t *testing.T) {
	fixtureEnv(t)
	o := &logOpts{}
	now := config.Now()
	since := now.AddDate(0, 0, -60)
	x, tb, err := leakReports(o, since, now, false, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"claude", "codex"} {
		a := leak.Compute(x.E.ForTool(tool), since, now, tool, "", time.Local)
		b := leak.Compute(leak.LoadEvents(tb, tool), since, now, tool, "", time.Local)
		if a.TotalUSD != b.TotalUSD || a.LeakUSD != b.LeakUSD || len(a.Sessions) != len(b.Sessions) || len(a.Habits) != len(b.Habits) || a.Info.PartialMiss != b.Info.PartialMiss || a.TotalUSD == 0 {
			t.Errorf("%s: %v/%v vs %v/%v", tool, a.TotalUSD, a.LeakUSD, b.TotalUSD, b.LeakUSD)
		}
	}
}

// `leak` compares with the previous window under the report's rule: not when Claude Code deleted part of it.
func TestLeakPreviousWindowNeedsCoverage(t *testing.T) {
	tmp := cmpEnv(t, 1, 3, 5, 7, 9, 40, 42, 44) // the oldest transcript left is 44 days old
	out, _, rc := run(t, "leak", "--json")
	if rc != 0 || strings.Contains(out, `"previous"`) {
		t.Errorf("logs from 44 days back, default retention: no previous window (rc %d)", rc)
	}
	if out, _, _ := run(t, "leak"); !strings.Contains(out, "Claude Code deleted its older logs, so there is no comparison with the previous window.") {
		t.Error("text output: the reason, in the report's words")
	}
	// some Codex logs (the fixtures; their dates do not matter here)
	if err := os.CopyFS(filepath.Join(tmp, "codex"), os.DirFS(repo("testdata", "logs", "codex"))); err != nil {
		t.Fatal(err)
	}
	if out, errs, _ := run(t, "leak", "--tool", "codex", "--json"); !strings.Contains(out, `"previous"`) {
		t.Log(errs)
		t.Error("--tool codex keeps its comparison: only Claude Code's logs are missing")
	}
	if out, _, _ := run(t, "leak", "--tool", "codex"); strings.Contains(out, "Claude Code deleted") {
		t.Error("--tool codex: no reason line")
	}
	claudeSettings(t, tmp, `{"cleanupPeriodDays": 90}`, cmpNow.AddDate(0, 0, -100))
	if out, _, _ := run(t, "leak", "--json"); !strings.Contains(out, `"previous"`) {
		t.Error("covered by the retention setting: the previous window is compared")
	}
}

// The cost baseline when the logs are gone: the most recent earlier report of the same length, at most 60 days old,
// whose window ends before this one starts. A 0.1.5/0.1.6 file (no "end") ends at start + length.
func TestCostRecordPick(t *testing.T) {
	cache := t.TempDir()
	src := map[string]string{"claude": "aaaa", "codex": "bbbb"}
	now := time.Date(2026, 10, 16, 20, 0, 0, 0, time.UTC)
	since := now.AddDate(0, 0, -30)
	a := func(total, leak float64, n int) report.Agg {
		return report.Agg{Total: total, Leak: leak, Sessions: n, Habits: map[string]float64{}}
	}
	nowAll := a(100, 8, 20)
	pick := func() (*report.CmpSide, string) {
		return costCompare(cache, src, false, true, nowAll, report.Agg{}, since, now, 720, map[string]any{})
	}
	if c, why := pick(); c != nil || why != "cmp_first" {
		t.Errorf("no report saved yet: %v %q", c, why)
	}
	d0 := now.AddDate(0, 0, -45)
	saveSnapshots(cache, src, d0.AddDate(0, 0, -30), d0, 720, map[string]report.Agg{"claude": a(80, 16, 15), "codex": a(20, 4, 5)}, map[string]bool{"claude": true, "codex": true})
	c, why := pick()
	if c == nil || why != "" || c.Base != report.BaseReport || !c.BaseAt.Equal(d0) || c.Prev != 0.2 || c.NPrv != 20 {
		t.Errorf("45-day-old report with both tools: %+v %q", c, why)
	}
	// a later report whose window overlaps this one is not a baseline; the earlier one still is
	saveSnapshots(cache, src, now.AddDate(0, 0, -40), now.AddDate(0, 0, -10), 720, map[string]report.Agg{"claude": a(50, 1, 12)}, map[string]bool{"claude": true})
	if c, _ := pick(); c == nil || !c.BaseAt.Equal(d0) {
		t.Errorf("overlapping report chosen: %+v", c)
	}
	// 61 days: too old
	cache = t.TempDir()
	old := now.AddDate(0, 0, -61)
	saveSnapshots(cache, src, old.AddDate(0, 0, -30), old, 720, map[string]report.Agg{"claude": a(80, 16, 15)}, map[string]bool{"claude": true})
	if c, why := pick(); c != nil || why != "cmp_why_cost_old" {
		t.Errorf("61 days: %v %q", c, why)
	}
	// another window length: no baseline
	cache = t.TempDir()
	saveSnapshots(cache, src, d0.AddDate(0, 0, -14), d0, 336, map[string]report.Agg{"claude": a(80, 16, 15)}, map[string]bool{"claude": true})
	if c, why := pick(); c != nil || why != "cmp_why_cost_gone" {
		t.Errorf("14-day report: %v %q", c, why)
	}
	// a file written by 0.1.6 (no end, no share): ends at start + length
	cache = t.TempDir()
	_ = os.MkdirAll(filepath.Join(cache, "compare"), 0o700)
	st := d0.AddDate(0, 0, -30)
	b, _ := json.Marshal(map[string]any{"start": st.Format(time.RFC3339), "hours": 720, "total_usd": 80, "leak_usd": 16, "max_session_leak_usd": 2, "sessions": 15, "habits_usd": map[string]float64{}})
	_ = os.WriteFile(filepath.Join(cache, "compare", snapName("claude", "aaaa", st, 720)), b, 0o600)
	if c, _ := pick(); c == nil || !c.BaseAt.Equal(d0) {
		t.Errorf("0.1.6 file: %+v", c)
	}
	// numbers that cannot be right are ignored
	_ = os.WriteFile(filepath.Join(cache, "compare", snapName("claude", "aaaa", st, 720)), []byte(`{"start":"`+st.Format(time.RFC3339)+`","hours":720,"total_usd":10,"leak_usd":12,"sessions":15}`), 0o600)
	if c, _ := pick(); c != nil {
		t.Error("leak above total accepted")
	}
}
