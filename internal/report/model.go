package report

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"

	"github.com/Mango-Juice/agent-leeway/internal/advice"
	"github.com/Mango-Juice/agent-leeway/internal/drift"
	"github.com/Mango-Juice/agent-leeway/internal/leak"
	"github.com/Mango-Juice/agent-leeway/internal/py"
	"github.com/Mango-Juice/agent-leeway/internal/secrets"
)

// The report model (docs/24 sections 3, 5 and 6): findings built from the judged records and the leak report,
// ranked, with per-tool advice and the lines that may be copied. Pure functions, no I/O; the renderers
// (Markdown, terminal, HTML) only lay it out.

// Verdicts in display order.
var Verdicts = []string{"agent_drift", "prompt_gap", "minor_drift", "as_intended", "undeterminable"}

// Kinds in their fixed order (docs/24 section 3.1).
var Kinds = []string{"RESERVED", "FALSE_DONE", "ASKED_ONLY", "BROKE_SAID", "MISTAKE", "PROMPT"}

// Levels of confidence, highest first.
var Levels = []string{"measured", "firm", "likely", "check"}

// KindOf maps a judged event to a finding kind (first match wins); "" when its cause is undeterminable, which
// never becomes a finding.
func KindOf(rule, cause string) string {
	switch {
	case rule == "R6a" || rule == "R6b":
		return "RESERVED"
	case rule == "R6c":
		return "FALSE_DONE"
	case rule == "R3a":
		return "ASKED_ONLY"
	case cause == "missing_or_ambiguous_instruction":
		return "PROMPT"
	case cause == "instruction_violation":
		return "BROKE_SAID"
	case cause == "agent_error":
		return "MISTAKE"
	}
	return ""
}

// Record is one judged session (the latest successful result per session and judge).
type Record struct {
	Key, Tool, Tier, Project, Verdict string
	RubricVersion, RubricSource       string
	JudgeCLI, Origin                  string
	Partial                           bool
	OmittedChars                      int
	InstrWarning                      bool
	Quotes, BadQuotes, Dropped        int
	Defects, Weak                     int
	Events                            []*Event
}

// Short is the last 12 characters of the session key, as shown in the report.
func (r *Record) Short() string { return py.Tail(r.Key, 12) }

// Judge is the judging model name of the record's tier ("opus" of "claude:opus").
func (r *Record) Judge() string {
	if _, m, ok := strings.Cut(r.Tier, ":"); ok {
		return m
	}
	return r.Tier
}

// Event is one judged event, with every displayed text masked.
type Event struct {
	Rec                  *Record
	Loc, Rule, Cause     string
	Size, Resolution     string
	Kind, CauseGroup     string
	Summary              string // judge-written: the user's language since TaskRevision t2, English in older results
	Quote                string // first evidence quote
	InstrLoc, InstrQuote string
	Fix                  string // the judge's suggested sentence
	Idx                  int    // position in the record's events
	Side                 string // cause_side: agent_side | prompt_side | undeterminable
}

// Ref is "<session tail>#<loc>".
func (e *Event) Ref() string { return e.Rec.Short() + "#" + e.Loc }

// Repeated names a repeated finding: the quote and where it comes from ("chat", "file" or "label").
type Repeated struct{ Text, From string }

// ToolAdvice is one tool's advice on a finding.
type ToolAdvice struct {
	Tool string
	A    advice.ToolAdvice
}

// CopyLine is one line that may be copied into an instruction file.
type CopyLine struct {
	Text string
	Kind string // sourced | draft | repeat
	Refs []string
	N    int // sessions, for "repeat"
}

// CopyScope is the lines for one file.
type CopyScope struct {
	Tool    string
	Project string // "" = the user-level file
	Lines   []CopyLine
}

// SettingExample is a setting offered as "example to adapt".
type SettingExample struct {
	Tool string
	S    advice.Setting
}

// Finding is one card or detail block.
type Finding struct {
	Side     string // drift | leak
	ID       string // anchor id, unique
	AdviceID string // catalogue id
	Kind     string // drift kind
	Habit    string // leak habit
	Tier     string // S | A | B | "" (leftover)
	N        int    // position in the top section (1-based), 0 when not on top

	// drift
	Events                 []*Event
	Sessions, NEvents, Maj int
	Repeated               *Repeated
	Partial                bool
	tiers                  map[string]bool

	// leak
	USD   float64
	Count int // 0 for automation
	Where map[string]leak.Where

	Tools []string
	Share float64 // drift: sessions / judged; leak: dollars / spend
	Level string
	Notes []string // message keys

	Advice   []ToolAdvice
	Against  []advice.Against
	Copy     []CopyScope
	Settings []SettingExample
}

// NCopy counts the lines and settings offered for copying.
func (f *Finding) NCopy() int {
	n := len(f.Settings)
	for _, c := range f.Copy {
		n += len(c.Lines)
	}
	return n
}

// Input is what the model is built from.
type Input struct {
	Leak          *leak.Report // nil: drift only
	Records       []py.Value   // judged results (the latest per session and judge is used)
	Advice        *advice.Catalogue
	Lang          string
	Now           time.Time
	Since, Until  time.Time // the period, used when Leak is nil
	PricesRead    string
	BundledRubric string
	QuoteChars    int
	Groups        []py.Value     // stored grouping records (drift.LoadGroups); empty = none
	LeftOut       map[string]int // sessions of a tool in the window that were not selected for judging (tool -> count)
	Tool          string         // the tool the report was asked for (--tool); "" = both
	Unjudged      map[string]int // the asked-for tools' sessions in the window with no judged result (tool -> count)
	OtherJudged   map[string]int // with Tool set: the other tool's judged sessions, left out of this report
}

// Model is everything the renderers show.
type Model struct {
	Lang                      string
	From, To                  string // YYYY-MM-DD
	Days                      int
	NSessions                 int // sessions in the period (leak side), -1 when unknown
	TotalUSD, LeakUSD         float64
	HasLeak                   bool
	LeakR                     *leak.Report
	Advice                    *advice.Catalogue
	Themes                    []*Theme
	LeftOut                   map[string]int
	Tool                      string
	Unjudged, OtherJudged     map[string]int
	Records                   []*Record
	J                         int // judged sessions (records)
	Verdicts                  []JudgeVerdicts
	Findings, Leftovers, Leak []*Finding
	Top                       []*Finding
	More                      int // ranked findings not on top
	Quotes, Bad, Dropped      int
	InstrCurrent              int
	Masked                    int
	RubricVersions, Custom    []string
	Bundled                   string
	PartialSessions           int
	OmittedChars              int
	OriginUncertain           int
	OriginUncertainUSD        float64
	CLIs                      []string
	Tiers                     []TierCount
	PricesRead                string
	PricesStale               bool
	PricesDays                int
	AdviceOldest              string
	AdviceDays                int
	AdviceStale               bool
	AdviceVersion, AdvicePath string
}

// JudgeVerdicts is one judge's verdict counts (one strip per judge; never on a shared axis).
type JudgeVerdicts struct {
	Tool, Tier string
	Counts     map[string]int
	Total      int
}

// TierCount is sessions per judging tier.
type TierCount struct {
	Tier string
	N    int
}

// NCopy is the number of lines and settings offered for the top findings.
func (m *Model) NCopy() int {
	n := 0
	for _, f := range m.Top {
		n += f.NCopy()
	}
	return n
}

// ---------------------------------------------------------------- text matching (docs/24 section 3.2)

var (
	wsRe    = regexp.MustCompile(`\s+`)
	trailRe = regexp.MustCompile(`[\s.!?。．！？]+$`)
	tokRe   = regexp.MustCompile(`[\p{L}\p{M}\p{N}_]+`)
	folder  = cases.Fold()
)

// Norm is NFC, case-folded, whitespace collapsed, trailing punctuation stripped.
func Norm(s string) string {
	s = folder.String(norm.NFC.String(s))
	s = wsRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(trailRe.ReplaceAllString(s, ""))
}

func tokens(s string) map[string]bool {
	m := map[string]bool{}
	for _, t := range tokRe.FindAllString(Norm(s), -1) {
		m[t] = true
	}
	return m
}

// Similar is equal after Norm, or token Jaccard >= th.
func Similar(a, b string, th float64) bool {
	if Norm(a) == Norm(b) {
		return true
	}
	ta, tb := tokens(a), tokens(b)
	if len(ta) == 0 || len(tb) == 0 {
		return false
	}
	inter := 0
	for t := range ta {
		if tb[t] {
			inter++
		}
	}
	return float64(inter)/float64(len(ta)+len(tb)-inter) >= th
}

func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// ---------------------------------------------------------------- records

func str(v py.Value) string {
	if v.K == py.Str {
		return v.S
	}
	return ""
}

func num(v py.Value) int {
	f, _ := v.Num()
	return int(f)
}

// Records reads the judged results; masked counts the event texts shown with a secret masked.
func Records(raw []py.Value, quoteChars int) (recs []*Record, masked int) {
	if quoteChars <= 0 {
		quoteChars = 160
	}
	mask := func(v py.Value, clip int) string {
		s, ok := secrets.Masked(str(v))
		if ok {
			masked++
		}
		if clip > 0 {
			return drift.ClipText(py.String(s), clip)
		}
		return collapse(s)
	}
	for _, r := range drift.Latest(raw) {
		out := r.Get("out")
		st := drift.SettingsFromValue(r.Get("settings"))
		evs := out.Get("events")
		var list []py.Value
		if evs.K == py.Arr {
			list = evs.A
		}
		th := 3
		if r.Get("settings").Truthy() {
			th = st.S1Threshold()
		}
		eff := drift.EffectiveSizes(list, th)
		verdict := str(r.Get("derived"))
		if verdict == "" {
			s := st
			if !r.Get("settings").Truthy() {
				s = nil
			}
			verdict = drift.DerivedVerdict(out, s)
		}
		rec := &Record{Key: str(r.Get("key")), Tool: str(r.Get("tool")), Tier: str(r.Get("tier")), Project: str(r.Get("project")), Verdict: verdict,
			RubricVersion: str(r.Get("rubric_version")), RubricSource: str(r.Get("rubric_source")), JudgeCLI: str(r.Get("judge_cli")),
			Origin: str(r.Get("origin")), InstrWarning: r.Get("instr_warning").Truthy(),
			Quotes: num(r.Get("verify").Get("quotes")), BadQuotes: num(r.Get("verify").Get("bad_quotes")), Dropped: num(r.Get("dropped_events"))}
		if p := r.Get("partial"); p.K == py.Obj {
			rec.Partial, rec.OmittedChars = true, num(p.Get("omitted_chars"))
		}
		if d := out.Get("defect_reports"); d.K == py.Arr {
			rec.Defects = len(d.A)
		}
		if w := out.Get("weak_signals"); w.K == py.Arr {
			rec.Weak = len(w.A)
		}
		for i, e := range list {
			ev := &Event{Rec: rec, Loc: str(e.Get("loc")), Rule: str(e.Get("rule")), Cause: str(e.Get("cause")), Size: eff[i],
				Resolution: str(e.Get("resolution")), CauseGroup: collapse(str(e.Get("cause_group"))), Idx: i, Side: str(e.Get("cause_side"))}
			ev.Kind = KindOf(ev.Rule, ev.Cause)
			if q := e.Get("evidence"); q.K == py.Arr {
				for j, x := range q.A {
					s := mask(x.Get("quote"), quoteChars)
					if j == 0 {
						ev.Quote = s
					}
				}
			}
			if ins := e.Get("instruction"); ins.K == py.Obj {
				ev.InstrLoc = str(ins.Get("loc"))
				ev.InstrQuote = mask(ins.Get("quote"), 0)
			}
			ev.Fix = mask(e.Get("fix_sentence"), 0)
			ev.Summary = mask(e.Get("summary"), 0)
			rec.Events = append(rec.Events, ev)
		}
		recs = append(recs, rec)
	}
	return recs, masked
}

// ---------------------------------------------------------------- drift findings (sections 3.2, 3.3, 5)

func newDrift(kind string, evs []*Event, rep *Repeated, judged int) *Finding {
	f := &Finding{Side: "drift", Kind: kind, Events: evs, Repeated: rep, tiers: map[string]bool{}}
	ss, tools := map[string]bool{}, map[string]bool{}
	for _, e := range evs {
		ss[e.Rec.Key] = true
		tools[e.Rec.Tool] = true
		f.tiers[e.Rec.Tier] = true
		if e.Size == "major" {
			f.Maj++
		}
		f.Partial = f.Partial || e.Rec.Partial
	}
	f.Sessions, f.NEvents = len(ss), len(evs)
	for _, t := range []string{"claude", "codex"} {
		if tools[t] {
			f.Tools = append(f.Tools, t)
		}
	}
	f.Share = float64(f.Sessions) / math.Max(float64(judged), 1)
	return f
}

// Findings builds the drift findings: within each kind, events whose cause_group or cited instruction match
// are clustered; a cluster spanning 2 or more sessions is a repeated finding (tier S) and leaves the kind's
// broad finding. Leftovers are findings too small for a tier.
func Findings(recs []*Record) (findings, leftovers []*Finding) {
	judged := len(recs)
	for _, k := range Kinds {
		var ke []*Event
		for _, r := range recs {
			for _, e := range r.Events {
				if e.Kind == k {
					ke = append(ke, e)
				}
			}
		}
		if len(ke) == 0 {
			continue
		}
		type cluster struct {
			label, iq string
			ev        []*Event
		}
		var cs []*cluster
		for _, e := range ke {
			placed := false
			for _, c := range cs {
				if (e.CauseGroup != "" && c.label != "" && Similar(e.CauseGroup, c.label, 0.5)) || (e.InstrQuote != "" && c.iq != "" && Similar(e.InstrQuote, c.iq, 0.5)) {
					c.ev = append(c.ev, e)
					placed = true
					break
				}
			}
			if !placed {
				cs = append(cs, &cluster{label: e.CauseGroup, iq: e.InstrQuote, ev: []*Event{e}})
			}
		}
		var rest []*Event
		hasRep := false
		for _, c := range cs {
			ss := map[string]bool{}
			for _, e := range c.ev {
				ss[e.Rec.Key] = true
			}
			if len(ss) < 2 {
				rest = append(rest, c.ev...)
				continue
			}
			var rep *Repeated
			var best *Event
			for _, e := range c.ev {
				if e.InstrQuote != "" && (best == nil || len([]rune(e.InstrQuote)) < len([]rune(best.InstrQuote))) {
					best = e
				}
			}
			switch {
			case best != nil:
				from := "file"
				if strings.HasPrefix(best.InstrLoc, "U") {
					from = "chat"
				}
				rep = &Repeated{collapse(best.InstrQuote), from}
			default:
				rep = &Repeated{c.label, "label"}
			}
			f := newDrift(k, c.ev, rep, judged)
			f.Tier = "S"
			findings = append(findings, f)
			hasRep = true
		}
		if len(rest) > 0 {
			f := newDrift(k, rest, nil, judged)
			switch {
			case (f.Sessions >= 2 && (f.Maj > 0 || f.NEvents >= 3)) || ((k == "RESERVED" || k == "FALSE_DONE") && !hasRep):
				f.Tier = "A"
			case f.NEvents >= 2 || f.Maj > 0:
				f.Tier = "B"
			}
			if f.Tier != "" {
				findings = append(findings, f)
			} else {
				leftovers = append(leftovers, f)
			}
		}
	}
	reps := 0
	for _, f := range append(append([]*Finding{}, findings...), leftovers...) {
		f.ID = "drift-" + strings.ToLower(f.Kind)
		if f.Repeated != nil {
			reps++
			f.ID += "-rep" + itoa(reps)
		}
		f.Level, f.Notes = Confidence(f)
	}
	return findings, leftovers
}

func itoa(n int) string { return py.PyStr(py.Value{K: py.Int, I: int64(n)}) }

// Confidence is the badge and the notes of a finding (docs/24 section 5).
func Confidence(f *Finding) (string, []string) {
	if f.Side == "leak" {
		return "measured", nil
	}
	lvl := "check"
	switch {
	case f.Sessions >= 2 && f.NEvents >= 3:
		lvl = "firm"
	case f.Sessions >= 2 || f.Maj > 0:
		lvl = "likely"
	}
	var notes []string
	codexOnly := len(f.tiers) > 0
	for t := range f.tiers {
		codexOnly = codexOnly && strings.HasPrefix(t, "codex:")
	}
	if codexOnly {
		notes = append(notes, "note_codex_only")
		switch lvl {
		case "firm":
			lvl = "likely"
		case "likely":
			lvl = "check"
		}
	}
	if f.Kind == "PROMPT" {
		notes = append(notes, "note_under")
	}
	if f.Partial {
		notes = append(notes, "note_partial")
	}
	return lvl, notes
}

// ---------------------------------------------------------------- leak findings

// LeakFindings are the habits with a leak, and an automation finding when automations lose at least 2% of
// spend. Tier A: at least 2% of spend and at least $1; tier B: at least 0.5%.
func LeakFindings(R *leak.Report) []*Finding {
	if R == nil {
		return nil
	}
	total := math.Max(R.TotalUSD, 1e-9)
	var out []*Finding
	for _, h := range R.Habits {
		if h.USD <= 0 {
			continue
		}
		f := &Finding{Side: "leak", ID: "leak-" + strings.ReplaceAll(h.Habit, "_", "-"), AdviceID: "leak." + h.Habit, Habit: h.Habit, USD: h.USD, Count: h.Count,
			Where: h.Where, Share: h.USD / total}
		switch {
		case f.Share >= 0.02 && h.USD >= 1:
			f.Tier = "A"
		case f.Share >= 0.005:
			f.Tier = "B"
		}
		f.Tools = habitTools(R, h.Causes, false)
		f.Level, f.Notes = Confidence(f)
		out = append(out, f)
	}
	auto := 0.0
	for _, a := range R.AutomationFixes {
		auto += a.Leak
	}
	if auto > 0 && auto/total >= 0.02 {
		f := &Finding{Side: "leak", ID: "leak-automation", AdviceID: "leak.automation", Habit: "automation", USD: auto, Share: auto / total, Tier: "A",
			Tools: habitTools(R, nil, true)}
		f.Level, f.Notes = Confidence(f)
		out = append(out, f)
	}
	return out
}

// habitTools are the tools with at least 10% of a habit's direct-conversation dollars (per session: the cause
// amounts times 1 - auto_share). For automation, the automated share of each session's leak is used instead.
func habitTools(R *leak.Report, causes []string, automation bool) []string {
	by := map[string]float64{}
	tot := 0.0
	for _, s := range R.Sessions {
		v := 0.0
		if automation {
			v = s.Leak * s.AutoShare
		} else {
			for _, c := range causes {
				v += s.ByCause[c]
			}
			v *= 1 - s.AutoShare
		}
		by[s.Tool] += v
		tot += v
	}
	var out []string
	for _, t := range []string{"claude", "codex"} {
		if tot > 0 && by[t]/tot >= 0.1 {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		for _, t := range []string{"claude", "codex"} {
			if _, ok := by[t]; ok {
				out = append(out, t)
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- ranking

// levelRank puts measured and firm together on top, then likely, then check.
func levelRank(l string) int {
	switch l {
	case "measured", "firm":
		return 3
	case "likely":
		return 2
	}
	return 1
}

// RepeatLift is how much a repeated finding's size counts within its confidence level.
const RepeatLift = 2.0

// Top picks the cards for "What to fix first". The rule (docs/24 section 3.3, as revised):
//  1. Only ranked findings are candidates: drift tiers S, A and B, leak tiers A and B.
//  2. Confidence comes first: measured and firm, then likely, then check. A "check this" finding therefore never
//     takes the first place while a firm or measured one exists.
//  3. Within a level, size decides: drift share is affected sessions / judged sessions, leak share is dollars /
//     spend. A repeated, specific finding counts its share twice (RepeatLift), so it can rise within its level
//     but never above a higher level.
//  4. Ties: repeated first, then more major events, then the kind order (leak after drift), then the id.
//  5. At most 5 cards and at most 3 per side.
func Top(findings, leaks []*Finding) []*Finding {
	var c []*Finding
	for _, f := range findings {
		if f.Tier != "" {
			c = append(c, f)
		}
	}
	for _, f := range leaks {
		if f.Tier == "A" || f.Tier == "B" {
			c = append(c, f)
		}
	}
	kindIdx := func(f *Finding) int {
		for i, k := range Kinds {
			if k == f.Kind {
				return i
			}
		}
		return len(Kinds)
	}
	weight := func(f *Finding) float64 {
		if f.Repeated != nil {
			return f.Share * RepeatLift
		}
		return f.Share
	}
	sort.SliceStable(c, func(i, j int) bool {
		a, b := c[i], c[j]
		if x, y := levelRank(a.Level), levelRank(b.Level); x != y {
			return x > y
		}
		if x, y := weight(a), weight(b); math.Abs(x-y) > 1e-12 {
			return x > y
		}
		if (a.Repeated != nil) != (b.Repeated != nil) {
			return a.Repeated != nil
		}
		if a.Maj != b.Maj {
			return a.Maj > b.Maj
		}
		if x, y := kindIdx(a), kindIdx(b); x != y {
			return x < y
		}
		return a.ID < b.ID
	})
	var top []*Finding
	side := map[string]int{}
	for _, f := range c {
		if len(top) == 5 {
			break
		}
		if side[f.Side] == 3 {
			continue
		}
		side[f.Side]++
		top = append(top, f)
		f.N = len(top)
	}
	return top
}

// ---------------------------------------------------------------- advice and copying (section 6)

// AdviceID is the catalogue id of a finding: BROKE_SAID uses the "file" variant only when more of its events
// cite an instruction-file line (I#) than a user turn (U#).
func AdviceID(f *Finding) string {
	if f.Side == "leak" {
		return f.AdviceID
	}
	switch f.Kind {
	case "BROKE_FILE":
		return "drift.BROKE_SAID.file"
	case "BROKE_CHAT":
		return "drift.BROKE_SAID.chat"
	}
	if f.Kind == "BROKE_SAID" {
		i, u := 0, 0
		for _, e := range f.Events {
			switch {
			case strings.HasPrefix(e.InstrLoc, "I"):
				i++
			case strings.HasPrefix(e.InstrLoc, "U"):
				u++
			}
		}
		if i > u {
			return "drift.BROKE_SAID.file"
		}
		return "drift.BROKE_SAID.chat"
	}
	return "drift." + f.Kind
}

// Advise sets the per-tool advice (claude, then codex) and the evidence against it.
func Advise(f *Finding, cat *advice.Catalogue) {
	f.AdviceID = AdviceID(f)
	e := cat.Get(f.AdviceID)
	f.Advice, f.Against = nil, nil
	if e == nil {
		return
	}
	for _, t := range f.Tools {
		if a, ok := e.Tools[t]; ok {
			f.Advice = append(f.Advice, ToolAdvice{t, a})
		}
	}
	for _, x := range e.Against {
		if x.Tool == "" || contains(f.Tools, x.Tool) {
			f.Against = append(f.Against, x)
		}
	}
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// CopyBlocks sets what may be copied (only standing rules):
//   - the catalogue's instruction line for each tool (user-level file);
//   - the judge's suggested sentence on a "missing or ambiguous instruction" event, as a draft;
//   - the name of a repeated finding whose quote came from a user turn (something said in 2 or more sessions).
//
// Drafts and repeated requirements go to the project's file when exactly one project is involved, else to the
// user-level file. Lines are de-duplicated at token Jaccard >= 0.8 (references merged), at most 5 per file.
// Settings are offered as examples to adapt.
func CopyBlocks(f *Finding, lang string) {
	f.Copy, f.Settings = nil, nil
	idx := map[[2]string]int{}
	add := func(tool, project string, l CopyLine) {
		k := [2]string{tool, project}
		i, ok := idx[k]
		if !ok {
			idx[k] = len(f.Copy)
			f.Copy = append(f.Copy, CopyScope{Tool: tool, Project: project})
			i = len(f.Copy) - 1
		}
		sc := &f.Copy[i]
		for j := range sc.Lines {
			if Similar(sc.Lines[j].Text, l.Text, 0.8) {
				sc.Lines[j].Refs = append(sc.Lines[j].Refs, l.Refs...)
				return
			}
		}
		if len(sc.Lines) < 5 {
			sc.Lines = append(sc.Lines, l)
		}
	}
	one := func(ps map[string]bool) string {
		if len(ps) == 1 {
			for p := range ps {
				return p
			}
		}
		return ""
	}
	for _, a := range f.Advice {
		if a.A.Setting != nil {
			f.Settings = append(f.Settings, SettingExample{a.Tool, *a.A.Setting})
		}
		if t := advice.Pick(a.A.Instruction, lang); t != "" {
			add(a.Tool, "", CopyLine{Text: t, Kind: "sourced", N: 1})
		}
	}
	if f.Side != "drift" {
		return
	}
	for _, e := range f.Events {
		if e.Cause != "missing_or_ambiguous_instruction" || strings.TrimSpace(e.Fix) == "" {
			continue
		}
		ps := map[string]bool{}
		for _, x := range f.Events {
			if x.Fix != "" && Similar(x.Fix, e.Fix, 0.8) {
				ps[x.Rec.Project] = true
			}
		}
		add(e.Rec.Tool, one(ps), CopyLine{Text: strings.TrimSpace(e.Fix), Kind: "draft", Refs: []string{e.Ref()}, N: 1})
	}
	if f.Repeated != nil && f.Repeated.From == "chat" {
		ps := map[string]bool{}
		var refs []string
		for _, e := range f.Events {
			ps[e.Rec.Project] = true
			refs = append(refs, e.Ref())
		}
		for _, t := range f.Tools {
			add(t, one(ps), CopyLine{Text: f.Repeated.Text, Kind: "repeat", Refs: refs, N: f.Sessions})
		}
	}
}

// ---------------------------------------------------------------- the whole model

// NewModel assembles the model.
func NewModel(in Input) *Model {
	m := &Model{Lang: in.Lang, NSessions: -1, Bundled: in.BundledRubric, PricesRead: in.PricesRead, Advice: in.Advice}
	if in.Leak != nil {
		m.HasLeak = true
		m.LeakR = in.Leak
		m.From, m.To = in.Leak.WindowStart.Format("2006-01-02"), in.Leak.WindowEnd.Format("2006-01-02")
		m.TotalUSD, m.LeakUSD = in.Leak.TotalUSD, in.Leak.LeakUSD
		m.NSessions = len(in.Leak.Sessions)
		m.OriginUncertain, m.OriginUncertainUSD = in.Leak.Info.Uncertain, in.Leak.Info.UncertainUSD
	} else {
		m.From, m.To = in.Since.UTC().Format("2006-01-02"), in.Until.UTC().Format("2006-01-02")
	}
	if a, err := time.Parse("2006-01-02", m.From); err == nil {
		if b, err := time.Parse("2006-01-02", m.To); err == nil {
			m.Days = int(b.Sub(a).Hours() / 24)
		}
	}
	m.Records, m.Masked = Records(in.Records, in.QuoteChars)
	m.J = len(m.Records)
	byJudge := map[string]*JudgeVerdicts{}
	tiers := map[string]int{}
	vers, custom, clis := map[string]bool{}, map[string]bool{}, map[string]bool{}
	recUncertain := 0
	for _, r := range m.Records {
		k := r.Tool + "|" + r.Tier
		jv := byJudge[k]
		if jv == nil {
			jv = &JudgeVerdicts{Tool: r.Tool, Tier: r.Tier, Counts: map[string]int{}}
			byJudge[k] = jv
		}
		jv.Counts[r.Verdict]++
		jv.Total++
		tiers[r.Tier]++
		m.Quotes += r.Quotes
		m.Bad += r.BadQuotes
		m.Dropped += r.Dropped
		if r.InstrWarning {
			m.InstrCurrent++
		}
		if r.RubricVersion != "" {
			vers[r.RubricVersion] = true
		}
		if r.RubricSource == "custom" {
			custom[r.RubricVersion] = true
		}
		if r.Partial {
			m.PartialSessions++
			m.OmittedChars += r.OmittedChars
		}
		if r.Origin == "uncertain" {
			recUncertain++
		}
		if r.JudgeCLI != "" {
			clis[r.JudgeCLI] = true
		}
	}
	if !m.HasLeak {
		m.OriginUncertain = recUncertain
	}
	for _, k := range sortedKeys(byJudge) {
		m.Verdicts = append(m.Verdicts, *byJudge[k])
	}
	for _, t := range sortedKeys(tiers) {
		m.Tiers = append(m.Tiers, TierCount{t, tiers[t]})
	}
	m.RubricVersions, m.Custom, m.CLIs = sortedKeys(vers), sortedKeys(custom), sortedKeys(clis)
	m.Themes = Themes(in.Groups, m.Records)
	m.LeftOut, m.Tool, m.Unjudged, m.OtherJudged = in.LeftOut, in.Tool, in.Unjudged, in.OtherJudged
	m.Findings, m.Leftovers = Findings(m.Records)
	m.Leak = LeakFindings(in.Leak)
	m.Top = Top(m.Findings, m.Leak)
	for _, f := range append(append(append([]*Finding{}, m.Findings...), m.Leftovers...), m.Leak...) {
		Advise(f, in.Advice)
		CopyBlocks(f, in.Lang)
		if f.Tier != "" && f.N == 0 {
			m.More++
		}
	}
	if in.PricesRead != "" {
		if t, err := time.Parse("2006-01-02", in.PricesRead); err == nil {
			m.PricesDays = int(in.Now.Sub(t).Hours() / 24)
			m.PricesStale = m.PricesDays > 90
		}
	}
	if in.Advice != nil {
		m.AdviceOldest, m.AdviceDays, m.AdviceStale = in.Advice.Stale(in.Now)
		m.AdviceVersion, m.AdvicePath = in.Advice.Version, in.Advice.Path
	}
	return m
}

func sortedKeys[V any](m map[string]V) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
