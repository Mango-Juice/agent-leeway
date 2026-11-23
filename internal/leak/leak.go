// Package leak is the leak check: mechanical cost leaks by cause and by habit, split into direct
// conversations and automations (research/oracle/agent_leeway/leak.py). No model calls.
package leak

import (
	"github.com/Mango-Juice/agent-leeway/internal/config"
	"math"
	"sort"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/logs"
	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"github.com/Mango-Juice/agent-leeway/internal/pipeline"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// CauseKeys in display order.
var CauseKeys = []string{"expiry", "reset_known", "reset_unknown", "repeat", "fail", "reread", "oversize", "image"}

// CauseSet numbers the set of causes counted: a cost from another set is not compared as if equal. 1: up to 0.1.7;
// 2: screenshots left in the conversation ("image") added.
const CauseSet = 2

// FixableEvents are reset causes the user can avoid (switching model, effort, settings or tools mid-session).
var FixableEvents = map[string]bool{"model_change": true, "effort_change": true, "config_update": true, "tools_change": true, "shape_change": true}

// Habits group causes (reset_unknown is not a habit).
var Habits = []struct {
	Key    string
	Causes []string
}{
	{"long_break", []string{"expiry"}}, {"mid_switch", []string{"reset_known"}}, {"let_repeat", []string{"repeat", "fail"}},
	{"no_map", []string{"reread"}}, {"big_output", []string{"oversize"}}, {"screenshots", []string{"image"}},
}

// causeKeys and habits are the ones counted: the parity tests (config.Legacy) keep the reference's set.
func causeKeys() []string {
	if config.Legacy {
		return CauseKeys[:len(CauseKeys)-1]
	}
	return CauseKeys
}

func habits() []struct {
	Key    string
	Causes []string
} {
	if config.Legacy {
		return Habits[:len(Habits)-1]
	}
	return Habits
}

// Event is one leak (one response or one reset).
type Event struct {
	TS       int64
	Key      string
	Tool     string
	ThreadID string
	Cause    string
	USD      float64
	N        float64
	Origin   string
	Reason   string
	Event    string // reset rows: the primary preceding event
}

// Resp is one response of step_waste with its origin.
type Resp struct {
	*pipeline.WasteRow
	Origin, Reason string
}

// Partial is a small cache shortfall (under 20% of the previous context).
type Partial struct {
	TS       int64
	Mid      float64
	ThreadID string
	Origin   string
	Tool     string
}

// Events is everything compute() needs, loaded once and filtered per window.
type Events struct {
	Ev       []Event
	SW       []Resp
	Partial  []Partial
	Sessions map[string]pipeline.Session
	// Uncertain are sessions whose origin (owner or automation) could not be told from the log.
	Uncertain map[string]bool
}

type tk struct {
	thread string
	seq    int
}

type tsk struct {
	thread string
	ts     int64
}

// LoadEvents collects all leak events from the pipeline tables (tool "" = all).
func LoadEvents(t *pipeline.Tables, tool string) *Events {
	org := map[tk]pipeline.OriginRow{}
	for _, o := range t.Origin {
		org[tk{o.ThreadID, o.Seq}] = o
	}
	E := &Events{Sessions: map[string]pipeline.Session{}, Uncertain: map[string]bool{}}
	for _, so := range t.SessOrigin {
		if so.Uncertain && so.SessionClass != "auto" {
			E.Uncertain[so.Key] = true
		}
	}
	for _, s := range t.Sessions {
		E.Sessions[s.Key] = s
	}
	for i := range t.Waste {
		w := &t.Waste[i]
		if tool != "" && w.Tool != tool {
			continue
		}
		r := Resp{WasteRow: w, Origin: "human", Reason: "unclassified"}
		if o, ok := org[tk{w.ThreadID, w.Seq}]; ok {
			if o.Origin.OK {
				r.Origin = o.Origin.S
			}
			if o.Reason.OK {
				r.Reason = o.Reason.S
			}
		}
		E.SW = append(E.SW, r)
	}
	add := func(cause string, usd func(Resp) float64, n func(Resp) float64) {
		for _, r := range E.SW {
			if u := usd(r); u > 0 {
				E.Ev = append(E.Ev, Event{TS: r.TS, Key: r.Key, Tool: r.Tool, ThreadID: r.ThreadID, Cause: cause, USD: u, N: n(r), Origin: r.Origin, Reason: r.Reason})
			}
		}
	}
	one := func(Resp) float64 { return 1 }
	add("expiry", func(r Resp) float64 { return r.MissExpiry }, one)
	add("repeat", func(r Resp) float64 { return r.CRepeat }, func(r Resp) float64 { return float64(r.NRepeat) })
	add("fail", func(r Resp) float64 { return r.CFail }, func(r Resp) float64 { return float64(r.NFail) })
	add("reread", func(r Resp) float64 { return r.CReread }, func(r Resp) float64 { return float64(r.NReread) })
	add("oversize", func(r Resp) float64 { return r.Oversize }, func(r Resp) float64 { return float64(r.NOversize) })
	add("image", func(r Resp) float64 { return r.Image }, func(r Resp) float64 { return float64(r.NImage) })
	if len(t.ResetEv) > 0 {
		// first response per (thread, ts) gives the origin of a reset
		first := map[tsk]Resp{}
		for _, r := range E.SW {
			k := tsk{r.ThreadID, r.TS}
			if _, ok := first[k]; !ok {
				first[k] = r
			}
		}
		for _, e := range t.ResetEv {
			if (tool != "" && e.Tool != tool) || e.Cls != "reset" {
				continue
			}
			p := e.Primary()
			cause := "reset_unknown"
			if FixableEvents[p] {
				cause = "reset_known"
			}
			ev := Event{TS: e.TS, Key: e.Key, Tool: e.Tool, ThreadID: e.ThreadID, Cause: cause, USD: e.USD, N: 1, Origin: "human", Reason: "unclassified", Event: p}
			if r, ok := first[tsk{e.ThreadID, e.TS}]; ok {
				ev.Origin, ev.Reason = r.Origin, r.Reason
			}
			E.Ev = append(E.Ev, ev)
		}
		for _, s := range t.ResetSteps {
			if (tool != "" && s.Tool != tool) || s.Cls != "partial" {
				continue
			}
			p := Partial{TS: s.TS, Mid: s.Mid, ThreadID: s.ThreadID, Origin: "human", Tool: s.Tool}
			if r, ok := first[tsk{s.ThreadID, s.TS}]; ok {
				p.Origin = r.Origin
			}
			E.Partial = append(E.Partial, p)
		}
	}
	return E
}

// ForTool is E limited to one tool's events, the same as LoadEvents(t, tool) without reading the tables again.
// The session and origin maps are shared: they are looked up by session key only.
func (E *Events) ForTool(tool string) *Events {
	out := &Events{Sessions: E.Sessions, Uncertain: E.Uncertain}
	for _, e := range E.Ev {
		if e.Tool == tool {
			out.Ev = append(out.Ev, e)
		}
	}
	for _, r := range E.SW {
		if r.Tool == tool {
			out.SW = append(out.SW, r)
		}
	}
	for _, p := range E.Partial {
		if p.Tool == tool {
			out.Partial = append(out.Partial, p)
		}
	}
	return out
}

// Cause is one row of the by-cause table.
type Cause struct {
	Cause string  `json:"cause"`
	Label string  `json:"label"`
	USD   float64 `json:"usd"`
	Count int     `json:"count"`
	Fix   string  `json:"fix"`
}

// Where says where a habit happens most (largest share by amount).
type Where struct {
	Top   string  `json:"top"`
	Share float64 `json:"share"`
}

// Habit is one row of the by-habit table.
type Habit struct {
	Habit  string           `json:"habit"`
	Label  string           `json:"label"`
	Causes []string         `json:"causes"`
	USD    float64          `json:"usd"`
	Count  int              `json:"count"`
	Per100 float64          `json:"per100"`
	Where  map[string]Where `json:"where"`
	Tip    string           `json:"tip"`
}

// AutoFix is a "fix it in the automation settings" block.
type AutoFix struct {
	Reason string   `json:"reason"`
	Label  string   `json:"label"`
	Cost   float64  `json:"cost"`
	Leak   float64  `json:"leak"`
	Tips   []string `json:"tips"`
}

// SessionLeak is one session's in-window leak and spend.
type SessionLeak struct {
	Key           string
	ByCause       map[string]float64
	Leak, Cost    float64
	First, Last   int64
	Tool, Project string
	Start         int64
	StartedBefore bool
	AutoShare     float64
}

// Report is one window's result (leak.compute).
type Report struct {
	WindowStart, WindowEnd time.Time
	Tool, Origin           string
	TotalUSD               float64
	AutomationFixes        []AutoFix
	ByTool                 [][2]any // (tool, usd) sorted by tool
	LeakUSD                float64
	Causes                 []Cause
	Habits                 []Habit
	Info                   struct {
		Subagent, PartialMiss float64
		PartialCount          int
		// Assumed lists models without a price row (priced with the provider's fallback row).
		Assumed []Assumed
		// Uncertain counts in-window sessions whose origin could not be told (counted as direct).
		Uncertain    int
		UncertainUSD float64
	}
	Sessions []SessionLeak // sorted by leak, largest first
}

// Compute builds the report for responses with since <= ts < until (origin "" | "human" | "auto").
func Compute(E *Events, since, until time.Time, tool, origin string, loc *time.Location) *Report {
	lo, hi := since.UnixNano(), until.UnixNano()
	in := func(ts int64) bool { return ts != logs.NaT && ts >= lo && ts < hi }
	var ev []Event
	for _, e := range E.Ev {
		if in(e.TS) && (origin == "" || e.Origin == origin) {
			ev = append(ev, e)
		}
	}
	var sw []Resp
	for _, r := range E.SW {
		if in(r.TS) && (origin == "" || r.Origin == origin) {
			sw = append(sw, r)
		}
	}
	var part []Partial
	for _, p := range E.Partial {
		if in(p.TS) && (origin == "" || p.Origin == origin) {
			part = append(part, p)
		}
	}
	R := &Report{WindowStart: since, WindowEnd: until, Tool: tool, Origin: origin}
	if R.Tool == "" {
		R.Tool = "all"
	}
	if R.Origin == "" {
		R.Origin = "all"
	}
	usdBy, nBy := map[string]*kahan{}, map[string]float64{}
	for _, e := range ev {
		if usdBy[e.Cause] == nil {
			usdBy[e.Cause] = &kahan{}
		}
		usdBy[e.Cause].add(e.USD)
		nBy[e.Cause] += e.N
	}
	// reset_known detail: preceding events by count (ties in order of first appearance)
	type kc struct {
		k string
		n int
	}
	var counts []kc
	idx := map[string]int{}
	for _, e := range ev {
		if e.Cause != "reset_known" {
			continue
		}
		if i, ok := idx[e.Event]; ok {
			counts[i].n++
		} else {
			idx[e.Event] = len(counts)
			counts = append(counts, kc{e.Event, 1})
		}
	}
	sort.SliceStable(counts, func(a, b int) bool { return counts[a].n > counts[b].n })
	detail := messages.EventFallback
	if len(counts) > 0 {
		detail = ""
		for i, c := range counts {
			if i > 0 {
				detail += ", "
			}
			name := messages.Events[c.k]
			if name == "" {
				name = c.k
			}
			detail += py.Format(messages.EventCount, map[string]any{"name": name, "n": c.n})
		}
	}
	for _, k := range causeKeys() {
		lt := messages.Causes[k]
		usd := 0.0
		if s := usdBy[k]; s != nil {
			usd = s.sum
		}
		n := int(nBy[k])
		fix := ""
		if n != 0 {
			fix = py.Format(lt[1], map[string]any{"n": n, "detail": detail})
		}
		R.Causes = append(R.Causes, Cause{Cause: k, Label: lt[0], USD: py.Round(usd, 2), Count: n, Fix: fix})
	}
	total := sumf(len(sw), func(i int) float64 { return sw[i].WCost })
	R.TotalUSD = py.Round(total, 2)
	// habits: where (weekday / time of day / project, local time) each habit happens
	for _, h := range habits() {
		lt := messages.Habits[h.Key]
		var d []Event
		for _, e := range ev {
			for _, c := range h.Causes {
				if e.Cause == c {
					d = append(d, e)
				}
			}
		}
		usd := sumf(len(d), func(i int) float64 { return d[i].USD })
		n := 0.0
		for _, e := range d {
			n += e.N
		}
		where := map[string]Where{}
		if usd > 0 {
			for _, dim := range []string{"weekday", "time", "project"} {
				group := func(e Event) string {
					t := time.Unix(0, e.TS).In(loc)
					switch dim {
					case "weekday":
						return messages.Weekdays[(int(t.Weekday())+6)%7]
					case "time":
						return messages.HourBands[min(t.Hour()/6, 3)]
					}
					if s, ok := E.Sessions[e.Key]; ok {
						return s.Project
					}
					return "?"
				}
				where[dim] = topShare(d, group, usd)
			}
		}
		per100 := 0.0
		if total != 0 {
			per100 = py.Round(100*usd/total, 3)
		}
		R.Habits = append(R.Habits, Habit{Habit: h.Key, Label: lt[0], Causes: h.Causes, USD: py.Round(usd, 2), Count: int(n), Per100: per100, Where: where, Tip: lt[1]})
	}
	// sessions: in-window leak and spend, and the in-window activity range
	S := map[string]*SessionLeak{}
	get := func(k string) *SessionLeak {
		s := S[k]
		if s == nil {
			s = &SessionLeak{Key: k, ByCause: map[string]float64{}, First: logs.NaT, Last: logs.NaT, Start: logs.NaT}
			S[k] = s
		}
		return s
	}
	causeSum := map[string]map[string]*kahan{}
	autoUSD, allUSD := map[string]*kahan{}, map[string]*kahan{}
	for _, e := range ev {
		get(e.Key)
		if causeSum[e.Key] == nil {
			causeSum[e.Key] = map[string]*kahan{}
		}
		if causeSum[e.Key][e.Cause] == nil {
			causeSum[e.Key][e.Cause] = &kahan{}
		}
		causeSum[e.Key][e.Cause].add(e.USD)
		if allUSD[e.Key] == nil {
			allUSD[e.Key], autoUSD[e.Key] = &kahan{}, &kahan{}
		}
		allUSD[e.Key].add(e.USD)
		if e.Origin == "auto" {
			autoUSD[e.Key].add(e.USD)
		}
	}
	costSum := map[string]*kahan{}
	for _, r := range sw {
		s := get(r.Key)
		if costSum[r.Key] == nil {
			costSum[r.Key] = &kahan{}
		}
		costSum[r.Key].add(r.WCost)
		if r.TS != logs.NaT {
			if s.First == logs.NaT || r.TS < s.First {
				s.First = r.TS
			}
			if s.Last == logs.NaT || r.TS > s.Last {
				s.Last = r.TS
			}
		}
	}
	var keys []string
	for k := range S {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s := S[k]
		leak := 0.0
		for _, c := range causeKeys() {
			v := 0.0
			if x := causeSum[k][c]; x != nil {
				v = x.sum
			}
			s.ByCause[c] = v
			leak += v
		}
		s.Leak = leak
		if c := costSum[k]; c != nil {
			s.Cost = c.sum
		}
		if ss, ok := E.Sessions[k]; ok {
			s.Tool, s.Project, s.Start = ss.Tool, ss.Project, ss.Start
		}
		s.StartedBefore = s.Start != logs.NaT && s.Start < lo
		if a := allUSD[k]; a != nil && a.sum != 0 {
			s.AutoShare = autoUSD[k].sum / a.sum
		}
		R.Sessions = append(R.Sessions, *s)
	}
	sort.SliceStable(R.Sessions, func(a, b int) bool { return R.Sessions[a].Leak > R.Sessions[b].Leak })
	var subCost []float64
	for _, r := range sw {
		if r.Sub {
			subCost = append(subCost, r.WCost)
		}
	}
	R.Info.Subagent = py.Round(sumf(len(subCost), func(i int) float64 { return subCost[i] }), 2)
	R.Info.PartialMiss = py.Round(sumf(len(part), func(i int) float64 { return part[i].Mid }), 2)
	R.Info.PartialCount = len(part)
	R.Info.Assumed = assumedPrices(sw)
	if !config.Legacy {
		u := map[string]bool{}
		var usd kahan
		for _, r := range sw {
			if E.Uncertain[r.Key] {
				u[r.Key] = true
				usd.add(r.WCost)
			}
		}
		R.Info.Uncertain, R.Info.UncertainUSD = len(u), py.Round(usd.sum, 2)
	}
	if origin == "auto" {
		R.AutomationFixes = automationFixes(ev, sw)
	}
	byTool := map[string]*kahan{}
	var tools []string
	for _, r := range sw {
		if byTool[r.Tool] == nil {
			byTool[r.Tool] = &kahan{}
			tools = append(tools, r.Tool)
		}
		byTool[r.Tool].add(r.WCost)
	}
	sort.Strings(tools)
	for _, tl := range tools {
		R.ByTool = append(R.ByTool, [2]any{tl, py.Round(byTool[tl].sum, 2)})
	}
	R.LeakUSD = py.Round(sumf(len(ev), func(i int) float64 { return ev[i].USD }), 2)
	return R
}

// topShare is the group with the largest amount and its share (ties: the alphabetically first group).
func topShare(d []Event, group func(Event) string, usd float64) Where {
	sums := map[string]*kahan{}
	var names []string
	for _, e := range d {
		g := group(e)
		if sums[g] == nil {
			sums[g] = &kahan{}
			names = append(names, g)
		}
		sums[g].add(e.USD)
	}
	sort.Strings(names)
	sort.SliceStable(names, func(a, b int) bool { return sums[names[a]].sum > sums[names[b]].sum })
	return Where{Top: names[0], Share: py.Round(sums[names[0]].sum/usd, 3)}
}

func automationFixes(ev []Event, sw []Resp) []AutoFix {
	reasons := map[string]bool{}
	var names []string
	for _, e := range ev {
		if !reasons[e.Reason] {
			reasons[e.Reason] = true
			names = append(names, e.Reason)
		}
	}
	sort.Strings(names)
	var out []AutoFix
	for _, reason := range names {
		var cost, leak kahan
		for _, r := range sw {
			if r.Reason == reason {
				cost.add(r.WCost)
			}
		}
		usd, n := map[string]*kahan{}, map[string]float64{}
		for _, e := range ev {
			if e.Reason != reason {
				continue
			}
			leak.add(e.USD)
			if usd[e.Cause] == nil {
				usd[e.Cause] = &kahan{}
			}
			usd[e.Cause].add(e.USD)
			n[e.Cause] += e.N
		}
		if leak.sum < 0.01 {
			continue
		}
		u := func(c string) float64 {
			if x := usd[c]; x != nil {
				return x.sum
			}
			return 0
		}
		var tips []string
		if usd["expiry"] != nil {
			tips = append(tips, py.Format(messages.AutoTips["expiry"], map[string]any{"times": messages.Times(int(n["expiry"])), "usd": u("expiry")}))
		}
		if loops := u("repeat") + u("fail"); loops >= 0.01 {
			tips = append(tips, py.Format(messages.AutoTips["loops"], map[string]any{"times": messages.Times(int(n["repeat"] + n["fail"])), "usd": loops}))
		}
		if usd["reset_known"] != nil {
			tips = append(tips, py.Format(messages.AutoTips["reset_known"], map[string]any{"times": messages.Times(int(n["reset_known"])), "usd": u("reset_known")}))
		}
		if other := u("reread") + u("oversize") + u("image"); other >= 0.01 {
			tips = append(tips, py.Format(messages.AutoTips["other"], map[string]any{"usd": other}))
		}
		label := messages.Automation[reason]
		if label == "" {
			label = reason
		}
		out = append(out, AutoFix{Reason: reason, Label: label, Cost: py.Round(cost.sum, 2), Leak: py.Round(leak.sum, 2), Tips: tips})
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Leak > out[b].Leak })
	return out
}

type kahan struct{ sum, c float64 }

func (k *kahan) add(x float64) {
	if math.IsNaN(x) {
		return
	}
	y := x - k.c
	t := k.sum + y
	k.c = (t - k.sum) - y
	k.sum = t
}

func sumf(n int, at func(int) float64) float64 {
	var k kahan
	for i := 0; i < n; i++ {
		k.add(at(i))
	}
	return k.sum
}

// Assumed is one model whose responses were priced with a fallback row.
type Assumed struct {
	Tool       string
	Model      string
	Responses  int
	TokenShare float64
}

func assumedPrices(sw []Resp) []Assumed {
	var total int64
	by := map[string]*Assumed{}
	tok := map[string]int64{}
	var names []string
	for _, r := range sw {
		total += r.Tokens
		if r.Assumed == "" {
			continue
		}
		a := by[r.Assumed]
		if a == nil {
			a = &Assumed{Tool: r.Tool, Model: r.Assumed}
			by[r.Assumed] = a
			names = append(names, r.Assumed)
		}
		a.Responses++
		tok[r.Assumed] += r.Tokens
	}
	sort.Strings(names)
	var out []Assumed
	for _, n := range names {
		a := *by[n]
		if total > 0 {
			a.TokenShare = float64(tok[n]) / float64(total)
		}
		out = append(out, a)
	}
	return out
}
