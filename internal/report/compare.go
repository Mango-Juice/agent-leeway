package report

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Mango-Juice/agent-leeway/internal/leak"
)

// "Compared with the previous period" (docs/how-it-works.md): this window against the window of the same length just
// before it. Lower is better on both sides. A side with no sound comparison is nil, with the reason in Why.

// Thresholds and minimums (docs/how-it-works.md, "Compared with the previous period").
const (
	CmpCostSame    = 0.02 // waste share: under 2 points is "similar"
	CmpDriftSame   = 0.08 // serious-miss and problem-kind rates: under 8 points (small samples)
	CmpPer100Same  = 1.00 // habit rows: under $1.00 per $100 spent
	CmpPer100Floor = 0.50 // habit rows under $0.50 per $100 in both periods are left out
	CmpMinSpend    = 5.0  // previous spend under $5: no cost comparison
	CmpMinSessions = 3    // previous period with fewer than 3 sessions: no cost comparison
	CmpMinJudged   = 8    // fewer than 8 checked sessions in either period: no in-depth comparison
	CmpMinCoverage = 0.6  // at least 60% of each period's sessions checked
	CmpMinHours    = 24   // windows shorter than a day are not compared
)

// Compare is the comparison shown in the report.
type Compare struct {
	Hours       int // the length of each period; whole days are said in days
	Cost, Drift *CmpSide
	CostWhy     string // message key when Cost is nil, or a note about what Cost covers ("" when none)
	DriftWhy    string // message key when Drift is nil, or a note about what Drift covers
}

// CmpSide is one side's comparison: the headline rate and up to 3 rows that changed most.
type CmpSide struct {
	State     string // better | worse | same
	Now, Prev float64
	Rows      []CmpRow
	Tool      string // "" when it covers what the report covers; else the only tool compared
}

// CmpRow is one problem kind (share of checked sessions) or habit (cost per $100 spent).
type CmpRow struct {
	Key       string
	Now, Prev float64
	State     string
}

// cmpState compares the values as shown (show rounds them the way the report prints them, so "12% → 12%" is never
// "better"). A change is better or worse only when it reaches the threshold and is larger than noise: what one
// session (in-depth) or the single largest leak (cost) of the smaller period could move on its own.
func cmpState(now, prev, same, noise float64, show func(float64) float64) string {
	d := show(now) - show(prev)
	const eps = 1e-9
	switch {
	case math.Abs(d) < same-eps || math.Abs(d) <= noise+eps:
		return "same"
	case d < 0:
		return "better"
	}
	return "worse"
}

// shownPct is a share as cmpPct prints it; shownMoney an amount as Money prints it.
func shownPct(x float64) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSuffix(cmpPct(x), "%"), 64)
	return v / 100
}

func shownMoney(x float64) float64 { return math.Round(x*100) / 100 }

// Agg is a period's cost in numbers only (also what the snapshot fallback stores).
type Agg struct {
	Total, Leak float64
	Sessions    int
	Habits      map[string]float64 // habit -> USD
	MaxLeak     float64            // the largest single session's leak, for the noise rule
}

// AggOf sums a leak report.
func AggOf(r *leak.Report) Agg {
	a := Agg{Habits: map[string]float64{}}
	if r == nil {
		return a
	}
	a.Total, a.Leak, a.Sessions = r.TotalUSD, r.LeakUSD, len(r.Sessions)
	for _, x := range r.Sessions {
		a.MaxLeak = math.Max(a.MaxLeak, x.Leak)
	}
	for _, h := range r.Habits {
		a.Habits[h.Habit] += h.USD
	}
	return a
}

// Add sums two periods' aggregates (one per tool).
func (a Agg) Add(b Agg) Agg {
	out := Agg{Total: a.Total + b.Total, Leak: a.Leak + b.Leak, Sessions: a.Sessions + b.Sessions, Habits: map[string]float64{}, MaxLeak: math.Max(a.MaxLeak, b.MaxLeak)}
	for k, v := range a.Habits {
		out.Habits[k] += v
	}
	for k, v := range b.Habits {
		out.Habits[k] += v
	}
	return out
}

// CostSide compares the waste share and, as rows, the habits whose cost per $100 changed most (nil when the
// previous period is too small to compare).
func CostSide(now, prev Agg) *CmpSide {
	if now.Total <= 0 || prev.Total < CmpMinSpend || prev.Sessions < CmpMinSessions {
		return nil
	}
	c := &CmpSide{Now: now.Leak / now.Total, Prev: prev.Leak / prev.Total}
	small := prev
	if now.Total < prev.Total {
		small = now
	}
	noise := small.MaxLeak / small.Total // the waste share one session's leak moves in the smaller period
	c.State = cmpState(c.Now, c.Prev, CmpCostSame, noise, shownPct)
	keys := map[string]bool{}
	for k := range now.Habits {
		keys[k] = true
	}
	for k := range prev.Habits {
		keys[k] = true
	}
	for k := range keys {
		a, b := now.Habits[k]/now.Total*100, prev.Habits[k]/prev.Total*100
		if a < CmpPer100Floor && b < CmpPer100Floor {
			continue
		}
		c.Rows = append(c.Rows, CmpRow{k, a, b, cmpState(a, b, CmpPer100Same, noise*100, shownMoney)})
	}
	c.Rows = sortRows(c.Rows)
	return c
}

func sortRows(rows []CmpRow) []CmpRow {
	sort.SliceStable(rows, func(i, j int) bool {
		di, dj := math.Abs(rows[i].Now-rows[i].Prev), math.Abs(rows[j].Now-rows[j].Prev)
		if di != dj {
			return di > dj
		}
		return rows[i].Key < rows[j].Key
	})
	if len(rows) > 3 {
		rows = rows[:3]
	}
	return rows
}

// DriftSide compares the serious-miss rate and, as rows, the problem kinds whose share of checked sessions changed
// most. Only tools checked in both periods with the same judges (tier, the exact model when recorded) and rubric
// versions count; each period needs CmpMinJudged checked sessions and CmpMinCoverage of its sessions checked.
// direct counts each period's sessions per tool. Returns nil and a message key when there is no sound comparison:
// cmp_why_judge (other judge or rubric), cmp_why_tools (no tool checked in both), cmp_why_few (a period has too
// few checked sessions), cmp_why_partial (too small a share checked).
func DriftSide(now, prev []*Record, nowDirect, prevDirect map[string]int) (*CmpSide, string) {
	sig := func(rs []*Record, tool string) (map[string]bool, int) {
		s, n := map[string]bool{}, 0
		for _, r := range rs {
			if r.Tool == tool {
				s[r.Tier+"|"+r.Model+"|"+r.RubricVersion] = true
				n++
			}
		}
		return s, n
	}
	same := func(a, b map[string]bool) bool {
		if len(a) != len(b) {
			return false
		}
		for k := range a {
			if !b[k] {
				return false
			}
		}
		return true
	}
	var tools []string
	nowTools, judgeDiffers := 0, false
	for _, t := range []string{"claude", "codex"} {
		a, na := sig(now, t)
		b, nb := sig(prev, t)
		if na > 0 {
			nowTools++
		}
		if na == 0 || nb == 0 {
			continue
		}
		if !same(a, b) {
			judgeDiffers = true
			continue
		}
		tools = append(tools, t)
	}
	if len(tools) == 0 {
		switch {
		case judgeDiffers:
			return nil, "cmp_why_judge"
		case len(now) >= CmpMinJudged && len(prev) >= CmpMinJudged:
			return nil, "cmp_why_tools"
		}
		return nil, "cmp_why_few"
	}
	pick := func(rs []*Record) []*Record {
		var out []*Record
		for _, r := range rs {
			for _, t := range tools {
				if r.Tool == t {
					out = append(out, r)
				}
			}
		}
		return out
	}
	a, b := pick(now), pick(prev)
	if len(a) < CmpMinJudged || len(b) < CmpMinJudged {
		return nil, "cmp_why_few"
	}
	da, db := 0, 0
	for _, t := range tools {
		da, db = da+nowDirect[t], db+prevDirect[t]
	}
	if da > 0 && float64(len(a))/float64(da) < CmpMinCoverage || db > 0 && float64(len(b))/float64(db) < CmpMinCoverage {
		return nil, "cmp_why_partial"
	}
	rate := func(rs []*Record, f func(*Record) bool) float64 {
		n := 0
		for _, r := range rs {
			if f(r) {
				n++
			}
		}
		return float64(n) / float64(len(rs))
	}
	noise := 1 / float64(min(len(a), len(b))) // what one session moves a rate in the smaller period
	serious := func(r *Record) bool { return r.Verdict == "agent_drift" || r.Verdict == "prompt_gap" }
	c := &CmpSide{Now: rate(a, serious), Prev: rate(b, serious)}
	c.State = cmpState(c.Now, c.Prev, CmpDriftSame, noise, shownPct)
	if len(tools) == 1 && nowTools > 1 {
		c.Tool = tools[0]
	}
	has := func(k string) func(*Record) bool {
		return func(r *Record) bool {
			for _, e := range r.Events {
				if storyKind(e) == k {
					return true
				}
			}
			return false
		}
	}
	for _, k := range []string{"BROKE_CHAT", "BROKE_FILE", "RESERVED", "FALSE_DONE", "ASKED_ONLY", "MISTAKE", "PROMPT"} {
		x, y := rate(a, has(k)), rate(b, has(k))
		if x > 0 || y > 0 {
			c.Rows = append(c.Rows, CmpRow{k, x, y, cmpState(x, y, CmpDriftSame, noise, shownPct)})
		}
	}
	c.Rows = sortRows(c.Rows)
	return c, ""
}
