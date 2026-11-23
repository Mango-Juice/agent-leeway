package report

import (
	"fmt"
	"math"
	"strings"
)

// Sections 6 and 7 after the owner's readability review (docs/24 §19, round 8): one message per block, the
// numbers on the bars with their denominators, the width used for two blocks side by side, and the reference
// section shown outright. Only the session list is cut, to the five worst, with "show all" for the rest.

const css8 = `.sec.tall8{padding-bottom:120px}
.sec.fix6{padding-bottom:124px}
.lead8{font-size:19px;line-height:1.45;margin-top:6px;max-width:34em}.lead8 b{font-weight:650}
.pic8{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:24px;margin-top:28px}
.blk8{background:var(--card);border:1px solid var(--line);border-radius:18px;padding:22px 24px 20px}
.blk8 h3{font-size:14px;font-weight:650;color:var(--t2);letter-spacing:.01em}
.bars8{list-style:none;margin:16px 0 0;padding:0;display:grid;gap:12px}
.bars8 li{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:4px 12px;align-items:baseline}
.bars8 .bl{font-size:16px;line-height:1.35}.bars8 .bv{font-size:15px;font-weight:650;white-space:nowrap;text-align:right}
.bars8 .bv small{font-weight:400;color:var(--t3);margin-left:6px;font-size:13px}
.bars8 .bt{grid-column:1/-1;height:10px;border-radius:5px;background:var(--well);overflow:hidden}
.bars8 .bt i{display:block;height:100%;border-radius:5px;min-width:3px}
.bars8 .d i{background:var(--v-agent_drift)}.bars8 .c i{background:var(--leak)}.bars8 .o i{background:var(--t3);opacity:.55}
.cap8{font-size:13.5px;line-height:1.5;color:var(--t3);margin-top:16px}
.note8{font-size:14.5px;line-height:1.5;margin-top:14px;padding:10px 14px;border-radius:10px;background:var(--well);color:var(--t2)}
.judges8{margin-top:24px}.judges8 .jrow{margin-top:16px}.judges8 .jname{font-size:15.5px;font-weight:600}.judges8 .jname span{font-weight:400;color:var(--t3)}
.stack8{display:flex;height:26px;border-radius:7px;overflow:hidden;margin-top:8px;gap:2px}
.stack8 i{display:flex;align-items:center;justify-content:center;font:600 12.5px/1 var(--sans);color:#fff;font-style:normal;min-width:4px}
.stack8 .f-minor_drift,.stack8 .f-undeterminable{color:var(--t1)}
.leg8{list-style:none;margin:8px 0 0;padding:0;display:flex;flex-wrap:wrap;gap:4px 18px;font-size:14px;color:var(--t2)}
.leg8 b{color:var(--t1);font-weight:650;margin-left:4px}
.jnote8{font-size:14px;color:var(--t3);margin-top:14px}.jnote8 code{white-space:nowrap}
.look8{display:grid;gap:40px;margin-top:28px}
.look8 h3{font-size:20px;font-weight:650;letter-spacing:-.01em}.look8 h3 .n{font-weight:400;color:var(--t3);margin-left:8px;font-size:16px}
.look8 .lsub{font-size:15px;color:var(--t2);margin-top:4px}
.cards8{display:grid;grid-template-columns:repeat(auto-fill,minmax(300px,1fr));gap:16px;margin-top:16px}
.card8{background:var(--card);border:1px solid var(--line);border-radius:16px;padding:18px 20px;display:flex;flex-direction:column;gap:6px}
.card8 .k{font-size:13px;color:var(--t3)}.card8 h4{font-size:17.5px;line-height:1.35;font-weight:650}
.card8 .hk8{font-size:15px;color:var(--t2);line-height:1.45}.card8 .why8{font-size:14px;color:var(--t3);line-height:1.45}
.card8 .btn8{align-self:flex-start;margin-top:6px;font-size:14px;font-weight:600;color:var(--ink);text-decoration:none}
.card8 .btn8:hover{text-decoration:underline}
.sl8{list-style:none;margin:16px 0 0;padding:0;border-top:1px solid var(--line2)}
.sl8>li{border-bottom:1px solid var(--line)}
.sl8 details>summary{display:grid;grid-template-columns:auto minmax(0,1fr) auto;gap:2px 12px;align-items:baseline;padding:14px 4px}
.sl8 .sv{font-size:14px;font-weight:650;white-space:nowrap}.sl8 .sv .dot{margin-right:6px}
.sl8 .sw8{font-size:15.5px;line-height:1.4}.sl8 .sm8{font-size:13px;color:var(--t3);white-space:nowrap;text-align:right}
.sl8 .sbody{padding:0 4px 16px 4px}
.more8>summary{display:inline-flex;align-items:center;gap:6px;margin-top:12px;font-size:15px;font-weight:600;color:var(--ink);padding:8px 0}
.meth8{display:grid;grid-template-columns:repeat(auto-fill,minmax(300px,1fr));gap:16px;margin-top:16px}
.meth8 .m{background:var(--card);border:1px solid var(--line);border-radius:14px;padding:16px 18px}
.meth8 .m b{display:block;font-size:14.5px;font-weight:650;margin-bottom:4px}.meth8 .m p,.meth8 .m li{font-size:14.5px;line-height:1.5;color:var(--t2)}
.meth8 .wide{grid-column:1/-1}.mtab8{width:100%;border-collapse:collapse;font-size:14.5px;margin-top:4px}
.mtab8 th,.mtab8 td{text-align:left;padding:7px 12px 7px 0;border-top:1px solid var(--line)}.mtab8 th{color:var(--t3);font-weight:500;font-size:13px}.mtab8 td:first-child{font-weight:650}.meth8 ul{margin:0;padding-left:18px}
@media (max-width:860px){.pic8{grid-template-columns:1fr;gap:16px;margin-top:20px}.blk8{padding:18px 16px}.lead8{font-size:17px}
.bars8 .bl{font-size:15px}.cards8,.meth8{grid-template-columns:1fr}.look8 h3{font-size:18px}
.sl8 details>summary{grid-template-columns:auto minmax(0,1fr)}.sl8 .sm8{white-space:normal}.sl8 .sw8{grid-column:1/-1;order:3}.sec.tall8{padding-bottom:110px}}
@media print{.blk8,.card8,.meth8 .m{break-inside:avoid}.card8 .btn8{display:none}.sl8 details>summary .chev{display:none}}`

func pctW(x float64) string { return fmt.Sprintf("%.1f%%", math.Max(0, math.Min(100, x*100))) }

// pictureSection is section 6: where the agent went off, where the money went, and each judge on its own.
func (s *Story) pictureSection() string {
	m := s.M
	drows, crows := s.PictureRows()
	var b strings.Builder
	fmt.Fprintf(&b, `<h2 class="sh" id="s-pic-h">%s</h2>`, E(s.T("pic_h")))
	b.WriteString(`<div class="pic8">`)
	if len(drows) > 0 {
		fmt.Fprintf(&b, `<div class="blk8"><h3>%s</h3><p class="lead8">%s</p><ul class="bars8">`, E(s.T("pic8_drift_h")),
			E(s.T("pic8_drift_lead", "a", drows[0].Label, "n", int(drows[0].Value), "j", m.J)))
		for _, r := range drows {
			fmt.Fprintf(&b, `<li class="d"><span class="bl">%s</span><span class="bv">%s</span><span class="bt" role="img" aria-label="%s"><i style="width:%s"></i></span></li>`,
				E(r.Label), E(s.T("pic8_drift_val", "n", int(r.Value), "j", m.J)), E(r.Label+": "+s.T("pic8_drift_val", "n", int(r.Value), "j", m.J)), pctW(r.Value/math.Max(float64(m.J), 1)))
		}
		fmt.Fprintf(&b, `</ul>%s<p class="cap8">%s</p>`, s.cmpDetail(true), E(s.T("pic8_drift_cap")))
		if f := s.mistakeNotOnTop(); f != nil {
			fmt.Fprintf(&b, `<p class="note8">%s</p>`, E(s.T("mistake_note", "n", f.Sessions)))
		}
		b.WriteString(`</div>`)
	}
	if m.HasLeak && len(crows) > 0 {
		lt := math.Max(m.LeakUSD, 1e-9)
		fmt.Fprintf(&b, `<div class="blk8"><h3>%s</h3><p class="lead8">%s</p><ul class="bars8">`, E(s.T("pic8_cost_h")),
			E(s.T("pic8_cost_lead", "a", crows[0].Label, "usd", Money(crows[0].Value), "total", Money(m.LeakUSD), "spend", Money(m.TotalUSD))))
		for _, r := range crows {
			cls := "c"
			if r.Label == s.T("prob_other") {
				cls = "o"
			}
			fmt.Fprintf(&b, `<li class="%s"><span class="bl">%s</span><span class="bv">%s<small>%s</small></span><span class="bt" role="img" aria-label="%s"><i style="width:%s"></i></span></li>`,
				cls, E(r.Label), E(Money(r.Value)), E(Pct(r.Value/lt)), E(r.Label+": "+Money(r.Value)), pctW(r.Value/lt))
		}
		fmt.Fprintf(&b, `</ul>%s<p class="cap8">%s</p></div>`, s.cmpDetail(false), E(s.T("pic8_cost_cap", "total", Money(m.LeakUSD))))
	}
	b.WriteString(`</div>`)
	// each judge on its own line, never on a shared axis
	head := s.T("pic_judge")
	if len(m.Verdicts) < 2 {
		head = s.T("pic_judge1")
	}
	if !s.DriftRun() {
		fmt.Fprintf(&b, `<div class="blk8 judges8"><h3>%s</h3><p class="lead8">%s</p><p class="jnote8">%s</p></div>`, E(s.T("pic8_drift_h")), E(s.NoneFact()+" "+s.T("drift_what")),
			strings.Replace(E(s.T("drift_none_cmd", "cmd", s.DriftCmd())), E(s.DriftCmd()), "<code>"+E(s.DriftCmd())+"</code>", 1))
		return b.String()
	}
	fmt.Fprintf(&b, `<div class="blk8 judges8"><h3>%s</h3>`, E(head))
	for _, v := range m.Verdicts {
		fmt.Fprintf(&b, `<div class="jrow"><p class="jname">%s <span>· %s</span></p><div class="stack8" role="img" aria-label="%s">`,
			E(s.X.Judge(v)), E(s.T("pic8_sessions", "n", v.Total)), E(s.X.Judge(v)+": "+s.verdictWords(v)))
		var leg strings.Builder
		for _, k := range Verdicts {
			c := v.Counts[k]
			if c == 0 {
				continue
			}
			w := float64(c) / math.Max(float64(v.Total), 1)
			num := ""
			if w >= 0.05 {
				num = itoa(c)
			}
			fmt.Fprintf(&b, `<i class="f-%s" style="flex:%d 1 0">%s</i>`, k, c, num)
			fmt.Fprintf(&leg, `<li><span class="sw f-%s"></span>%s<b>%d</b></li>`, k, E(s.T("leg.v_"+k)), c)
		}
		fmt.Fprintf(&b, `</div><ul class="leg8">%s</ul></div>`, leg.String())
	}
	if nc := s.noCompare(); nc != "" {
		fmt.Fprintf(&b, `<p class="jnote8">%s</p>`, E(nc))
	}
	for _, l := range s.LeftOutLines() {
		fmt.Fprintf(&b, `<p class="jnote8">%s</p>`, CmdHTML(l))
	}
	b.WriteString(`</div>`)
	return b.String()
}

func (s *Story) verdictWords(v JudgeVerdicts) string {
	var parts []string
	for _, k := range Verdicts {
		if c := v.Counts[k]; c > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", s.T("leg.v_"+k), c))
		}
	}
	return strings.Join(parts, " · ")
}

// lookSection is section 7: the other findings as cards, the sessions with problems (five worst, then "show all")
// and the method, all shown outright.
func (s *Story) lookSection() string {
	m := s.M
	var b strings.Builder
	fmt.Fprintf(&b, `<h2 class="sh" id="s-look-h">%s</h2><p class="sub">%s</p><div class="look8">`, E(s.T("look_h")), E(s.T("look8_sub")))
	if others := s.Others(); len(others) > 0 {
		fmt.Fprintf(&b, `<div><h3>%s<span class="n">%d</span></h3><p class="lsub">%s</p><div class="cards8">`, E(s.T("look_more")), len(others), E(s.T("look8_more_lead")))
		for _, f := range others {
			p := s.Plain(f)
			evid := "ev-" + f.ID
			if f.Side == "setup" {
				fmt.Fprintf(&b, `<article class="card8" id="%s"><p class="k">%s</p><h4>%s</h4><p class="hk8">%s</p><p class="why8">%s %s</p><a class="btn8 noprint" href="#%s" data-ev="%s">%s →</a><div class="ev6" id="%s" data-title="%s">%s</div></article>`,
					f.ID, E(s.setupCardK(f)), E(p.Action), E(p.Hook), E(p.Why), aLink(p.SrcURL, s.T("source")+": "+p.SrcName), evid, evid, E(s.T("how_fix")), evid, E(p.Action), s.setupEvidence(f))
				continue
			}
			fmt.Fprintf(&b, `<article class="card8" id="%s"><p class="k">%s · %s</p><h4>%s</h4><p class="why8">%s %s</p><a class="btn8 noprint" href="#%s" data-ev="%s">%s →</a><div class="ev6" id="%s" data-title="%s">%s</div></article>`,
				f.ID, E(s.prob(f)), E(s.unit(f)), E(p.Action), E(p.Why), aLink(p.SrcURL, s.T("source")+": "+p.SrcName),
				evid, evid, E(s.T("show_ev")), evid, E(p.Action), s.evidenceBody(f, PickEvents(f, 3), true))
		}
		b.WriteString(`</div></div>`)
	}
	b.WriteString(s.setupBlock())
	rs := sessionsWorstFirst(m)
	if len(rs) > 0 {
		fmt.Fprintf(&b, `<div><h3>%s<span class="n">%d</span></h3><p class="lsub">%s</p><ul class="sl8">`, E(s.T("look_sess")), len(rs),
			E(s.T("look8_sess_lead", "n", len(rs), "j", m.J)))
		for i, r := range rs {
			if i == 5 {
				fmt.Fprintf(&b, `</ul><details class="more8" data-fold open><summary>%s %s</summary><ul class="sl8">`, iconChev, E(s.T("look8_show_all", "n", len(rs))))
			}
			b.WriteString(s.sessionRow(r))
		}
		b.WriteString(`</ul>`)
		if len(rs) > 5 {
			b.WriteString(`</details>`)
		}
		fmt.Fprintf(&b, `<p class="cap8">%s · %s</p></div>`, E(s.X.Tr("sess_ok", "n", countVerdict(m, "as_intended"))), E(s.X.Tr("sess_unclear", "n", countVerdict(m, "undeterminable"))))
	}
	b.WriteString(s.methodBlock())
	fmt.Fprintf(&b, `</div><p class="toplink noprint"><a href="#s-verdict">↑ %s</a></p>`, E(s.T("top")))
	return b.String()
}

// sessionRow is one session: verdict, what went wrong in plain words, project and tool; its problems unfold.
func (s *Story) sessionRow(r *Record) string {
	x := s.X
	first := ""
	if len(r.Events) > 0 {
		k := storyKind(r.Events[0])
		key := "cap." + k
		if _, ok := s.S[key]; !ok {
			key = "cap.MISTAKE"
		}
		first = s.T(key, "file", fileOf[r.Tool])
	}
	tags := ""
	if r.Partial {
		tags += ` <span class="badge">` + E(x.T["partial"]) + `</span>`
	}
	if r.Origin == "uncertain" {
		tags += ` <span class="badge">` + E(x.T["origin_uncertain"]) + `</span>`
	}
	var ev strings.Builder
	for _, e := range r.Events {
		fmt.Fprintf(&ev, `<li><span class="evmeta">%s · %s</span><blockquote><span class="who">%s</span>%s</blockquote><p class="xs t3">%s: %s</p></li>`,
			E(x.Rule(e.Rule)), E(x.SizeWord(e.Size)), E(x.T["you"]), E(e.Quote), E(x.T["judge_note"]), E(e.Summary))
	}
	for _, l := range []string{map[bool]string{true: x.Tr("defects", "n", r.Defects)}[r.Defects > 0], map[bool]string{true: x.Tr("weak", "n", r.Weak)}[r.Weak > 0]} {
		if l != "" {
			fmt.Fprintf(&ev, `<li class="xs t3">%s</li>`, E(l))
		}
	}
	return fmt.Sprintf(`<li id="s-%s"><details data-fold open><summary><span class="sv"><span class="dot f-%s"></span>%s</span><span class="sw8">%s%s</span><span class="sm8">%s · %s · %s</span></summary><ul class="ev sbody">%s</ul></details></li>`,
		E(r.Short()), E(r.Verdict), E(s.T("leg.v_"+r.Verdict)), E(first), tags, E(orQ(r.Project)), E(x.Tool(r.Tool)), E(s.T("look8_problems", "n", len(r.Events))), ev.String())
}

// methodBlock is "How we checked", shown outright: what was read, how sessions were judged, the rating rules,
// where the dollars come from, privacy, the notes that apply, and one link out.
func (s *Story) methodBlock() string {
	rows := s.MethodRows()
	var b strings.Builder
	fmt.Fprintf(&b, `<div><h3>%s</h3><div class="meth8">`, E(s.T("look_method")))
	for _, r := range rows {
		fmt.Fprintf(&b, `<div class="m"><b>%s</b><p>%s</p></div>`, E(r[0]), E(r[1]))
	}
	var lv strings.Builder
	lv.WriteString(`<table class="mtab8"><thead><tr>`)
	for c := 0; c < 3; c++ {
		fmt.Fprintf(&lv, `<th>%s</th>`, E(s.S[fmt.Sprintf("m_axis.%d", c)]))
	}
	lv.WriteString(`</tr></thead><tbody>`)
	for r := 0; r < 3; r++ {
		lv.WriteString(`<tr>`)
		for c := 0; c < 3; c++ {
			fmt.Fprintf(&lv, `<td>%s</td>`, E(s.S[fmt.Sprintf("m_level_rows.%d.%d", r, c)]))
		}
		lv.WriteString(`</tr>`)
	}
	lv.WriteString(`</tbody></table>`)
	fmt.Fprintf(&b, `<div class="m wide"><b>%s</b>%s<p class="xs t3" style="margin-top:6px">%s</p></div>`, E(s.T("m_levels")), lv.String(), E(s.T("m_note")))
	if notes := s.Notes(); len(notes) > 0 {
		var nb strings.Builder
		for _, n := range notes {
			nb.WriteString(`<li>` + E(n) + `</li>`)
		}
		fmt.Fprintf(&b, `<div class="m wide"><b>%s</b><ul>%s</ul></div>`, E(s.T("m_notes")), nb.String())
	}
	fmt.Fprintf(&b, `</div><p class="cap8">%s</p></div>`, aLink(docsURL(), s.T("m_more")+" →"))
	return b.String()
}
