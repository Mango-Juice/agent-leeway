package report

import (
	"fmt"
	"html"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

// The HTML report (docs/24 §17): one self-contained page of full-screen sections. Everything is in the markup in
// reading order; the inline script only adds snapping (when every section fits), the position dots, the case
// stepper and the evidence dialog. No external resource is referenced (CSP: inline style and script only).

// E escapes text for HTML.
func E(s string) string { return html.EscapeString(s) }

func fmt1(x float64) string { return fmt.Sprintf("%.1f", x) }

// dots is a session dot grid: n of all filled.
func dots(on, all int, cls string, perRow int, r, gap float64) string {
	if all <= 0 {
		return ""
	}
	rows := int(math.Ceil(float64(all) / float64(perRow)))
	w, h := float64(perRow)*(2*r+gap), float64(rows)*(2*r+gap)
	var b strings.Builder
	for i := 0; i < all; i++ {
		x := float64(i%perRow)*(2*r+gap) + r
		y := float64(i/perRow)*(2*r+gap) + r
		c := "dot-off"
		if i < on {
			c = cls
		}
		fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="%s" class="%s"/>`, trimF(x), trimF(y), trimF(r), c)
	}
	return fmt.Sprintf(`<svg class="dots" viewBox="0 0 %s %s" role="img" aria-label="%d / %d">%s</svg>`, trimF(w), trimF(h), on, all, b.String())
}

func trimF(x float64) string {
	if x == math.Trunc(x) {
		return fmt.Sprintf("%d", int(x))
	}
	return fmt1(x)
}

// waffle is 100 squares, parts filled in order (count, class).
func waffle(parts [][2]any) string {
	var cls []string
	for _, p := range parts {
		for i := 0; i < p[0].(int); i++ {
			cls = append(cls, p[1].(string))
		}
	}
	const cols, rows, sz, gap = 25, 4, 8, 3
	var b strings.Builder
	for i := 0; i < cols*rows; i++ {
		c := "dot-off"
		if i < len(cls) {
			c = cls[i]
		}
		fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="%d" rx="1.5" class="%s"/>`, (i%cols)*(sz+gap), (i/cols)*(sz+gap), sz, sz, c)
	}
	return fmt.Sprintf(`<svg class="dots" viewBox="0 0 %d %d" role="img" aria-label="%d / 100">%s</svg>`, cols*(sz+gap), rows*(sz+gap), min(len(cls), 100), b.String())
}

// track is the zoned scale with the exact position marked.
func track(value float64, cuts [2]float64, vmax float64, words [3]string, lv string) string {
	const W = 300.0
	pos := func(v float64) float64 { return W * math.Min(v, vmax) / vmax }
	z := [][3]any{{0.0, cuts[0], "z-good"}, {cuts[0], cuts[1], "z-watch"}, {cuts[1], vmax, "z-attention"}}
	lvs := []string{"good", "watch", "attention"}
	var b strings.Builder
	for i, x := range z {
		on := ""
		if lvs[i] == lv {
			on = " on"
		}
		a, c := pos(x[0].(float64)), pos(x[1].(float64))
		fmt.Fprintf(&b, `<rect class="%s%s" x="%s" y="10" width="%s" height="8" rx="2"/>`, x[2], on, fmt1(a), fmt1(c-a-2))
	}
	xv := pos(value)
	fmt.Fprintf(&b, `<path class="mk" d="M%s 0 L%s 0 L%s 8 Z"/><rect class="mk" x="%s" y="8" width="2" height="12"/>`, fmt1(xv-6), fmt1(xv+6), fmt1(xv), fmt1(xv-1))
	for i, x := range z {
		on := ""
		if lvs[i] == lv {
			on = " on"
		}
		fmt.Fprintf(&b, `<text class="zl%s" x="%s" y="34" text-anchor="middle">%s</text>`, on, fmt1((pos(x[0].(float64))+pos(x[1].(float64)))/2), E(words[i]))
	}
	return `<svg class="track" viewBox="0 -2 300 40" aria-hidden="true">` + b.String() + `</svg>`
}

type barRow struct {
	Label string
	Value float64
}

// hbars is labelled horizontal bars with the value at the bar end.
func hbars(rows []barRow, width float64, format func(float64) string, cls string) string {
	if len(rows) == 0 {
		return ""
	}
	mx := 0.0
	for _, r := range rows {
		mx = math.Max(mx, r.Value)
	}
	if mx == 0 {
		mx = 1
	}
	var b strings.Builder
	var aria []string
	for i, r := range rows {
		y := float64(i * 30)
		w := math.Max((width-70)*r.Value/mx, 2)
		lab := r.Label
		if utf8.RuneCountInString(lab) > 62 {
			lab = string([]rune(lab)[:61]) + "…"
		}
		fmt.Fprintf(&b, `<text class="lbl" x="0" y="%s">%s</text><rect class="f-track" x="0" y="%s" width="%s" height="8" rx="2"/><rect class="%s" x="0" y="%s" width="%s" height="8" rx="2"/><text class="lbl-s" x="%s" y="%s">%s</text>`,
			trimF(y+10), E(lab), trimF(y+15), trimF(width-70), cls, trimF(y+15), fmt1(w), trimF(width-64), trimF(y+23), E(format(r.Value)))
		aria = append(aria, r.Label+" "+format(r.Value))
	}
	return fmt.Sprintf(`<svg class="chart" viewBox="0 0 %s %d" role="img" aria-label="%s">%s</svg>`, trimF(width), len(rows)*30, E(strings.Join(aria, "; ")), b.String())
}

func (s *Story) verdictChart(v JudgeVerdicts, width float64) string {
	n := float64(v.Total)
	if n == 0 {
		n = 1
	}
	x := 0.0
	var segs strings.Builder
	var aria, leg []string
	for _, k := range Verdicts {
		c := v.Counts[k]
		if c == 0 {
			continue
		}
		w := width * float64(c) / n
		fmt.Fprintf(&segs, `<rect class="f-%s" x="%s" y="0" width="%s" height="18" rx="2"/>`, k, fmt1(x), fmt1(math.Max(w-2, 1)))
		if w > 22 {
			fmt.Fprintf(&segs, `<text class="lbl-w" x="%s" y="13" text-anchor="middle">%d</text>`, fmt1(x+w/2-1), c)
		}
		x += w
		word := s.T("leg.v_" + k)
		aria = append(aria, fmt.Sprintf("%s %d", word, c))
		leg = append(leg, fmt.Sprintf(`<li><span class="sw f-%s"></span>%s %d</li>`, k, E(word), c))
	}
	lab := s.X.Judge(v)
	return fmt.Sprintf(`<figure class="vchart"><figcaption class="sm t2">%s <span class="t3">· %d</span></figcaption><svg class="chart" viewBox="0 0 %s 18" role="img" aria-label="%s">%s</svg><ul class="legend">%s</ul></figure>`,
		E(lab), v.Total, trimF(width), E(lab+": "+strings.Join(aria, " · ")), segs.String(), strings.Join(leg, ""))
}

func (s *Story) badge(level string) string {
	return fmt.Sprintf(`<span class="badge %s" title="%s"><i></i>%s</span>`, level, E(s.X.T["sure_"+level+"_d"]), E(s.X.T["sure_"+level]))
}

func (s *Story) copyButton() string {
	return fmt.Sprintf(`<button type="button" hidden data-copy data-done="%s">%s<span>%s</span></button>`, E(s.X.T["copied"]), iconCopy, E(s.X.T["copy_btn"]))
}

func aLink(url, text string) string {
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
		return fmt.Sprintf(`<a href="%s" rel="noreferrer">%s</a>`, E(url), E(text))
	}
	return E(text)
}

func (s *Story) sourcesHTML(f *Finding) string {
	var b strings.Builder
	for _, a := range f.Advice {
		pre := ""
		if len(f.Advice) > 1 {
			pre = `<span class="tool">` + E(s.X.Tool(a.Tool)) + `</span>`
		}
		b.WriteString(`<ul class="src">`)
		for _, src := range a.A.Sources {
			fmt.Fprintf(&b, `<li>%s%s: %s <span class="t3">· %s</span><br><span class="claim">“%s”</span></li>`, pre, E(s.X.Basis(a.A.Basis)), aLink(src.URL, src.Title), E(src.Read), E(src.Claim))
		}
		b.WriteString(`</ul>`)
	}
	for _, x := range f.Against {
		fmt.Fprintf(&b, `<p class="against">%s: %s — %s</p>`, E(s.X.T["against"]), E(pickLabel(x.Note, s.Lang)), aLink(x.URL, x.Title))
	}
	return b.String()
}

var wordRe = regexp.MustCompile(`[\p{L}\p{M}\p{N}_]+`)

// emph marks the words a message repeats from the earlier instruction: 3+ characters, and only when they are a
// minority of the message.
func emph(text, other string) string {
	shared := map[string]bool{}
	to := tokens(other)
	for t := range tokens(text) {
		if to[t] && utf8.RuneCountInString(t) >= 3 {
			shared[t] = true
		}
	}
	words := wordRe.FindAllString(text, -1)
	n := 0
	for _, w := range words {
		if shared[Norm(w)] {
			n++
		}
	}
	if len(shared) == 0 || float64(n) > float64(len(words))/2 {
		return E(text)
	}
	var b strings.Builder
	last := 0
	for _, loc := range wordRe.FindAllStringIndex(text, -1) {
		b.WriteString(E(text[last:loc[0]]))
		w := text[loc[0]:loc[1]]
		if shared[Norm(w)] {
			b.WriteString("<mark>" + E(w) + "</mark>")
		} else {
			b.WriteString(E(w))
		}
		last = loc[1]
	}
	b.WriteString(E(text[last:]))
	return b.String()
}

func (s *Story) caseHTML(f *Finding, e *Event) string {
	k := Key(f)
	fn := fileOf[e.Rec.Tool]
	side := e.Side
	if side != "agent_side" && side != "prompt_side" {
		side = "agent_side"
		if k == "PROMPT" {
			side = "prompt_side"
		}
	}
	capKey := "cap." + k
	if _, ok := s.S[capKey]; !ok {
		capKey = "cap.MISTAKE"
	}
	var turns []string
	if e.InstrQuote != "" {
		if strings.HasPrefix(e.InstrLoc, "I") {
			turns = append(turns, fmt.Sprintf(`<div class="t6 rule"><p class="who">%s</p><p class="say">“%s”</p></div>`, E(s.T("rule_in", "file", fn)), E(e.InstrQuote)))
		} else {
			turns = append(turns, fmt.Sprintf(`<div class="t6 me"><p class="who">%s</p><p class="say">“%s”</p></div>`, E(s.T("you_before")), E(e.InstrQuote)))
		}
	}
	if e.Summary != "" {
		turns = append(turns, fmt.Sprintf(`<div class="t6 ag"><p class="who"><span class="av" aria-hidden="true"></span>%s</p><p class="say">%s</p></div>`, E(s.T("agent_did")), E(ShortText(e.Summary, 240))))
	}
	if !SameWords(e.Quote, e.InstrQuote) {
		turns = append(turns, fmt.Sprintf(`<div class="t6 me"><p class="who">%s</p><p class="say">“%s”</p></div>`, E(s.T("you_after")), emph(e.Quote, e.InstrQuote)))
	}
	if len(turns) > 3 {
		turns = turns[len(turns)-3:]
	}
	res := e.Resolution
	ico := map[string]string{"resolved": "✓", "user_took_over": "↺", "unresolved": "✕", "unknown": "?"}
	if _, ok := ico[res]; !ok {
		res = "unknown"
	}
	return fmt.Sprintf(`<li class="case"><p class="cap6">%s <span class="side %s">%s</span></p><div class="chat">%s</div><p class="led led-%s"><span aria-hidden="true">%s</span> %s</p><details class="which"><summary>%s</summary><p>%s · %s · <span class="mono">%s</span></p></details></li>`,
		E(s.T(capKey, "file", fn)), side, E(s.T("side."+side)), strings.Join(turns, ""), res, ico[res], E(s.T("led."+res)), E(s.T("which")),
		E(orQ(e.Rec.Project)), E(s.X.Tool(e.Rec.Tool)), E(e.Rec.Short()))
}

func orQ(s string) string {
	if s == "" {
		return "?"
	}
	return s
}

func (s *Story) draftBox(d *Draft) string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf(`<div class="copybox draft"><div class="copyhead"><span>%s</span>%s</div><pre>%s</pre></div>`, E(d.Label), s.copyButton(), E(d.Line))
}

// evidenceBody is "what happened" (one case at a time), "how to fix it" and the details.
func (s *Story) evidenceBody(f *Finding, evs []*Event, wrap bool) string {
	var b strings.Builder
	if len(evs) > 0 {
		hidden := ""
		if len(evs) < 2 {
			hidden = " hidden"
		}
		fmt.Fprintf(&b, `<h3>%s</h3><div class="cases" data-cases data-fmt="%s"><div class="cnav noprint"%s><button type="button" class="cprev" aria-label="%s">←</button><p class="cpos" aria-live="polite">%s</p><button type="button" class="cnext" aria-label="%s">→</button></div><ol class="clist">`,
			E(s.T("what_hap")), E(s.S["case_of"]), hidden, E(s.T("prev")), E(s.T("case_of", "i", 1, "n", len(evs))), E(s.T("nextc")))
		for _, e := range evs {
			b.WriteString(s.caseHTML(f, e))
		}
		fmt.Fprintf(&b, `</ol><p class="sel">%s</p></div>`, E(s.T("sel6")))
	}
	th := s.Thing(f)
	fmt.Fprintf(&b, `<h3>%s</h3>`, E(s.T("how_fix")))
	if Key(f) == "BROKE_CHAT" && (th == nil || th.Mode == "example") {
		p := s.Plain(f)
		fmt.Fprintf(&b, `<p class="tooldo"><b>%s.</b> %s</p><p class="tooldo t2">%s</p>`, E(strings.TrimRight(p.Action, ".")), E(p.Why), E(s.T("if_every")))
	}
	for _, a := range f.Advice {
		b.WriteString(`<p class="tooldo">`)
		if len(f.Advice) > 1 {
			b.WriteString(`<span class="tool">` + E(s.X.Tool(a.Tool)) + `</span>`)
		}
		b.WriteString(E(s.X.Action(a)))
		if len(a.A.How) > 0 {
			var hs []string
			for _, h := range a.A.How {
				hs = append(hs, "<code>"+E(h)+"</code>")
			}
			b.WriteString(" → " + strings.Join(hs, ", "))
		}
		b.WriteString(`</p>`)
	}
	b.WriteString(s.draftBox(s.Draft(f, th)))
	fmt.Fprintf(&b, `<details class="more5" data-fold><summary>%s %s</summary><dl class="facts5"><dt>%s</dt><dd>%s</dd><dt>%s</dt><dd>%s</dd></dl><h3>%s</h3>%s</details>`,
		iconChev, E(s.T("more")), E(s.T("how_often")), E(s.X.Size(s.M, f)), E(s.T("how_sure")), s.badge(f.Level)+notesHTML(s.X.Notes(f)), E(s.T("all_sources")), s.sourcesHTML(f))
	if wrap {
		return `<div class="evb">` + b.String() + `</div>`
	}
	return b.String()
}

// sessionsHTML is every session with problems, worst first.
func (s *Story) sessionsHTML() string {
	x := s.X
	var b strings.Builder
	fmt.Fprintf(&b, `<p class="sm t2">%s · %s</p>`, E(x.Tr("sess_ok", "n", countVerdict(s.M, "as_intended"))), E(x.Tr("sess_unclear", "n", countVerdict(s.M, "undeterminable"))))
	for _, r := range sessionsWorstFirst(s.M) {
		tags := ""
		if r.Partial {
			tags += ` <span class="badge">` + E(x.T["partial"]) + `</span>`
		}
		if r.Origin == "uncertain" {
			tags += ` <span class="badge">` + E(x.T["origin_uncertain"]) + `</span>`
		}
		var ev strings.Builder
		for _, e := range r.Events {
			fmt.Fprintf(&ev, `<li><span class="evmeta">%s · %s · %s</span><blockquote><span class="who">%s</span>%s</blockquote><p class="xs t3">%s: %s</p></li>`,
				E(e.Loc), E(x.Rule(e.Rule)), E(x.SizeWord(e.Size)), E(x.T["you"]), E(e.Quote), E(x.T["judge_note"]), E(e.Summary))
		}
		for _, l := range []string{map[bool]string{true: x.Tr("defects", "n", r.Defects)}[r.Defects > 0], map[bool]string{true: x.Tr("weak", "n", r.Weak)}[r.Weak > 0]} {
			if l != "" {
				fmt.Fprintf(&ev, `<li class="xs t3">%s</li>`, E(l))
			}
		}
		fmt.Fprintf(&b, `<details data-fold class="sess" id="s-%s"><summary>%s<span class="dot f-%s"></span><span class="sname">%s · %s%s</span><span class="sside t3 xs"><span class="mono">%s</span> · %s · %d</span></summary><ul class="ev sbody">%s</ul></details>`,
			E(r.Short()), iconChev, E(r.Verdict), E(s.T("leg.v_"+r.Verdict)), E(orQ(r.Project)), tags, E(r.Short()), E(s.X.Tool(r.Tool)), len(r.Events), ev.String())
	}
	return b.String()
}

// MethodRows are the method's rows (label, text); the levels table follows the second row.
func (s *Story) MethodRows() [][2]string {
	m := s.M
	vers := strings.Join(m.RubricVersions, ", ")
	if vers == "" {
		vers = "?"
	}
	pr := m.PricesRead
	if pr == "" {
		pr = "?"
	}
	n := m.NSessions
	if n < 0 {
		n = m.J
	}
	var rows [][2]string
	for i := 0; i < 4; i++ {
		rows = append(rows, [2]string{s.T(fmt.Sprintf("m_rows.%d.0", i)), s.T(fmt.Sprintf("m_rows.%d.1", i), "a", m.From, "b", m.To, "n", n, "v", vers, "d", pr)})
	}
	return rows
}

// Notes are the facts about how this report was made that apply (edited rubric, partly judged sessions, unclear
// origin, masked quotes, judge programs, old data, sessions left out); shown as one extra method row.
func (s *Story) Notes() []string {
	x, m := s.X, s.M
	var out []string
	out = append(out, x.Warnings(m)...)
	if m.PartialSessions > 0 {
		out = append(out, x.Tr("m_partial", "n", m.PartialSessions, "c", Thousands(m.OmittedChars)))
	}
	if m.OriginUncertain > 0 {
		out = append(out, x.Tr("m_origin", "n", m.OriginUncertain))
	}
	if m.InstrCurrent > 0 {
		out = append(out, x.Tr("m_instr", "n", m.InstrCurrent))
	}
	if m.Masked > 0 {
		out = append(out, x.Tr("m_masked", "n", m.Masked))
	}
	if len(m.CLIs) > 0 {
		out = append(out, x.Tr("m_cli", "x", strings.Join(m.CLIs, ", ")))
	}
	if m.J > 0 {
		out = append(out, x.Tr("m_cite", "q", m.Quotes, "b", m.Bad, "d", m.Dropped))
	}
	if len(m.Themes) > 0 {
		out = append(out, s.T("m_themes", "n", len(m.Themes)))
	}
	return append(out, s.LeftOutLines()...)
}

// LeftOutLines say which sessions of another tool were not judged, with the flag that includes them.
func (s *Story) LeftOutLines() []string {
	var out []string
	for _, t := range []string{"claude", "codex"} {
		if n := s.M.LeftOut[t]; n > 0 {
			out = append(out, s.T("unjudged", "n", n, "tool", s.X.Tool(t), "flag", "agent-leeway drift --tool all"))
		}
	}
	return out
}

// mistakeNotOnTop is the agent-mistake finding when it is not one of the fixes (it gets a one-line note).
func (s *Story) mistakeNotOnTop() *Finding {
	for _, f := range s.Findings {
		if f.Kind == "MISTAKE" {
			for _, t := range s.Top {
				if t == f {
					return nil
				}
			}
			return f
		}
	}
	return nil
}

// FirstAction is the first screen's action and its one line of why: in theme mode the theme label, named as a
// rule for the theme's tool's file (unless it already lived in a file), and "You had to say this in N sessions."
func (s *Story) FirstAction() (action, why string, p Plain) {
	f := s.Top[0]
	p = s.Plain(f)
	th := s.Thing(f)
	action, why = p.Action, p.Why
	if th != nil && th.Mode == "theme" {
		why = s.T("th_said", "gs", th.GS)
		if !th.Theme.InFile {
			action = s.T("verdict_th", "label", th.Theme.Label, "file", fileOf[th.Theme.Tool])
		}
	}
	return action, why, p
}

// firstPanel is the dark "Fix first" panel on the first screen.
func (s *Story) firstPanel() string {
	act, why, p := s.FirstAction()
	src := E(s.T("source"))
	if strings.HasPrefix(p.SrcURL, "https://") {
		src = fmt.Sprintf(`<a href="%s" rel="noreferrer" title="%s">%s</a>`, E(p.SrcURL), E(p.SrcName), E(s.T("source")))
	}
	return fmt.Sprintf(`<div class="panel7"><div><p class="lab">%s</p><p class="act">%s</p><p class="why">%s %s</p></div><a class="btn5" href="#s-fix1">%s <span aria-hidden="true">↓</span></a></div>`,
		E(s.T("first_fix")), E(act), E(why), src, E(s.T("see_fix")))
}

// noCompare says the judges are not to be compared; only with two judges.
func (s *Story) noCompare() string {
	if len(s.M.Verdicts) > 1 {
		return s.T("judge_note")
	}
	return ""
}

// HTML renders the page.
func HTML(m *Model) (string, error) { return NewStory(m).HTML(), nil }

// HTML renders the story as one self-contained page.
func (s *Story) HTML() string {
	m, st := s.M, s.St
	type secT struct{ id, name string }
	secs := []secT{{"s-verdict", s.T("n_verdict")}}
	for i := range s.Top {
		secs = append(secs, secT{fmt.Sprintf("s-fix%d", i+1), s.T("fix_n", "i", i+1, "n", len(s.Top))})
	}
	if len(s.Good) > 0 {
		secs = append(secs, secT{"s-good", s.T("n_good")})
	}
	secs = append(secs, secT{"s-pic", s.T("n_pic")}, secT{"s-look", s.T("n_look")})
	N := len(secs)
	hint := func(i int, label, text string) string {
		if i+1 >= N {
			return ""
		}
		switch label {
		case "":
			label = s.T("next")
		case "-":
			label = ""
		}
		if text == "" {
			text = secs[i+1].name
		}
		return fmt.Sprintf(`<a class="hint noprint" href="#%s"><span class="hl">%s</span><span class="ht">%s</span>%s</a>`, secs[i+1].id, E(label), E(text), iconChevDown)
	}
	sec := func(i int, cls, inner, h string) string {
		return fmt.Sprintf(`<section class="sec %s" id="%s" aria-labelledby="%s-h"><span class="pos noprint" aria-hidden="true">%d / %d</span><div class="in">%s</div>%s</section>`,
			cls, secs[i].id, secs[i].id, i+1, N, inner, h)
	}
	scale := [3]string{s.T("scale.0"), s.T("scale.1"), s.T("scale.2")}
	var out strings.Builder

	// 1 overall: what is rated, the rating, two cards; the hint names the first fix
	title := fmt.Sprintf(`<div class="title"><p class="kicker" id="s-verdict-h">%s</p><p class="period">%s</p><h1 class="r-%s">%s</h1></div>`,
		E(s.T("kicker")), E(s.T("period", "a", m.From, "b", m.To)), st.Overall, E(s.T("rating."+st.Overall)))
	c1 := fmt.Sprintf(`<div class="card5"><p class="lab">%s</p><p class="fact">%s</p>%s<p class="dist">%s</p>`, E(s.T("c_drift")), E(s.T("f_drift", "n", st.Serious, "j", m.J)),
		dots(st.Serious, m.J, "f-agent_drift", 15, 5, 4), E(st.DDrift))
	if st.Drift != "na" {
		c1 += track(float64(st.Serious)/math.Max(float64(m.J), 1), DriftCuts, 0.4, scale, st.Drift)
	}
	c1 += `</div>`
	cards := c1
	kLeak := int(math.Round(st.Share * 100))
	if m.HasLeak {
		wf := strings.Replace(waffle([][2]any{{kLeak, "f-agent_drift"}}), `role="img"`, `role="img" title="`+E(s.T("sq"))+`"`, 1)
		c2 := fmt.Sprintf(`<div class="card5"><p class="lab">%s</p><p class="fact">%s (%s)</p>%s<p class="dist">%s</p>`, E(s.T("c_cost")),
			E(s.T("f_cost", "leak", Money(m.LeakUSD), "total", Money(m.TotalUSD))), Pct(st.Share), wf, E(st.DCost))
		if st.Cost != "na" {
			c2 += track(st.Share, CostCuts, 0.25, scale, st.Cost)
		}
		cards += c2 + `</div>`
	}
	v := `<div class="hero">` + title + `<div class="cards">` + cards + `</div></div>`
	if len(s.Top) > 0 {
		v += s.firstPanel()
	}
	out.WriteString(sec(0, "verdict", v, hint(0, "-", "")))

	// 2-4 fixes
	for i, f := range s.Top {
		q := s.Plain(f)
		th := s.Thing(f)
		var viz string
		if f.Side == "drift" {
			nn, capKey := f.Sessions, "sess_cap"
			if th != nil && th.Mode == "theme" {
				nn, capKey = th.GS, "th_cap"
			}
			viz = dots(nn, m.J, "f-agent_drift", 15, 6, 4) + `<p class="cap">` + E(s.T(capKey, "n", nn, "j", m.J)) + `</p>`
		} else {
			kh := max(int(math.Round(f.USD/math.Max(m.TotalUSD, 1e-9)*100)), 1)
			viz = waffle([][2]any{{kh, "f-agent_drift"}, {max(kLeak-kh, 0), "w-other"}}) + `<p class="cap">` + E(s.T("cost_cap")) + `</p>`
		}
		ctx, qhtml, lab := q.Hook, "", ""
		if th != nil && th.Mode == "theme" {
			lab, ctx = s.ThingText(f, th)
			by := s.T("th_by", "tool", s.X.Tool(th.Theme.Tool), "draft", map[bool]string{true: s.T("th_by_draft")}[s.Draft(f, th) != nil])
			var qs strings.Builder
			for _, x := range th.Quotes {
				fmt.Fprintf(&qs, `<p class="tq">“%s”</p>`, E(x))
			}
			qhtml = fmt.Sprintf(`<figure class="bq theme"><p class="tlabel">%s</p><figcaption class="ql">%s</figcaption>%s<p class="tby">%s</p></figure>`, E(th.Theme.Label), E(s.T("th_words")), qs.String(), E(by))
		} else if th != nil {
			lab, ctx = s.ThingText(f, th)
			then := th.E.Quote
			thenH := ""
			if then != "" && !SameWords(then, th.Quote) && utf8.RuneCountInString(then) <= 220 {
				thenH = fmt.Sprintf(`<p class="then"><span>%s</span> “%s”</p>`, E(s.T("then_said")), E(then))
			}
			qhtml = fmt.Sprintf(`<figure class="bq"><figcaption class="ql">%s</figcaption><p>“%s”</p>%s</figure>`, E(lab), E(th.Quote), thenH)
		}
		dr := s.Draft(f, th)
		if dr != nil && th != nil && SameWords(dr.Line, th.Quote) {
			// the draft is the user's own words: show them once, as the thing, ready to copy
			qhtml = fmt.Sprintf(`<figure class="bq copybox"><figcaption class="ql"><span>%s · %s</span>%s</figcaption><pre class="qpre">%s</pre></figure>`,
				E(lab), E(s.T("as_draft")), s.copyButton(), E(th.Quote))
			dr = nil
		}
		sid := secs[i+1].id
		evid := "ev-" + f.ID
		btn := s.T("show_ev")
		if f.Side == "leak" {
			btn = s.T("how_fix")
		}
		big := "big"
		if f.Side == "leak" {
			big = "big leak"
		}
		inner := fmt.Sprintf(`<p class="step">%s</p><div class="fix"><div><p class="%s">%s</p><p class="hk">%s</p>%s</div><div>%s<div class="act6"><p class="lab">%s</p><h2 id="%s-h">%s</h2>%s<p class="why">%s %s</p><a class="btn5" href="#%s" data-ev="%s">%s <span aria-hidden="true">→</span></a></div></div></div><div class="ev6" id="%s" data-title="%s" data-kick="%s">%s</div>`,
			E(secs[i+1].name), big, E(s.BigNum(f)), E(ctx), viz, qhtml, E(s.T("do")), sid, E(q.Action), s.draftBox(dr), E(q.Why),
			aLink(q.SrcURL, s.T("source")+": "+q.SrcName), evid, evid, E(btn), evid, E(q.Action), E(secs[i+1].name), s.evidenceBody(f, s.CasesFor(f, th), true))
		h := ""
		if i+2 < N {
			txt := secs[i+2].name
			if i+1 < len(s.Top) {
				txt = s.Plain(s.Top[i+1]).Short
			}
			h = hint(i+1, s.T("next"), txt)
		}
		out.WriteString(sec(i+1, "fix6", inner, h))
	}

	// 5 going well
	j := len(s.Top) + 1
	if len(s.Good) > 0 {
		var stats strings.Builder
		for _, g := range s.Good {
			viz := ""
			switch {
			case g.Waffle:
				viz = waffle([][2]any{{g.On, "f-as_intended"}})
			case g.Key == "fixed":
				viz = dots(g.On, g.All, "f-as_intended", 20, 4, 3)
			default:
				viz = dots(g.On, g.All, "f-as_intended", 15, 5, 4)
			}
			fmt.Fprintf(&stats, `<div class="stat"><p class="v">%s</p><p class="t">%s</p>%s<p class="cap">%s</p></div>`, E(g.Value), E(g.Text), viz, E(g.Caption))
		}
		out.WriteString(sec(j, "good6", fmt.Sprintf(`<h2 class="sh" id="s-good-h">%s</h2><p class="sub">%s</p><div class="stats">%s</div>`, E(s.T("good_h")), E(s.T("good_sub")), stats.String()), hint(j, "", "")))
		j++
	}

	// 6 the whole picture and 7 look things up (round 8 layout, html8.go); both may be taller than one screen
	out.WriteString(sec(j, "pic6 tall8", s.pictureSection(), hint(j, "", "")))
	j++
	out.WriteString(sec(j, "look6 tall8", s.lookSection(), ""))

	var nav strings.Builder
	fmt.Fprintf(&nav, `<nav class="dots6 noprint" aria-label="%s"><ol>`, E(s.T("nav")))
	for _, x := range secs {
		fmt.Fprintf(&nav, `<li><a href="#%s"><span>%s</span><i></i></a></li>`, x.id, E(x.name))
	}
	nav.WriteString(`</ol></nav>`)
	dlg := fmt.Sprintf(`<dialog class="dlg6" id="dlg6" aria-labelledby="dlg6-t"><div class="dlg-head"><div><p class="dlg-kick"></p><p class="dlg-title" id="dlg6-t"></p></div><button type="button" class="dlg-close">%s ✕</button></div><div class="dlg-body"></div></dialog>`, E(s.T("close")))
	body := nav.String() + `<main>` + out.String() + `</main>` + dlg
	return `<!doctype html><html lang="` + s.Lang + `"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; img-src data:">` +
		`<meta name="referrer" content="no-referrer"><meta name="color-scheme" content="light dark"><title>` + E(s.T("kicker")+" · "+m.To) + `</title><style>` + pageCSS + css8 +
		`</style></head><body>` + body + `<script>(function(){` + pageJS + `})();</script></body></html>`
}

func firstLabel(rows []barRow) string {
	if len(rows) == 0 {
		return "-"
	}
	return rows[0].Label
}

// PictureRows are the whole-picture bars: sessions affected per problem, and money lost per habit with an
// "Other (cause not known)" row so the bars add up to the first-screen total.
func (s *Story) PictureRows() (drift, cost []barRow) {
	fs := append([]*Finding(nil), s.Findings...)
	sortBy(fs, func(f *Finding) float64 { return float64(f.Sessions) })
	for _, f := range fs {
		drift = append(drift, barRow{s.prob(f), float64(f.Sessions)})
	}
	lk := append([]*Finding(nil), s.Leak...)
	sortBy(lk, func(f *Finding) float64 { return f.USD })
	sum := 0.0
	for _, f := range lk {
		cost = append(cost, barRow{s.prob(f), f.USD})
		sum += f.USD
	}
	if rest := s.M.LeakUSD - sum; rest >= 0.5 {
		cost = append(cost, barRow{s.T("prob_other"), rest})
	}
	return drift, cost
}

func (s *Story) prob(f *Finding) string {
	if v, ok := s.S["prob."+Key(f)]; ok {
		return v
	}
	return s.Plain(f).Short
}

func sortBy(fs []*Finding, key func(*Finding) float64) {
	for i := 1; i < len(fs); i++ {
		for j := i; j > 0 && key(fs[j]) > key(fs[j-1]); j-- {
			fs[j], fs[j-1] = fs[j-1], fs[j]
		}
	}
}

func notesHTML(notes []string) string {
	out := ""
	for _, n := range notes {
		out += ` <span class="xs t3">· ` + E(n) + `</span>`
	}
	return out
}
