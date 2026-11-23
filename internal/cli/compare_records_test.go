package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/drift"
	"github.com/Mango-Juice/agent-leeway/internal/report"
)

// A small world for the record-based comparison: made-up Claude
// Code sessions, Claude Code's own cleanup by modification time, results written as the judge would leave them, the
// check records the real code writes after a check, and the real `report` command.
type world struct {
	t                     *testing.T
	tmp, claudeDir, cache string
	n                     int
	rb                    *drift.Rubric
}

var day0 = time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC)

func newWorld(t *testing.T) *world {
	t.Helper()
	tmp := t.TempDir()
	old := time.Local
	time.Local = time.UTC // dates on the report are local: keep them as written here
	t.Cleanup(func() { time.Local = old })
	w := &world{t: t, tmp: tmp, claudeDir: filepath.Join(tmp, "claude"), cache: filepath.Join(tmp, "cache")}
	_ = os.MkdirAll(filepath.Join(tmp, "codex"), 0o700)
	_ = os.MkdirAll(filepath.Join(tmp, "claude-home"), 0o700)
	for k, v := range map[string]string{"LEEWAY_CLAUDE_DIR": w.claudeDir, "LEEWAY_CODEX_DIR": filepath.Join(tmp, "codex"), "LEEWAY_CONFIG": filepath.Join(tmp, "config"),
		"LEEWAY_CACHE": w.cache, "CLAUDE_CONFIG_DIR": filepath.Join(tmp, "claude-home")} {
		t.Setenv(k, v)
	}
	rb, err := drift.LoadRubric("")
	if err != nil {
		t.Fatal(err)
	}
	w.rb = rb
	return w
}

// sessions writes n sessions spread over the days before end (one every span/n), in project proj.
func (w *world) sessions(end time.Time, days, n int, proj string) {
	for i := 0; i < n; i++ {
		at := end.Add(-time.Duration(float64(days*24)*float64(i+1)/float64(n+1)*float64(time.Hour)) - 2*time.Hour)
		w.n++
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", w.n)
		dir := filepath.Join(w.claudeDir, "-tmp-"+proj)
		_ = os.MkdirAll(dir, 0o700)
		line := func(kind string, ts time.Time, body string) string {
			return fmt.Sprintf(`{"type":%q,"sessionId":%q,"cwd":"/tmp/%s","version":"1.0.80","timestamp":%q,"uuid":"%s-%s","message":%s,"origin":{"kind":"human"}}`,
				kind, id, proj, ts.Format("2006-01-02T15:04:05.000Z"), id, kind, body)
		}
		body := line("user", at, fmt.Sprintf(`{"role":"user","content":"Task %d: update the module."}`, w.n)) + "\n" +
			line("assistant", at.Add(10*time.Second), fmt.Sprintf(`{"id":"m%d","model":"claude-opus-5-5","role":"assistant","content":[{"type":"text","text":"Done."}],"usage":{"input_tokens":10,"cache_read_input_tokens":0,"cache_creation_input_tokens":4000,"output_tokens":%d}}`, w.n, 20000+w.n*37)) + "\n" +
			// resumed after a long break: the prompt cache has expired and is written again (a leak, so the cost part shows)
			line("user", at.Add(100*time.Minute), `{"role":"user","content":"Continue please."}`) + "\n" +
			line("assistant", at.Add(101*time.Minute), fmt.Sprintf(`{"id":"r%d","model":"claude-opus-5-5","role":"assistant","content":[{"type":"text","text":"Done."}],"usage":{"input_tokens":10,"cache_read_input_tokens":0,"cache_creation_input_tokens":%d,"output_tokens":500}}`, w.n, 30000+w.n*101)) + "\n"
		p := filepath.Join(dir, id+".jsonl")
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			w.t.Fatal(err)
		}
		_ = os.Chtimes(p, at.Add(102*time.Minute), at.Add(102*time.Minute))
	}
}

// cleanup is Claude Code deleting transcripts older than days at time at.
func (w *world) cleanup(at time.Time, days int) {
	_ = filepath.Walk(w.claudeDir, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() && fi.ModTime().Before(at.AddDate(0, 0, -days)) {
			os.Remove(p)
		}
		return nil
	})
}

type visitOpts struct {
	most     int    // --max-sessions (default 50)
	project  string // --project
	model    string // the exact model the judge reports
	tier     string
	noRecord bool // an earlier version: results only, no check record
	days     int  // --since, in days (default 30)
	recheck  bool // judge sessions already checked again (another judge)
	auto     bool // --include-auto
}

// check is an in-depth check at time at: the most recent sessions not checked yet in the 30-day window get a result
// (serious(i) for the i-th, most recent first), then the check record is written as runDrift writes it.
func (w *world) check(at time.Time, serious func(i int) bool, v visitOpts) []string {
	w.t.Helper()
	w.t.Setenv("LEEWAY_NOW", at.Format(time.RFC3339))
	if v.most == 0 {
		v.most = 50
	}
	if v.model == "" {
		v.model = "claude-opus-5-5"
	}
	if v.tier == "" {
		v.tier = "claude:opus"
	}
	if v.days == 0 {
		v.days = 30
	}
	o := &driftOpts{maxSessions: v.most, project: v.project, includeAuto: v.auto}
	since := at.AddDate(0, 0, -v.days)
	_, tb, err := leakReports(&o.logOpts, since, at, false, io.Discard)
	if err != nil {
		w.t.Fatal(err)
	}
	done := map[string]bool{}
	for _, r := range drift.LoadResults(w.cache) {
		done[r.Get("key").S] = true
	}
	sel := drift.Select(tb, since, at.Add(time.Minute), []string{"claude"}, v.project, v.auto, 0) // most recent first
	_ = os.MkdirAll(filepath.Join(w.cache, "drift"), 0o700)
	f, _ := os.OpenFile(drift.ResultsPath(w.cache), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	var keys []string
	for _, s := range sel {
		if (done[s.Key] && !v.recheck) || len(keys) == v.most {
			continue
		}
		i := len(keys)
		verdict, events := "as_intended", []any{}
		if serious(i) {
			verdict = "agent_drift"
			events = []any{map[string]any{"loc": "U1", "rule": "R2a", "cause": "agent_error", "size": "major", "cause_side": "agent_side", "resolution": "resolved",
				"evidence": []any{map[string]any{"loc": "U1", "quote": "update the module"}}, "summary": "made up"}}
		}
		rec := map[string]any{"run_id": s.Key + "|" + v.tier + "|" + w.rb.Version + "|x", "key": s.Key, "tool": "claude", "project": s.Project, "tier": v.tier,
			"model": v.model, "usage": map[string]any{"model": v.model}, "rubric_version": w.rb.Version, "derived": verdict, "error": nil,
			"out": map[string]any{"events": events, "verdict": verdict}, "ts": float64(at.Unix()) + float64(i)}
		b, _ := json.Marshal(rec)
		f.Write(append(b, '\n'))
		keys = append(keys, s.Key)
	}
	f.Close()
	if !v.noRecord {
		saveCheckRecords(o, tb, since, at, map[string]string{"claude": v.tier}, w.rb, 0)
	}
	return keys
}

// report runs `report` at time at and returns the HTML, the Markdown and the terminal summary.
func (w *world) report(at time.Time, args ...string) (string, string, string) {
	w.t.Helper()
	w.t.Setenv("LEEWAY_NOW", at.Format(time.RFC3339))
	dir := filepath.Join(w.tmp, "out-"+at.Format("0102150405"))
	out, errs, rc := run(w.t, append([]string{"report", "--tool", "claude", "--lang", "en", "--out", dir}, args...)...)
	if rc != 0 {
		w.t.Fatalf("report rc %d\n%s", rc, errs)
	}
	read := func(ext string) string {
		m, _ := filepath.Glob(filepath.Join(dir, "*."+ext))
		sort.Strings(m)
		b, _ := os.ReadFile(m[len(m)-1])
		return string(b)
	}
	return read("html"), read("md"), out
}

func every(k int) func(int) bool { return func(i int) bool { return i%k == 0 } }

var pillRE = regexp.MustCompile(`<p class="chg (\w+)">(.*?)</p>`)

func pills(h string) []string {
	var out []string
	for _, m := range pillRE.FindAllStringSubmatch(h, -1) {
		out = append(out, m[1]+": "+strings.Join(strings.Fields(regexp.MustCompile(`<[^>]+>`).ReplaceAllString(m[2], " ")), " "))
	}
	return out
}

// (a) A Claude Code user with the default retention comes back on day 45: the logs of the previous 30 days are gone.
// The cost card compares with the day-0 report, the in-depth card with the day-0 check, which shares no session.
func TestReturnDay45DeletedLogs(t *testing.T) {
	w := newWorld(t)
	w.sessions(day0, 30, 24, "app")
	k0 := w.check(day0, every(2), visitOpts{})
	w.report(day0)
	d1 := day0.AddDate(0, 0, 45)
	w.sessions(d1, 30, 24, "app")
	w.cleanup(d1, 30)
	k1 := w.check(d1, every(6), visitOpts{})
	h, md, term := w.report(d1)
	for _, k := range k1 {
		for _, j := range k0 {
			if k == j {
				t.Fatal("fixture: the two checks share a session")
			}
		}
	}
	p := strings.Join(pills(h), " | ")
	if !strings.Contains(p, "vs 1 Sep check") || !strings.Contains(p, "50% → 17%") || !strings.Contains(p, "vs 1 Sep report") {
		t.Errorf("cards: %s", p)
	}
	if !strings.Contains(h, "Compared with the 1 Sep check (2 Aug – 1 Sep): 24 sessions then, 24 now") {
		t.Error("in-depth detail heading with the baseline's dates and each side's n")
	}
	agree(t, h, md, term)
}

// agree: every card line's rates and baseline label are in the Markdown table and the terminal summary.
func agree(t *testing.T, h, md, term string) {
	t.Helper()
	flat := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	for _, p := range pills(h) {
		_, line, _ := strings.Cut(p, ": ")
		line = strings.TrimPrefix(strings.TrimPrefix(line, "✓ Better · "), "! Worse · ")
		for _, part := range strings.Split(line, " · ") {
			if !strings.Contains(flat(md), part) || !strings.Contains(flat(term), part) {
				t.Errorf("%q is on a card but not in the Markdown table or the terminal", part)
			}
		}
	}
}

// (d) A heavy user (150 sessions per window, the 50 most recent checked each visit) at a 30-day gap gets the in-depth
// comparison, with the selection line.
func TestHeavyUserCompares(t *testing.T) {
	w := newWorld(t)
	claudeSettings(t, w.tmp, `{"cleanupPeriodDays": 90}`, day0.AddDate(0, 0, -200))
	w.sessions(day0, 30, 150, "app")
	w.check(day0, every(2), visitOpts{})
	w.report(day0)
	d1 := day0.AddDate(0, 0, 30)
	w.sessions(d1, 30, 150, "app")
	w.check(d1, every(5), visitOpts{})
	h, md, term := w.report(d1)
	if p := strings.Join(pills(h), " | "); !strings.Contains(p, "better: ✓ Better · 50% → 20% · vs 1 Sep check") {
		t.Errorf("cards: %s", p)
	}
	if !strings.Contains(h, "Each check: the 50 most recent of about 150.") {
		t.Error("the selection line")
	}
	agree(t, h, md, term)
}

// (e) A check of one project is not compared with a check of all projects, and the reason names it.
func TestProjectCheckNotComparedWithAll(t *testing.T) {
	w := newWorld(t)
	claudeSettings(t, w.tmp, `{"cleanupPeriodDays": 90}`, day0.AddDate(0, 0, -200))
	w.sessions(day0, 30, 15, "app")
	w.sessions(day0, 30, 15, "api")
	w.check(day0, every(2), visitOpts{})
	d1 := day0.AddDate(0, 0, 30)
	w.sessions(d1, 30, 25, "app")
	w.sessions(d1, 30, 5, "api")
	w.check(d1, every(5), visitOpts{project: "app"})
	h, _, _ := w.report(d1, "--project", "app")
	if strings.Contains(h, "vs 1 Sep check") || !strings.Contains(h, "The last check covered all projects; this one covers app only.") {
		t.Errorf("cards %v", pills(h))
	}
}

// Upgrade: a user of 0.1.5/0.1.6 has results but no check record. The next report rebuilds the record from the cache
// (one judge, 50 or fewer sessions, several projects or one in the logs), so the return visit already compares.
func TestUpgradeBuildsRecordFromCache(t *testing.T) {
	w := newWorld(t)
	claudeSettings(t, w.tmp, `{"cleanupPeriodDays": 90}`, day0.AddDate(0, 0, -200))
	w.sessions(day0, 30, 24, "app")
	w.check(day0, every(2), visitOpts{noRecord: true})
	d1 := day0.AddDate(0, 0, 31)
	w.sessions(d1, 30, 24, "app")
	w.check(d1, every(6), visitOpts{})
	h, _, _ := w.report(d1)
	if !strings.Contains(strings.Join(pills(h), " | "), "vs 1 Sep check") {
		t.Errorf("cards %v", pills(h))
	}
	var derived int
	for _, r := range loadChecks(w.cache) {
		if r.Derived {
			derived++
		}
	}
	if derived != 1 {
		t.Errorf("%d derived records, want 1 (the day-0 run)", derived)
	}
}

// (c, end to end) A newer exact version of the same model: the detail list only, with its note; another family:
// nothing, with the reason. A baseline 61 days old: nothing.
func TestModelAndAgeEndToEnd(t *testing.T) {
	for _, c := range []struct {
		name, model, tier, want string
		gap                     int
		card                    bool
	}{
		{"newer version", "claude-opus-5-6", "claude:opus", "The judging model was updated.", 30, false},
		{"other family", "claude-sonnet-5-5", "claude:claude-sonnet-5-5", "The last check used a different judging model or rubric version", 30, false},
		{"61 days", "claude-opus-5-5", "claude:opus", "The last check (1 Sep) is more than 60 days old", 61, false},
		{"same model", "claude-opus-5-5", "claude:opus", "Compared with the 1 Sep check", 30, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := newWorld(t)
			claudeSettings(t, w.tmp, `{"cleanupPeriodDays": 120}`, day0.AddDate(0, 0, -200))
			w.sessions(day0, 30, 24, "app")
			w.check(day0, every(2), visitOpts{})
			d1 := day0.AddDate(0, 0, c.gap)
			w.sessions(d1, 30, 24, "app")
			w.check(d1, every(6), visitOpts{model: c.model, tier: c.tier})
			h, _, _ := w.report(d1)
			if !strings.Contains(h, c.want) || strings.Contains(strings.Join(pills(h), ""), "check") != c.card {
				t.Errorf("cards %v; notes %s", pills(h), regexp.MustCompile(`<p class="nb">[^<]*</p>`).FindAllString(h, -1))
			}
		})
	}
}

// A first visit says once that the next report compares with this one, never that logs were deleted.
func TestFirstVisitLine(t *testing.T) {
	w := newWorld(t)
	w.sessions(day0, 30, 24, "app")
	w.check(day0, every(2), visitOpts{})
	h, _, _ := w.report(day0)
	if !strings.Contains(h, "Next report compares with this one.") || strings.Contains(h, "deleted its older logs, so this part") {
		t.Error("first visit")
	}
	_ = report.CmpMinPill
}
