package report

import "strings"

// TermWidth is the wrap width of the terminal summary, in display columns (a Korean character counts 2).
const TermWidth = 96

func plain(s string) string { return s }

// cardsTerm is the "What to fix first" cards: title (and the repeated name), size and confidence, one Do line
// per tool.
func cardsTerm(x Text, m *Model, top []*Finding) []string {
	var out []string
	if len(top) == 0 {
		return append(out, "  "+x.T["top_none"])
	}
	for i, f := range top {
		head := itoa(i+1) + ". " + x.Title(f)
		if f.Repeated != nil {
			head += " — “" + f.Repeated.Text + "”"
		}
		out = append(out, Wrap(head, TermWidth, "")...)
		out = append(out, Wrap(x.Size(m, f)+" · "+x.Level(f.Level), TermWidth, "   ")...)
		for _, a := range f.Advice {
			pre := ""
			if len(f.Advice) > 1 {
				pre = x.Tool(a.Tool) + ": "
			}
			out = append(out, Wrap(x.T["do"]+": "+pre+x.Do(a, plain), TermWidth, "   ")...)
		}
	}
	return out
}

func judgesTerm(x Text, m *Model) []string {
	var out []string
	for _, v := range m.Verdicts {
		out = append(out, x.Judge(v)+": "+x.VerdictLine(v))
	}
	if len(m.Verdicts) > 1 {
		out = append(out, Wrap(x.T["nocompare"], TermWidth, "")...)
	}
	return out
}

func factsTerm(x Text, m *Model) string {
	parts := []string{}
	if m.NSessions >= 0 {
		parts = append(parts, x.T["h_sessions"]+" "+itoa(m.NSessions))
	}
	parts = append(parts, x.T["h_judged"]+" "+itoa(m.J))
	if m.HasLeak {
		parts = append(parts, x.T["h_spend"]+" "+x.Tr("h_spend_v", "s", Money(m.TotalUSD), "l", Money(m.LeakUSD)))
	}
	return strings.Join(parts, " · ")
}

// TerminalLeak is the leak-only cards appended to `leak`'s output.
func TerminalLeak(m *Model) string {
	x := NewText(m.Lang)
	out := []string{x.T["leak_only_head"]}
	out = append(out, cardsTerm(x, m, Top(nil, m.Leak))...)
	for _, w := range x.Stale(m) {
		out = append(out, Wrap("! "+w, TermWidth, "")...)
	}
	return strings.Join(append(out, x.T["scope_more"]), "\n") + "\n"
}
