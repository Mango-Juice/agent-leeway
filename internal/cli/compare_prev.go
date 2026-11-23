package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/drift"
	"github.com/Mango-Juice/agent-leeway/internal/leak"
	"github.com/Mango-Juice/agent-leeway/internal/pipeline"
	"github.com/Mango-Juice/agent-leeway/internal/py"
	"github.com/Mango-Juice/agent-leeway/internal/report"
)

// compareWithPrevious compares this window with the previous window of the same length (docs/how-it-works.md): the cost
// side from the logs (a saved numbers-only snapshot fills in when Claude Code deleted its older logs), the
// in-depth side from the judged records of sessions in the previous window. It also saves this window's
// snapshot for later runs, per tool, only when the logs cover this whole window. Windows under a day: nil.
// curKeys are the sessions of this window (a session spanning the boundary counts here, the period of its last
// turn); all is every cached result, read once by the caller.
func compareWithPrevious(o *driftOpts, tb *pipeline.Tables, x leakSet, m *report.Model, since, now time.Time, tools []string, inWin map[string]int, curKeys map[string]bool, all []py.Value) *report.Compare {
	span := now.Sub(since)
	if span < report.CmpMinHours*time.Hour {
		return nil
	}
	hours := int(math.Round(span.Hours()))
	prevSince := since.Add(-span)
	c := &report.Compare{Hours: hours}
	until := now.Add(time.Minute)
	ev := map[string]*leak.Events{"claude": x.E.ForTool("claude"), "codex": x.E.ForTool("codex")}
	agg := func(tool string, s, u time.Time) report.Agg {
		return report.AggOf(leak.Compute(ev[tool], s, u, tool, "", time.Local))
	}
	src := snapSources(&o.logOpts)
	claudeNow, claudePrev := claudeCovers(tb, since, now), claudeCovers(tb, prevSince, now)
	saveSnapshots(o.cacheDir(), src, since, hours, map[string]report.Agg{"claude": agg("claude", since, until), "codex": agg("codex", since, until)},
		map[string]bool{"claude": claudeNow, "codex": true})

	// cost: the logs must cover the previous window for each tool (Codex keeps its logs)
	c.Cost, c.CostWhy = costCompare(claudePrev, report.AggOf(x.R), report.AggOf(x.P), agg("codex", since, until), agg("codex", prevSince, since),
		func() (report.Agg, bool) {
			return readSnapshot(o.cacheDir(), "claude", src["claude"], prevSince, since, hours)
		})

	// in-depth: judged records of the previous window's sessions, each session once (sessions that go on into
	// this window count here); Claude Code sessions only when its logs cover the previous window
	keys, prevWin := prevSessions(tb, prevSince, since, tools, o, curKeys, claudePrev)
	var raw []py.Value
	for _, r := range all {
		if keys[r.Get("key").S] {
			raw = append(raw, r)
		}
	}
	prevRecs, _ := report.Records(raw, 0)
	c.Drift, c.DriftWhy = report.DriftSide(m.Records, prevRecs, inWin, prevWin)
	if !claudePrev && hasTool(m.Records, "claude") {
		switch {
		case c.Drift != nil && c.Drift.Tool == "codex":
			c.DriftWhy = "cmp_why_drift_deleted"
		case c.Drift == nil && c.DriftWhy != "cmp_why_judge" && c.DriftWhy != "cmp_why_partial":
			c.DriftWhy = "cmp_why_drift_gone"
		}
	}
	return c
}

// costCompare is the cost side: from the logs when Claude Code's cover the previous window; else this window
// against the previous Codex logs plus Claude Code's saved snapshot; else Codex alone, named; else nothing.
func costCompare(claudePrev bool, nowAll, prevAll, codexNow, codexPrev report.Agg, snap func() (report.Agg, bool)) (*report.CmpSide, string) {
	if claudePrev {
		if c := report.CostSide(nowAll, prevAll); c != nil {
			return c, ""
		}
		return nil, "cmp_why_cost_few"
	}
	if s, ok := snap(); ok {
		if c := report.CostSide(nowAll, codexPrev.Add(s)); c != nil {
			return c, ""
		}
		return nil, "cmp_why_cost_few"
	}
	if codexNow.Total > 0 {
		if c := report.CostSide(codexNow, codexPrev); c != nil {
			c.Tool = "codex"
			return c, "cmp_why_cost_deleted"
		}
	}
	return nil, "cmp_why_cost_gone"
}

// prevSessions are the previous window's sessions and their count per tool: not those that go on into this window
// (curKeys; a session counts in the period of its last turn), and Claude Code's only when its logs cover the window.
func prevSessions(tb *pipeline.Tables, prevSince, since time.Time, tools []string, o *driftOpts, curKeys map[string]bool, claudeCovered bool) (map[string]bool, map[string]int) {
	keys, n := map[string]bool{}, map[string]int{}
	for _, s := range drift.Select(tb, prevSince, since, tools, o.project, o.includeAuto, 0) {
		if curKeys[s.Key] || (s.Tool == "claude" && !claudeCovered) {
			continue
		}
		keys[s.Key] = true
		n[s.Tool]++
	}
	return keys, n
}

func hasTool(rs []*report.Record, tool string) bool {
	for _, r := range rs {
		if r.Tool == tool {
			return true
		}
	}
	return false
}

// claudeCovers says whether the Claude Code logs read reach back to start: there are none (Claude Code is not
// used), or the oldest transcript is at most a day after start, or Claude Code's retention setting reaches back past
// start and has not changed since before start (a raised setting does not bring back logs deleted before).
func claudeCovers(tb *pipeline.Tables, start, now time.Time) bool {
	if tb.OldestClaude == 0 || tb.OldestClaude <= start.Add(24*time.Hour).UnixNano() {
		return true
	}
	days, changed := config.ClaudeCleanupDays()
	return !now.AddDate(0, 0, -days).After(start) && changed.Before(start)
}

// snapshot is a window's cost in numbers only, saved so a later report can compare with it when the logs of that
// window are gone (Claude Code deletes transcripts after 30 days by default).
type snapshot struct {
	Start    string             `json:"start"` // RFC 3339, UTC
	Hours    int                `json:"hours"`
	TotalUSD float64            `json:"total_usd"`
	LeakUSD  float64            `json:"leak_usd"`
	MaxLeak  float64            `json:"max_session_leak_usd"`
	Sessions int                `json:"sessions"`
	Habits   map[string]float64 `json:"habits_usd"`
}

func snapDir(cache string) string { return filepath.Join(cache, "compare") }

// snapSources names, per tool, where the numbers come from: a short hash of the log folders read (and of a fixed
// clock, LEEWAY_NOW), so runs on other folders or at a made-up time never mix with the real ones.
func snapSources(o *logOpts) map[string]string {
	claude, _, codexDirs := o.dirs()
	h := func(parts ...string) string {
		for i, p := range parts {
			if a, err := filepath.Abs(p); err == nil {
				parts[i] = a
			}
		}
		sum := sha256.Sum256([]byte(strings.Join(append(parts, "now="+os.Getenv("LEEWAY_NOW")), "\x00")))
		return hex.EncodeToString(sum[:])[:10]
	}
	return map[string]string{"claude": h(claude), "codex": h(codexDirs...)}
}

func snapName(tool, src string, start time.Time, hours int) string {
	return fmt.Sprintf("%s-%s-%s-%dh.json", tool, src, start.UTC().Format("2006-01-02"), hours)
}

// saveSnapshots writes each tool's window whose logs cover it (ok) and that has any spend.
func saveSnapshots(cache string, src map[string]string, since time.Time, hours int, aggs map[string]report.Agg, ok map[string]bool) {
	for tool, a := range aggs {
		if !ok[tool] || a.Total <= 0 {
			continue
		}
		_ = os.MkdirAll(snapDir(cache), 0o700)
		b, _ := json.MarshalIndent(snapshot{since.UTC().Format(time.RFC3339), hours, a.Total, a.Leak, a.MaxLeak, a.Sessions, a.Habits}, "", " ")
		_ = os.WriteFile(filepath.Join(snapDir(cache), snapName(tool, src[tool], since, hours)), b, 0o600)
	}
}

// readSnapshot finds the saved window of the same length, from the same log folders, whose start is closest to
// want: within 10% of the window and at most 2 days, and before cur (this window's start: never itself). A file
// whose fields disagree with its name or whose numbers cannot be right is ignored.
func readSnapshot(cache, tool, src string, want, cur time.Time, hours int) (report.Agg, bool) {
	tol := min(time.Duration(hours)*time.Hour/10, 48*time.Hour)
	files, _ := filepath.Glob(filepath.Join(snapDir(cache), fmt.Sprintf("%s-%s-*-%dh.json", tool, src, hours)))
	var best report.Agg
	bestOff, found := time.Duration(math.MaxInt64), false
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var s snapshot
		if json.Unmarshal(b, &s) != nil || s.Hours != hours {
			continue
		}
		st, err := time.Parse(time.RFC3339, s.Start)
		if err != nil || !st.Before(cur) || filepath.Base(f) != snapName(tool, src, st, hours) {
			continue
		}
		off := st.Sub(want)
		if off < 0 {
			off = -off
		}
		if off > tol || off >= bestOff || !snapSane(s) {
			continue
		}
		best, bestOff, found = report.Agg{Total: s.TotalUSD, Leak: s.LeakUSD, MaxLeak: s.MaxLeak, Sessions: s.Sessions, Habits: s.Habits}, off, true
	}
	return best, found
}

func snapSane(s snapshot) bool {
	const eps = 1e-6
	if s.TotalUSD < 0 || s.LeakUSD < 0 || s.LeakUSD > s.TotalUSD+eps || s.Sessions < 0 || s.MaxLeak < 0 || s.MaxLeak > s.LeakUSD+eps {
		return false
	}
	for _, v := range s.Habits {
		if v < 0 || v > s.LeakUSD+eps {
			return false
		}
	}
	return true
}
