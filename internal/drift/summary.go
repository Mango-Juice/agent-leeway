package drift

import (
	"github.com/Mango-Juice/agent-leeway/internal/secrets"
	"sort"
	"strings"

	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// latest keeps one record per (session, tier): the most recent successful one.
func latest(records []py.Value) []py.Value {
	type k struct{ key, tier string }
	best := map[k]int{}
	var order []k
	var out []py.Value
	for _, r := range records {
		if r.Get("error").Truthy() || !r.Get("out").Truthy() {
			continue
		}
		kk := k{r.Get("key").S, r.Get("tier").S}
		ts := func(v py.Value) float64 { f, _ := v.Get("ts").Or(py.Value{K: py.Int}).Num(); return f }
		if i, ok := best[kk]; !ok {
			best[kk] = len(out)
			order = append(order, kk)
			out = append(out, r)
		} else if ts(r) >= ts(out[i]) {
			out[i] = r
		}
	}
	_ = order
	return out
}

// Latest keeps one record per (session, tier): the most recent successful one.
func Latest(records []py.Value) []py.Value { return latest(records) }

// MaskedTexts counts the texts of a record's events that are shown with a secret masked: every evidence quote,
// the cited instruction quote, the suggested sentence and the summary.
func MaskedTexts(r py.Value) int {
	n := 0
	count := func(v py.Value) {
		if v.K == py.Str {
			if _, ok := secrets.Masked(v.S); ok {
				n++
			}
		}
	}
	for _, e := range listOf(r.Get("out").Get("events")) {
		for _, q := range listOf(e.Get("evidence")) {
			count(q.Get("quote"))
		}
		count(e.Get("instruction").Get("quote"))
		count(e.Get("fix_sentence"))
		count(e.Get("summary"))
	}
	return n
}

// ClipText collapses whitespace and shortens to n code points with "…".
func ClipText(v py.Value, n int) string {
	s := ""
	if v.Truthy() {
		s = py.PyStr(v)
	}
	s = strings.Join(py.Fields(s), " ")
	if py.Len(s) <= n {
		return s
	}
	return py.Head(s, n) + "…"
}

// counter counts in first-seen order (Python's Counter / dict order).
type counter struct {
	keys []py.Value
	n    map[string]int
}

func newCounter() *counter { return &counter{n: map[string]int{}} }

func (c *counter) add(k py.Value, by int) {
	h := hashKey(k)
	if _, ok := c.n[h]; !ok {
		c.keys = append(c.keys, k)
	}
	c.n[h] += by
}

func (c *counter) value() py.Value {
	o := &py.Object{}
	for _, k := range c.keys {
		o.Set(py.PyStr(k), py.Value{K: py.Int, I: int64(c.n[hashKey(k)])})
	}
	return py.Value{K: py.Obj, O: o}
}

// Summarize aggregates judged records for the report (no model calls). Verdicts are recomputed in code.
func Summarize(records []py.Value, quoteChars int) py.Value {
	recs := latest(records)
	verdicts, classes, details, sizes, models := newCounter(), newCounter(), newCounter(), newCounter(), newCounter()
	clis := newCounter()
	nPartial, omitted, nCustom, nUncertain, nMasked := 0, 0, 0, 0, 0
	var sessions, fixes []py.Value
	nDefects, nWeak, quotes, bad, dropped, instrWarn := 0, 0, 0, 0, 0, 0
	rv := map[string]bool{}
	str := py.String
	null := py.Value{}
	type sess struct {
		v     py.Value
		order int
		nev   int
	}
	var ss []sess
	order := map[string]int{}
	for i, v := range Verdicts {
		order[v] = i
	}
	for _, r := range recs {
		out := r.Get("out")
		st := SettingsFromValue(r.Get("settings"))
		ev := listOf(out.Get("events"))
		th := 3
		if r.Get("settings").Truthy() {
			th = st.S1Threshold()
		}
		eff := EffectiveSizes(ev, th)
		verdict := r.Get("derived")
		if !verdict.Truthy() {
			s := st
			if !r.Get("settings").Truthy() {
				s = nil
			}
			verdict = str(DerivedVerdict(out, s))
		}
		verdicts.add(verdict, 1)
		models.add(r.Get("model").Or(r.Get("tier")), 1)
		if v := r.Get("rubric_version"); v.Truthy() {
			rv[py.PyStr(v)] = true
		}
		if p := r.Get("partial"); p.K == py.Obj {
			nPartial++
			if n, ok := p.Get("omitted_chars").Num(); ok {
				omitted += int(n)
			}
		}
		if isStrV(r.Get("rubric_source"), "custom") {
			nCustom++
		}
		if isStrV(r.Get("origin"), "uncertain") {
			nUncertain++
		}
		if v := r.Get("judge_cli"); v.K == py.Str {
			clis.add(v, 1)
		}
		if r.Get("instr_warning").Truthy() {
			instrWarn++
		}
		vf := r.Get("verify")
		n := func(v py.Value) int { f, _ := v.Or(py.Value{K: py.Int}).Num(); return int(f) }
		quotes += n(vf.Get("quotes"))
		bad += n(vf.Get("bad_quotes"))
		dropped += n(r.Get("dropped_events"))
		nDefects += len(listOf(out.Get("defect_reports")))
		nWeak += len(listOf(out.Get("weak_signals")))
		nMasked += MaskedTexts(r)
		var evs []py.Value
		for i, e := range ev {
			cause := e.Get("cause")
			cls := "undeterminable"
			if cause.K == py.Str {
				if c, ok := CauseSide[cause.S]; ok {
					cls = c
				}
			}
			classes.add(str(cls), 1)
			details.add(cause, 1)
			sizes.add(str(eff[i]), 1)
			q := py.Value{K: py.Obj, O: &py.Object{}}
			if l := listOf(e.Get("evidence")); len(l) > 0 && e.Get("evidence").Truthy() {
				q = l[0]
			}
			ins := e.Get("instruction")
			o := &py.Object{}
			o.Set("loc", e.Get("loc"))
			o.Set("rule", e.Get("rule"))
			o.Set("cause_class", str(cls))
			o.Set("cause", cause)
			o.Set("size", str(eff[i]))
			o.Set("summary", str(MaskedClip(e.Get("summary"), 160)))
			o.Set("excerpt", str(MaskedClip(q.Get("quote"), quoteChars)))
			if ins.K == py.Obj {
				o.Set("instruction", ins.Get("loc"))
			} else {
				o.Set("instruction", null)
			}
			o.Set("resolution", e.Get("resolution"))
			evs = append(evs, py.Value{K: py.Obj, O: o})
			if isStrV(cause, "missing_or_ambiguous_instruction") && e.Get("fix_sentence").Truthy() {
				f := &py.Object{}
				f.Set("key", r.Get("key"))
				f.Set("project", r.Get("project"))
				f.Set("sentence", str(MaskedClip(e.Get("fix_sentence"), 200)))
				fixes = append(fixes, py.Value{K: py.Obj, O: f})
			}
		}
		var defects []py.Value
		for _, d := range listOf(out.Get("defect_reports")) {
			o := &py.Object{}
			o.Set("loc", d.Get("loc"))
			o.Set("summary", str(MaskedClip(d.Get("summary"), 160)))
			defects = append(defects, py.Value{K: py.Obj, O: o})
		}
		s := &py.Object{}
		s.Set("key", r.Get("key"))
		s.Set("tool", r.Get("tool"))
		s.Set("project", r.Get("project"))
		s.Set("tier", r.Get("tier"))
		s.Set("verdict", verdict)
		s.Set("model_verdict", out.Get("verdict"))
		s.Set("events", arrOf(evs))
		s.Set("defects", arrOf(defects))
		s.Set("instr_warning", r.Get("instr_warning"))
		o := 9
		if verdict.K == py.Str {
			if x, ok := order[verdict.S]; ok {
				o = x
			}
		}
		ss = append(ss, sess{py.Value{K: py.Obj, O: s}, o, len(evs)})
	}
	sort.SliceStable(ss, func(a, b int) bool {
		if ss[a].order != ss[b].order {
			return ss[a].order < ss[b].order
		}
		return ss[a].nev > ss[b].nev
	})
	for _, s := range ss {
		sessions = append(sessions, s.v)
	}
	var versions []string
	for v := range rv {
		versions = append(versions, v)
	}
	sort.Strings(versions)
	var vs, lim []py.Value
	for _, v := range versions {
		vs = append(vs, str(v))
	}
	for _, l := range messages.Limitations {
		lim = append(lim, str(l))
	}
	D := &py.Object{}
	i := func(x int) py.Value { return py.Value{K: py.Int, I: int64(x)} }
	D.Set("n_sessions", i(len(recs)))
	D.Set("verdicts", verdicts.value())
	D.Set("cause_classes", classes.value())
	D.Set("causes", details.value())
	D.Set("sizes", sizes.value())
	D.Set("n_defects", i(nDefects))
	D.Set("n_weak", i(nWeak))
	cc := &py.Object{}
	cc.Set("quotes", i(quotes))
	cc.Set("not_found", i(bad))
	cc.Set("dropped_events", i(dropped))
	D.Set("citation_check", py.Value{K: py.Obj, O: cc})
	D.Set("models", models.value())
	D.Set("rubric_versions", arrOf(vs))
	D.Set("instr_current_sessions", i(instrWarn))
	D.Set("sessions", arrOf(sessions))
	D.Set("fix_sentences", arrOf(fixes))
	D.Set("limitations", arrOf(lim))
	// how the judgments were made (Go additions): partly judged sessions, custom rubrics, unclear origins, CLIs
	D.Set("partial_sessions", i(nPartial))
	D.Set("omitted_chars", i(omitted))
	D.Set("custom_rubric_sessions", i(nCustom))
	D.Set("origin_uncertain_sessions", i(nUncertain))
	D.Set("judge_clis", clis.value())
	D.Set("masked_quotes", i(nMasked))
	return py.Value{K: py.Obj, O: D}
}

func arrOf(xs []py.Value) py.Value {
	if xs == nil {
		xs = []py.Value{}
	}
	return py.Value{K: py.Arr, A: xs}
}

// MaskedClip is ClipText of the text with secrets masked first (so a token cut by the clip is still masked).
// Display copies only: the stored results and the citation check keep the original text.
func MaskedClip(v py.Value, n int) string {
	if v.Truthy() {
		v = py.String(secrets.Mask(py.PyStr(v)))
	}
	return ClipText(v, n)
}
