package report

import (
	"fmt"
	"math"
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
	"codex/idle":    {"Codex docs", "https://learn.chatgpt.com/docs/build-skills"},
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

// IdleCardShare is when unused skills become a card of their own: half or more of a tool's listed skills (our rule
// of thumb, not a vendor's line).
const IdleCardShare = 0.5

// idleFindings are the cards for unused skills, per tool: half or more of the list went unused, and the list is
// not already a fix (the skill-list fix carries the count then). They go last in Other things worth fixing and
// never into the first three fixes or the rating.
func (s *Story) idleFindings() []*Finding {
	var out []*Finding
	for _, x := range s.setupShown() {
		if x.Idle == nil || x.Idle.Listed == 0 || float64(x.Idle.Unused) < IdleCardShare*float64(x.Idle.Listed) || (x.Kind == "skills" && x.Over) {
			continue
		}
		f := &Finding{Side: "setup", Kind: "SETUP_IDLE", ID: "f-setup-idle-" + x.Tool, AdviceID: "setup.unused", Tools: []string{x.Tool},
			Level: "measured", Tier: "A", Share: x.Idle.Share(), Setup: x}
		Advise(f, s.M.Advice)
		out = append(out, f)
	}
	return out
}

// idleAlso are the tools whose unused skills get the terminal's "also" line: each card, and an over-budget skill
// list with half or more unused whose fix is not among the first three (in them, its own lines say it).
func (s *Story) idleAlso() []*SetupItem {
	var out []*SetupItem
	for _, f := range s.Idle {
		out = append(out, f.Setup)
	}
	top := map[*Finding]bool{}
	for _, f := range s.Top {
		top[f] = true
	}
	for _, f := range s.Setup {
		x := f.Setup
		if f.Kind == "SETUP_SKILLS" && !top[f] && x.Idle != nil && x.Idle.Listed > 0 && float64(x.Idle.Unused) >= IdleCardShare*float64(x.Idle.Listed) {
			out = append(out, x)
		}
	}
	return out
}

// idleRules are the method's lines for unused skills: the card's cut-off (ours) and, with a Codex row, what counts
// as a Codex use.
func (s *Story) idleRules() []string {
	var out []string
	for _, x := range s.setupShown() {
		if x.Idle == nil {
			continue
		}
		if len(out) == 0 {
			out = append(out, s.T("su.idle.rule", "pct", int(IdleCardShare*100)))
		}
		if x.Tool == "codex" {
			out = append(out, s.T("su.idle.rule.codex"))
		}
	}
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
	if f.Kind == "SETUP_IDLE" {
		kv = append(s.idleArgs(f.Setup.Idle), "tool", s.X.Tool(f.Setup.Tool))
	}
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
	out := s.T(s.setupKey(f, "term"), s.setupArgs(f.Setup)...)
	if x := f.Setup.Idle; f.Setup.Kind == "skills" && x != nil && x.Unused > 0 {
		out += " · " + s.T("su.idle.term", s.idleArgs(x)...)
	}
	return out
}

// setupCardK is the small line over a setup card in Other things worth fixing.
func (s *Story) setupCardK(f *Finding) string {
	x := f.Setup
	if f.Kind == "SETUP_IDLE" {
		return s.T("su.card.idle", "tool", s.X.Tool(x.Tool))
	}
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
	if f.Kind == "SETUP_IDLE" {
		return fmt.Sprintf(`<div class="evb"><h3>%s</h3>%s<details class="more5" data-fold><summary>%s %s</summary><h3>%s</h3>%s</details></div>`,
			E(s.T("how_fix")), s.idleBody(f.Setup.Tool, f.Setup.Idle, false), iconChev, E(s.T("more")), E(s.T("all_sources")), s.sourcesHTML(f))
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<h3>%s</h3><p class="tooldo">%s</p>`, E(s.T("how_fix")), ticks(E(s.T(s.setupKey(f, "do")))))
	if n := s.setupNames(f); n != "" {
		fmt.Fprintf(&b, `<p class="tooldo t2">%s</p>`, E(n))
	}
	if l := s.idleFix(f); len(l) > 0 {
		fmt.Fprintf(&b, `<p class="tooldo t2">%s</p>`, E(strings.Join(l, " ")))
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
	Tool                                          string
	Idle                                          *SkillIdle // the unused-skills row
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
		if x.Kind == "idle" { // a tool whose list is not checked against a budget: only its unused skills
			rows = append(rows, s.idleRow(x, r))
			continue
		}
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
		if x.Idle != nil {
			rows = append(rows, s.idleRow(x, r))
		}
	}
	return rows
}

// idleRow is the unused-skills row under a tool's skill list: a count to look at, not a vendor line crossed.
func (s *Story) idleRow(x *SetupItem, list setupRow) setupRow {
	kv := s.idleArgs(x.Idle)
	r := setupRow{Name: s.X.Tool(x.Tool) + " · " + s.T("su.idle.name"), DocName: list.DocName, URL: list.URL, Tool: x.Tool, Idle: x.Idle,
		Value: s.T("su.idle.val0", kv...), State: s.T("su.idle.state0"), Line: s.T("su.idle.line0", kv...)}
	if x.Idle.New > 0 { // "none unused" is of the skills counted: say how many were not
		r.Line += s.T("su.idle.line.new", "n", x.Idle.New)
	}
	if x.Idle.Unused > 0 {
		r.Value, r.State, r.Line = s.T("su.idle.val", kv...), s.T("su.idle.state"), s.T("su.idle.line", kv...)
	}
	return r
}

func (s *Story) idleArgs(x *SkillIdle) []any {
	return []any{"u", x.Unused, "n", x.Listed, "days", x.Days, "s", x.Sessions, "pct", idlePct(x), "k", x.New, "min", IdleMinSessions}
}

// idlePct is the unused skills' share of the list in whole percent (at least 1 when there is any).
func idlePct(x *SkillIdle) int {
	p := int(math.Round(x.Share() * 100))
	if p == 0 && x.Idle > 0 {
		p = 1
	}
	return p
}

// idleGroups are the unused skills by source, one sentence each with every name (the fold is where they are
// looked up): a plugin none of whose skills was used says so, so turning the plugin off is the step to take.
func (s *Story) idleGroups(x *SkillIdle, allOnly bool) []string {
	var out []string
	for _, g := range x.Groups {
		switch {
		case g.All():
			out = append(out, s.T("su.idle.all", "src", g.Source, "n", g.Listed, "days", x.Days))
		case allOnly:
		case g.Source != "":
			out = append(out, s.T("su.idle.some", "src", g.Source, "u", len(g.Names), "n", g.Listed, "names", strings.Join(g.Names, ", ")))
		default:
			out = append(out, s.T("su.idle.rest", "names", strings.Join(g.Names, ", ")))
		}
	}
	return out
}

// idleAdvice is the tool's advice for unused skills (nil when the catalogue has none).
func (s *Story) idleAdvice(tool string) *ToolAdvice {
	e := s.M.Advice.Get("setup.unused")
	if e == nil {
		return nil
	}
	a, ok := e.Tools[tool]
	if !ok {
		return nil
	}
	return &ToolAdvice{tool, a}
}

// idleHTML is the unused-skills row's fold: why it matters, the skills by source, what to do and the sources.
func (s *Story) idleHTML(r setupRow) string {
	x := r.Idle
	if x.Unused == 0 {
		return ""
	}
	return fmt.Sprintf(`<details class="idle9"><summary>%s %s</summary><p>%s</p>%s</details>`, iconChev, E(s.T("su.idle.more")), E(s.idleWhy(r.Tool)),
		s.idleBody(r.Tool, x, true))
}

// idleWhy is why unused skills matter, from the tool's own docs.
func (s *Story) idleWhy(tool string) string { return s.T("su.idle.why." + tool) }

// idleNotes are the lines on what was not counted: skills listed lately, sessions whose list was hidden or cut.
func (s *Story) idleNotes(x *SkillIdle) []string {
	var out []string
	if x.New > 0 {
		out = append(out, s.T("su.idle.new", "n", x.New, "min", IdleMinSessions))
	}
	if x.Uncertain > 0 {
		out = append(out, s.T("su.idle.unsure", "n", x.Uncertain))
	}
	return out
}

// idleBody is the skills by source, the notes, what to do and (with sources) the vendor's words, in HTML.
func (s *Story) idleBody(tool string, x *SkillIdle, sources bool) string {
	var b strings.Builder
	b.WriteString(`<ul>`)
	for _, l := range s.idleGroups(x, false) {
		fmt.Fprintf(&b, `<li>%s</li>`, E(l))
	}
	b.WriteString(`</ul>`)
	for _, n := range s.idleNotes(x) {
		fmt.Fprintf(&b, `<p class="t3">%s</p>`, E(n))
	}
	if a := s.idleAdvice(tool); a != nil {
		var how []string
		for _, h := range a.A.How {
			how = append(how, "<code>"+E(h)+"</code>")
		}
		line := E(s.X.Action(*a))
		if len(how) > 0 {
			line += " → " + strings.Join(how, ", ")
		}
		fmt.Fprintf(&b, `<p class="tooldo">%s</p>`, line)
		if sources {
			b.WriteString(s.sourcesHTML(&Finding{Advice: []ToolAdvice{*a}}))
		}
	}
	return b.String()
}

// idleMD is the same fold in Markdown, after the table.
func (s *Story) idleMD(r setupRow) []string {
	x := r.Idle
	if x.Unused == 0 {
		return nil
	}
	md := []string{"**" + r.Name + "**: " + s.idleWhy(r.Tool), ""}
	for _, l := range s.idleGroups(x, false) {
		md = append(md, "- "+mq(l))
	}
	md = append(md, "")
	for _, n := range s.idleNotes(x) {
		md = append(md, n, "")
	}
	if a := s.idleAdvice(r.Tool); a != nil {
		md = append(md, "**"+s.X.T["do"]+":** "+s.X.Do(*a, code), "")
		for _, src := range a.A.Sources {
			md = append(md, "- "+s.X.Basis(a.A.Basis)+": "+link(src.Title, src.URL)+" · "+src.Read+" · “"+mq(src.Claim)+"”")
		}
		md = append(md, "")
	}
	return md
}

// idleFix is what a skill-list fix adds about unused skills: the count, and each plugin none of whose skills was used.
func (s *Story) idleFix(f *Finding) []string {
	if f.Side != "setup" || f.Setup == nil || f.Setup.Kind != "skills" {
		return nil
	}
	x := f.Setup.Idle
	if x == nil || x.Unused == 0 {
		return nil
	}
	return append([]string{s.T("pl.SETUP_SKILLS.idle", s.idleArgs(x)...)}, s.idleGroups(x, true)...)
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
		id := ""
		if r.Idle != nil {
			id = ` id="setup-idle-` + E(r.Tool) + `"`
			if r.Idle.Unused > 0 {
				cls, ic = "idle", "i"
			}
		}
		fmt.Fprintf(&b, `<li class="%s"%s><span class="ci" role="img" aria-label="%s">%s</span><span class="nm">%s</span><span class="vv">%s</span><span class="ln">%s: %s · %s%s</span>`,
			cls, id, E(r.State), ic, E(r.Name), E(r.Value), E(r.State), E(r.Line), aLink(r.URL, r.DocName), fx)
		if r.Idle != nil {
			b.WriteString(s.idleHTML(r))
		}
		b.WriteString(`</li>`)
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
	md = append(md, "")
	for _, r := range rows {
		if r.Idle != nil {
			md = append(md, s.idleMD(r)...)
		}
	}
	return md
}

const setupCSS = `.su9{list-style:none;margin:16px 0 0;padding:0;border-top:1px solid var(--line)}.su9 li{display:grid;grid-template-columns:22px minmax(150px,.8fr) minmax(0,1.1fr) minmax(0,1.6fr);gap:4px 12px;align-items:baseline;padding:11px 0;border-bottom:1px solid var(--line);font-size:14.5px}.su9 .ci{display:inline-grid;place-items:center;width:18px;height:18px;border-radius:50%;font-size:11px;font-weight:700;border:1.5px solid currentColor;align-self:center}.su9 li.ok .ci{color:var(--good)}.su9 li.over .ci{color:var(--poor)}.su9 .nm{font-weight:600}.su9 .vv{color:var(--t1)}.su9 li.ok .vv{color:var(--t2)}.su9 .ln{font-size:13px;color:var(--t3)}.su9 li.over .ln{color:var(--t2)}.su9 li.idle .ci{color:var(--t2)}.su9 .idle9{grid-column:2/-1;font-size:13.5px;color:var(--t2)}.su9 .idle9 summary{cursor:pointer;color:var(--t2)}.su9 .idle9 ul{margin:6px 0;padding-left:18px}.su9 .idle9 li{display:list-item;padding:2px 0;border:0;font-size:13.5px}.su9 .idle9 p{margin:6px 0}.hk8{font-size:14.5px;margin-top:4px}@media (max-width:760px){.su9 li{grid-template-columns:22px minmax(0,1fr)}.su9 .vv,.su9 .ln{grid-column:2}}`

// setupFixMD is a setup fix's how-to-fix, names and details in Markdown (nothing for other findings).
func (s *Story) setupFixMD(f *Finding) []string {
	if f.Side != "setup" {
		return nil
	}
	if f.Kind == "SETUP_IDLE" {
		x := f.Setup.Idle
		var r []string
		if a := s.idleAdvice(f.Setup.Tool); a != nil {
			r = append(r, "**"+s.T("how_fix")+":** "+s.X.Do(*a, code), "")
		}
		for _, l := range s.idleGroups(x, false) {
			r = append(r, "- "+mq(l))
		}
		r = append(r, "")
		for _, n := range s.idleNotes(x) {
			r = append(r, n, "")
		}
		return r
	}
	r := []string{"**" + s.T("how_fix") + ":** " + s.T(s.setupKey(f, "do")), ""}
	if n := s.setupNames(f); n != "" {
		r = append(r, n, "")
	}
	if l := s.idleFix(f); len(l) > 0 {
		r = append(r, mq(strings.Join(l, " ")), "")
	}
	r = append(r, s.T("su.what")+": "+s.setupCount(f), "")
	for _, x := range f.Against {
		r = append(r, "_"+s.againstLabel(x)+": "+pickLabel(x.Note, s.Lang)+" "+link(x.Title, x.URL)+"_", "")
	}
	return r
}
