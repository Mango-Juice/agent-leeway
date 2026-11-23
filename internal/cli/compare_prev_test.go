package cli

import (
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
	if c := runCompare(t, 30, nil); c.Cost != nil || c.CostWhy != "cmp_why_cost_gone" {
		t.Errorf("default 30 days, logs from 44 days back, no snapshot: %+v", c)
	}
	claudeSettings(t, tmp, `{"cleanupPeriodDays": 90}`, cmpNow.AddDate(0, 0, -1))
	if c := runCompare(t, 30, nil); c.Cost != nil || c.CostWhy != "cmp_why_cost_gone" {
		t.Errorf("retention raised yesterday must not count: %+v", c)
	}
	claudeSettings(t, tmp, `{"cleanupPeriodDays": 90}`, cmpNow.AddDate(0, 0, -100))
	if c := runCompare(t, 30, nil); c.Cost == nil || c.CostWhy != "" {
		t.Errorf("retention 90 days, set 100 days ago: the logs cover the previous window: %+v", c)
	}
}

// A2: the in-depth side leaves out Claude Code when its logs do not cover the previous window (the survivors would
// be a biased subset), and says why.
func TestCompareInDepthLeavesOutDeletedClaude(t *testing.T) {
	days := []int{40, 41, 42, 43, 44, 45, 46, 47, 48}
	for i := 1; i <= 20; i++ {
		days = append(days, i)
	}
	tmp := cmpEnv(t, days...)
	o := &driftOpts{}
	all := judgedAll(t, o, cmpNow.AddDate(0, 0, -30))
	c := runCompare(t, 30, all)
	if c.Drift != nil || c.DriftWhy != "cmp_why_drift_gone" {
		t.Errorf("9 surviving previous sessions, logs not covering: %+v %q", c.Drift, c.DriftWhy)
	}
	claudeSettings(t, tmp, `{"cleanupPeriodDays": 365}`, cmpNow.AddDate(0, 0, -400))
	if c := runCompare(t, 30, all); c.Drift == nil || c.DriftWhy != "" {
		t.Errorf("covered by the retention setting: compared: %+v %q", c.Drift, c.DriftWhy)
	}
}

// A14: a session with turns on both sides of the boundary counts once, in the period of its last turn.
func TestCompareBoundarySessionCountsOnce(t *testing.T) {
	tmp := cmpEnv(t)
	cl := filepath.Join(tmp, "claude", "-tmp-cmp")
	id := "00000000-0000-4000-8000-000000000999"
	at := func(d int) string { return cmpNow.AddDate(0, 0, -d).Format("2006-01-02T15:04:05.000Z") }
	body := fmt.Sprintf(`{"type":"user","sessionId":%q,"cwd":"/tmp/cmp","timestamp":%q,"uuid":"u1","message":{"role":"user","content":"Start."},"origin":{"kind":"human"}}`+"\n"+
		`{"type":"user","sessionId":%q,"cwd":"/tmp/cmp","timestamp":%q,"uuid":"u2","message":{"role":"user","content":"Go on."},"origin":{"kind":"human"}}`+"\n", id, at(35), id, at(25))
	_ = os.WriteFile(filepath.Join(cl, id+".jsonl"), []byte(body), 0o600)
	claudeSettings(t, tmp, `{"cleanupPeriodDays": 365}`, cmpNow.AddDate(0, 0, -400)) // the logs cover both windows
	o := &driftOpts{}
	since := cmpNow.AddDate(0, 0, -30)
	_, tb, err := leakReports(&o.logOpts, since, cmpNow, true, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	cur := map[string]bool{}
	for _, s := range drift.Select(tb, since, cmpNow.Add(time.Minute), nil, "", false, 0) {
		cur[s.Key] = true
	}
	prev := drift.Select(tb, since.AddDate(0, 0, -30), since, nil, "", false, 0)
	if len(cur) != 1 || len(prev) != 1 || !cur[prev[0].Key] {
		t.Fatalf("fixture: the session should touch both windows (%d, %d)", len(cur), len(prev))
	}
	keys, n := prevSessions(tb, since.AddDate(0, 0, -30), since, nil, o, cur, true)
	if len(keys) != 0 || n["claude"] != 0 {
		t.Errorf("the boundary session was counted in the previous period too: %v", keys)
	}
	if keys, _ := prevSessions(tb, since.AddDate(0, 0, -30), since, nil, o, map[string]bool{}, true); len(keys) != 1 {
		t.Error("fixture: without this window's sessions the previous period has it")
	}
}

// The cost branches: logs, snapshot, Codex alone (named), nothing; and the right reason for each.
func TestCompareCostBranches(t *testing.T) {
	a := func(total, leak float64, n int) report.Agg {
		return report.Agg{Total: total, Leak: leak, Sessions: n, Habits: map[string]float64{}}
	}
	none := func() (report.Agg, bool) { return report.Agg{}, false }
	if c, why := costCompare(true, a(100, 8, 10), a(100, 15, 10), a(40, 2, 4), a(40, 6, 4), none); c == nil || c.Tool != "" || why != "" {
		t.Errorf("logs cover: %+v %q", c, why)
	}
	if c, why := costCompare(true, a(100, 8, 10), a(3, 1, 10), a(0, 0, 0), a(0, 0, 0), none); c != nil || why != "cmp_why_cost_few" {
		t.Errorf("too little use: %+v %q", c, why)
	}
	snap := func() (report.Agg, bool) { return a(60, 9, 6), true }
	if c, why := costCompare(false, a(100, 8, 10), report.Agg{}, a(40, 2, 4), a(40, 6, 4), snap); c == nil || c.Tool != "" || why != "" || c.Prev != 15.0/100 {
		t.Errorf("snapshot plus Codex logs: %+v %q", c, why)
	}
	if c, why := costCompare(false, a(100, 8, 10), report.Agg{}, a(40, 2, 4), a(40, 6, 4), none); c == nil || c.Tool != "codex" || why != "cmp_why_cost_deleted" {
		t.Errorf("Codex alone: %+v %q", c, why)
	}
	// deleted Claude Code logs and too little Codex use: the reason is the deletion, not "too little use"
	if c, why := costCompare(false, a(100, 8, 10), report.Agg{}, a(4, 1, 2), a(4, 1, 2), none); c != nil || why != "cmp_why_cost_gone" {
		t.Errorf("Codex too small: %+v %q", c, why)
	}
}

// A8: snapshots are kept apart by log folders, and a read checks the window, the tolerance, the values and that
// it is not this window itself.
func TestCompareSnapshotRead(t *testing.T) {
	cache := t.TempDir()
	start := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	cur := start.AddDate(0, 0, 30)
	ok := map[string]bool{"claude": true}
	good := report.Agg{Total: 80, Leak: 12, MaxLeak: 4, Sessions: 9, Habits: map[string]float64{"long_break": 10}}
	saveSnapshots(cache, map[string]string{"claude": "aaaa"}, start, 720, map[string]report.Agg{"claude": good}, ok)
	if a, found := readSnapshot(cache, "claude", "aaaa", start.AddDate(0, 0, 2), cur, 720); !found || a.Total != 80 || a.Habits["long_break"] != 10 || a.MaxLeak != 4 {
		t.Errorf("within 2 days: %+v %v", a, found)
	}
	if _, found := readSnapshot(cache, "claude", "aaaa", start.AddDate(0, 0, 3), cur, 720); found {
		t.Error("3 days off must not match a 30-day window")
	}
	if _, found := readSnapshot(cache, "claude", "bbbb", start, cur, 720); found {
		t.Error("other log folders must not match")
	}
	if _, found := readSnapshot(cache, "claude", "aaaa", start, cur, 168); found {
		t.Error("another window length must not match")
	}
	if _, found := readSnapshot(cache, "claude", "aaaa", start, start, 720); found {
		t.Error("a snapshot starting at this window's start is this window itself")
	}
	// a 7-day window: at most 10% (16.8 hours)
	saveSnapshots(cache, map[string]string{"claude": "aaaa"}, start, 168, map[string]report.Agg{"claude": good}, ok)
	if _, found := readSnapshot(cache, "claude", "aaaa", start.Add(20*time.Hour), cur, 168); found {
		t.Error("20 hours off a 7-day window is beyond 10%")
	}
	// values that cannot be right
	for _, bad := range []report.Agg{{Total: 10, Leak: 12}, {Total: -1}, {Total: 10, Leak: 2, Sessions: -3}, {Total: 10, Leak: 2, Habits: map[string]float64{"x": 5}}} {
		c := t.TempDir()
		saveSnapshots(c, map[string]string{"claude": "aaaa"}, start, 720, map[string]report.Agg{"claude": bad}, ok)
		if bad.Total <= 0 {
			continue // never written
		}
		if _, found := readSnapshot(c, "claude", "aaaa", start, cur, 720); found {
			t.Errorf("impossible values accepted: %+v", bad)
		}
	}
	// not written when the window is not covered
	c2 := t.TempDir()
	saveSnapshots(c2, map[string]string{"claude": "aaaa"}, start, 720, map[string]report.Agg{"claude": good}, map[string]bool{})
	if f, _ := filepath.Glob(filepath.Join(c2, "compare", "*")); len(f) != 0 {
		t.Errorf("uncovered window saved: %v", f)
	}
	files, _ := filepath.Glob(filepath.Join(cache, "compare", "*.json"))
	for _, f := range files {
		b, _ := os.ReadFile(f)
		if strings.Contains(string(b), "quote") || strings.Contains(string(b), "/") {
			t.Errorf("snapshot holds more than numbers: %s", b)
		}
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
	t.Setenv("LEEWAY_NOW", "2026-01-20T00:00:00Z")
	if c := snapSources(&logOpts{claudeDir: "/x/claude", codexDir: "/x/codex"}); c["claude"] == a["claude"] {
		t.Error("a fixed clock must not share snapshots with real runs")
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
	if out, _, _ := run(t, "leak"); !strings.Contains(out, "Claude Code deleted its older logs, so there is no cost comparison.") {
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
