package report

import (
	"fmt"
	"math"
	"strings"

	"golang.org/x/text/width"

	"github.com/Mango-Juice/agent-leeway/internal/advice"
	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// Text is the fixed text of one language with the helpers every surface shares.
type Text struct {
	Lang string
	T    map[string]string
}

// NewText is the message table for lang (English when unknown).
func NewText(lang string) Text {
	t, ok := messages.ReportText[lang]
	if !ok {
		lang, t = "en", messages.ReportText["en"]
	}
	return Text{lang, t}
}

// Tr formats a message: Tr("leak_size", "n", 3, ...).
func (x Text) Tr(key string, kv ...any) string {
	m := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return py.Format(x.T[key], m)
}

// Money is "$1,234.56".
func Money(v float64) string {
	neg := v < 0
	c := int64(math.Round(math.Abs(v) * 100))
	s := fmt.Sprintf("%d", c/100)
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	out := fmt.Sprintf("$%s.%02d", b.String(), c%100)
	if neg {
		out = "-" + out
	}
	return out
}

// Pct has one decimal below 9.5%, none above.
func Pct(x float64) string {
	if x >= 0.095 {
		return fmt.Sprintf("%.0f%%", x*100)
	}
	return fmt.Sprintf("%.1f%%", x*100)
}

// Thousands is "1,234".
func Thousands(n int) string {
	s := Money(float64(n))
	return strings.TrimSuffix(strings.TrimPrefix(s, "$"), ".00")
}

// Title is the kind title or the habit name.
func (x Text) Title(f *Finding) string {
	if f.Side == "leak" {
		return x.T["H_"+f.Habit]
	}
	return x.T["K_"+f.Kind]
}

// RepeatedFrom is "(you said this)" and the like, without the brackets.
func (x Text) RepeatedFrom(r *Repeated) string {
	return x.T[map[string]string{"label": "judge_label", "file": "in_file", "chat": "in_chat"}[r.From]]
}

// Size is the plain size line of a card.
func (x Text) Size(m *Model, f *Finding) string {
	if f.Side == "leak" {
		if f.Count > 0 {
			return x.Tr("leak_size", "n", Thousands(f.Count), "usd", Money(f.USD), "total", Money(m.TotalUSD), "pct", Pct(f.Share))
		}
		return x.Tr("leak_size_auto", "usd", Money(f.USD), "total", Money(m.TotalUSD), "pct", Pct(f.Share))
	}
	s := x.Tr("drift_size", "s", f.Sessions, "j", m.J, "e", f.NEvents)
	if f.Maj > 0 {
		s += x.Tr("drift_major", "m", f.Maj)
	}
	return s
}

// Level is the confidence badge word.
func (x Text) Level(l string) string { return x.T["sure_"+l] }

// Notes are the finding's notes in this language.
func (x Text) Notes(f *Finding) []string {
	var out []string
	for _, n := range f.Notes {
		out = append(out, x.T[n])
	}
	return out
}

// Tool is "Claude Code" or "Codex".
func (x Text) Tool(t string) string { return x.T["tool_"+t] }

// Action is the Do sentence of one tool's advice.
func (x Text) Action(a ToolAdvice) string { return advice.Pick(a.A.Action, x.Lang) }

// How is the exact commands or settings shown after the sentence, joined.
func (x Text) How(a ToolAdvice, quote func(string) string) string {
	var parts []string
	for _, h := range a.A.How {
		parts = append(parts, quote(h))
	}
	return strings.Join(parts, ", ")
}

// Do is the plain Do line: the sentence, then "→ <command>" when there is one.
func (x Text) Do(a ToolAdvice, quote func(string) string) string {
	s := x.Action(a)
	if h := x.How(a, quote); h != "" {
		s += " → " + h
	}
	return s
}

// Basis is "Official docs" and the like.
func (x Text) Basis(b string) string { return x.T["basis_"+b] }

// Judge is "Claude Code · judged by opus".
func (x Text) Judge(v JudgeVerdicts) string {
	j := v.Tier
	if _, m, ok := strings.Cut(v.Tier, ":"); ok {
		j = m
	}
	return x.Tr("judged_by", "tool", x.Tool(v.Tool), "judge", j)
}

// VerdictLine is "Agent drift 4 · Undeterminable 2".
func (x Text) VerdictLine(v JudgeVerdicts) string {
	var parts []string
	for _, k := range Verdicts {
		if n := v.Counts[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", x.T["v_"+k], n))
		}
	}
	return strings.Join(parts, " · ")
}

// Period is "2026-10-23 – 2026-11-22 · 30 days".
func (x Text) Period(m *Model) string {
	return x.Tr("period", "a", m.From, "b", m.To, "d", m.Days)
}

// JudgedFact is "20 Claude Code · 10 Codex" (or "none").
func (x Text) JudgedFact(m *Model) string {
	by := map[string]int{}
	for _, v := range m.Verdicts {
		by[v.Tool] += v.Total
	}
	var parts []string
	for _, t := range []string{"claude", "codex"} {
		if by[t] > 0 {
			parts = append(parts, x.Tr("h_judges", "n", by[t], "j", x.Tool(t)))
		}
	}
	if len(parts) == 0 {
		return x.T["h_judged_none"]
	}
	return strings.Join(parts, " · ")
}

// Stale is the old-data warnings (price table, advice catalogue).
func (x Text) Stale(m *Model) []string {
	var w []string
	if m.PricesStale {
		w = append(w, x.Tr("stale", "what", x.T["w_prices"], "d", m.PricesRead, "n", m.PricesDays))
	}
	if m.AdviceStale {
		w = append(w, x.Tr("stale", "what", x.T["w_advice"], "d", m.AdviceOldest, "n", m.AdviceDays))
	}
	return w
}

// Warnings go under the header facts and in the terminal summary: old data and an edited rubric.
func (x Text) Warnings(m *Model) []string {
	w := x.Stale(m)
	if len(m.Custom) > 0 {
		w = append(w, x.Tr("w_custom", "v", strings.Join(m.Custom, ", "), "b", m.Bundled))
	}
	return w
}

// Method is the "How this was made" lines.
func (x Text) Method(m *Model) []string {
	var tiers []string
	for _, t := range m.Tiers {
		tiers = append(tiers, fmt.Sprintf("%s (%d)", t.Tier, t.N))
	}
	vers := strings.Join(m.RubricVersions, ", ")
	if vers == "" {
		vers = "?"
	}
	var out []string
	if m.J > 0 {
		out = append(out, x.Tr("m_judges", "x", strings.Join(tiers, ", "), "v", vers))
		if len(m.CLIs) > 0 {
			out = append(out, x.Tr("m_cli", "x", strings.Join(m.CLIs, ", ")))
		}
		out = append(out, x.Tr("m_cite", "q", m.Quotes, "b", m.Bad, "d", m.Dropped), x.T["m_limits"])
		if len(m.Custom) > 0 {
			out = append(out, x.Tr("m_custom", "v", strings.Join(m.Custom, ", "), "b", m.Bundled))
		}
		if m.InstrCurrent > 0 {
			out = append(out, x.Tr("m_instr", "n", m.InstrCurrent))
		}
		if m.PartialSessions > 0 {
			out = append(out, x.Tr("m_partial", "n", m.PartialSessions, "c", Thousands(m.OmittedChars)))
		}
	}
	if m.OriginUncertain > 0 {
		out = append(out, x.Tr("m_origin", "n", m.OriginUncertain))
	}
	if m.Masked > 0 {
		out = append(out, x.Tr("m_masked", "n", m.Masked))
	}
	var conf []string
	for _, l := range Levels {
		conf = append(conf, x.T["sure_"+l]+": "+x.T["sure_"+l+"_d"])
	}
	out = append(out, x.Tr("m_conf", "x", strings.Join(conf, "; ")))
	pr := m.PricesRead
	if pr == "" {
		pr = "?"
	}
	out = append(out, x.Tr("m_price", "d", pr), x.Tr("m_advice", "v", m.AdviceVersion))
	if m.AdvicePath != "" {
		out = append(out, x.Tr("m_advice_own", "p", m.AdvicePath))
	}
	out = append(out, x.Stale(m)...)
	return append(out, x.T["m_priv"])
}

// Resolution is the outcome word of an event.
func (x Text) Resolution(r string) string {
	if r == "" {
		r = "unknown"
	}
	if s, ok := x.T["res_"+r]; ok {
		return s
	}
	return x.T["res_unknown"]
}

// Rule is the plain label of a rule code (the code itself when it has none).
func (x Text) Rule(r string) string {
	if s, ok := x.T["rule_"+r]; ok {
		return s
	}
	return r
}

// SizeWord is "undone or redone" or "small fix".
func (x Text) SizeWord(s string) string {
	if v, ok := x.T["size_"+s]; ok {
		return v
	}
	return s
}

// Scope is the file a copy block goes to.
func (x Text) Scope(c CopyScope) string {
	if c.Project == "" {
		return x.T["scope_user_"+c.Tool]
	}
	return x.Tr("scope_proj_"+c.Tool, "p", c.Project)
}

// LineLabel says where a copyable line comes from.
func (x Text) LineLabel(l CopyLine) string {
	switch l.Kind {
	case "sourced":
		return x.T["line_sourced"]
	case "draft":
		return x.T["line_draft"]
	}
	return x.Tr("line_repeat", "n", l.N)
}

// ---------------------------------------------------------------- display width

// Width counts East Asian wide and full-width characters as 2 columns.
func Width(s string) int {
	n := 0
	for _, r := range s {
		switch width.LookupRune(r).Kind() {
		case width.EastAsianWide, width.EastAsianFullwidth:
			n += 2
		default:
			n++
		}
	}
	return n
}

// Wrap breaks s at spaces so that no line is wider than w columns (a single long word stays whole); every line
// starts with indent.
func Wrap(s string, w int, indent string) []string {
	var out []string
	line := ""
	for _, word := range strings.Split(s, " ") {
		cand := word
		if line != "" {
			cand = line + " " + word // keeps runs of spaces (column alignment)
		}
		if line != "" && word != "" && Width(indent+cand) > w {
			out = append(out, indent+line)
			line = word
		} else {
			line = cand
		}
	}
	if line != "" {
		out = append(out, indent+line)
	}
	return out
}
