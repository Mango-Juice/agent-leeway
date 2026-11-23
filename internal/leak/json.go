package leak

import (
	"fmt"
	"github.com/Mango-Juice/agent-leeway/internal/config"
	"sort"
	"strings"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/logs"
	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// object builder for ordered JSON
type obj struct{ o *py.Object }

func newObj() obj                          { return obj{&py.Object{}} }
func (b obj) set(k string, v py.Value) obj { b.o.Set(k, v); return b }
func (b obj) v() py.Value                  { return py.Value{K: py.Obj, O: b.o} }

func fv(x float64) py.Value { return py.Value{K: py.Float, F: x} }
func iv(x int) py.Value     { return py.Value{K: py.Int, I: int64(x)} }
func sv(s string) py.Value  { return py.String(s) }
func bv(b bool) py.Value    { return py.Value{K: py.Bool, B: b} }
func arr(xs []py.Value) py.Value {
	if xs == nil {
		xs = []py.Value{}
	}
	return py.Value{K: py.Arr, A: xs}
}

// isoTime is pandas Timestamp.isoformat() for a UTC time.
func isoTime(t time.Time) string {
	t = t.UTC()
	s := t.Format("2006-01-02T15:04:05")
	if ns := t.Nanosecond(); ns != 0 {
		if ns%1000 == 0 {
			s += fmt.Sprintf(".%06d", ns/1000)
		} else {
			s += fmt.Sprintf(".%09d", ns)
		}
	}
	return s + "+00:00"
}

// tsStr is str(pandas.Timestamp) for a UTC time, "NaT" when missing.
func tsStr(ns int64) string {
	if ns == logs.NaT {
		return "NaT"
	}
	return strings.Replace(isoTime(time.Unix(0, ns)), "T", " ", 1)
}

// ToValue is the JSON form of one window's report (the reference's to_json js()).
func (R *Report) ToValue(withSessions bool) py.Value {
	b := newObj().set("window_start", sv(isoTime(R.WindowStart))).set("window_end", sv(isoTime(R.WindowEnd))).
		set("tool", sv(R.Tool)).set("origin", sv(R.Origin)).set("total_usd", fv(R.TotalUSD))
	var af []py.Value
	for _, f := range R.AutomationFixes {
		var tips []py.Value
		for _, t := range f.Tips {
			tips = append(tips, sv(t))
		}
		af = append(af, newObj().set("reason", sv(f.Reason)).set("label", sv(f.Label)).set("cost", fv(f.Cost)).set("leak", fv(f.Leak)).set("tips", arr(tips)).v())
	}
	b.set("automation_fixes", arr(af))
	bt := newObj()
	for _, x := range R.ByTool {
		bt.set(x[0].(string), fv(x[1].(float64)))
	}
	b.set("by_tool", bt.v()).set("leak_usd", fv(R.LeakUSD))
	var cs []py.Value
	for _, c := range R.Causes {
		cs = append(cs, newObj().set("cause", sv(c.Cause)).set("label", sv(c.Label)).set("usd", fv(c.USD)).set("count", iv(c.Count)).set("fix", sv(c.Fix)).v())
	}
	b.set("causes", arr(cs))
	var hs []py.Value
	for _, h := range R.Habits {
		var cl []py.Value
		for _, c := range h.Causes {
			cl = append(cl, sv(c))
		}
		w := newObj()
		for _, k := range []string{"weekday", "time", "project"} {
			if x, ok := h.Where[k]; ok {
				w.set(k, newObj().set("top", sv(x.Top)).set("share", fv(x.Share)).v())
			}
		}
		hs = append(hs, newObj().set("habit", sv(h.Habit)).set("label", sv(h.Label)).set("causes", arr(cl)).set("usd", fv(h.USD)).
			set("count", iv(h.Count)).set("per100", fv(h.Per100)).set("where", w.v()).set("tip", sv(h.Tip)).v())
	}
	b.set("habits", arr(hs))
	info := newObj().set("subagent", fv(R.Info.Subagent)).set("partial_miss", fv(R.Info.PartialMiss)).set("partial_count", iv(R.Info.PartialCount))
	if !config.Legacy {
		info.set("origin_uncertain", newObj().set("sessions", iv(R.Info.Uncertain)).set("usd", fv(R.Info.UncertainUSD)).v())
	}
	if len(R.Info.Assumed) > 0 {
		var as []py.Value
		for _, a := range R.Info.Assumed {
			as = append(as, newObj().set("model", sv(a.Model)).set("responses", iv(a.Responses)).set("token_share", fv(py.Round(a.TokenShare, 4))).v())
		}
		info.set("assumed_prices", arr(as))
	}
	b.set("info", info.v())
	if withSessions {
		var ss []py.Value
		causes := append([]string(nil), causeKeys()...)
		sort.Strings(causes)
		for _, s := range R.Sessions {
			o := newObj().set("key", sv(s.Key))
			for _, c := range causes {
				o.set(c, fv(py.RoundNumpy(s.ByCause[c], 4)))
			}
			o.set("leak", fv(py.RoundNumpy(s.Leak, 4))).set("cost", fv(py.RoundNumpy(s.Cost, 4))).set("first", sv(tsStr(s.First))).set("last", sv(tsStr(s.Last))).
				set("tool", sv(s.Tool)).set("project", sv(s.Project)).set("start", sv(tsStr(s.Start))).set("started_before", bv(s.StartedBefore)).
				set("auto_share", fv(py.RoundNumpy(s.AutoShare, 4)))
			ss = append(ss, o.v())
		}
		b.set("sessions", arr(ss))
	}
	return b.v()
}

// JSON is the `leak --json` document: the window with its sessions, the direct/automation split and,
// when given, the previous window.
func JSON(R, H, A, P, PH, PA *Report) py.Value {
	out := R.ToValue(true)
	out.O.Set("by_origin", newObj().set("human", H.ToValue(false)).set("auto", A.ToValue(false)).v())
	if P != nil {
		pv := P.ToValue(false)
		pv.O.Set("by_origin", newObj().set("human", PH.ToValue(false)).set("auto", PA.ToValue(false)).v())
		out.O.Set("previous", pv)
	}
	out.O.Set("currency", sv("USD (API list-price equivalent)"))
	if !config.Legacy {
		out.O.Set("cause_set", iv(CauseSet))
		days, stale := config.Staleness(config.Prices().Read, config.Now())
		pt := newObj().set("read", sv(config.Prices().Read)).set("age_days", iv(days)).set("stale", bv(stale))
		if id, end := config.Prices().EndedPromotion(config.Now()); id != "" {
			pt.set("ended_promotion", newObj().set("model", sv(id)).set("until", sv(end)).v())
		}
		out.O.Set("price_table", pt.v())
	}
	return out
}

// Compare is the `compare` text for two `leak --json` documents (per-cause leak per $100 of spend).
func Compare(A, B py.Value) string {
	T := messages.Compare
	num := func(v py.Value) float64 { f, _ := v.Num(); return f }
	pct := func(X py.Value) float64 {
		if t := num(X.Get("total_usd")); t != 0 {
			return num(X.Get("leak_usd")) / t
		}
		return 0
	}
	out := []string{T["head"], F(T["windows"], "a", py.Head(A.Get("window_start").S, 10), "b", py.Head(B.Get("window_start").S, 10),
		"ta", A.Get("total_usd"), "tb", B.Get("total_usd"))}
	// window lengths, and a warning when they differ or overlap (a busier or shared stretch skews the rates)
	span := func(X py.Value) (int64, int64) {
		return logs.ParseTime(X.Get("window_start").S), logs.ParseTime(X.Get("window_end").S)
	}
	as, ae := span(A)
	bs, be := span(B)
	if as != logs.NaT && ae != logs.NaT && bs != logs.NaT && be != logs.NaT {
		la, lb := float64(ae-as)/864e11, float64(be-bs)/864e11
		out = append(out, F(T["lengths"], "a", la, "b", lb))
		if la > 0 && (lb/la > 1.05 || lb/la < 0.95) {
			out = append(out, T["warn_lengths"])
		}
		if as < be && bs < ae && !(ae-bs <= int64(2*time.Minute)) {
			out = append(out, T["warn_overlap"])
		}
	}
	byKey := func(list py.Value, k string) map[string]py.Value {
		m := map[string]py.Value{}
		for _, x := range list.A {
			m[x.Get(k).S] = x
		}
		return m
	}
	ca, cb := byKey(A.Get("causes"), "cause"), byKey(B.Get("causes"), "cause")
	per100 := func(c, R py.Value) float64 {
		if t := num(R.Get("total_usd")); t != 0 {
			return 100 * num(c.Get("usd")) / t
		}
		return 0
	}
	for _, k := range causeKeys() {
		a, okA := ca[k]
		b, okB := cb[k]
		if !okA || !okB {
			continue
		}
		x, y := per100(a, A), per100(b, B)
		out = append(out, F(T["line"], "x", x, "y", y, "d", y-x, "na", a.Get("count"), "nb", b.Get("count"), "label", messages.Causes[k][0]))
	}
	if causeSet(A) != causeSet(B) { // another set of causes: the totals and habits are not like for like
		out = append(out, T["warn_causes"], T["caveat"])
		return strings.Join(out, "\n")
	}
	lx, ly := 100*pct(A), 100*pct(B)
	out = append(out, F(T["total"], "x", lx, "y", ly, "d", ly-lx))
	habitLabel := func(h py.Value) string {
		if l, ok := messages.Habits[h.Get("habit").S]; ok {
			return l[0]
		}
		return h.Get("label").S
	}
	habitLines := func(X, Y py.Value, indent string) {
		hb := byKey(Y.Get("habits"), "habit")
		for _, h := range X.Get("habits").A {
			y := 0.0
			if o, ok := hb[h.Get("habit").S]; ok && o.Has("per100") {
				y = num(o.Get("per100"))
			}
			x := num(h.Get("per100"))
			out = append(out, indent+F(T["habit_line"], "x", x, "y", y, "d", y-x, "label", habitLabel(h)))
		}
	}
	if A.Has("habits") && B.Has("habits") {
		out = append(out, T["habits"])
		habitLines(A, B, "")
	}
	if A.Has("by_origin") && B.Has("by_origin") {
		for _, o := range []struct{ k, name string }{{"human", messages.Leak["origin_human"]}, {"auto", messages.Leak["origin_auto"]}} {
			X, Y := A.Get("by_origin").Get(o.k), B.Get("by_origin").Get(o.k)
			x, y := 100*pct(X), 100*pct(Y)
			out = append(out, F(T["origin"], "name", o.name, "a", X.Get("total_usd"), "b", Y.Get("total_usd"), "x", x, "y", y, "d", y-x))
			if o.k == "human" {
				habitLines(X, Y, "  ")
			} else {
				yc := byKey(Y.Get("causes"), "cause")
				for _, c := range X.Get("causes").A {
					yv := py.Value{K: py.Int}
					if o, ok := yc[c.Get("cause").S]; ok && o.Has("usd") {
						yv = o.Get("usd")
					}
					if c.Get("usd").Truthy() || yv.Truthy() {
						label := c.Get("label").S
						if l, ok := messages.Causes[c.Get("cause").S]; ok {
							label = l[0]
						}
						out = append(out, F(T["auto_line"], "a", c.Get("usd"), "b", yv, "label", label))
					}
				}
			}
		}
	}
	out = append(out, T["caveat"])
	return strings.Join(out, "\n")
}

// causeSet is the set of causes a `leak --json` file counted (files without the field: 1, up to 0.1.7).
func causeSet(X py.Value) int64 {
	if v := X.Get("cause_set"); v.K == py.Int {
		return v.I
	}
	return 1
}
