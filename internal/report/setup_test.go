package report

import (
	"html"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/advice"
	"github.com/Mango-Juice/agent-leeway/internal/leak"
	"github.com/Mango-Juice/agent-leeway/internal/logs"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

var seen = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

func ccSkills(n, k int) SetupItem {
	it := SetupItem{Tool: "claude", Kind: "skills", N: n, K: k, Over: k > 0, Project: "trail-planner", SeenAt: seen, Share: float64(k) / float64(n)}
	for i := 0; i < k; i++ {
		it.Names = append(it.Names, "skill-"+itoa(i))
	}
	return it
}

func ccInstr(lines int) SetupItem {
	return SetupItem{Tool: "claude", Kind: "instr", N: lines, Limit: 200, Over: lines > 200, Project: "trail-planner", File: "CLAUDE.md", SeenAt: seen,
		Share: float64(lines-200) / 200}
}

func cxAgents(bytes, limit int) SetupItem {
	return SetupItem{Tool: "codex", Kind: "instr", N: bytes, Limit: limit, Over: bytes >= limit, Project: "invoice-sync", SeenAt: seen}
}

func setupModel(t *testing.T, lang string, R *leak.Report, recs []py.Value, items ...SetupItem) *Model {
	t.Helper()
	m := NewModel(Input{Leak: R, Records: recs, Advice: advice.Builtin(), Lang: lang, Now: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC),
		Since: time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)})
	m.Setup = items
	return m
}

// firm drift (2 sessions, 3 events), measured cost, and a likely drift finding (2 sessions, 2 minor events)
func threeFixes(t *testing.T) (*leak.Report, []py.Value) {
	recs := judged(t, 10, 1)
	recs = append(recs,
		withVerdict(rec(t, "a", "claude", "p", ev{Loc: "U1", Rule: "R6c", Cause: "agent_error", Size: "major", Quote: "it was not done"},
			ev{Loc: "U2", Rule: "R6c", Cause: "agent_error", Size: "major", Quote: "still not done"}), "agent_drift"),
		withVerdict(rec(t, "b", "claude", "p", ev{Loc: "U1", Rule: "R6c", Cause: "agent_error", Size: "major", Quote: "not done again"}), "agent_drift"),
		withVerdict(rec(t, "c", "claude", "p", ev{Loc: "U1", Rule: "R3a", Cause: "agent_error", Size: "minor", Quote: "I only asked"}), "minor_drift"),
		withVerdict(rec(t, "d", "claude", "p", ev{Loc: "U1", Rule: "R3a", Cause: "agent_error", Size: "minor", Quote: "just answer"}), "minor_drift"))
	return leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 9, Count: 3}), recs
}

// §4: a setup fix comes after every firm or measured fix and before every likely or check one; at most one is in the
// top three; it is never Fix 1 while a behaviour fix exists; a displaced fix leads Other things worth fixing.
func TestSetupRanking(t *testing.T) {
	R, recs := threeFixes(t)
	base := NewStory(setupModel(t, "en", R, recs))
	if len(base.Top) != 3 || base.Top[2].Level != "likely" {
		t.Fatalf("fixture: %v", sides(base.Top))
	}
	s := NewStory(setupModel(t, "en", R, recs, ccSkills(72, 22), cxAgents(32772, AgentsMDDefault)))
	if got := sides(s.Top); len(got) != 3 || s.Top[2].Side != "setup" || s.Top[2].Kind != "SETUP_SKILLS" || s.Top[0].Side == "setup" {
		t.Fatalf("top: %v", got)
	}
	o := s.Others()
	// rank order: the second setup fix (measured) first, then the likely fix it pushed out (docs/33 §3 "both")
	if len(o) < 2 || o[0].Kind != "SETUP_INSTR" || o[1].Kind != base.Top[2].Kind {
		t.Fatalf("others: %v", sides(o))
	}
	n := 0
	for _, f := range s.Top {
		if f.Side == "setup" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d setup fixes on top", n)
	}
	// only one behaviour fix, likely: the setup fix is second, never first
	recs2 := append(judged(t, 10, 0),
		withVerdict(rec(t, "c", "claude", "p", ev{Loc: "U1", Rule: "R3a", Cause: "agent_error", Size: "minor", Quote: "I only asked"}), "minor_drift"),
		withVerdict(rec(t, "d", "claude", "p", ev{Loc: "U1", Rule: "R3a", Cause: "agent_error", Size: "minor", Quote: "just answer"}), "minor_drift"))
	s = NewStory(setupModel(t, "en", nil, recs2, ccSkills(72, 22)))
	if len(s.Top) != 2 || s.Top[0].Side == "setup" || s.Top[1].Side != "setup" {
		t.Errorf("likely only: %v", sides(s.Top))
	}
	// nothing else to fix: the setup fix is Fix first
	s = NewStory(setupModel(t, "en", nil, judged(t, 10, 0), ccInstr(236)))
	if len(s.Top) != 1 || s.Top[0].Side != "setup" {
		t.Errorf("alone: %v", sides(s.Top))
	}
	// skill lists before instruction files
	s = NewStory(setupModel(t, "en", nil, judged(t, 10, 0), ccInstr(900), ccSkills(20, 1)))
	if s.Top[0].Kind != "SETUP_SKILLS" {
		t.Errorf("order: %v", sides(s.Top))
	}
	// within the line: never a fix
	s = NewStory(setupModel(t, "en", R, recs, ccSkills(41, 0), ccInstr(97), cxAgents(12000, AgentsMDDefault)))
	if sides(s.Top)[2] != sides(base.Top)[2] || len(s.Setup) != 0 {
		t.Errorf("within: %v", sides(s.Top))
	}
}

var claudeTerms = regexp.MustCompile(`Claude Code|CLAUDE\.md|code\.claude\.com|disable-model-invocation|skill-doctor|skillListingBudgetFraction|\.claude/`)
var codexTerms = regexp.MustCompile(`Codex|AGENTS\.md|learn\.chatgpt\.com|config\.toml|project_doc_max_bytes|skills\.config`)

// A Codex-only user never sees a Claude Code term or source, and the reverse (setup block, fixes, Markdown, terminal).
func TestSetupPerTool(t *testing.T) {
	for _, lang := range []string{"en", "ko"} {
		cx := NewModel(Input{Leak: &leak.Report{TotalUSD: 50, WindowStart: seen.AddDate(0, 0, -30), WindowEnd: seen}, Advice: advice.Builtin(), Lang: lang})
		cx.Setup = []SetupItem{cxAgents(32772, AgentsMDDefault)}
		h, md, term := NewStory(cx).setupOnly(), mdOnly(cx), Terminal(cx, "", "")
		for name, out := range map[string]string{"html": h, "md": md, "term": term} {
			if m := claudeTerms.FindString(out); m != "" {
				t.Errorf("%s %s: Codex-only report has %q", lang, name, m)
			}
		}
		if !strings.Contains(md, "AGENTS.md") || !strings.Contains(h, "Codex docs: Custom instructions with AGENTS.md") || !strings.Contains(h, "Keep it small.") ||
			strings.Contains(h, "f-setup-skills-codex") {
			t.Errorf("%s: Codex rows missing", lang)
		}
		cc := NewModel(Input{Leak: &leak.Report{TotalUSD: 50, WindowStart: seen.AddDate(0, 0, -30), WindowEnd: seen}, Advice: advice.Builtin(), Lang: lang})
		cc.Setup = []SetupItem{ccSkills(72, 22), ccInstr(236)}
		h, md, term = NewStory(cc).setupOnly(), mdOnly(cc), Terminal(cc, "", "")
		if !strings.Contains(h, "Claude Code docs: Extend Claude with skills") || !strings.Contains(h, "skillListingBudgetFraction") || strings.Contains(md, "arxiv.org") ||
			!strings.Contains(html.UnescapeString(h), "don't reduce its context cost") {
			t.Errorf("%s: the fix's sources and the other way are missing", lang)
		}
		for name, out := range map[string]string{"html": h, "md": md, "term": term} {
			if m := codexTerms.FindString(out); m != "" {
				t.Errorf("%s %s: Claude Code-only report has %q", lang, name, m)
			}
		}
	}
}

// setupOnly is the page without the parts that do not depend on the setup (method notes name both tools).
func (s *Story) setupOnly() string {
	h := s.HTML()
	if i := strings.Index(h, `<div><h3>`+E(s.T("look_method"))); i >= 0 {
		h = h[:i]
	}
	return h
}

func mdOnly(m *Model) string {
	md := Markdown(m)
	if i := strings.Index(md, "### "+NewStory(m).T("look_method")); i >= 0 {
		md = md[:i]
	}
	return md
}

// Within every line: the block shows quiet ✓ rows, no fix, and the first screen is unchanged.
func TestSetupWithinState(t *testing.T) {
	R, recs := threeFixes(t)
	base := NewStory(setupModel(t, "en", R, recs))
	m := setupModel(t, "en", R, recs, ccSkills(41, 0), ccInstr(97), cxAgents(12000, AgentsMDDefault))
	s := NewStory(m)
	h := s.HTML()
	if strings.Count(h, `<li class="ok"><span class="ci" role="img" aria-label="Within">✓</span>`) != 3 || strings.Contains(h, `class="over"`) || strings.Contains(h, "f-setup-") {
		t.Errorf("within rows")
	}
	first := func(h string) string {
		i := strings.Index(h, `<section class="sec verdict"`)
		return h[i : i+strings.Index(h[i:], "</section>")]
	}
	if first(h) != first(base.HTML()) {
		t.Error("the first screen changed")
	}
	md := Markdown(m)
	if !strings.Contains(md, "### Your setup") || strings.Index(md, "### Your setup") > strings.Index(md, "### "+s.T("look_sess")) {
		t.Error("Markdown: the setup table comes before the sessions")
	}
	if Terminal(m, "", "") != Terminal(setupModel(t, "en", R, recs), "", "") {
		t.Error("terminal: within prints nothing")
	}
}

// The first-screen budget (75 English words, 155 Hangul syllables) holds when a setup fix is Fix first.
func TestSetupFirstScreenBudget(t *testing.T) {
	tag := regexp.MustCompile(`<[^>]+>`)
	enw := regexp.MustCompile(`[A-Za-z][A-Za-z'’.-]*`)
	ko := regexp.MustCompile(`[가-힣]`)
	for _, items := range [][]SetupItem{{ccSkills(72, 22)}, {ccInstr(236)}, {cxAgents(32772, AgentsMDDefault)}} {
		for _, lang := range []string{"en", "ko"} {
			s := NewStory(setupModel(t, lang, leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 0.2, Count: 1}), judged(t, 10, 0), items...))
			if len(s.Top) == 0 || s.Top[0].Side != "setup" {
				t.Fatalf("fixture: %v", sides(s.Top))
			}
			h := s.HTML()
			i := strings.Index(h, `<section class="sec verdict"`)
			txt := html.UnescapeString(tag.ReplaceAllString(h[i:i+strings.Index(h[i:], "</section>")], " "))
			if n := len(enw.FindAllString(txt, -1)); lang == "en" && n > 75 {
				t.Errorf("%s %s: %d words", items[0].Tool, items[0].Kind, n)
			}
			if n := len(ko.FindAllString(txt, -1)); lang == "ko" && n > 155 {
				t.Errorf("%s %s: %d syllables", items[0].Tool, items[0].Kind, n)
			}
		}
	}
}

func obsAt(tool, project, cwd string, at time.Time, s logs.Setup) SetupObs {
	for _, p := range []*int64{&s.SkillsAt, &s.InstrAt, &s.AgentsAt} {
		if *p == 0 {
			*p = logs.NaT
		}
	}
	if s.Skills > 0 {
		s.SkillsAt = at.UnixNano()
	}
	if len(s.Instr) > 0 {
		s.InstrAt = at.UnixNano()
	}
	if s.AgentsBytes > 0 {
		s.AgentsAt = at.UnixNano()
	}
	return SetupObs{Tool: tool, Project: project, Cwd: cwd, S: s}
}

// Rows: the newest session per project, the worst project shown; "name-only" skills are not counted; the AGENTS.md
// line is the configured limit; Codex "at the limit" means the logged text reached it.
func TestBuildSetup(t *testing.T) {
	d1, d2 := seen.AddDate(0, 0, -3), seen
	obs := []SetupObs{
		obsAt("claude", "a", "/w/a", d1, logs.Setup{Skills: 70, Bare: []string{"x", "y", "z"}}),                                                                 // privacy-ok: made-up path
		obsAt("claude", "a", "/w/a", d2, logs.Setup{Skills: 70, Bare: []string{"x", "y"}}),                                                                      // privacy-ok: made-up path
		obsAt("claude", "b", "/w/b", d1, logs.Setup{Skills: 40}),                                                                                                // privacy-ok: made-up path
		obsAt("claude", "b", "/w/b", d1, logs.Setup{Instr: []logs.InstrFile{{Path: "/w/b/CLAUDE.md", Lines: 120}, {Path: "/h/.claude/CLAUDE.md", Lines: 260}}}), // privacy-ok: made-up path
		obsAt("codex", "c", "/w/c", d2, logs.Setup{AgentsBytes: 20002, AgentsEndsNL: true}),                                                                     // privacy-ok: made-up path
	}
	got := BuildSetup(obs, SetupOpts{NameOnly: func(cwd string) map[string]bool { return map[string]bool{"y": cwd == "/w/a"} }, Home: "/h", // privacy-ok: made-up path
		Uses: map[string]int{"x": 5}})
	by := map[string]SetupItem{}
	for _, x := range got {
		by[x.Tool+"/"+x.Kind] = x
	}
	if x := by["claude/skills"]; x.Project != "a" || x.K != 1 || x.N != 70 || !x.Over || strings.Join(x.Names, ",") != "x" {
		t.Errorf("skills (newest per project, name-only left out): %+v", x)
	}
	if x := by["claude/instr"]; x.N != 260 || x.File != "~/.claude/CLAUDE.md" || !x.Over {
		t.Errorf("CLAUDE.md: %+v", x)
	}
	if x := by["codex/instr"]; x.Over || x.Limit != AgentsMDDefault {
		t.Errorf("AGENTS.md under the default: %+v", x)
	}
	got = BuildSetup(obs, SetupOpts{AgentsLimit: 20000})
	for _, x := range got {
		if x.Tool == "codex" && x.Kind == "instr" && (!x.Over || x.Limit != 20000) {
			t.Errorf("configured limit: %+v", x)
		}
	}
	if _, ok := by["codex/skills"]; ok {
		t.Error("the Codex skill list is not checked (a skill turned off looks the same as one left out)")
	}
	if len(BuildSetup(nil, SetupOpts{})) != 0 {
		t.Error("nothing read: no rows")
	}
}

// A1: the axes alternate, so a likely fix can come before a measured one: the setup fix goes after the last firm or
// measured fix and never pushes one out of the top three.
func TestSetupAfterLastMeasured(t *testing.T) {
	m := &Finding{Side: "leak", Level: "measured", ID: "m1"}
	l := &Finding{Side: "drift", Level: "likely", ID: "l1"}
	m2 := &Finding{Side: "leak", Level: "measured", ID: "m2"}
	su := &Finding{Side: "setup", Level: "measured", ID: "s"}
	top := withSetup([]*Finding{m, l, m2}, []*Finding{su})
	if len(top) != 3 || top[0] != m || top[1] != l || top[2] != m2 {
		t.Errorf("top: %v", sides(top))
	}
	if top := withSetup([]*Finding{m, l}, []*Finding{su}); top[1] != su || top[2] != l {
		t.Errorf("after the last measured fix, the setup fix may displace no one: %v", sides(top))
	}
	if top := withSetup([]*Finding{l, l}, []*Finding{su}); top[0] != l || top[1] != su {
		t.Errorf("never first while a behaviour fix exists: %v", sides(top))
	}
}

// A15: Other things worth fixing keeps its order with or without a setup fix (the setup card goes before the first
// likely or check card).
func TestSetupOthersOrder(t *testing.T) {
	R, recs := threeFixes(t)
	recs = append(recs, withVerdict(rec(t, "e", "claude", "p", ev{Loc: "U1", Rule: "R2a", Cause: "agent_error", Size: "major", Quote: "wrong"}), "minor_drift"),
		withVerdict(rec(t, "f", "claude", "p", ev{Loc: "U1", Rule: "R2a", Cause: "agent_error", Size: "major", Quote: "wrong again"}), "minor_drift"))
	ids := func(fs []*Finding) []string {
		var o []string
		for _, f := range fs {
			if f.Side != "setup" {
				o = append(o, f.ID)
			}
		}
		return o
	}
	base := NewStory(setupModel(t, "en", R, recs))
	with := NewStory(setupModel(t, "en", R, recs, ccSkills(72, 22), ccInstr(300)))
	var want []string
	for _, f := range base.Top {
		if f.Level == "likely" { // pushed out of the top by the setup fix: listed where it would be
			want = append(want, f.ID)
		}
	}
	b, w := strings.Join(ids(base.Others()), ","), strings.Join(ids(with.Others()), ",")
	for _, id := range want {
		b = strings.Replace(b, ","+id, "", 1)
		w = strings.Replace(w, ","+id, "", 1)
		b = strings.Replace(b, id+",", "", 1)
		w = strings.Replace(w, id+",", "", 1)
	}
	if b != w {
		t.Errorf("order changed:\n%s\n%s", b, w)
	}
}

// A16: the first screen is the same when the setup fix is Fix 3, in both languages; both tools over.
func TestSetupFix3FirstScreen(t *testing.T) {
	R, recs := threeFixes(t)
	first := func(h string) string {
		i := strings.Index(h, `<section class="sec verdict"`)
		return h[i : i+strings.Index(h[i:], "</section>")]
	}
	for _, lang := range []string{"en", "ko"} {
		base := NewStory(setupModel(t, lang, R, recs))
		s := NewStory(setupModel(t, lang, R, recs, ccSkills(72, 22), ccInstr(260), cxAgents(32772, AgentsMDDefault)))
		if s.Top[2].Side != "setup" || first(s.HTML()) != first(base.HTML()) {
			t.Errorf("%s: %v", lang, sides(s.Top))
		}
		if strings.Count(s.HTML(), `<li class="over">`) != 3 {
			t.Errorf("%s: both tools' rows over", lang)
		}
	}
}

// A2, A14: AGENTS.md at the limit only when the text reached it (cut mid-line, or separators past it); far past the
// limit means another limit was in force: no row.
func TestAgentsLimitRules(t *testing.T) {
	row := func(bytes int, endsNL bool, limit int) (SetupItem, bool) {
		obs := []SetupObs{obsAt("codex", "c", "/w/c", seen, logs.Setup{AgentsBytes: bytes, AgentsEndsNL: endsNL})} // privacy-ok: made-up path
		for _, x := range BuildSetup(obs, SetupOpts{AgentsLimit: limit}) {
			if x.Tool == "codex" {
				return x, true
			}
		}
		return SetupItem{}, false
	}
	for _, c := range []struct {
		bytes  int
		endsNL bool
		over   bool
	}{
		{32766 + 2, true, false}, // two files 2 bytes under the limit, joined: not cut
		{32767, true, false},
		{32768, false, true},    // one file cut mid-line
		{32768 + 4, true, true}, // three files, cut
		{30000, false, false},
	} {
		x, ok := row(c.bytes, c.endsNL, 0)
		if !ok || x.Over != c.over {
			t.Errorf("%d bytes, newline %v: over %v (row %v)", c.bytes, c.endsNL, x.Over, ok)
		}
	}
	if _, ok := row(40000, false, 0); ok {
		t.Error("40,000 bytes cannot have been sent under a 32 KiB limit: the limit is not the one assumed, no row")
	}
	if x, ok := row(40002, true, 40000); !ok || !x.Over {
		t.Error("with the limit configured, 40,002 bytes is at the limit")
	}
}

// A8, A10: your user-level CLAUDE.md counts once, not once per project; a project whose session loaded no CLAUDE.md
// of its own adds nothing.
func TestClaudeMDUserFileOnce(t *testing.T) {
	u := logs.InstrFile{Path: "/h/.claude/CLAUDE.md", Type: "User", Lines: 260} // privacy-ok: made-up path
	obs := []SetupObs{
		obsAt("claude", "a", "/w/a", seen, logs.Setup{Instr: []logs.InstrFile{u, {Path: "/w/a/CLAUDE.md", Type: "Project", Lines: 50}}}),  // privacy-ok: made-up path
		obsAt("claude", "b", "/w/b", seen, logs.Setup{Instr: []logs.InstrFile{u}}),                                                        // privacy-ok: made-up path
		obsAt("claude", "c", "/w/c", seen, logs.Setup{Instr: []logs.InstrFile{u, {Path: "/w/c/CLAUDE.md", Type: "Project", Lines: 230}}}), // privacy-ok: made-up path
	}
	var x SetupItem
	for _, it := range BuildSetup(obs, SetupOpts{Home: "/h"}) { // privacy-ok: made-up path
		if it.Kind == "instr" {
			x = it
		}
	}
	if x.N != 260 || x.File != "~/.claude/CLAUDE.md" || x.Project != "" || strings.Join(x.OverIn, ",") != "c" {
		t.Errorf("%+v", x)
	}
	// only a project's AGENTS.md was loaded (no CLAUDE.md): no row from another file
	obs = []SetupObs{obsAt("claude", "a", "/w/a", seen, logs.Setup{Instr: []logs.InstrFile{{Path: "/w/a/CLAUDE.md", Type: "Project", Lines: 0}}})} // privacy-ok: made-up path
	for _, it := range BuildSetup(obs, SetupOpts{}) {
		if it.Kind == "instr" {
			t.Errorf("row from nothing: %+v", it)
		}
	}
}

// Singular forms for a count of 1 (English).
func TestSetupSingular(t *testing.T) {
	s := NewStory(setupModel(t, "en", nil, nil))
	for _, c := range [][2]string{
		{s.T("su.claude.skills.ok", "n", 1), "1 skill, its description kept"},
		{s.T("su.claude.skills.ok", "n", 2), "2 skills, every description kept"},
		{s.T("su.claude.instr.val", "n", 1, "file", "CLAUDE.md"), "Longest: 1 line (CLAUDE.md)"},
		{s.T("pl.SETUP_SKILLS.term", "n", 1, "k", 1), "1 of 1 skill lost its description"},
	} {
		if c[0] != c[1] {
			t.Errorf("%q, want %q", c[0], c[1])
		}
	}
}

// The cost card's reason when only a record of an older cause set exists: one line, in both languages.
func TestCostReasonCauseSet(t *testing.T) {
	for _, lang := range []string{"en", "ko"} {
		m := setupModel(t, lang, leakReport(100, leak.Habit{Habit: "screenshots", Causes: []string{"image"}, USD: 3, Count: 4}), judged(t, 10, 0))
		m.Compare = &Compare{Hours: 720, CostWhy: "cmp_why_cost_causes", CostArgs: map[string]any{}, DriftArgs: map[string]any{}}
		s := NewStory(m)
		want := s.T("cmp_why_cost_causes")
		if !strings.Contains(want, map[string]string{"en": "one more cause", "ko": "원인을 하나 더"}[lang]) || !strings.Contains(html.UnescapeString(s.HTML()), want) {
			t.Errorf("%s: reason missing", lang)
		}
		if s.prob(s.Leak[0]) != map[string]string{"en": "Leaving screenshots in the conversation", "ko": "스크린샷을 대화에 남겨 두기"}[lang] {
			t.Errorf("%s: habit label %q", lang, s.prob(s.Leak[0]))
		}
	}
}

// B3: screenshots as Fix first keep the first screen within its budget (75 English words, 155 Hangul syllables).
func TestScreenshotsFirstScreenBudget(t *testing.T) {
	tag := regexp.MustCompile(`<[^>]+>`)
	enw := regexp.MustCompile(`[A-Za-z][A-Za-z'’.-]*`)
	ko := regexp.MustCompile(`[가-힣]`)
	for _, tool := range []string{"claude", "codex"} {
		for _, lang := range []string{"en", "ko"} {
			R := &leak.Report{TotalUSD: 114.84, LeakUSD: 11.63, WindowStart: seen.AddDate(0, 0, -30), WindowEnd: seen,
				Habits:   []leak.Habit{{Habit: "screenshots", Causes: []string{"image"}, USD: 11.63, Count: 40}},
				Sessions: []leak.SessionLeak{{Key: tool + ":a", Tool: tool, ByCause: map[string]float64{"image": 6}}, {Key: tool + ":b", Tool: tool, ByCause: map[string]float64{"image": 5.63}}}}
			s := NewStory(setupModel(t, lang, R, judged(t, 21, 2)))
			if len(s.Top) == 0 || s.Top[0].Habit != "screenshots" {
				t.Fatalf("fixture: %v", sides(s.Top))
			}
			h := s.HTML()
			i := strings.Index(h, `<section class="sec verdict"`)
			txt := html.UnescapeString(tag.ReplaceAllString(h[i:i+strings.Index(h[i:], "</section>")], " "))
			if n := len(enw.FindAllString(txt, -1)); lang == "en" && n > 75 {
				t.Errorf("%s en: %d words", tool, n)
			}
			if n := len(ko.FindAllString(txt, -1)); lang == "ko" && n > 155 {
				t.Errorf("%s ko: %d syllables", tool, n)
			}
		}
	}
}

// A4, A12, B5, B10: wording that must match the vendor pages, and Codex sources quoted as written.
func TestReviewWording(t *testing.T) {
	en, ko := NewStory(setupModel(t, "en", nil, nil)), NewStory(setupModel(t, "ko", nil, nil))
	for _, c := range [][2]string{
		{en.T("su.codex.instr.line", "lim", 32), "nothing past 32 KiB in total is read"},
		{ko.T("su.codex.instr.line", "lim", 32), "합쳐서 32 KiB를 넘는 부분은 읽지 않음"},
		{ko.T("pl.SETUP_SKILLS.short"), "직접 부르는 스킬은 목록에서 빼기"},
		{ko.T("su.docs", "tool", "Codex"), "Codex 문서"},
		{ko.T("pl.SETUP_INSTR.term", "file", "CLAUDE.md", "n", 236), "CLAUDE.md: 236줄"},
		{en.T("pl.screenshots.hook", "n", 9, "usd", "$2.40"), "9 screenshots left in the conversation cost about $2.40."},
	} {
		if c[0] != c[1] {
			t.Errorf("%q, want %q", c[0], c[1])
		}
	}
	e := advice.Builtin().Get("leak.screenshots")
	var claims []string
	for _, s := range e.Tools["codex"].Sources {
		claims = append(claims, s.Claim)
	}
	all := strings.Join(claims, "\n")
	for _, want := range []string{"all previous input tokens for responses in the chain are billed as input tokens in the API.",
		"/compact | Summarize the visible chat to free tokens. | Use after long runs so Codex retains key points without blowing the context window."} {
		if !strings.Contains(all, want) {
			t.Errorf("Codex sources lack %q", want)
		}
	}
}

// Item 3 (second pass): one screenshot reads in the singular.
func TestScreenshotSingular(t *testing.T) {
	R := &leak.Report{TotalUSD: 100, LeakUSD: 3, WindowStart: seen.AddDate(0, 0, -30), WindowEnd: seen,
		Habits:   []leak.Habit{{Habit: "screenshots", Causes: []string{"image"}, USD: 3, Count: 1}},
		Sessions: []leak.SessionLeak{{Key: "claude:a", Tool: "claude", ByCause: map[string]float64{"image": 3}}}}
	for lang, want := range map[string]string{"en": "1 screenshot left in the conversation cost about $3.00.", "ko": "스크린샷 1장 때문에 약 $3.00가 나갔습니다."} {
		s := NewStory(setupModel(t, lang, R, judged(t, 10, 0)))
		if h := s.Plain(s.Leak[0]).Hook; !strings.Contains(h, want) {
			t.Errorf("%s: %q", lang, h)
		}
	}
	if got := Number("en", "1 screenshots, 1 skills, 1 lines, 2 images"); got != "1 screenshot, 1 skill, 1 line, 2 images" {
		t.Errorf("%q", got)
	}
}

func idleObs(n int, names ...string) []SetupObs {
	var obs []SetupObs
	for i := 0; i < n; i++ {
		var l []logs.SkillLine
		for _, x := range names {
			src, _, ok := strings.Cut(x, ":")
			if !ok {
				src = ""
			}
			l = append(l, logs.SkillLine{Name: x, Chars: 10 * len(x), Source: src})
		}
		obs = append(obs, obsAt("claude", "a", "/w/a", seen.Add(time.Duration(i-n)*time.Hour), logs.Setup{Skills: len(names), Listed: l, ListLen: 100 * len(names)})) // privacy-ok: made-up path
	}
	return obs
}

// Unused skills: only with IdleMinSessions sessions that recorded the list; a skill listed in fewer is not counted;
// used means called or typed; a plugin none of whose skills was used comes first; the share is of the list's
// characters.
func TestBuildSetupIdle(t *testing.T) {
	names := []string{"svg-icons", "gif-maker", "atlas:maps", "atlas:tiles", "ledger:export", "ledger:audit"}
	opts := SetupOpts{Uses: map[string]int{"svg-icons": 2, "ledger:export": 1, "model": 3}, Days: 30}
	if got := BuildSetup(idleObs(IdleMinSessions-1, names...), opts); len(got) != 1 || got[0].Idle != nil {
		t.Fatalf("too few sessions: %+v", got)
	}
	obs := idleObs(IdleMinSessions, names...)
	obs = append(obs, obsAt("claude", "a", "/w/a", seen, logs.Setup{Skills: 7, ListLen: 700, // privacy-ok: made-up path
		Listed: append(obs[0].S.Listed, logs.SkillLine{Name: "late-one", Chars: 80})}))
	got := BuildSetup(obs, opts)
	x := got[0].Idle
	if x == nil || x.Sessions != IdleMinSessions+1 || x.Listed != 7 || x.Unused != 4 || x.New != 1 || x.Days != 30 || x.Chars != 700 {
		t.Fatalf("idle: %+v", x)
	}
	if want := 10 * len("gif-maker"+"atlas:maps"+"atlas:tiles"+"ledger:audit"); x.Idle != want {
		t.Errorf("idle chars %d, want %d", x.Idle, want)
	}
	var gs []string
	for _, g := range x.Groups {
		gs = append(gs, g.Source+"="+strings.Join(g.Names, "+")+map[bool]string{true: " all", false: ""}[g.All()])
	}
	if strings.Join(gs, ", ") != "atlas=atlas:maps+atlas:tiles all, ledger=ledger:audit, =gif-maker" {
		t.Errorf("groups: %v", gs)
	}
	if got[0].listed != nil {
		t.Error("the list is not kept on the row")
	}
}

func idleItem(over bool) SetupItem {
	it := ccSkills(41, 0)
	if over {
		it = ccSkills(72, 22)
	}
	it.Idle = &SkillIdle{Days: 30, Sessions: 31, Listed: it.N, Unused: 5, Chars: 9000, Idle: 4410,
		Groups: []SkillGroup{{Source: "atlas", Listed: 3, Names: []string{"atlas:maps", "atlas:routes", "atlas:tiles"}}, {Names: []string{"gif-maker", "pdf-forms"}}}}
	return it
}

// The unused-skills row: shown under the skill list whether or not the list is over its budget; the first screen and
// the terminal are unchanged within the budget; over it, the fix's details and its terminal line say it too.
func TestSetupIdleRow(t *testing.T) {
	R, recs := threeFixes(t)
	for _, lang := range []string{"en", "ko"} {
		base := NewStory(setupModel(t, lang, R, recs, ccSkills(41, 0)))
		m := setupModel(t, lang, R, recs, idleItem(false))
		s := NewStory(m)
		h := html.UnescapeString(s.HTML())
		val := map[string]string{"en": "5 of 41 not used in the last 30 days", "ko": "최근 30일 동안 41개 중 5개를 쓰지 않음"}[lang]
		all := map[string]string{"en": "atlas: none of this plugin's 3 skills was used in the last 30 days.", "ko": "atlas: 최근 30일 동안 이 플러그인의 스킬 3개를 하나도 쓰지 않았습니다."}[lang]
		pct := map[string]string{"en": "49% of the list; list seen in 31 sessions", "ko": "목록 글자 수의 49%, 목록이 기록된 세션 31개"}[lang]
		for what, out := range map[string]string{"html": h, "md": Markdown(m)} {
			if !strings.Contains(out, val) || !strings.Contains(out, all) || !strings.Contains(out, pct) || !strings.Contains(out, "/plugin disable") {
				t.Errorf("%s %s: row, plugin line, share or advice missing", lang, what)
			}
		}
		if !strings.Contains(h, `<li class="idle" id="setup-idle-claude">`) || strings.Count(h, `class="over"`) != 0 {
			t.Errorf("%s: the row is a note, not over a line", lang)
		}
		first := func(h string) string {
			i := strings.Index(h, `<section class="sec verdict"`)
			return h[i : i+strings.Index(h[i:], "</section>")]
		}
		if first(s.HTML()) != first(base.HTML()) {
			t.Errorf("%s: the first screen changed", lang)
		}
		if Terminal(m, "", "") != Terminal(base.M, "", "") {
			t.Errorf("%s: terminal changed within the budget", lang)
		}
		// over: the fix's details link to the row and name the plugin; the terminal's one-liner adds the count
		o := NewStory(setupModel(t, lang, R, recs, idleItem(true)))
		var fix *Finding
		for _, f := range o.Setup {
			fix = f
		}
		if fix == nil {
			t.Fatal("no setup fix")
		}
		ev := html.UnescapeString(o.setupEvidence(fix))
		if !strings.Contains(ev, all) || strings.Contains(ev, `href="#`) { // a link would scroll behind the dialog
			t.Errorf("%s: fix details: %s", lang, ev)
		}
		term := map[string]string{"en": "5 of 72 not used in 30 days", "ko": "30일 동안 72개 중 5개 안 씀"}[lang]
		if !strings.Contains(o.setupFact(fix), term) {
			t.Errorf("%s: terminal fact %q", lang, o.setupFact(fix))
		}
	}
	// none unused: a quiet ✓ row and no fold
	it := idleItem(false)
	it.Idle.Unused, it.Idle.Idle, it.Idle.Groups = 0, 0, nil
	h := NewStory(setupModel(t, "en", R, recs, it)).HTML()
	if !strings.Contains(h, "none unused in the last 30 days") || strings.Contains(h, `class="idle9"`) || strings.Contains(h, `class="idle"`) ||
		strings.Contains(h, "not counted") {
		t.Error("none unused")
	}
	// none unused of those counted, one too new to count: the row says so
	it.Idle.New = 1
	if h := NewStory(setupModel(t, "en", R, recs, it)).HTML(); !strings.Contains(h, "list seen in 31 sessions; 1 newer skill not counted") {
		t.Error("none unused with a newer skill")
	}
}

// The files of one session count once; the period said is no longer than the oldest list; a use logged without the
// plugin prefix counts for every listed skill it could be ("tiles": atlas:tiles and ledger:tiles).
func TestBuildSetupIdleSessionsAndPrefix(t *testing.T) {
	names := []string{"atlas:maps", "atlas:tiles", "ledger:tiles", "gif-maker"}
	obs := idleObs(IdleMinSessions, names...)
	for i := range obs {
		obs[i].Key = "k" + itoa(i%(IdleMinSessions-1)) // two files share a session: 19 sessions
	}
	if got := BuildSetup(obs, SetupOpts{Days: 30}); got[0].Idle != nil {
		t.Errorf("files of one session counted twice: %+v", got[0].Idle)
	}
	obs = idleObs(IdleMinSessions, names...)
	got := BuildSetup(obs, SetupOpts{Days: 90, Now: seen, Uses: map[string]int{"maps": 1, "tiles": 1}})
	x := got[0].Idle
	if x.Days != 1 {
		t.Errorf("days: %d, want the logs' reach (1)", x.Days)
	}
	if x.Unused != 1 || len(x.Groups) != 1 || strings.Join(x.Groups[0].Names, ",") != "gif-maker" {
		t.Errorf("short names: %+v", x.Groups)
	}
	for _, g := range x.Groups {
		for _, n := range g.Names {
			if n == "atlas:maps" {
				t.Error("atlas:maps was used as /maps")
			}
		}
	}
}

// The unused-skills card: per tool, when half or more of the list went unused; last in Other things worth fixing,
// never among the first three fixes, never in the rating; none when the skill list is already a fix (the fix
// carries the count) or under half.
func TestIdleCard(t *testing.T) {
	R, recs := threeFixes(t)
	half := func(tool string, unused, listed int, over bool) SetupItem {
		it := idleItem(over)
		it.Tool = tool
		if tool == "codex" {
			it = SetupItem{Tool: "codex", Kind: "idle", N: listed, Project: "invoice-sync", SeenAt: seen, Idle: it.Idle}
		}
		it.Idle.Listed, it.Idle.Unused = listed, unused
		return it
	}
	base := NewStory(setupModel(t, "en", R, recs))
	for _, c := range []struct {
		name  string
		items []SetupItem
		cards []string
	}{
		{"both tools, half or more", []SetupItem{half("claude", 21, 41, false), half("codex", 3, 6, false)}, []string{"f-setup-idle-claude", "f-setup-idle-codex"}},
		{"under half", []SetupItem{half("claude", 20, 41, false)}, nil},
		{"the list is a fix already", []SetupItem{half("claude", 60, 72, true)}, nil},
	} {
		for _, lang := range []string{"en", "ko"} {
			s := NewStory(setupModel(t, lang, R, recs, c.items...))
			var got []string
			for _, f := range s.Idle {
				got = append(got, f.ID)
			}
			if strings.Join(got, ",") != strings.Join(c.cards, ",") {
				t.Errorf("%s %s: cards %v", c.name, lang, got)
			}
			others := s.Others()
			for i, id := range c.cards {
				if others[len(others)-len(c.cards)+i].ID != id {
					t.Errorf("%s %s: the card is not last in Other things worth fixing", c.name, lang)
				}
			}
			for _, f := range s.Top {
				if f.Kind == "SETUP_IDLE" {
					t.Errorf("%s %s: a card among the first fixes", c.name, lang)
				}
			}
			if s.St != NewStory(setupModel(t, lang, R, recs)).St && !c.items[0].Over {
				t.Errorf("%s %s: the rating changed", c.name, lang)
			}
			if len(c.cards) > 0 && strings.Join(sides(s.Top), ",") != strings.Join(sides(base.Top), ",") {
				t.Errorf("%s %s: the first fixes changed: %v", c.name, lang, sides(s.Top))
			}
		}
	}
}

// Uses match the list by folder at report time (a sub-agent thread's command, "~/" or $CODEX_HOME, another plugin
// version, a Windows path in another case, a folder named unlike the skill), and a name without the plugin prefix.
func TestIdleMatchesFolders(t *testing.T) {
	list := []logs.SkillLine{
		{Name: "cloud-kit", Dir: "/home/u/.agents/skills/cloud-kit"},                                           // privacy-ok: made-up path
		{Name: "desk-pilot", Dir: "/home/u/.agents/skills/pilot"},                                              // privacy-ok: made-up path
		{Name: "atlas:maps", Source: "atlas", Dir: "/home/u/.codex/plugins/cache/mkt/atlas/1.3/skills/maps"},   // privacy-ok: made-up path
		{Name: "skyline:eve", Source: "skyline", Dir: "/home/u/.codex/plugins/cache/mkt/skyline/2/skills/eve"}, // privacy-ok: made-up path
		{Name: "sys-x", Dir: "/home/u/.codex/skills/.system/sys-x"},                                            // privacy-ok: made-up path
		{Name: "win", Dir: "c:/Users/U/.agents/skills/win"},
		{Name: "idle-one", Dir: "/home/u/.agents/skills/idle-one"}, // privacy-ok: made-up path
	}
	var obs []SetupObs
	for i := 0; i < IdleMinSessions; i++ {
		obs = append(obs, obsAt("codex", "a", "/w/a", seen.Add(time.Duration(-i)*time.Hour), logs.Setup{Skills: len(list), Listed: list, ListLen: 700})) // privacy-ok: made-up path
	}
	use := SkillUse{Named: map[string]int{"eve": 1}, Mentions: map[string]int{}}
	for _, cmd := range []string{"~/.agents/skills/cloud-kit/SKILL.md", "/home/u/.agents/skills/pilot/SKILL.md", // privacy-ok: made-up path
		"/home/u/.codex/plugins/cache/mkt/atlas/1.2/skills/maps/SKILL.md", "$CODEX_HOME/skills/.system/sys-x/SKILL.md", `C:\users\u\.agents\skills\WIN\SKILL.md`} { // privacy-ok: made-up path
		for _, k := range mentionKeys(cmd) {
			use.Mentions[k]++
		}
	}
	got := BuildSetup(obs, SetupOpts{UseBy: map[string]SkillUse{"codex": use}, Days: 30})
	var idle *SkillIdle
	for _, x := range got {
		if x.Tool == "codex" {
			idle = x.Idle
		}
	}
	if idle == nil {
		t.Fatal("no codex row")
	}
	var unused []string
	for _, g := range idle.Groups {
		unused = append(unused, g.Names...)
	}
	if strings.Join(unused, ",") != "idle-one" {
		t.Errorf("unused %v, want only idle-one", unused)
	}
}

// Review round 4: an over-budget skill list with half or more unused says so in the terminal wherever its fix
// lands: on its own line (Fix 1), in its one-liner (Fix 2 or 3), or as an "also" line when it is not among them.
func TestIdleTerminalWhereverTheFix(t *testing.T) {
	it := idleItem(true)
	it.Idle.Listed, it.Idle.Unused = 72, 40
	for name, R := range map[string]*leak.Report{
		"not in the first three": leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 9, Count: 3},
			leak.Habit{Habit: "mid_switch", Causes: []string{"reset_known"}, USD: 8, Count: 3}, leak.Habit{Habit: "big_output", Causes: []string{"oversize"}, USD: 7, Count: 3}),
		"fix 1": leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 0.2, Count: 1}),
	} {
		for _, lang := range []string{"en", "ko"} {
			m := setupModel(t, lang, R, judged(t, 10, 0), it)
			s := NewStory(m)
			term := Terminal(m, "", "")
			want := map[string]string{"en": "not used in", "ko": "안 씀"}[lang]
			if lang == "ko" {
				want = "쓰지 않"
			}
			if !strings.Contains(term, want) && !strings.Contains(term, map[string]string{"en": "40 of 72", "ko": "72개 중 40개"}[lang]) {
				t.Errorf("%s %s: no unused-skills words in the terminal (top %v)\n%s", name, lang, sides(s.Top), term)
			}
		}
	}
}

// mentionKeys are the keys a tool call naming path leaves (as logs reads them).
func mentionKeys(path string) []string {
	var keys []string
	for _, l := range []string{path} {
		parts := strings.Split(strings.ToLower(strings.ReplaceAll(l, `\`, "/")), "/")
		for i := 0; i+1 < len(parts); i++ {
			if parts[i] == "skills" || parts[i] == ".system" {
				keys = append(keys, parts[i]+"/"+parts[i+1])
			}
		}
	}
	return keys
}

// A use without the plugin prefix counts for every listed skill it could be (a:review and b:review both).
func TestIdleShortNameAll(t *testing.T) {
	list := []logs.SkillLine{{Name: "a:review"}, {Name: "b:review"}, {Name: "notes"}}
	var obs []SetupObs
	for i := 0; i < IdleMinSessions; i++ {
		obs = append(obs, obsAt("claude", "a", "/w/a", seen.Add(time.Duration(-i)*time.Hour), logs.Setup{Skills: 3, Listed: list, ListLen: 300})) // privacy-ok: made-up path
	}
	got := BuildSetup(obs, SetupOpts{Uses: map[string]int{"review": 1}, Days: 30})
	var unused []string
	for _, g := range got[0].Idle.Groups {
		unused = append(unused, g.Names...)
	}
	if strings.Join(unused, ",") != "notes" {
		t.Errorf("unused %v, want only notes", unused)
	}
}
