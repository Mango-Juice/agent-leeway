package report

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/leak"
)

// "Compared with" (docs/how-it-works.md). Lower is better on both sides.
//   - Cost: this window against the window of the same length just before it, from the logs; when Claude Code has
//     deleted those logs, against the numbers an earlier report saved, labelled with that report's date.
//   - In-depth: this window's checked sessions against the most recent earlier check with the same selection, scope,
//     window length, judging model family and rubric, at most 60 days old and sharing no session, labelled with
//     its date.
// A side with no sound comparison is nil, with the reason in Why.

// Thresholds and minimums.
const (
	CmpCostSame     = 0.02 // waste share: a change under 2 points is not called better or worse
	CmpPer100Same   = 1.00 // habit rows: under $1.00 per $100 spent
	CmpPer100Floor  = 0.50 // habit rows under $0.50 per $100 in both periods are left out
	CmpMinSpend     = 5.0  // previous spend under $5: no cost comparison
	CmpMinSessions  = 10   // fewer than 10 sessions in either period: no cost comparison
	CmpMinDetail    = 10   // fewer than 10 checked sessions on either side: no in-depth comparison
	CmpMinPill      = 15   // the in-depth card line needs 15 checked sessions on each side
	CmpMinHours     = 24   // windows shorter than a day are not compared
	CmpMaxAgeDays   = 60   // a stored check or report older than this is not a baseline
	CmpSessionSlack = time.Hour
)

// Baseline kinds.
const (
	BasePrior  = "prior"  // the previous window of the same length, from the logs
	BaseReport = "report" // the numbers an earlier report saved
	BaseCheck  = "check"  // an earlier in-depth check
)

// Compare is the comparison shown in the report.
type Compare struct {
	Hours       int // the length of this period; whole days are said in days
	Cost, Drift *CmpSide
	CostWhy     string         // message key when Cost is nil, or a note about what it covers ("" when none)
	DriftWhy    string         // message key when Drift is nil, or a note about what it covers
	CostArgs    map[string]any // values for the cost reason (a date)
	DriftArgs   map[string]any // values for the in-depth reason (a date, counts, the project)
}

// CmpSide is one side's comparison.
type CmpSide struct {
	State      string // better | worse | neutral: the word is shown only for better or worse
	Now, Prev  float64
	NNow, NPrv int // sessions on each side (checked sessions for the in-depth side)
	Rows       []CmpRow
	Tool       string // "" when it covers what the report covers; else the only tool compared
	Pill       bool   // shown on the first screen
	Base       string // prior | report | check
	BaseAt     time.Time
	BaseFrom   time.Time // the baseline's window
	BaseTo     time.Time
	Updated    bool           // in-depth: the judging model is a newer version of the same family
	OtherModel bool           // in-depth: another (older or unknown) version of the same family; no card line
	Other      string         // in-depth, two tools: the tool not compared ("" when none)
	OtherWhy   string         // its reason (a message key)
	OtherArgs  map[string]any // values for it
	InWindow   int            // in-depth: sessions in this window, when more than were checked (the "most recent N of M" line)
	Counts     bool           // rows are counts (in-depth) rather than rates
}

// CmpRow is one problem kind (counts of checked sessions, out of NNow and NPrv) or habit (cost per $100 spent).
type CmpRow struct {
	Key        string
	Now, Prev  float64
	State      string
	NNow, NPrv int
}

// cmpState compares the values as shown (show rounds them the way the report prints them, so "12% → 12%" is never
// "better"). A change is better or worse only when it reaches the threshold and is larger than what the single
// largest leak of the smaller period could move on its own; otherwise neutral.
func cmpState(now, prev, same, noise float64, show func(float64) float64) string {
	d := show(now) - show(prev)
	const eps = 1e-9
	switch {
	case math.Abs(d) < same-eps || math.Abs(d) <= noise+eps:
		return "neutral"
	case d < 0:
		return "better"
	}
	return "worse"
}

// Significant says whether two rates differ by more than normal swings for this many sessions: the difference is at
// least twice its standard error under the pooled rate, 2*sqrt(p(1-p)(1/n1+1/n2)), and a two-sided Fisher exact test
// on the two counts gives p < 0.05 (the second guards the small samples the card line allows).
func Significant(k1, n1, k2, n2 int) bool {
	if n1 <= 0 || n2 <= 0 || k1 < 0 || k2 < 0 || k1 > n1 || k2 > n2 {
		return false
	}
	p1, p2 := float64(k1)/float64(n1), float64(k2)/float64(n2)
	p := float64(k1+k2) / float64(n1+n2)
	se := math.Sqrt(p * (1 - p) * (1/float64(n1) + 1/float64(n2)))
	return p1 != p2 && math.Abs(p1-p2) >= 2*se && FisherP(k1, n1, k2, n2) < 0.05
}

// FisherP is the two-sided Fisher exact test of k1 of n1 against k2 of n2: the probability, with the margins fixed,
// of a table at most as likely as the one seen.
func FisherP(k1, n1, k2, n2 int) float64 {
	k := k1 + k2
	lnC := func(n, r int) float64 {
		a, _ := math.Lgamma(float64(n + 1))
		b, _ := math.Lgamma(float64(r + 1))
		c, _ := math.Lgamma(float64(n - r + 1))
		return a - b - c
	}
	total := lnC(n1+n2, k)
	prob := func(x int) float64 { return math.Exp(lnC(n1, x) + lnC(n2, k-x) - total) }
	seen := prob(k1)
	sum := 0.0
	for x := max(0, k-n2); x <= min(k, n1); x++ {
		if q := prob(x); q <= seen*(1+1e-7) {
			sum += q
		}
	}
	return math.Min(sum, 1)
}

// newerModel says whether model a is a newer version than b of the same family: their version numbers compared
// part by part ("claude-opus-5-6" after "claude-opus-5-5", "gpt-6.3-astra" after "gpt-6.2-astra"). false when either
// has no version.
func newerModel(a, b string) bool {
	num := regexp.MustCompile(`\d+`)
	x, y := num.FindAllString(a, -1), num.FindAllString(b, -1)
	if len(x) == 0 || len(y) == 0 {
		return false
	}
	for i := 0; i < len(x) && i < len(y); i++ {
		xi, _ := strconv.Atoi(x[i])
		yi, _ := strconv.Atoi(y[i])
		if xi != yi {
			return xi > yi
		}
	}
	return len(x) > len(y)
}

// shownPct is a share as cmpPct prints it; shownMoney an amount as Money prints it.
func shownPct(x float64) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSuffix(cmpPct(x), "%"), 64)
	return v / 100
}

func shownMoney(x float64) float64 { return math.Round(x*100) / 100 }

// Agg is a period's cost in numbers only (also what a report saves).
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

// CostSide compares the waste share and, as rows, the habits whose cost per $100 changed most (nil when either period
// is too small to compare). The card line always shows both rates; the word only for a change past the threshold and
// past what the largest single leak of the smaller period moves.
func CostSide(now, prev Agg) *CmpSide {
	if now.Total <= 0 || prev.Total < CmpMinSpend || prev.Sessions < CmpMinSessions || now.Sessions < CmpMinSessions {
		return nil
	}
	c := &CmpSide{Now: now.Leak / now.Total, Prev: prev.Leak / prev.Total, NNow: now.Sessions, NPrv: prev.Sessions, Pill: true}
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
		c.Rows = append(c.Rows, CmpRow{Key: k, Now: a, Prev: b, State: cmpState(a, b, CmpPer100Same, noise*100, shownMoney)})
	}
	c.Rows = sortRows(c.Rows)
	return c
}

// sortRows keeps the 3 rows that changed most: by rate when the rows are counts (out of NNow and NPrv), else by value.
func sortRows(rows []CmpRow) []CmpRow {
	change := func(r CmpRow) float64 {
		if r.NNow > 0 && r.NPrv > 0 {
			return math.Abs(r.Now/float64(r.NNow) - r.Prev/float64(r.NPrv))
		}
		return math.Abs(r.Now - r.Prev)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		di, dj := change(rows[i]), change(rows[j])
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

// CheckAgg is one in-depth check in numbers only: a stored record (after each check), or this report's checked
// sessions. No text, no project names, no paths.
type CheckAgg struct {
	Tool        string         `json:"tool"`
	CheckedAt   time.Time      `json:"checked_at"`
	WindowStart time.Time      `json:"window_start"`
	WindowHours int            `json:"window_hours"`
	Checked     int            `json:"checked"`
	Serious     int            `json:"serious"`
	Kinds       map[string]int `json:"kinds"`
	InWindow    int            `json:"sessions_in_window"`
	Tier        string         `json:"tier"`
	Model       string         `json:"model"`
	Family      string         `json:"model_family"`
	Rubric      string         `json:"rubric"`
	Rule        int            `json:"most_recent"`            // the N most recent sessions not checked yet; 0 = all in the window
	Scope       string         `json:"scope"`                  // "all", or "project:<hash>"; "+auto" when automations were included
	First       time.Time      `json:"first_session,omitzero"` // left out when unknown
	Last        time.Time      `json:"last_session,omitzero"`
	Keys        []string       `json:"sessions"`
	Source      string         `json:"source,omitempty"`  // which log folders (a hash), so other folders never mix
	Derived     bool           `json:"derived,omitempty"` // rebuilt from the results cache of an earlier version
}

var versionPart = regexp.MustCompile(`[-.]?\d+(?:[.-]\d+)*`)

// ModelFamily is a judging model without its version numbers: claude-opus-5-5 and claude-opus-5-6 are one family,
// gpt-6.2-astra and gpt-6.3-astra another.
func ModelFamily(tier, model string) string {
	m := model
	if m == "" {
		_, m, _ = strings.Cut(tier, ":")
	}
	m = strings.ToLower(versionPart.ReplaceAllString(m, ""))
	if p, _, ok := strings.Cut(tier, ":"); ok {
		return p + ":" + strings.Trim(m, "-.")
	}
	return strings.Trim(m, "-.")
}

// AggChecks counts a set of checked sessions (one record per session).
func AggChecks(recs []*Record) (n, serious int, kinds map[string]int) {
	kinds = map[string]int{}
	for _, r := range recs {
		n++
		if r.Verdict == "agent_drift" || r.Verdict == "prompt_gap" {
			serious++
		}
		seen := map[string]bool{}
		for _, e := range r.Events {
			if k := storyKind(e); k != "" && !seen[k] {
				seen[k] = true
				kinds[k]++
			}
		}
	}
	return
}

// PickCheck chooses the baseline for the in-depth side of one tool: the most recent earlier check (before cur was
// made) with the same selection rule, scope, window length, judging model family and rubric, at most 60 days old,
// sharing no session with cur. When none qualifies, the reason names the cause found on the most recent earlier
// check that shares no session ("" baseline and cmp_first when there was no earlier check at all).
func PickCheck(cur *CheckAgg, recs []CheckAgg, now time.Time) (*CheckAgg, string, map[string]any) {
	keys := map[string]bool{}
	for _, k := range cur.Keys {
		keys[k] = true
	}
	var earlier []CheckAgg
	for _, r := range recs {
		if r.Tool == cur.Tool && r.Source == cur.Source && r.CheckedAt.Before(cur.CheckedAt.Add(-CmpSessionSlack)) {
			earlier = append(earlier, r)
		}
	}
	if len(earlier) == 0 {
		return nil, "cmp_first", nil
	}
	sort.SliceStable(earlier, func(i, j int) bool { return earlier[i].CheckedAt.After(earlier[j].CheckedAt) })
	shares := func(r CheckAgg) bool {
		for _, k := range r.Keys {
			if keys[k] {
				return true
			}
		}
		return false
	}
	why := func(r CheckAgg) (string, map[string]any) {
		switch {
		case now.Sub(r.CheckedAt) > CmpMaxAgeDays*24*time.Hour:
			return "cmp_why_old", map[string]any{"date": r.CheckedAt}
		case r.Scope != cur.Scope:
			return ScopeWhy(r.Scope, cur.Scope), nil
		case r.WindowHours != cur.WindowHours:
			return "cmp_why_window", map[string]any{"then": r.WindowHours / 24, "now": cur.WindowHours / 24}
		case r.Rule != cur.Rule:
			return "cmp_why_rule", nil
		case r.Family != cur.Family || r.Rubric != cur.Rubric:
			return "cmp_why_judge", nil
		}
		return "", nil
	}
	first := ""
	var firstArgs map[string]any
	for _, r := range earlier {
		if shares(r) {
			continue
		}
		w, a := why(r)
		if w == "" {
			b := r
			return &b, "", nil
		}
		if first == "" {
			first, firstArgs = w, a
		}
	}
	if first == "" {
		return nil, "cmp_why_overlap", nil
	}
	return nil, first, firstArgs
}

// ScopeWhy names how two scopes differ ("all" or "project:<hash>", each with "+auto" when automations were
// included): the projects first, then the automations. then is the earlier check's, now this one's.
func ScopeWhy(then, now string) string {
	pt, at := strings.CutSuffix(then, "+auto")
	pn, an := strings.CutSuffix(now, "+auto")
	switch {
	case pt != pn && pt == "all":
		return "cmp_why_scope_one"
	case pt != pn && pn == "all":
		return "cmp_why_scope_all"
	case pt != pn:
		return "cmp_why_scope_other"
	case at && !an:
		return "cmp_why_auto_then"
	case an && !at:
		return "cmp_why_auto_now"
	}
	return ""
}

// DriftCompare is the in-depth side: cur against its baseline (counts per side). nil when either side has fewer than
// CmpMinDetail checked sessions. The card line needs CmpMinPill on each side and the same exact judging model; its
// word only when the change is significant.
func DriftCompare(cur, base *CheckAgg) *CmpSide {
	if cur.Checked < CmpMinDetail || base.Checked < CmpMinDetail {
		return nil
	}
	c := &CmpSide{Now: float64(cur.Serious) / float64(cur.Checked), Prev: float64(base.Serious) / float64(base.Checked),
		NNow: cur.Checked, NPrv: base.Checked, Base: BaseCheck, BaseAt: base.CheckedAt, BaseFrom: base.WindowStart,
		BaseTo: base.CheckedAt, Counts: true, State: "neutral"}
	if cur.Model != base.Model { // same family (PickCheck), another exact version: the detail only, with a note
		if newerModel(cur.Model, base.Model) {
			c.Updated = true
		} else {
			c.OtherModel = true
		}
	}
	c.Pill = !c.Updated && !c.OtherModel && cur.Checked >= CmpMinPill && base.Checked >= CmpMinPill
	if Significant(cur.Serious, cur.Checked, base.Serious, base.Checked) {
		c.State = map[bool]string{true: "better", false: "worse"}[c.Now < c.Prev]
	}
	if cur.InWindow > cur.Checked || base.InWindow > base.Checked {
		c.InWindow = max(cur.InWindow, cur.Checked)
	}
	c.Rows = append(c.Rows, CmpRow{"SERIOUS", float64(cur.Serious), float64(base.Serious), "", cur.Checked, base.Checked})
	var kinds []CmpRow
	for _, k := range []string{"BROKE_CHAT", "BROKE_FILE", "RESERVED", "FALSE_DONE", "ASKED_ONLY", "MISTAKE", "PROMPT"} {
		x, y := cur.Kinds[k], base.Kinds[k]
		if x > 0 || y > 0 {
			kinds = append(kinds, CmpRow{k, float64(x), float64(y), "", cur.Checked, base.Checked})
		}
	}
	c.Rows = append(c.Rows, sortRows(kinds)...)
	return c
}

// Combine adds two tools' in-depth comparisons against the same check (one check run covering both tools).
func Combine(a, b *CheckAgg) *CheckAgg {
	out := *a
	out.Checked, out.Serious, out.InWindow = a.Checked+b.Checked, a.Serious+b.Serious, a.InWindow+b.InWindow
	out.Kinds = map[string]int{}
	for k, v := range a.Kinds {
		out.Kinds[k] += v
	}
	for k, v := range b.Kinds {
		out.Kinds[k] += v
	}
	out.Keys = append(append([]string{}, a.Keys...), b.Keys...)
	if b.WindowStart.Before(out.WindowStart) {
		out.WindowStart = b.WindowStart
	}
	if b.CheckedAt.After(out.CheckedAt) {
		out.CheckedAt = b.CheckedAt
	}
	if a.Model != b.Model { // two judges: "same model" is decided per tool before combining
		out.Model = ""
	}
	return &out
}
