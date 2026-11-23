package report

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/advice"
)

// "Your setup" in the story (docs/33): an item over its vendor line is a fix ranked by §4; every item is a row of
// the setup block in Look things up. Nothing is added to the first screen, and nothing is compared with before.

// setupSrc is each item's own vendor page: the row's link and the fix's source.
var setupSrc = map[string][2]string{
	"claude/skills": {"Claude Code docs", "https://code.claude.com/docs/en/skills.md"},
	"claude/instr":  {"Claude Code docs", "https://code.claude.com/docs/en/memory.md"},
	"codex/instr":   {"Codex docs", "https://learn.chatgpt.com/docs/agent-configuration/agents-md"},
}

// setupShown are the rows this report shows: only the tools it is about (--tool), only tools with logs.
func (s *Story) setupShown() []*SetupItem {
	var out []*SetupItem
	for i := range s.M.Setup {
		x := &s.M.Setup[i]
		if s.M.Tool == "" || s.M.Tool == x.Tool {
			out = append(out, x)
		}
	}
	return out
}

// setupFindings are the items over their line, as fixes: measured, skill lists before instruction files, and
// within a kind the one further over first.
func (s *Story) setupFindings() []*Finding {
	var out []*Finding
	for _, x := range s.setupShown() {
		if !x.Over {
			continue
		}
		f := &Finding{Side: "setup", Kind: "SETUP_" + strings.ToUpper(x.Kind), ID: "f-setup-" + x.Kind + "-" + x.Tool, AdviceID: "setup." + x.Kind,
			Tools: []string{x.Tool}, Level: "measured", Tier: "A", Share: x.Share, Setup: x}
		Advise(f, s.M.Advice)
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if a, b := out[i].Kind == "SETUP_SKILLS", out[j].Kind == "SETUP_SKILLS"; a != b {
			return a
		}
		return out[i].Share > out[j].Share
	})
	return out
}

// withSetup puts the setup fixes into the ranked behaviour fixes (docs/33 §4) and returns the top three: a setup fix
// goes after the last firm or measured fix (the two axes alternate, so a likely one can come before a measured
// one), never first while a behaviour fix exists, and at most one is on top. It never displaces a firm or
// measured fix.
func withSetup(ranked, setup []*Finding) []*Finding {
	p := 0
	for i, f := range ranked {
		if confRank[f.Level] == 3 {
			p = i + 1
		}
	}
	if p == 0 && len(ranked) > 0 {
		p = 1
	}
	merged := append(append(append([]*Finding{}, ranked[:p]...), setup...), ranked[p:]...)
	var top []*Finding
	one := false
	for _, f := range merged {
		if len(top) < 3 && (f.Side != "setup" || !one) {
			top = append(top, f)
			one = one || f.Side == "setup"
		}
	}
	return top
}

// withSetupOthers puts the setup fixes that are not on top into Other things worth fixing, before the first likely
// or check card; the other cards keep the order they have without a setup fix.
func withSetupOthers(others, setup []*Finding, top []*Finding) []*Finding {
	in := map[*Finding]bool{}
	for _, f := range top {
		in[f] = true
	}
	var rest []*Finding
	for _, f := range setup {
		if !in[f] {
			rest = append(rest, f)
		}
	}
	if len(rest) == 0 {
		return others
	}
	p := len(others)
	for i, f := range others {
		if confRank[f.Level] < 3 {
			p = i
			break
		}
	}
	return append(append(append([]*Finding{}, others[:p]...), rest...), others[p:]...)
}

func (s *Story) setupKey(f *Finding, k string) string {
	if f.Setup.Tool == "codex" {
		if _, ok := s.S["pl."+f.Kind+"."+k+".codex"]; ok {
			return "pl." + f.Kind + "." + k + ".codex"
		}
	}
	return "pl." + f.Kind + "." + k
}

func (s *Story) setupArgs(x *SetupItem) []any {
	return []any{"n", s.setupNum(x), "k", x.K, "file", x.File, "proj", x.Project, "lim", KiB(x.Limit), "date", s.cmpDate(x.SeenAt), "tool", s.X.Tool(x.Tool)}
}

// setupNum is the item's number as shown: KiB for AGENTS.md, else the count.
func (s *Story) setupNum(x *SetupItem) int {
	if x.Tool == "codex" && x.Kind == "instr" {
		return KiB(x.N)
	}
	return x.N
}

func (s *Story) setupPlain(f *Finding) Plain {
	kv := s.setupArgs(f.Setup)
	src := setupSrc[f.Setup.Tool+"/"+f.Setup.Kind]
	return Plain{Action: s.T(s.setupKey(f, "action"), kv...), Short: s.T(s.setupKey(f, "short"), kv...), Hook: s.T(s.setupKey(f, "hook"), kv...),
		Why: s.T(s.setupKey(f, "why"), kv...), SrcName: src[0], SrcURL: src[1]}
}

// setupBig is the fix's number: "22 / 72", "236" or "32 KiB".
func (s *Story) setupBig(f *Finding) string {
	x := f.Setup
	switch {
	case x.Kind == "skills":
		return fmt.Sprintf("%d / %d", x.K, x.N)
	case x.Tool == "codex":
		return fmt.Sprintf("%d KiB", KiB(x.N))
	}
	return itoa(x.N)
}

// setupFact is the terminal's fact after a setup fix ("22 of 72 skills lost their description").
func (s *Story) setupFact(f *Finding) string {
	return s.T(s.setupKey(f, "term"), s.setupArgs(f.Setup)...)
}

// setupCardK is the small line over a setup card in Other things worth fixing.
func (s *Story) setupCardK(f *Finding) string {
	x := f.Setup
	if x.Kind == "skills" {
		return s.T("su.card.skills", s.setupArgs(x)...)
	}
	return s.T("su.card."+x.Tool+".instr", s.setupArgs(x)...)
}

// squares is a grid of small squares, dark first (or, with tail, dark last).
func squares(dark, all, cols int, cls string, tail bool) string {
	if all <= 0 {
		return ""
	}
	rows := (all + cols - 1) / cols
	var b strings.Builder
	for i := 0; i < all; i++ {
		c := "dot-off"
		if (!tail && i < dark) || (tail && i >= all-dark) {
			c = cls
		}
		fmt.Fprintf(&b, `<rect x="%d" y="%d" width="8" height="8" rx="1.5" class="%s"/>`, (i%cols)*11, (i/cols)*11, c)
	}
	return fmt.Sprintf(`<svg class="dots" viewBox="0 0 %d %d" role="img" aria-label="%d / %d">%s</svg>`, cols*11, rows*11, dark, all, b.String())
}

// setupViz is the fix's squares and their caption.
func (s *Story) setupViz(f *Finding) string {
	x := f.Setup
	var v string
	switch {
	case x.Kind == "skills":
		v = squares(x.K, x.N, 25, "f-agent_drift", false)
	case x.Tool == "codex":
		n := KiB(x.Limit)
		v = squares(n, n, 16, "f-leak", false)
	default:
		over := (x.N - ClaudeMDLines + 9) / 10
		v = squares(over, ClaudeMDLines/10+over, 25, "f-agent_drift", true)
	}
	return v + `<p class="cap">` + E(s.T(s.setupKey(f, "cap"), s.setupArgs(x)...)) + `</p>`
}

var tickRe = regexp.MustCompile("`([^`]+)`")

// ticks shows `x` in escaped text as code.
func ticks(escaped string) string { return tickRe.ReplaceAllString(escaped, "<code>$1</code>") }

// setupNames is "Descriptions dropped, used least first: a, b, c and 19 more."
func (s *Story) setupNames(f *Finding) string {
	ns := f.Setup.Names
	if len(ns) == 0 {
		return ""
	}
	shown := ns[:min(5, len(ns))]
	if len(ns) > len(shown) {
		return s.T("pl.SETUP_SKILLS.names", "names", strings.Join(shown, ", "), "more", len(ns)-len(shown))
	}
	return s.T("pl.SETUP_SKILLS.names_all", "names", strings.Join(shown, ", "))
}

// setupCount is the Details line: what was counted, in which list or file, when.
func (s *Story) setupCount(f *Finding) string {
	x := f.Setup
	kv := append(s.setupArgs(x), "date", x.SeenAt.In(time.Local).Format("2006-01-02"))
	out := s.T(s.setupKey(f, "count"), kv...)
	if x.Project == "" { // your user-level file: no project
		out = strings.TrimSuffix(strings.TrimSuffix(out, " ()"), "()")
	}
	if len(x.OverIn) > 0 {
		out += " " + s.T("su.also", "list", strings.Join(x.OverIn, ", "))
	}
	return out
}

// setupEvidence is the fix's "How to fix it" drawer: the steps, the names, the details and the vendor's words.
func (s *Story) setupEvidence(f *Finding) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<h3>%s</h3><p class="tooldo">%s</p>`, E(s.T("how_fix")), ticks(E(s.T(s.setupKey(f, "do")))))
	if n := s.setupNames(f); n != "" {
		fmt.Fprintf(&b, `<p class="tooldo t2">%s</p>`, E(n))
	}
	fmt.Fprintf(&b, `<details class="more5" data-fold><summary>%s %s</summary><dl class="facts5"><dt>%s</dt><dd>%s</dd><dt>%s</dt><dd>%s</dd></dl><h3>%s</h3>%s</details>`,
		iconChev, E(s.T("more")), E(s.T("su.what")), E(s.setupCount(f)), E(s.T("how_sure")), s.badge(f.Level), E(s.T("all_sources")), s.sourcesHTML(f))
	return `<div class="evb">` + b.String() + `</div>`
}

// againstLabel is "Another way" for an alternative, else "Evidence against".
func (s *Story) againstLabel(x advice.Against) string {
	if x.Kind == "another_way" {
		return s.T("su.another")
	}
	return s.X.T["against"]
}

// setupRow is one row's parts: name, value, state word, line, link and whether it is over.
type setupRow struct {
	Name, Value, State, Line, DocName, URL, FixID string
	Over                                          bool
}

func (s *Story) setupRows() []setupRow {
	fix := map[*SetupItem]string{}
	for _, f := range s.Setup {
		fix[f.Setup] = f.ID
	}
	var rows []setupRow
	for _, x := range s.setupShown() {
		kv := s.setupArgs(x)
		src := setupSrc[x.Tool+"/"+x.Kind]
		r := setupRow{DocName: s.T("su.docs", "tool", s.X.Tool(x.Tool)), URL: src[1], Over: x.Over, FixID: fix[x]}
		switch {
		case x.Kind == "skills":
			r.Name = s.X.Tool(x.Tool) + " · " + s.T("su.skill")
			r.Value = s.T(map[bool]string{true: "su." + x.Tool + ".skills.over", false: "su." + x.Tool + ".skills.ok"}[x.Over], kv...)
			r.Line = s.T("su."+x.Tool+".skills.line", kv...)
		case x.Tool == "claude":
			r.Name, r.Value, r.Line = s.X.Tool(x.Tool)+" · CLAUDE.md", s.T("su.claude.instr.val", kv...), s.T("su.claude.instr.line", kv...)
		default:
			r.Name, r.Value, r.Line = s.X.Tool(x.Tool)+" · AGENTS.md", s.T("su.codex.instr.val", kv...), s.T("su.codex.instr.line", kv...)
			if x.N < 1024 {
				r.Value = s.T("su.codex.instr.small", kv...)
			}
		}
		r.State = s.T("su.ok")
		if x.Over {
			r.State = s.T("su.over")
			if x.Tool == "codex" && x.Kind == "instr" {
				r.State = s.T("su.atlimit")
			}
		}
		rows = append(rows, r)
	}
	return rows
}

// setupBlock is "Your setup" in Look things up (every state: the within rows are the quiet "checked" state).
func (s *Story) setupBlock() string {
	rows := s.setupRows()
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<div class="setup9" id="setup"><h3>%s</h3><p class="lsub">%s</p><ul class="su9">`, E(s.T("su.h")), E(s.T("su.sub")))
	for _, r := range rows {
		cls, ic := "ok", "✓"
		if r.Over {
			cls, ic = "over", "!"
		}
		fx := ""
		if r.FixID != "" {
			fx = fmt.Sprintf(` · <a href="#%s">%s</a>`, r.FixID, E(s.T("su.fixlink")))
		}
		fmt.Fprintf(&b, `<li class="%s"><span class="ci" role="img" aria-label="%s">%s</span><span class="nm">%s</span><span class="vv">%s</span><span class="ln">%s: %s · %s%s</span></li>`,
			cls, E(r.State), ic, E(r.Name), E(r.Value), E(r.State), E(r.Line), aLink(r.URL, r.DocName), fx)
	}
	b.WriteString(`</ul></div>`)
	return b.String()
}

// setupMD is the same block as a Markdown table.
func (s *Story) setupMD() []string {
	rows := s.setupRows()
	if len(rows) == 0 {
		return nil
	}
	md := []string{"### " + s.T("su.h"), "", s.T("su.sub"), "", "| | | |", "|:--|:--|:--|"}
	for _, r := range rows {
		st := r.State
		if r.Over {
			st = "**" + st + "**"
		}
		md = append(md, "| "+r.Name+" | "+mq(r.Value)+" | "+st+": "+r.Line+" · "+link(r.DocName, r.URL)+" |")
	}
	return append(md, "")
}

const setupCSS = `.su9{list-style:none;margin:16px 0 0;padding:0;border-top:1px solid var(--line)}.su9 li{display:grid;grid-template-columns:22px minmax(150px,.8fr) minmax(0,1.1fr) minmax(0,1.6fr);gap:4px 12px;align-items:baseline;padding:11px 0;border-bottom:1px solid var(--line);font-size:14.5px}.su9 .ci{display:inline-grid;place-items:center;width:18px;height:18px;border-radius:50%;font-size:11px;font-weight:700;border:1.5px solid currentColor;align-self:center}.su9 li.ok .ci{color:var(--good)}.su9 li.over .ci{color:var(--poor)}.su9 .nm{font-weight:600}.su9 .vv{color:var(--t1)}.su9 li.ok .vv{color:var(--t2)}.su9 .ln{font-size:13px;color:var(--t3)}.su9 li.over .ln{color:var(--t2)}.hk8{font-size:14.5px;margin-top:4px}@media (max-width:760px){.su9 li{grid-template-columns:22px minmax(0,1fr)}.su9 .vv,.su9 .ln{grid-column:2}}`

// setupFixMD is a setup fix's how-to-fix, names and details in Markdown (nothing for other findings).
func (s *Story) setupFixMD(f *Finding) []string {
	if f.Side != "setup" {
		return nil
	}
	r := []string{"**" + s.T("how_fix") + ":** " + s.T(s.setupKey(f, "do")), ""}
	if n := s.setupNames(f); n != "" {
		r = append(r, n, "")
	}
	r = append(r, s.T("su.what")+": "+s.setupCount(f), "")
	for _, x := range f.Against {
		r = append(r, "_"+s.againstLabel(x)+": "+pickLabel(x.Note, s.Lang)+" "+link(x.Title, x.URL)+"_", "")
	}
	return r
}
