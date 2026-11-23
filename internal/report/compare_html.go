package report

import (
	"fmt"
	"strings"
	"time"
)

var cmpIcon = map[string]string{"better": "✓", "worse": "!", "neutral": "="}

// cmpPct: shares under 10% with one decimal, others whole; exactly zero is "0%".
func cmpPct(x float64) string {
	if x == 0 {
		return "0%"
	}
	return Pct(x)
}

// cmpSpan is the length of the period as said in the text: "30 days", "day", "36 hours" (never "1 days").
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

// cmpDate is a day as the report says it: "1 Sep" / "9월 1일" (local time).
func (s *Story) cmpDate(t time.Time) string {
	t = t.In(time.Local)
	if s.Lang == "ko" {
		return fmt.Sprintf("%d월 %d일", int(t.Month()), t.Day())
	}
	return fmt.Sprintf("%d %s", t.Day(), t.Month().String()[:3])
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

// cmpBase names a side's baseline: "the previous 30 days", "the 1 Sep report", "the 1 Sep check".
func (s *Story) cmpBase(c *CmpSide) string {
	if c.Base == BasePrior || c.Base == "" {
		return s.T("cmp_base_prior", "span", s.cmpSpan())
	}
	return s.T("cmp_base_"+c.Base, "date", s.cmpDate(c.BaseAt))
}

// cmpLabel is the short label on a card: "vs prior 30 days", "vs 1 Sep report", "vs 1 Sep check".
func (s *Story) cmpLabel(c *CmpSide) string {
	if c.Base == BasePrior || c.Base == "" {
		return s.T("cmp_lab_prior", "span", s.cmpSpan())
	}
	return s.T("cmp_lab_"+c.Base, "date", s.cmpDate(c.BaseAt))
}

// cmpPill is a card's change line: both rates and the baseline; the word (with its icon, the full sentence as the
// accessible name) only for a change bigger than normal swings. Nothing when the side has no card line.
func (s *Story) cmpPill(c *CmpSide) string {
	if c == nil || !c.Pill || c.Tool != "" {
		return ""
	}
	rates := cmpPct(c.Prev) + " → " + cmpPct(c.Now) + " · " + s.cmpLabel(c)
	if c.State != "better" && c.State != "worse" {
		return fmt.Sprintf(`<p class="chg neutral"><span class="was">%s</span></p>`, E(rates))
	}
	full := s.T("cmp_full_"+c.State, "base", s.cmpBase(c))
	// screen readers hear the full sentence once (the icon's name), then the rates; the short word is hidden from them
	return fmt.Sprintf(`<p class="chg %s"><span class="ci" role="img" aria-label="%s">%s</span><span><span aria-hidden="true">%s </span><span class="was">· %s</span></span></p>`,
		c.State, E(full), cmpIcon[c.State], E(s.T("cmp_"+c.State+"_s")), E(rates))
}

// CmpTerm is the short form after a terminal or Markdown row's fact: "better, 15% → 8.6% vs prior 30 days". It
// follows the first screen: nothing when the card shows no change line.
func (s *Story) CmpTerm(drift bool) string {
	c := s.cmpOn(drift)
	if c == nil || !c.Pill || c.Tool != "" {
		return ""
	}
	if c.State == "better" || c.State == "worse" {
		return s.T("cmp_term", "word", s.T("cmp_word_"+c.State), "prev", cmpPct(c.Prev), "now", cmpPct(c.Now), "lab", s.cmpLabel(c))
	}
	return s.T("cmp_term_n", "prev", cmpPct(c.Prev), "now", cmpPct(c.Now), "lab", s.cmpLabel(c))
}

// CmpAny says whether either card shows a change line (the Markdown table then has its column).
func (s *Story) CmpAny() bool { return s.CmpTerm(true) != "" || s.CmpTerm(false) != "" }

// cmpSpanAdj is the length as an adjective: "30-day", "7-day", "36-hour" / "30일".
func (s *Story) cmpSpanAdj() string {
	h := s.M.Compare.Hours
	if h%24 == 0 {
		return s.T("cmp_spanadj_days", "n", h/24)
	}
	return s.T("cmp_spanadj_hours", "n", h)
}

// cmpWhy is a reason line with its values (each side has its own).
func (s *Story) cmpWhy(key string, args map[string]any) string {
	kv := []any{"span", s.cmpSpan(), "spanadj", s.cmpSpanAdj(), "m", CmpMinDetail, "project", s.T("cmp_one_project")}
	for k, v := range args {
		if t, ok := v.(time.Time); ok {
			v = s.cmpDate(t)
		}
		kv = append(kv, k, v)
	}
	return s.T(key, kv...)
}

// cmpDetail is the list under a whole-picture block: what it is compared with (its dates and each side's number of
// sessions), the headline and up to 3 rows that changed most (counts on the in-depth side), the selection line for a
// partial check, and one closing sentence; or one line saying why there is nothing to compare.
func (s *Story) cmpDetail(drift bool) string {
	cm := s.M.Compare
	if cm == nil || (drift && !s.DriftRun()) {
		return ""
	}
	c, why, args := cm.Cost, cm.CostWhy, cm.CostArgs
	if drift {
		c, why, args = cm.Drift, cm.DriftWhy, cm.DriftArgs
	}
	if c == nil {
		if why == "" {
			return ""
		}
		return fmt.Sprintf(`<div class="cmp8"><p class="nb">%s</p></div>`, E(s.cmpWhy(why, args)))
	}
	head := s.T("cmp_head_prior", "span", s.cmpSpan())
	if c.Base == BaseReport || c.Base == BaseCheck {
		head = s.T("cmp_head_"+c.Base, "date", s.cmpDate(c.BaseAt), "from", s.cmpDate(c.BaseFrom), "to", s.cmpDate(c.BaseTo))
	}
	if c.Counts {
		head += s.T("cmp_head_n", "then", c.NPrv, "now", c.NNow)
	}
	if c.Tool != "" {
		head += s.T("cmp_head_tool", "tool", s.X.Tool(c.Tool))
	}
	row := func(state, label, now, prev string) string {
		word, icon := "", ""
		if state == "better" || state == "worse" { // a neutral row has no word and no icon, as on the card
			word, icon = " · "+s.T("cmp_word_"+state), cmpIcon[state]
		}
		return fmt.Sprintf(`<li class="%s"><span class="ci" aria-hidden="true">%s</span><span>%s%s</span><span class="cv"><b>%s</b> · %s</span></li>`,
			state, icon, E(label), E(word), E(now), E(s.T("cmp_was", "x", prev)))
	}
	count := func(label string, r CmpRow) string { // in-depth rows: counts out of each side's checked sessions, no word
		return fmt.Sprintf(`<li class="cnt"><span>%s</span><span class="cv"><b>%s</b></span></li>`, E(label),
			E(s.T("cmp_of", "k", int(r.Prev), "n", r.NPrv)+" → "+s.T("cmp_of", "k", int(r.Now), "n", r.NNow)))
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<div class="cmp8"><h4>%s</h4><ul>`, E(head))
	if drift {
		for _, r := range c.Rows {
			label := s.S["prob."+r.Key]
			if r.Key == "SERIOUS" {
				label = s.T("cmp_serious")
			}
			b.WriteString(count(label, r))
		}
	} else {
		b.WriteString(row(c.State, s.T("cmp_waste"), cmpPct(c.Now), cmpPct(c.Prev)))
		for _, r := range c.Rows {
			b.WriteString(row(r.State, s.S["prob."+r.Key], s.T("cmp_per100", "v", Money(r.Now)), Money(r.Prev)))
		}
	}
	b.WriteString(`</ul>`)
	var notes []string
	if c.InWindow > c.NNow {
		notes = append(notes, s.T("cmp_slice", "n", c.NNow, "m", c.InWindow))
	}
	if c.Updated {
		notes = append(notes, s.T("cmp_updated"))
	}
	if c.OtherModel {
		notes = append(notes, s.T("cmp_other_version"))
	}
	if c.Other != "" { // two tools, one compared: why the other is not
		notes = append(notes, s.T("cmp_other_tool", "tool", s.X.Tool(c.Other), "why", s.cmpWhy(c.OtherWhy, c.OtherArgs)))
	}
	if why != "" { // a note on what the comparison covers
		notes = append(notes, s.cmpWhy(why, args))
	}
	notes = append(notes, s.T("cmp_caveat"))
	for _, n := range notes {
		fmt.Fprintf(&b, `<p class="nb">%s</p>`, E(n))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// cmpCSS is the comparison's styling. A line without a word is neutral: no icon, no colour.
const cmpCSS = `.chg{display:inline-flex;align-items:center;gap:7px;margin-top:6px;font-size:13px;line-height:1.35;color:var(--t1);padding:3px 10px 3px 4px;border-radius:999px;background:var(--well);width:max-content;max-width:100%}
.chg.neutral{padding:3px 10px}
.chg .ci{display:inline-grid;place-items:center;width:18px;height:18px;border-radius:50%;font-size:11px;font-weight:700;flex:none;background:var(--card);border:1.5px solid currentColor}
.chg.better .ci{color:var(--good,#4b8a5c)}.chg.worse .ci{color:var(--poor,#b14a30)}.chg .was{color:var(--t3)}.chg.neutral .was{color:var(--t2)}
.cmp8{margin-top:16px;border-top:1px solid var(--line);padding-top:12px}.cmp8 h4{font-size:13px;font-weight:650;color:var(--t2);margin:0 0 8px}
.cmp8 ul{list-style:none;margin:0;padding:0;display:grid;gap:7px}.cmp8 li{display:grid;grid-template-columns:20px minmax(0,1fr);gap:2px 8px;font-size:14px;line-height:1.4}
.cmp8 li.cnt{grid-template-columns:minmax(0,1fr) auto;gap:8px}
.cmp8 li .ci{grid-row:span 2;display:inline-grid;place-items:center;width:18px;height:18px;border-radius:50%;font-size:11px;font-weight:700;border:1.5px solid currentColor;margin-top:1px}
.cmp8 li.better .ci{color:var(--good,#4b8a5c)}.cmp8 li.worse .ci{color:var(--poor,#b14a30)}.cmp8 li.neutral .ci{color:var(--t3)}
.cmp8 .cv{font-size:12.5px;color:var(--t3)}.cmp8 .cv b{font-weight:600;color:var(--t2)}.cmp8 .nb{font-size:12.5px;color:var(--t3);margin:4px 0 0}
`
