package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/pipeline"
	"github.com/Mango-Juice/agent-leeway/internal/report"
)

func hasCheckCard(h string) bool {
	for _, p := range pills(h) {
		if strings.Contains(p, " check") {
			return true
		}
	}
	return false
}

// 1. A report narrowed to one project after two all-project checks compares nothing: its sessions are not what the
// checks covered, and the reason says so.
func TestReportScopeMustMatchItsCheck(t *testing.T) {
	w := newWorld(t)
	claudeSettings(t, w.tmp, `{"cleanupPeriodDays": 90}`, day0.AddDate(0, 0, -200))
	w.sessions(day0, 30, 20, "app")
	w.sessions(day0, 30, 20, "api")
	w.check(day0, every(2), visitOpts{})
	d1 := day0.AddDate(0, 0, 30)
	w.sessions(d1, 30, 20, "app")
	w.sessions(d1, 30, 20, "api")
	w.check(d1, every(5), visitOpts{})
	h, _, _ := w.report(d1, "--project", "app")
	if hasCheckCard(h) || !strings.Contains(h, "This report covers other sessions than its check did") {
		t.Errorf("report --project app after all-project checks: cards %v", pills(h))
	}
}

// 2. The current side counts one judge, one result per session: a re-check under a newer exact model (another tier)
// does not double the count; sessions checked with two model families are not compared.
func TestCurrentSideOneJudge(t *testing.T) {
	w := newWorld(t)
	claudeSettings(t, w.tmp, `{"cleanupPeriodDays": 90}`, day0.AddDate(0, 0, -200))
	w.sessions(day0, 30, 24, "app")
	w.check(day0, every(2), visitOpts{})
	d1 := day0.AddDate(0, 0, 30)
	w.sessions(d1, 30, 24, "app")
	w.check(d1, every(6), visitOpts{})
	w.check(d1.Add(time.Hour), every(6), visitOpts{recheck: true, tier: "claude:opus-5-6", model: "claude-opus-5-6"})
	h, _, _ := w.report(d1.Add(2 * time.Hour))
	if !strings.Contains(h, "24 sessions then, 24 now") || strings.Contains(h, "48 now") {
		t.Errorf("re-check under a newer exact model counted twice")
	}
	w2 := newWorld(t)
	claudeSettings(t, w2.tmp, `{"cleanupPeriodDays": 90}`, day0.AddDate(0, 0, -200))
	w2.sessions(day0, 30, 24, "app")
	w2.check(day0, every(2), visitOpts{})
	w2.sessions(d1, 30, 24, "app")
	w2.check(d1, every(6), visitOpts{})
	w2.check(d1.Add(time.Hour), every(1), visitOpts{recheck: true, tier: "claude:claude-sonnet-5-5", model: "claude-sonnet-5-5"})
	h, _, _ = w2.report(d1.Add(2 * time.Hour))
	if hasCheckCard(h) || !strings.Contains(h, "checked with more than one judging model") {
		t.Errorf("two model families in this period: cards %v", pills(h))
	}
}

// 3. Each side has its own reason values: the cost line names the report's date, not the check's.
func TestReasonDatesPerSide(t *testing.T) {
	w := newWorld(t)
	w.sessions(day0, 30, 24, "app")
	w.report(day0) // the cost record: 1 Sep
	d10 := day0.AddDate(0, 0, 10)
	w.sessions(d10, 10, 10, "app")
	w.check(d10, every(2), visitOpts{}) // the check record: 11 Sep
	d71 := day0.AddDate(0, 0, 71)
	w.sessions(d71, 30, 24, "app")
	w.cleanup(d71, 30)
	w.check(d71, every(6), visitOpts{})
	h, _, _ := w.report(d71)
	if !strings.Contains(h, "the last report (1 Sep) is more than 60 days old") || !strings.Contains(h, "The last check (11 Sep) is more than 60 days old") {
		t.Errorf("reason dates mixed between the sides: %s", regexp.MustCompile(`<p class="nb">[^<]*</p>`).FindAllString(h, -1))
	}
}

// 4. The check's own window: a 7-day check under a 30-day report is not compared, and the reason says why.
func TestCheckWindowMustMatchReport(t *testing.T) {
	w := newWorld(t)
	claudeSettings(t, w.tmp, `{"cleanupPeriodDays": 90}`, day0.AddDate(0, 0, -200))
	w.sessions(day0, 30, 24, "app")
	w.check(day0, every(2), visitOpts{})
	d1 := day0.AddDate(0, 0, 30)
	w.sessions(d1, 30, 40, "app")
	w.check(d1, every(6), visitOpts{days: 7})
	h, _, _ := w.report(d1)
	if hasCheckCard(h) || !strings.Contains(h, "but its sessions were checked for 7 days") {
		t.Errorf("7-day check, 30-day report: cards %v", pills(h))
	}
}

// 5. Records are rebuilt from an earlier version's results only when nothing must be guessed: not for a check with a
// smaller --max-sessions or --since, nor for results whose sessions are not in these log folders (another folder, or
// a made-up-clock run in the real cache).
func TestDeriveOnlyExactDefaults(t *testing.T) {
	for name, v := range map[string]visitOpts{
		"max-sessions 12": {noRecord: true, most: 12},
		"since 7d":        {noRecord: true, days: 7},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			claudeSettings(t, w.tmp, `{"cleanupPeriodDays": 90}`, day0.AddDate(0, 0, -200))
			w.sessions(day0, 30, 24, "app")
			w.check(day0, every(2), v)
			d1 := day0.AddDate(0, 0, 31)
			w.sessions(d1, 30, 24, "app")
			w.check(d1, every(6), visitOpts{})
			h, _, _ := w.report(d1)
			for _, r := range loadChecks(w.cache) {
				if r.Derived {
					t.Errorf("a %s check became a default baseline", name)
				}
			}
			if hasCheckCard(h) {
				t.Errorf("compared with a guessed baseline: %v", pills(h))
			}
		})
	}
	t.Run("other folder", func(t *testing.T) {
		w := newWorld(t)
		claudeSettings(t, w.tmp, `{"cleanupPeriodDays": 90}`, day0.AddDate(0, 0, -200))
		w.sessions(day0, 30, 24, "app")
		w.check(day0, every(2), visitOpts{noRecord: true})
		// the same cache, now read with other log folders: those results' sessions are not here
		other := filepath.Join(w.tmp, "claude-other")
		_ = os.Rename(w.claudeDir, filepath.Join(w.tmp, "claude-moved"))
		w.claudeDir = other
		t.Setenv("LEEWAY_CLAUDE_DIR", other)
		d1 := day0.AddDate(0, 0, 31)
		w.sessions(d1, 30, 24, "app")
		w.check(d1, every(6), visitOpts{})
		w.report(d1)
		for _, r := range loadChecks(w.cache) {
			if r.Derived {
				t.Error("results of other log folders became a baseline")
			}
		}
	})
}

// 5 (positive): an exact default 0.1.6 check is still rebuilt.
func TestDeriveExactDefault(t *testing.T) {
	w := newWorld(t)
	claudeSettings(t, w.tmp, `{"cleanupPeriodDays": 90}`, day0.AddDate(0, 0, -200))
	w.sessions(day0, 30, 24, "app")
	w.check(day0, every(2), visitOpts{noRecord: true})
	d1 := day0.AddDate(0, 0, 31)
	w.sessions(d1, 30, 24, "app")
	w.check(d1, every(6), visitOpts{})
	h, _, _ := w.report(d1)
	if !hasCheckCard(h) {
		t.Errorf("exact default check not rebuilt: %v", pills(h))
	} // the rebuilt record carries the checked sessions' dates (their last turns, as a live record does), never year 1
	raw, _ := os.ReadFile(checksPath(w.cache))
	if strings.Contains(string(raw), "0001-01-01") {
		t.Errorf("a record with an empty date: %s", raw)
	}
	for _, r := range loadChecks(w.cache) {
		if r.Derived && (r.First.IsZero() || r.Last.IsZero() || r.Last.Before(r.First) || r.Last.After(day0) || r.First.Before(day0.AddDate(0, 0, -30))) {
			t.Errorf("rebuilt record dates %v – %v", r.First, r.Last)
		}
	}
}

// 6. Automations are their own difference, not "one project".
func TestScopeReasonAutomation(t *testing.T) {
	w := newWorld(t)
	claudeSettings(t, w.tmp, `{"cleanupPeriodDays": 90}`, day0.AddDate(0, 0, -200))
	w.sessions(day0, 30, 24, "app")
	w.check(day0, every(2), visitOpts{auto: true})
	d1 := day0.AddDate(0, 0, 30)
	w.sessions(d1, 30, 24, "app")
	w.check(d1, every(6), visitOpts{})
	h, _, _ := w.report(d1)
	if !strings.Contains(h, "The last check included automations; this one does not.") || strings.Contains(h, "one project only") {
		t.Error("automation scope reason")
	}
}

// 7. "Next report compares with this one." only when something is stored: not when no record exists (an earlier
// version's cache that could not be rebuilt), not on the cost side when this report saves nothing.
func TestFirstVisitLineOnlyWhenTrue(t *testing.T) {
	// the cost side saves this report's numbers (its logs cover the window): its line is true; the in-depth side has
	// no record (an earlier version's check that cannot be rebuilt): no line there
	w := newWorld(t)
	w.sessions(day0, 30, 24, "app")
	w.check(day0, every(2), visitOpts{noRecord: true, most: 12})
	h, _, _ := w.report(day0)
	if n := strings.Count(h, "Next report compares with this one."); n != 1 {
		t.Errorf("%d first-visit lines, want 1 (cost only)", n)
	}
	// Claude Code keeps 7 days and the logs do not reach the window's start: nothing is saved on either side
	w = newWorld(t)
	claudeSettings(t, w.tmp, `{"cleanupPeriodDays": 7}`, day0.AddDate(0, 0, -200))
	w.sessions(day0, 30, 24, "app")
	w.check(day0, every(2), visitOpts{noRecord: true, most: 12})
	if h, _, _ := w.report(day0); strings.Contains(h, "Next report compares with this one.") {
		t.Error("first-visit line although nothing is saved")
	}
}

// A damaged records file: an unfinished last line, a broken line, an over-long line; the valid lines still count,
// the next record starts on its own line, and nothing fails.
func TestChecksFileDamage(t *testing.T) {
	cache := t.TempDir()
	_ = os.MkdirAll(filepath.Join(cache, "compare"), 0o700)
	good := `{"tool":"claude","checked_at":"2026-09-01T00:00:00Z","checked":20,"serious":5}`
	body := good + "\n" + "{broken\n" + `{"tool":"claude","pad":"` + strings.Repeat("x", 5<<20) + `"}` + "\n" + good + "\n" + `{"tool":"cla`
	_ = os.WriteFile(checksPath(cache), []byte(body), 0o600)
	if n := len(loadChecks(cache)); n != 2 {
		t.Fatalf("%d records read, want 2", n)
	}
	r := loadChecks(cache)[0]
	r.CheckedAt = time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	appendChecks(cache, r.CheckedAt, r)
	if n := len(loadChecks(cache)); n != 3 {
		t.Errorf("%d records after appending to an unfinished line, want 3", n)
	}
	// records older than 400 days go when one is written
	appendChecks(cache, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, 400).Add(12*time.Hour), r)
	if n := len(loadChecks(cache)); n != 2 {
		t.Errorf("%d records after pruning, want 2 (the 2 Sep one and the new one)", n)
	}
}

// 10. A run that judged no Codex session (all Codex calls failed) writes no Codex record.
func TestRecordOnlyForToolsJudgedNow(t *testing.T) {
	w := newWorld(t)
	w.sessions(day0, 30, 24, "app")
	w.t.Setenv("LEEWAY_NOW", day0.Format(time.RFC3339))
	w.check(day0, every(2), visitOpts{noRecord: true})
	o := &driftOpts{maxSessions: 50}
	_, tb, err := leakReports(&o.logOpts, day0.AddDate(0, 0, -30), day0, false, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	// results were written at day0 (ts); a run starting after them judged nothing
	saveCheckRecords(o, tb, day0.AddDate(0, 0, -30), day0, map[string]string{"claude": "claude:opus"}, w.rb, float64(day0.Unix())+1e6)
	if n := len(loadChecks(w.cache)); n != 0 {
		t.Errorf("%d records for a run that judged nothing", n)
	}
}

// 9. Two tools: compared as one when both baselines come from one check run; otherwise the one that can be compared
// is named (no card line) and the note says why the other is not.
func TestTwoToolsCombination(t *testing.T) {
	cache := t.TempDir()
	src := map[string]string{"claude": "s1", "codex": "s2"}
	d1 := day0.AddDate(0, 0, 30)
	tier := map[string]string{"claude": "claude:opus", "codex": "codex:gpt-6.2-astra"}
	model := map[string]string{"claude": "claude-opus-5-5", "codex": "gpt-6.2-astra"}
	keys := func(tool, p string, n int) []string {
		var out []string
		for i := 0; i < n; i++ {
			out = append(out, fmt.Sprintf("%s:%s%02d", tool, p, i))
		}
		return out
	}
	rec := func(tool string, at time.Time, ks []string, serious int) report.CheckAgg {
		return report.CheckAgg{Tool: tool, CheckedAt: at, WindowStart: at.AddDate(0, 0, -30), WindowHours: 720, Checked: len(ks), Serious: serious,
			Kinds: map[string]int{}, Tier: tier[tool], Model: model[tool], Family: report.ModelFamily(tier[tool], model[tool]), Rubric: "r1",
			Rule: 50, Scope: "all", Keys: ks, Source: src[tool]}
	}
	var cur []*report.Record
	for _, tool := range []string{"claude", "codex"} {
		for i, k := range keys(tool, "now", 12) {
			v := "as_intended"
			if i < 2 {
				v = "agent_drift"
			}
			cur = append(cur, &report.Record{Key: k, Tool: tool, Tier: tier[tool], Model: model[tool], RubricVersion: "r1", Verdict: v})
		}
	}
	run := func(codexBase time.Time) *report.CmpSide {
		c := t.TempDir()
		appendChecks(c, d1, rec("claude", day0, keys("claude", "then", 12), 6), rec("codex", codexBase, keys("codex", "then", 12), 6),
			rec("claude", d1, keys("claude", "now", 12), 2), rec("codex", d1, keys("codex", "now", 12), 2))
		side, _ := driftCompare(c, src, &pipeline.Tables{}, cur, nil, d1, 720, map[string]int{}, "all", map[string]any{})
		return side
	}
	if s := run(day0.Add(30 * time.Minute)); s == nil || s.Tool != "" || s.NNow != 24 || s.NPrv != 24 || !s.Pill {
		t.Errorf("one check run: %+v", s)
	}
	if s := run(day0.AddDate(0, 0, 5)); s == nil || s.Tool != "claude" || s.Pill || s.Other != "codex" || s.OtherWhy != "cmp_why_other_day" {
		t.Errorf("different checks: %+v", s)
	}
	_ = cache
}
