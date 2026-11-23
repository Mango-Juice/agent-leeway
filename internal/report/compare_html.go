package report

import (
	"fmt"
	"strings"
)

var cmpIcon = map[string]string{"better": "✓", "worse": "!", "same": "="}

// cmpPct: shares under 10% with one decimal, others whole; exactly zero is "0%".
func cmpPct(x float64) string {
	if x == 0 {
		return "0%"
	}
	return Pct(x)
}

// cmpSpan is the length of each period as said in the text: "30 days", "day", "36 hours" (never "1 days").
func (s *Story) cmpSpan() string {
	h := s.M.Compare.Hours
	switch {
	case h == 24:
		return s.T("cmp_span_day")
	case h%24 == 0:
		return s.T("cmp_span_days", "n", h/24)
	}
	return s.T("cmp_span_hours", "n", h)
}

// cmpOn says whether a side is shown: the in-depth side only when this period has checked sessions.
func (s *Story) cmpOn(drift bool) *CmpSide {
	c := s.M.Compare
	if c == nil {
		return nil
	}
	if drift {
		if !s.DriftRun() {
			return nil
		}
		return c.Drift
	}
	return c.Cost
}

// CmpVs is the note after the dates when a card shows a change ("compared with the 30 days before").
func (s *Story) CmpVs() string {
	d, c := s.cmpOn(true), s.cmpOn(false)
	if (d == nil || d.Tool != "") && (c == nil || c.Tool != "") {
		return ""
	}
	return s.T("cmp_vs", "span", s.cmpSpan())
}

// cmpPill is a card's change line: icon + one word + the two rates, the full sentence as the accessible name.
func (s *Story) cmpPill(c *CmpSide) string {
	if c == nil || c.Tool != "" { // one tool only: the card's fact covers more, so the change is said in the detail
		return ""
	}
	full := s.T("cmp_"+c.State, "span", s.cmpSpan())
	rates := cmpPct(c.Prev) + " → " + cmpPct(c.Now)
	// screen readers hear the full sentence once (the icon's name), then the rates; the short word is hidden from them
	return fmt.Sprintf(`<p class="chg %s"><span class="ci" role="img" aria-label="%s">%s</span><span><span aria-hidden="true">%s </span><span class="was">· %s</span></span></p>`,
		c.State, E(full), cmpIcon[c.State], E(s.T("cmp_"+c.State+"_s")), E(rates))
}

// CmpTerm is the short form after a terminal or Markdown row's fact: "better (15% → 8.6%)". It follows the first
// screen: nothing when the card shows no pill (a side covering one tool only is said in the detail).
func (s *Story) CmpTerm(drift bool) string {
	c := s.cmpOn(drift)
	if c == nil || c.Tool != "" {
		return ""
	}
	return s.T("cmp_term", "word", s.T("cmp_word_"+c.State), "prev", cmpPct(c.Prev), "now", cmpPct(c.Now))
}

// cmpDetail is the list under a whole-picture block: the headline rate, then up to 3 rows that changed most; or
// one line saying why this part is not compared.
func (s *Story) cmpDetail(drift bool) string {
	cm := s.M.Compare
	if cm == nil || (drift && !s.DriftRun()) {
		return ""
	}
	c, why := cm.Cost, cm.CostWhy
	if drift {
		c, why = cm.Drift, cm.DriftWhy
	}
	head := s.T("cmp_head", "span", s.cmpSpan())
	if c != nil && c.Tool != "" {
		head = s.T("cmp_head_tool", "span", s.cmpSpan(), "tool", s.X.Tool(c.Tool))
	}
	if c == nil {
		if why == "" {
			return ""
		}
		return fmt.Sprintf(`<div class="cmp8"><h4>%s</h4><p class="nb">%s</p></div>`, E(head), E(s.T(why, "span", s.cmpSpan(), "m", CmpMinJudged)))
	}
	row := func(state, label, now, prev string) string {
		return fmt.Sprintf(`<li class="%s"><span class="ci" aria-hidden="true">%s</span><span>%s · %s</span><span class="cv"><b>%s</b> · %s</span></li>`,
			state, cmpIcon[state], E(label), E(s.T("cmp_word_"+state)), E(now), E(s.T("cmp_was", "x", prev)))
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<div class="cmp8"><h4>%s</h4><ul>`, E(head))
	if drift {
		b.WriteString(row(c.State, s.T("cmp_serious"), s.T("cmp_share", "v", cmpPct(c.Now)), cmpPct(c.Prev)))
		for _, r := range c.Rows {
			b.WriteString(row(r.State, s.S["prob."+r.Key], s.T("cmp_share", "v", cmpPct(r.Now)), cmpPct(r.Prev)))
		}
	} else {
		b.WriteString(row(c.State, s.T("cmp_waste"), cmpPct(c.Now), cmpPct(c.Prev)))
		for _, r := range c.Rows {
			b.WriteString(row(r.State, s.S["prob."+r.Key], s.T("cmp_per100", "v", Money(r.Now)), Money(r.Prev)))
		}
	}
	b.WriteString(`</ul>`)
	if why != "" { // a note on what the comparison covers (Claude Code logs deleted)
		fmt.Fprintf(&b, `<p class="nb">%s</p>`, E(s.T(why, "span", s.cmpSpan())))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// cmpCSS is the comparison's styling.
const cmpCSS = `.chg{display:inline-flex;align-items:center;gap:7px;margin-top:6px;font-size:13px;line-height:1.35;color:var(--t1);padding:3px 10px 3px 4px;border-radius:999px;background:var(--well);width:max-content;max-width:100%}
.chg .ci{display:inline-grid;place-items:center;width:18px;height:18px;border-radius:50%;font-size:11px;font-weight:700;flex:none;background:var(--card);border:1.5px solid currentColor}
.chg.better .ci{color:var(--good,#4b8a5c)}.chg.worse .ci{color:var(--poor,#b14a30)}.chg.same .ci{color:var(--t3)}.chg .was{color:var(--t3)}
.cmp8{margin-top:16px;border-top:1px solid var(--line);padding-top:12px}.cmp8 h4{font-size:13px;font-weight:650;color:var(--t2);margin:0 0 8px}
.cmp8 ul{list-style:none;margin:0;padding:0;display:grid;gap:7px}.cmp8 li{display:grid;grid-template-columns:20px minmax(0,1fr);gap:2px 8px;font-size:14px;line-height:1.4}
.cmp8 li .ci{grid-row:span 2;display:inline-grid;place-items:center;width:18px;height:18px;border-radius:50%;font-size:11px;font-weight:700;border:1.5px solid currentColor;margin-top:1px}
.cmp8 li.better .ci{color:var(--good,#4b8a5c)}.cmp8 li.worse .ci{color:var(--poor,#b14a30)}.cmp8 li.same .ci{color:var(--t3)}
.cmp8 .cv{font-size:12.5px;color:var(--t3)}.cmp8 .cv b{font-weight:600;color:var(--t2)}.cmp8 .nb{font-size:12.5px;color:var(--t3);margin:0}
`
