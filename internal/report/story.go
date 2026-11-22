package report

import (
	"fmt"
	"github.com/Mango-Juice/agent-leeway/internal/advice"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/Mango-Juice/agent-leeway/internal/leak"
	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"github.com/Mango-Juice/agent-leeway/internal/meta"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// The report as a sequence of sections (docs/24 §17): the overall rating, three fixes, what is going well, the
// whole picture and a folded look-up section. Story holds what all three surfaces (HTML, Markdown, terminal)
// show, in that order. Python reference: docs/mockups/round6/build_round6.py (with round 5 and round 3).

// Status cut-offs (docs/24 §15.3): serious-miss sessions at most 10% good, at most 25% fair; money lost to
// habits under 5% good... (inclusive: exactly 5% is good), at most 15% fair. Fewer than MinJudged judged sessions
// gives no drift rating.
var (
	DriftCuts = [2]float64{0.10, 0.25}
	CostCuts  = [2]float64{0.05, 0.15}
)

// MinJudged is the fewest judged sessions that get a drift rating.
const MinJudged = 5

// StoryKinds are the drift kinds of the story, in order; "went against what you said" is split by where the
// instruction lived (BROKE_FILE: an instruction file; BROKE_CHAT: the conversation).
var StoryKinds = []string{"RESERVED", "FALSE_DONE", "ASKED_ONLY", "BROKE_FILE", "BROKE_CHAT", "MISTAKE", "PROMPT"}

var levelOrder = map[string]int{"good": 0, "watch": 1, "attention": 2}

// Status is the rating of each axis.
type Status struct {
	Drift, Cost, Overall string  // good | watch | attention | na
	Serious              int     // judged sessions with a serious miss (agent drift or prompt gap)
	Share                float64 // money lost to habits / spend
	DDrift, DCost        string  // distance to the next better level ("" when not rated)
}

// Thing is the specific thing a fix names (§17.6, §18.2).
type Thing struct {
	Mode       string // theme (grouped by meaning) | repeat (same words in 2+ sessions) | example
	E          *Event
	Quote      string // theme mode: the label
	GS, GE, NG int    // sessions and events of the group; number of different things
	Theme      *Theme
	Quotes     []string   // theme mode: up to 3 distinct user quotes from the member events
	Others     []ThemeRef // theme mode: the finding's other themes (2+ of its sessions), in the same order
}

// ThemeRef is a theme as counted inside one finding: its label and the finding's sessions and events it covers.
type ThemeRef struct {
	Theme  *Theme
	S, E   int
	Events []*Event
}

// Plain is a finding's plain copy.
type Plain struct {
	Action, Short, Hook, Why string
	SrcName, SrcURL, File    string
}

// GoodStat is one measured statement of what is going well.
type GoodStat struct {
	Key, Value, Text, Caption string
	On, All                   int
	Waffle                    bool // a 100-square spend waffle instead of dots
}

// Story is the report content in section order.
type Story struct {
	M                   *Model
	Lang                string
	S                   map[string]string
	X                   Text
	Findings, Leftovers []*Finding // drift findings, one per story kind
	Leak                []*Finding
	St                  Status
	Top                 []*Finding // the fixes, at most 3
	Good                []GoodStat
}

// T is a story message with {name} placeholders filled.
func (s *Story) T(key string, kv ...any) string {
	m := map[string]any{"docs": docsURL()}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	v, ok := s.S[key]
	if !ok {
		v = messages.Story["en"][key]
	}
	return py.Format(v, m)
}

func docsURL() string { return meta.RepoURL() + "/blob/main/docs/validation.md" }

func storyKind(e *Event) string {
	if e.Kind == "BROKE_SAID" {
		if strings.HasPrefix(e.InstrLoc, "I") {
			return "BROKE_FILE"
		}
		return "BROKE_CHAT"
	}
	return e.Kind
}

// NewStory builds the story of a model.
func NewStory(m *Model) *Story {
	s := &Story{M: m, Lang: m.Lang, S: messages.Story[m.Lang], X: NewText(m.Lang)}
	if s.S == nil {
		s.S = messages.Story["en"]
		s.Lang = "en"
	}
	for _, k := range StoryKinds {
		var evs []*Event
		for _, r := range m.Records {
			for _, e := range r.Events {
				if e.Kind != "" && storyKind(e) == k {
					evs = append(evs, e)
				}
			}
		}
		if len(evs) == 0 {
			continue
		}
		f := newDrift(k, evs, nil, m.J)
		f.ID = "f-drift-" + strings.ToLower(strings.ReplaceAll(k, "_", "-"))
		switch {
		case (f.Sessions >= 2 && (f.Maj > 0 || f.NEvents >= 3)) || k == "RESERVED" || k == "FALSE_DONE":
			f.Tier = "A"
		case f.NEvents >= 2 || f.Maj > 0:
			f.Tier = "B"
		}
		f.Level, f.Notes = Confidence(f)
		Advise(f, m.Advice)
		CopyBlocks(f, m.Lang)
		if f.Tier != "" {
			s.Findings = append(s.Findings, f)
		} else {
			s.Leftovers = append(s.Leftovers, f)
		}
	}
	for _, f := range m.Leak {
		g := *f
		g.ID = "f-" + f.ID
		s.Leak = append(s.Leak, &g)
	}
	s.St = s.status()
	s.Top = s.firstThree()
	s.Good = s.goingWell()
	return s
}

func level(x float64, cuts [2]float64) string {
	switch {
	case x <= cuts[0]:
		return "good"
	case x <= cuts[1]:
		return "watch"
	}
	return "attention"
}

func (s *Story) status() Status {
	m := s.M
	st := Status{Drift: "na", Cost: "na", Overall: "na"}
	for _, v := range m.Verdicts {
		st.Serious += v.Counts["agent_drift"] + v.Counts["prompt_gap"]
	}
	if m.TotalUSD > 0 {
		st.Share = m.LeakUSD / m.TotalUSD
	}
	if m.J >= MinJudged {
		st.Drift = level(float64(st.Serious)/float64(m.J), DriftCuts)
		if st.Drift == "good" {
			st.DDrift = s.T("d_best")
		} else {
			st.DDrift = s.T("d_good_drift", "k", st.Serious-int(math.Floor(DriftCuts[0]*float64(m.J))))
		}
	}
	if m.HasLeak && m.TotalUSD > 0 {
		st.Cost = level(st.Share, CostCuts)
		if st.Cost == "good" {
			st.DCost = s.T("d_best")
		} else {
			gap := m.LeakUSD - CostCuts[0]*m.TotalUSD
			usd := fmt.Sprintf("$%d", int(math.Ceil(gap)))
			if gap < 10 {
				usd = fmt.Sprintf("$%.2f", gap) // "$3 less" read wrong when only $2.58 was lost
			}
			st.DCost = s.T("d_good_cost", "usd", usd)
		}
	}
	for _, l := range []string{st.Drift, st.Cost} {
		if l != "na" && (st.Overall == "na" || levelOrder[l] > levelOrder[st.Overall]) {
			st.Overall = l
		}
	}
	return st
}

var confRank = map[string]int{"firm": 3, "measured": 3, "likely": 2, "check": 1}

// firstThree picks the fixes: drift findings other than agent mistakes (a symptom with generic advice) and ranked
// leak habits, each list by confidence then share; the axis with the worse rating starts, then they alternate.
func (s *Story) firstThree() []*Finding {
	var d, l []*Finding
	for _, f := range s.Findings {
		if f.Kind != "MISTAKE" {
			d = append(d, f)
		}
	}
	for _, f := range s.Leak {
		if f.Tier != "" {
			l = append(l, f)
		}
	}
	by := func(xs []*Finding) {
		sort.SliceStable(xs, func(i, j int) bool {
			if a, b := confRank[xs[i].Level], confRank[xs[j].Level]; a != b {
				return a > b
			}
			return xs[i].Share > xs[j].Share
		})
	}
	by(d)
	by(l)
	a, b := d, l
	if s.St.Cost != "na" && s.St.Drift != "na" && levelOrder[s.St.Cost] > levelOrder[s.St.Drift] {
		a, b = l, d
	}
	var out []*Finding
	for (len(a) > 0 || len(b) > 0) && len(out) < 3 {
		for _, q := range []*[]*Finding{&a, &b} {
			if len(*q) > 0 && len(out) < 3 {
				out = append(out, (*q)[0])
				*q = (*q)[1:]
			}
		}
	}
	return out
}

// Others are the ranked findings that are not fixes, then the leftovers.
func (s *Story) Others() []*Finding {
	in := map[*Finding]bool{}
	for _, f := range s.Top {
		in[f] = true
	}
	var out []*Finding
	for _, f := range s.Findings {
		if !in[f] {
			out = append(out, f)
		}
	}
	for _, f := range s.Leak {
		if f.Tier != "" && !in[f] {
			out = append(out, f)
		}
	}
	return append(out, s.Leftovers...)
}

// Key is the copy key of a finding: its kind or habit.
func Key(f *Finding) string {
	if f.Side == "leak" {
		return f.Habit
	}
	return f.Kind
}

var fileOf = map[string]string{"claude": "CLAUDE.md", "codex": "AGENTS.md"}

var srcs = map[string][2]string{
	"memory":  {"Claude Code docs", "https://code.claude.com/docs/en/memory.md"},
	"caching": {"Claude Code docs", "https://code.claude.com/docs/en/prompt-caching.md"},
	"perm":    {"Claude Code docs", "https://code.claude.com/docs/en/permissions.md"},
	"best":    {"Claude Code docs", "https://code.claude.com/docs/en/best-practices.md"},
	"plan":    {"Claude Code docs", "https://code.claude.com/docs/en/permission-modes.md"},
	"costs":   {"Claude Code docs", "https://code.claude.com/docs/en/costs.md"},
	"rot":     {"Chroma study", "https://www.trychroma.com/research/context-rot"},
}

func (s *Story) join(xs []string, en, ko string) string {
	if s.Lang == "ko" {
		return strings.Join(xs, ko)
	}
	return strings.Join(xs, en)
}

// Plain is the plain copy of a finding (§16, §17.6): the action follows from the specific thing.
func (s *Story) Plain(f *Finding) Plain {
	k := Key(f)
	th := s.Thing(f)
	if k == "BROKE_CHAT" {
		switch {
		case th != nil && th.Mode == "theme" && th.Theme.InFile:
			k = "BROKE_FILE" // the themed thing already lived in an instruction file: make it checkable
		case th != nil && th.Mode == "theme":
			k = "BROKE_CHAT_TH"
		case th == nil || th.Mode == "example":
			k = "BROKE_CHAT_EX" // nothing repeats across sessions: the data does not support "write it down"
		}
	}
	var files, tools []string
	for _, t := range f.Tools {
		if fn, ok := fileOf[t]; ok {
			files = append(files, fn)
		}
		tools = append(tools, s.X.Tool(t))
	}
	fn := s.join(files, " or ", "나 ")
	if fn == "" {
		fn = "CLAUDE.md"
	}
	n := ""
	if f.Count > 0 {
		n = itoa(f.Count)
	}
	sv, t1 := f.Sessions, "claude"
	if len(f.Tools) > 0 {
		t1 = f.Tools[0]
	}
	if th != nil && th.Mode == "theme" {
		t1 = th.Theme.Tool // sv stays the finding's sessions: the size sentence is about the finding
	}
	kv := []any{"s", sv, "e", f.NEvents, "j", s.M.J, "usd", Money(f.USD), "n", n, "file", fn, "tools", s.join(tools, " and ", "와 "),
		"file1", fileOf[t1], "tool1", s.X.Tool(t1)}
	src := s.S["pl."+k+".src"]
	x := Plain{Action: s.T("pl."+k+".action", kv...), Short: s.T("pl."+k+".short", kv...), Hook: s.T("pl."+k+".hook", kv...),
		Why: s.T("pl."+k+".why", kv...), File: fn}
	if v, ok := srcs[src]; ok {
		x.SrcName, x.SrcURL = v[0], v[1]
	} else {
		x.SrcName, x.SrcURL = s.T("src_ours"), docsURL()
	}
	switch {
	case len(f.Tools) == 1 && f.Tools[0] == "codex":
		// a Codex finding: the why line and the short label come from the Codex sources in the advice catalogue
		// (the ones the detail panel lists), never from the Claude Code docs
		if _, ok := s.S["pl."+k+".why.codex"]; ok {
			x.Why = s.T("pl."+k+".why.codex", kv...)
		}
		x.SrcName, x.SrcURL = s.codexSource(f, k)
	case len(f.Tools) > 1 && src != "ours":
		// both tools contributed: a neutral label ("Official docs" only when both tools' advice rests on official
		// docs); the detail panel lists each tool's sources
		x.SrcName = s.T("src_both")
		for _, a := range f.Advice {
			if a.A.Basis != "official" {
				x.SrcName = s.T("src_mixed")
			}
		}
	}
	if f.Side == "leak" {
		if top := s.leakTop(f); f.USD > 0 && top/f.USD >= 0.3 {
			x.Hook += " " + s.T("one_sess", "top", Money(top))
		}
	}
	return x
}

// codexSrc names, per plain key, the Codex source the Codex why line rests on (a part of its title in the advice
// catalogue); "ours" is our own measurement or rule of thumb.
var codexSrc = map[string]string{
	"ASKED_ONLY": "Prompting", "BROKE_CHAT": "AGENTS.md", "BROKE_CHAT_TH": "AGENTS.md", "BROKE_SAID": "AGENTS.md",
	"BROKE_FILE": "AGENTS.md", "PROMPT": "AGENTS.md", "BROKE_CHAT_EX": "ours", "FALSE_DONE": "Prompting",
	"MISTAKE": "Prompting", "RESERVED": "Rules", "automation": "Prompt caching", "long_break": "Prompt caching",
	"mid_switch": "ours", "big_output": "Context Rot", "let_repeat": "ours", "no_map": "Pricing",
}

// codexSource is the short name and link of the Codex source behind a Codex finding's why line: "Codex docs",
// "OpenAI docs", a study's name, or our own measurement.
func (s *Story) codexSource(f *Finding, k string) (string, string) {
	want := codexSrc[k]
	var srcs []advice.Source
	for _, a := range f.Advice {
		if a.Tool == "codex" {
			srcs = a.A.Sources
		}
	}
	if want != "ours" {
		for _, x := range srcs {
			if strings.Contains(x.Title, want) {
				return s.shortSource(x.Title), x.URL
			}
		}
		if len(srcs) > 0 {
			return s.shortSource(srcs[0].Title), srcs[0].URL
		}
	}
	return s.T("src_ours"), docsURL()
}

// shortSource is "Codex docs" for "Codex docs: Prompting", the study's name for a study, and "Leeway measurement"
// for our own.
func (s *Story) shortSource(title string) string {
	if strings.HasPrefix(title, "Leeway") {
		return s.T("src_ours")
	}
	head, tail, ok := strings.Cut(title, ": ")
	if !ok {
		return title
	}
	if strings.HasSuffix(head, "docs") {
		return head
	}
	return tail
}

// leakTop is the habit's dollars in its single most expensive session.
func (s *Story) leakTop(f *Finding) float64 {
	if s.M.LeakR == nil {
		return 0
	}
	var causes []string
	for _, h := range leak.Habits {
		if h.Key == f.Habit {
			causes = h.Causes
		}
	}
	top := 0.0
	for _, ss := range s.M.LeakR.Sessions {
		v := 0.0
		for _, c := range causes {
			v += ss.ByCause[c]
		}
		top = math.Max(top, v)
	}
	return top
}

// disp is the words that name the thing for one event: the instruction (a rule or an earlier request), the
// missing line, or the user's words.
func disp(f *Finding, e *Event) string {
	switch Key(f) {
	case "BROKE_FILE", "BROKE_CHAT":
		return e.InstrQuote
	case "PROMPT":
		if e.Fix != "" {
			return e.Fix
		}
	}
	return e.Quote
}

// GroupKey normalises words for the deterministic grouping: NFKC, lower case, letters and digits only.
func GroupKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(norm.NFKC.String(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var linkRe = regexp.MustCompile(`https?://`)

func quotable(q string) bool {
	n := utf8.RuneCountInString(strings.TrimSpace(q))
	return n >= 4 && n <= 220 && !linkRe.MatchString(q)
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func thingScore(e *Event) int {
	return 2*b2i(e.Size == "major") + b2i(e.InstrQuote != "") + b2i(e.Resolution == "unresolved") + b2i(e.Fix != "") + b2i(Wordy(e.Quote))
}

func best(evs []*Event, f *Finding) *Event {
	var b *Event
	for _, e := range evs {
		if !quotable(disp(f, e)) {
			continue
		}
		if b == nil || thingScore(e) > thingScore(b) {
			b = e
		}
	}
	return b
}

var quoteKinds = map[string]bool{"BROKE_FILE": true, "BROKE_CHAT": true, "PROMPT": true}

// Thing names the specific thing of a drift finding (§17.6). A theme from the grouping step that spans two or
// more of the finding's sessions comes first; then the same normalised words in two or more sessions; otherwise
// one example, chosen by score.
func (s *Story) Thing(f *Finding) *Thing {
	if f.Side != "drift" {
		return nil
	}
	in := map[*Event]bool{}
	for _, e := range f.Events {
		in[e] = true
	}
	// 1. themes whose kept events overlap the finding's events in 2+ sessions. The fix shows the one covering
	// the most of the finding's sessions (ties: more events, then label order); the others are listed under it.
	// GS and GE count inside the finding, so "of those" never exceeds the finding's own sessions.
	var refs []ThemeRef
	for _, th := range s.M.Themes {
		var evs []*Event
		ss := map[string]bool{}
		for _, e := range th.Events {
			if in[e] {
				evs = append(evs, e)
				ss[e.Rec.Key] = true
			}
		}
		if len(ss) >= 2 {
			refs = append(refs, ThemeRef{Theme: th, S: len(ss), E: len(evs), Events: evs})
		}
	}
	sort.SliceStable(refs, func(a, b int) bool {
		if refs[a].S != refs[b].S {
			return refs[a].S > refs[b].S
		}
		if refs[a].E != refs[b].E {
			return refs[a].E > refs[b].E
		}
		return refs[a].Theme.Label < refs[b].Theme.Label
	})
	k := Key(f)
	if len(refs) > 0 {
		bt, bevs := refs[0].Theme, refs[0].Events
		seen := map[string]bool{}
		var quotes []string
		for _, e := range bevs {
			q := disp(f, e)
			if q == "" {
				q = e.Quote
			}
			if g := GroupKey(q); q != "" && !seen[g] {
				seen[g] = true
				quotes = append(quotes, q)
			}
		}
		if len(quotes) > 3 {
			quotes = quotes[:3]
		}
		return &Thing{Mode: "theme", E: bevs[0], Quote: bt.Label, GS: refs[0].S, GE: refs[0].E, NG: 1, Theme: bt, Quotes: quotes, Others: refs[1:]}
	}
	groups := map[string][]*Event{}
	var order []string
	if quoteKinds[k] {
		for _, e := range f.Events {
			if g := GroupKey(disp(f, e)); g != "" {
				if _, ok := groups[g]; !ok {
					order = append(order, g)
				}
				groups[g] = append(groups[g], e)
			}
		}
	}
	ng := len(groups)
	if ng == 0 {
		ng = len(f.Events)
	}
	var rep []*Event
	rs := 0
	for _, g := range order {
		ss := map[string]bool{}
		for _, e := range groups[g] {
			ss[e.Rec.Key] = true
		}
		if len(ss) >= 2 && (len(ss) > rs || (len(ss) == rs && len(groups[g]) > len(rep))) {
			rep, rs = groups[g], len(ss)
		}
	}
	if rep != nil {
		e := rep[0]
		for _, x := range rep {
			if thingScore(x) > thingScore(e) {
				e = x
			}
		}
		return &Thing{Mode: "repeat", E: e, Quote: disp(f, e), GS: rs, GE: len(rep), NG: ng}
	}
	e := best(f.Events, f)
	if e == nil {
		return nil
	}
	return &Thing{Mode: "example", E: e, Quote: disp(f, e), NG: ng, GS: 1, GE: 1}
}

// ThingText is the label above the quote and the context sentence of a fix section.
func (s *Story) ThingText(f *Finding, th *Thing) (string, string) {
	k := Key(f)
	kk := k
	if !quoteKinds[k] {
		kk = "other"
	}
	fn := fileOf[th.E.Rec.Tool]
	lab := s.T("th_lab."+kk, "file", fn)
	switch th.Mode {
	case "theme":
		return th.Theme.Label, s.SizeLine(f)
	case "repeat":
		return lab, s.T("th_rep."+kk, "gs", th.GS, "ge", th.GE)
	}
	return lab, s.T("th_ex."+kk, "ng", th.NG, "s", f.Sessions, "hook", s.Plain(f).Hook)
}

// SizeLine is the size of a drift finding in plain words: "In 12 of 50 sessions it went against something you
// had already said." In theme mode it comes before the theme, so the order of the fixes (by finding) is visible.
func (s *Story) SizeLine(f *Finding) string {
	k := "sz." + Key(f)
	if _, ok := s.S[k]; !ok {
		k = "sz.other"
	}
	return s.T(k, "s", f.Sessions, "j", s.M.J)
}

// WithinLine introduces the theme inside its finding: "Of those, the same request came up in 2 sessions:".
func (s *Story) WithinLine(th *Thing) string {
	if th.Theme.InFile {
		return s.T("th_within_rule", "gs", th.GS)
	}
	return s.T("th_within", "gs", th.GS)
}

// MoreLine lists the finding's other themes briefly ("" when there are none).
func (s *Story) MoreLine(th *Thing) string {
	if th == nil || len(th.Others) == 0 {
		return ""
	}
	var xs []string
	for _, o := range th.Others {
		xs = append(xs, s.T("th_more_one", "label", o.Theme.Label, "gs", o.S))
	}
	return s.T("th_more", "list", s.join(xs, " · ", " · "))
}

// Draft is a ready line from the theme's rule, the judge's suggested sentence or the user's repeated words. A rule
// that is already in an instruction file never gets one.
type Draft struct{ Line, Label string }

// Draft is the copyable line of a fix, or nil.
func (s *Story) Draft(f *Finding, th *Thing) *Draft {
	k := Key(f)
	if th != nil && th.Mode == "theme" {
		if th.Theme.InFile || th.Theme.DraftRule == "" {
			return nil // never ask to write down a rule that is already in a file
		}
		return &Draft{Line: th.Theme.DraftRule, Label: s.T("draft_theme", "file", fileOf[th.Theme.Tool])}
	}
	if th == nil || (k != "BROKE_CHAT" && k != "PROMPT") || (k == "BROKE_CHAT" && th.Mode == "example") {
		return nil
	}
	line := ""
	switch {
	case th.E.Fix != "":
		line = th.E.Fix
	case k == "BROKE_CHAT":
		line = th.Quote
	}
	if line == "" {
		return nil
	}
	lab := "draft_chat"
	if k == "PROMPT" {
		lab = "draft_prompt"
	}
	return &Draft{Line: line, Label: s.T(lab, "file", s.Plain(f).File)}
}

// BigNum is the fix section's number: sessions hit, or dollars.
func (s *Story) BigNum(f *Finding) string {
	if f.Side == "leak" {
		return Money(f.USD)
	}
	return fmt.Sprintf("%d / %d", f.Sessions, s.M.J) // theme mode too: the finding's size comes first
}

// goingWell is only measured, true statements (§17.4); empty means the section is left out.
func (s *Story) goingWell() []GoodStat {
	m := s.M
	var out []GoodStat
	ok, und := 0, 0
	for _, v := range m.Verdicts {
		ok += v.Counts["as_intended"] + v.Counts["minor_drift"]
		und += v.Counts["undeterminable"]
	}
	if m.J >= MinJudged && float64(ok)/float64(m.J) >= 0.6 {
		out = append(out, GoodStat{Key: "ok", Value: fmt.Sprintf("%d / %d", ok, m.J), Text: s.T("g_ok"), Caption: s.T("g_ok_cap", "j", m.J, "u", und), On: ok, All: m.J})
	}
	if m.HasLeak && m.TotalUSD > 0 && s.St.Share <= CostCuts[1] {
		k := 100 - int(math.Round(s.St.Share*100))
		out = append(out, GoodStat{Key: "spend", Value: Pct(1 - s.St.Share), Text: s.T("g_spend"), Caption: s.T("g_spend_cap"), On: k, All: 100, Waffle: true})
	}
	known, fixed := 0, 0
	for _, r := range m.Records {
		for _, e := range r.Events {
			switch e.Resolution {
			case "resolved", "user_took_over":
				fixed++
				known++
			case "unresolved":
				known++
			}
		}
	}
	if known >= 3 && float64(fixed)/float64(known) >= 0.5 {
		out = append(out, GoodStat{Key: "fixed", Value: fmt.Sprintf("%d / %d", fixed, known), Text: s.T("g_fixed"), Caption: s.T("g_fixed_cap", "m", known), On: fixed, All: known})
	}
	return out
}

// ---------------------------------------------------------------- evidence selection (§16.6)

var wordyBad = regexp.MustCompile(`https?://|\.(md|pdf|py|go|js|ts|json)\b|#page=`)

// Wordy says whether a quote reads as natural language: 8 to 160 characters, at least 60% letters, not a link,
// path or file name.
func Wordy(q string) bool {
	q = strings.TrimSpace(q)
	n := utf8.RuneCountInString(q)
	if n < 8 || n > 160 || wordyBad.MatchString(q) {
		return false
	}
	for _, p := range []string{"[", "/", "`", "http"} {
		if strings.HasPrefix(q, p) {
			return false
		}
	}
	letters, all := 0, 0
	for _, r := range q {
		if r == ' ' {
			continue
		}
		all++
		if unicode.IsLetter(r) {
			letters++
		}
	}
	return all > 0 && float64(letters)/float64(all) >= 0.6
}

// PickEvents chooses up to k cases that illustrate a finding: one per session, readable quotes only, bigger misses
// and ones that quote the instruction first.
func PickEvents(f *Finding, k int) []*Event {
	score := func(e *Event) int {
		overlap := 0
		if e.InstrQuote != "" {
			ti := tokens(e.InstrQuote)
			for t := range tokens(e.Quote) {
				if ti[t] {
					overlap = 1
				}
			}
		}
		known := e.Resolution == "resolved" || e.Resolution == "unresolved" || e.Resolution == "user_took_over"
		return 2*b2i(e.Size == "major") + b2i(e.InstrQuote != "") + overlap + b2i(known)
	}
	evs := append([]*Event(nil), f.Events...)
	sort.SliceStable(evs, func(i, j int) bool { return score(evs[i]) > score(evs[j]) })
	seen := map[string]bool{}
	var out []*Event
	for _, e := range evs {
		if seen[e.Rec.Key] || !Wordy(e.Quote) {
			continue
		}
		seen[e.Rec.Key] = true
		out = append(out, e)
		if len(out) == k {
			break
		}
	}
	return out
}

var sentRe = regexp.MustCompile(`^(.+?[.!?。])(\s|$)`)

// ShortText is the agent turn: the first sentence when it is long enough, cut at a word boundary.
func ShortText(t string, n int) string {
	t = collapse(t)
	if m := sentRe.FindStringSubmatch(t); m != nil && utf8.RuneCountInString(m[1]) >= 30 {
		t = m[1]
	}
	if utf8.RuneCountInString(t) > n {
		r := []rune(t)
		t = string(r[:n])
		if i := strings.LastIndex(t, " "); i > 0 {
			t = t[:i]
		}
		t = strings.TrimRight(t, ",;:") + "…"
	}
	return t
}

// Clip shows at most n characters of a quote (openings: 44).
func Clip(t string, n int) string {
	t = collapse(t)
	r := []rune(t)
	if len(r) <= n {
		return t
	}
	return strings.TrimRight(string(r[:n-1]), " ") + "…"
}

// SameWords says whether two texts are the same after GroupKey.
func SameWords(a, b string) bool { return GroupKey(a) == GroupKey(b) }

// CasesFor is the evidence cases of a fix: in theme mode the theme's own events first (one per session), then the
// usual pick.
func (s *Story) CasesFor(f *Finding, th *Thing) []*Event {
	evs := PickEvents(f, 3)
	if th == nil || th.Mode != "theme" {
		return evs
	}
	in := map[*Event]bool{}
	for _, e := range f.Events {
		in[e] = true
	}
	seen := map[string]bool{}
	var out []*Event
	for _, e := range append(append([]*Event{}, th.Theme.Events...), evs...) {
		if in[e] && !seen[e.Rec.Key] {
			seen[e.Rec.Key] = true
			out = append(out, e)
		}
	}
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

// DriftRun says whether any session in the period was judged; without it the rating rests on cost only.
func (s *Story) DriftRun() bool { return s.M.J > 0 }

// DriftCmd is the command that runs the drift check for this report's period.
func (s *Story) DriftCmd() string {
	if s.M.Tool != "" {
		return fmt.Sprintf("agent-leeway drift --tool %s --since %dd", s.M.Tool, s.days())
	}
	return fmt.Sprintf("agent-leeway drift --since %dd", s.days())
}

// days is the period in days (30 when unknown).
func (s *Story) days() int {
	if s.M.Days <= 0 {
		return 30
	}
	return s.M.Days
}
