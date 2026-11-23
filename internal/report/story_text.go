package report

import (
	"strings"
)

// The terminal summary and the Markdown report follow the HTML's section order (docs/24 §17.8).

func (s *Story) scale(lv string) string {
	if lv == "none" {
		return s.T("drift_none_word")
	}
	if lv == "na" {
		return s.T("rating.na")
	}
	var w []string
	for i, l := range []string{"good", "watch", "attention"} {
		word := s.T("scale." + itoa(i))
		if l == lv {
			w = append(w, "["+strings.ToUpper(word)+"]")
		} else {
			w = append(w, strings.ToLower(word))
		}
	}
	return strings.Join(w, " ")
}

type axisRow struct{ label, lv, fact, dist string }

func (s *Story) axisRows() []axisRow {
	m, st := s.M, s.St
	rows := []axisRow{{s.T("c_drift"), st.Drift, joinDot(s.T("f_drift", "n", st.Serious, "j", m.J), s.CmpTerm(true)), st.DDrift}}
	if !s.DriftRun() {
		rows[0] = axisRow{s.T("c_drift"), "none", s.NoneFact(), s.T("drift_none_cmd", "cmd", s.DriftCmd())}
	}
	if m.HasLeak {
		rows = append(rows, axisRow{s.T("c_cost"), st.Cost, joinDot(s.T("f_cost", "leak", Money(m.LeakUSD), "total", Money(m.TotalUSD)), s.CmpTerm(false)), st.DCost})
	}
	return rows
}

func (s *Story) unit(f *Finding) string {
	if f.Side == "setup" {
		return s.setupFact(f)
	}
	if f.Side == "drift" {
		return s.T("unit_s", "n", f.Sessions, "j", s.M.J)
	}
	return s.T("unit_usd", "usd", Money(f.USD))
}

func joinDot(a, b string) string {
	if b == "" {
		return a
	}
	return a + " · " + b
}

// Terminal is the summary printed by `report` and `drift`: the rating and the two axes, the first fix with its
// example and source, the other fixes in one line each, one going-well line, warnings and the report path.
// html is the report file ("" when no report was written).
func Terminal(m *Model, md, html string) string {
	s := NewStory(m)
	st := s.St
	out := []string{s.T("kicker") + ": " + strings.ToUpper(s.T("rating."+st.Overall)) + "   (" + s.T("period", "a", m.From, "b", m.To) + ")", ""}
	if pl := s.PartsLine(); pl != "" { // first, so an agent relaying the summary keeps the order: what is done, what is not
		out = append([]string{pl, ""}, out...) // one line, not wrapped: agents relay it word for word
	}
	w := 0
	rows := s.axisRows()
	for _, r := range rows {
		w = max(w, Width(r.label))
	}
	for _, r := range rows {
		if r.lv == "none" { // not checked yet: the command on a line of its own, never split
			out = append(out, Wrap(r.label+strings.Repeat(" ", w-Width(r.label))+"  "+s.scale(r.lv)+"  "+r.fact, TermWidth, "  ")...)
			out = append(out, "  "+strings.Repeat(" ", w)+"  "+r.dist)
			continue
		}
		out = append(out, Wrap(r.label+strings.Repeat(" ", w-Width(r.label))+"  "+s.scale(r.lv)+"  "+joinDot(r.fact, r.dist), TermWidth, "  ")...)
	}
	if !s.DriftRun() && st.Cost != "na" {
		out = append(out, Wrap(s.T("rating_cost_only"), TermWidth, "  ")...)
	}
	if s.DriftRun() { // which model checked, per tool
		var js []string
		for _, v := range m.Verdicts {
			js = append(js, s.X.Judge(v))
		}
		out = append(out, Wrap(strings.Join(js, "; "), TermWidth, "  ")...)
	}
	if len(s.Top) > 0 {
		q := s.Plain(s.Top[0])
		out = append(out, "", s.T("first_fix")+":")
		out = append(out, Wrap("1. "+q.Action, TermWidth, "  ")...)
		switch th := s.Thing(s.Top[0]); {
		case th != nil && th.Mode == "theme": // the model's label, not the user's words: no quote marks
			// the finding's size first, then the theme inside it
			out = append(out, Wrap(s.SizeLine(s.Top[0]), TermWidth, "     ")...)
			out = append(out, Wrap(s.WithinLine(th)+" "+Clip(th.Quote, 44), TermWidth, "     ")...)
		case th != nil:
			lead := s.T("eg")
			if th.Mode == "repeat" {
				lead = s.T("rep_short", "gs", th.GS)
			}
			out = append(out, Wrap(lead+" “"+Clip(th.Quote, 44)+"”", TermWidth, "     ")...)
		default: // cost fixes: the specific numbers; the why stays in the report
			out = append(out, Wrap(q.Hook, TermWidth, "     ")...)
		}
		out = append(out, "     "+s.T("source")+": "+q.SrcText())
		for i, f := range s.Top[1:] {
			out = append(out, Wrap(itoa(i+2)+". "+s.Plain(f).Short+" · "+s.unit(f), TermWidth, "  ")...)
		}
	}
	if len(s.Good) > 0 {
		out = append(out, "", s.T("t_good."+s.Good[0].Key, "v", s.Good[0].Value))
	}
	var lo []string
	nj := s.NotJudgedLine()
	for _, l := range s.LeftOutLines() {
		if l != nj {
			lo = append(lo, l)
		}
	}
	extra := append(s.X.Warnings(m), lo...)
	if len(extra) > 0 {
		out = append(out, "")
		for _, x := range extra {
			out = append(out, Wrap("! "+x, TermWidth, "")...)
		}
	}
	out = append(out, "")
	switch {
	case html != "":
		out = append(out, "→ "+html)
	case m.J == 0 && !m.HasLeak:
		out = append(out, s.X.T["no_drift"])
	default:
		out = append(out, s.X.T["scope_more"])
	}
	if nj != "" { // the command, then line ⑤ last, so an agent relays it as the closing question
		out = append(out, "", nj, "", s.AskLine())
	}
	return strings.Join(out, "\n") + "\n"
}

// Markdown is the report as Markdown, in the HTML's order: the opening (rating table, first fix, the others),
// then each fix with its cases, what is going well, the whole picture and the look-up material.
func Markdown(m *Model) string {
	s := NewStory(m)
	st := s.St
	scl := func(lv string) string {
		if lv == "na" {
			return s.T("rating.na")
		}
		return "**" + s.T("scale."+itoa(map[string]int{"good": 0, "watch": 1, "attention": 2}[lv])) + "**"
	}
	md := []string{"# " + s.T("kicker") + ": " + s.T("rating."+st.Overall), "", s.T("period", "a", m.From, "b", m.To), ""}
	md = append(md,
		"| | "+s.T("md_rating")+" | "+s.T("md_detail")+" |", "|:--|:--|:--|",
		"| "+s.T("c_drift")+" | "+scl(st.Drift)+" | "+joinDot(joinDot(s.T("f_drift", "n", st.Serious, "j", m.J), st.DDrift), s.PartialLine())+" |")
	if !s.DriftRun() {
		md[len(md)-1] = "| " + s.T("c_drift") + " | " + s.T("drift_none_word") + " | " + s.NoneFact() + " " + strings.Replace(s.T("drift_none_cmd", "cmd", s.DriftCmd()), s.DriftCmd(), code(s.DriftCmd()), 1) + " |"
	}
	if s.CmpAny() { // a column for the comparison, when a card shows one
		n := len(md)
		md[n-3] += " " + s.T("cmp_md_col") + " |"
		md[n-2] += ":--|"
		md[n-1] += " " + s.CmpTerm(true) + " |"
	}
	if m.HasLeak {
		md = append(md, "| "+s.T("c_cost")+" | "+scl(st.Cost)+" | "+joinDot(s.T("f_cost", "leak", Money(m.LeakUSD), "total", Money(m.TotalUSD))+" ("+Pct(st.Share)+")", st.DCost)+" |")
		if s.CmpAny() {
			md[len(md)-1] += " " + s.CmpTerm(false) + " |"
		}
	}
	md = append(md, "")
	if !s.DriftRun() && st.Cost != "na" {
		md = append(md, "_"+s.T("rating_cost_only")+"_", "")
	}
	for _, w := range s.X.Warnings(m) {
		md = append(md, "> **!** "+w, "")
	}
	if len(s.Top) > 0 {
		f0 := s.Top[0]
		x := s.Plain(f0)
		md = append(md, "### "+s.T("first_fix")+": "+x.Action, "")
		switch th := s.Thing(f0); {
		case th != nil && th.Mode == "theme":
			md = append(md, s.SizeLine(f0)+" "+s.WithinLine(th)+" **"+mq(th.Quote)+"**", "")
		case th != nil:
			lead := s.T("eg")
			if th.Mode == "repeat" {
				lead = s.T("rep_short", "gs", th.GS)
			}
			md = append(md, lead+" “"+mq(Clip(th.Quote, 44))+"”", "")
		default:
			md = append(md, x.Hook, "")
		}
		md = append(md, x.SrcMD(s.T("source"))+" · **→ ["+s.T("show_ev")+"](#"+f0.ID+")**", "")
		if len(s.Top) > 1 {
			var l []string
			for _, f := range s.Top[1:] {
				l = append(l, "["+s.Plain(f).Short+"](#"+f.ID+")")
			}
			md = append(md, s.T("md_then")+": "+strings.Join(l, " · "), "")
		}
	}
	md = append(md, "---", "")
	fixMD := func(i int, f *Finding, head string) []string {
		x := s.Plain(f)
		th := s.Thing(f)
		r := []string{`<a id="` + f.ID + `"></a>`, head + " " + s.T("fix_n", "i", i+1, "n", len(s.Top)) + ": " + x.Action, ""}
		if th != nil && th.Mode == "theme" {
			_, ctx := s.ThingText(f, th)
			r = append(r, "**"+ctx+"**", "", "> "+s.WithinLine(th)+"  ", "> **"+mq(th.Theme.Label)+"**  ", "> "+s.T("th_words")+":  ")
			for _, q := range th.Quotes {
				r = append(r, "> “"+mq(q)+"”  ")
			}
			r = append(r, "> _"+s.T("th_by", "tool", s.X.Tool(th.Theme.Tool), "draft", map[bool]string{true: s.T("th_by_draft")}[s.Draft(f, th) != nil])+"_", "")
			if ml := s.MoreLine(th); ml != "" {
				r = append(r, mq(ml), "")
			}
		} else if th != nil {
			lab, ctx := s.ThingText(f, th)
			r = append(r, "**"+ctx+"**", "", "> "+mq(lab)+": “"+mq(th.Quote)+"”", "")
		} else {
			r = append(r, "**"+x.Hook+"**", "")
		}
		r = append(r, x.Why+" "+x.SrcMD(s.T("source")), "", "_"+s.T("how_sure")+": "+s.X.Level(f.Level)+joinPre(s.X.Notes(f), " · ")+"_", "")
		if d := s.Draft(f, th); d != nil {
			r = append(r, d.Label, "", "```text", d.Line, "```", "")
		}
		return append(r, s.setupFixMD(f)...)
	}
	evMD := func(f *Finding) []string {
		var r []string
		for _, e := range s.CasesFor(f, s.Thing(f))[:min(2, len(s.CasesFor(f, s.Thing(f))))] {
			fn := fileOf[e.Rec.Tool]
			capKey := "cap." + Key(f)
			if _, ok := s.S[capKey]; !ok {
				capKey = "cap.MISTAKE"
			}
			r = append(r, "**"+s.T(capKey, "file", fn)+"**", "")
			if e.InstrQuote != "" {
				who := s.T("you_before")
				if strings.HasPrefix(e.InstrLoc, "I") {
					who = s.T("rule_in", "file", fn)
				}
				r = append(r, "> "+who+": “"+mq(e.InstrQuote)+"”  ")
			}
			if e.Summary != "" {
				r = append(r, "> "+s.T("agent_did")+": "+mq(ShortText(e.Summary, 150))+"  ")
			}
			res := e.Resolution
			if res == "" {
				res = "unknown"
			}
			r = append(r, "> "+s.T("you_after")+": “"+mq(e.Quote)+"”", "", "→ "+s.T("led."+res), "")
		}
		for _, c := range f.Copy {
			r = append(r, code(s.X.Scope(c)), "", "```text")
			for _, l := range c.Lines {
				r = append(r, l.Text)
			}
			r = append(r, "```", "")
		}
		return r
	}
	for i, f := range s.Top {
		md = append(md, fixMD(i, f, "##")...)
		md = append(md, evMD(f)...)
	}
	if len(s.Good) > 0 {
		md = append(md, "## "+s.T("good_h"), "")
		for _, g := range s.Good {
			md = append(md, "- **"+g.Value+"** "+g.Text)
		}
		md = append(md, "")
	}
	drows, crows := s.PictureRows()
	md = append(md, "## "+s.T("pic_h"), "")
	if len(drows) > 0 {
		md = append(md, "### "+s.T("pic8_drift_h"), "", s.T("pic8_drift_lead", "a", drows[0].Label, "n", int(drows[0].Value), "j", m.J), "",
			"| | "+s.T("md_detail")+" |", "|:--|--:|")
		for _, r := range drows {
			md = append(md, "| "+r.Label+" | "+s.T("pic8_drift_val", "n", int(r.Value), "j", m.J)+" |")
		}
		md = append(md, "", "_"+s.T("pic8_drift_cap")+"_", "")
		if f := s.mistakeNotOnTop(); f != nil {
			md = append(md, s.T("mistake_note", "n", f.Sessions), "")
		}
	}
	if m.HasLeak && len(crows) > 0 {
		lt := maxf(m.LeakUSD, 1e-9)
		md = append(md, "### "+s.T("pic8_cost_h"), "", s.T("pic8_cost_lead", "a", crows[0].Label, "usd", Money(crows[0].Value), "total", Money(m.LeakUSD), "spend", Money(m.TotalUSD)), "",
			"| | "+s.T("md_detail")+" |", "|:--|--:|")
		for _, r := range crows {
			md = append(md, "| "+r.Label+" | "+Money(r.Value)+" ("+Pct(r.Value/lt)+") |")
		}
		md = append(md, "", "_"+s.T("pic8_cost_cap", "total", Money(m.LeakUSD))+"_", "")
	}
	if !s.DriftRun() {
		md = append(md, "### "+s.T("pic8_drift_h"), "", s.NoneFact()+" "+s.T("drift_what")+" "+strings.Replace(s.T("drift_none_cmd", "cmd", s.DriftCmd()), s.DriftCmd(), code(s.DriftCmd()), 1), "")
	} else {
		md = append(md, "### "+map[bool]string{true: s.T("pic_judge"), false: s.T("pic_judge1")}[len(m.Verdicts) > 1], "")
	}
	for _, v := range m.Verdicts {
		var parts []string
		for _, k := range Verdicts {
			if n := v.Counts[k]; n > 0 {
				parts = append(parts, s.T("leg.v_"+k)+" "+itoa(n))
			}
		}
		md = append(md, "- "+s.X.Judge(v)+": "+strings.Join(parts, " · "))
	}
	if nc := s.noCompare(); nc != "" {
		md = append(md, "", "_"+nc+"_")
	}
	for _, l := range s.LeftOutLines() {
		md = append(md, "", l)
	}
	md = append(md, "", "## "+s.T("look_h"), "", s.T("look8_sub"), "")
	if others := s.Others(); len(others) > 0 {
		md = append(md, "### "+s.T("look_more"), "")
		for _, f := range others {
			x := s.Plain(f)
			md = append(md, `<a id="`+f.ID+`"></a>`, "#### "+x.Action, "", "**"+x.Hook+"** "+x.Why+" "+x.SrcMD(x.SrcName), "",
				"_"+s.T("how_sure")+": "+s.X.Level(f.Level)+joinPre(s.X.Notes(f), " · ")+"_", "")
			md = append(md, s.setupFixMD(f)...)
			md = append(md, evMD(f)...)
		}
	}
	md = append(md, s.setupMD()...)
	if len(sessionsWorstFirst(m)) > 0 {
		md = append(md, "### "+s.T("look_sess"), "", s.T("look8_sess_lead", "n", len(sessionsWorstFirst(m)), "j", m.J), "")
	}
	for _, r := range sessionsWorstFirst(m) {
		tags := ""
		if r.Partial {
			tags += " · " + s.X.T["partial"]
		}
		if r.Origin == "uncertain" {
			tags += " · " + s.X.T["origin_uncertain"]
		}
		md = append(md, "#### "+s.T("leg.v_"+r.Verdict)+" · "+mq(r.Project)+" · "+code(r.Short())+" · "+s.X.Tool(r.Tool)+tags, "")
		for _, e := range r.Events {
			md = append(md, "- "+e.Loc+" · "+s.X.Rule(e.Rule)+" · "+s.X.SizeWord(e.Size)+": “"+mq(e.Quote)+"”", "  - "+s.X.T["judge_note"]+": "+mq(e.Summary))
		}
		md = append(md, "")
	}
	md = append(md, "### "+s.T("look_method"), "")
	for _, r := range s.MethodRows() {
		md = append(md, "- **"+r[0]+"**: "+r[1])
	}
	for _, n := range s.Notes() {
		md = append(md, "- "+n)
	}
	md = append(md, "", "["+s.T("m_more")+"]("+docsURL()+")", "")
	return strings.Join(md, "\n")
}
