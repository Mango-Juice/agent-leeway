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
