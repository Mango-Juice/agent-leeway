package report

import (
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/advice"
	"github.com/Mango-Juice/agent-leeway/internal/leak"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

func withVerdict(r py.Value, v string) py.Value {
	r.O.Set("derived", py.String(v))
	return r
}

func storyOf(t *testing.T, R *leak.Report, groups []py.Value, recs ...py.Value) *Story {
	t.Helper()
	return NewStory(NewModel(Input{Leak: R, Records: recs, Advice: advice.Builtin(), Lang: "en", Now: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC),
		Since: time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC), Groups: groups}))
}

func judged(t *testing.T, n, serious int) []py.Value {
	var rs []py.Value
	for i := 0; i < n; i++ {
		v := "as_intended"
		if i < serious {
			v = "agent_drift"
		}
		rs = append(rs, withVerdict(rec(t, "k"+itoa(i), "claude", "p"), v))
	}
	return rs
}

// Cut-offs are inclusive (exactly 10% is good, exactly 5% of spend is good); fewer than 5 judged sessions gives no
// drift rating; the overall rating is the worse one; the distance line says how far Good is.
func TestStatusLevels(t *testing.T) {
	s := storyOf(t, leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 5, Count: 1}), nil, judged(t, 10, 1)...)
	if s.St.Drift != "good" || s.St.Cost != "good" || s.St.Overall != "good" || s.St.DDrift != s.T("d_best") {
		t.Fatalf("%+v", s.St)
	}
	s = storyOf(t, leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 16, Count: 1}), nil, judged(t, 10, 2)...)
	if s.St.Drift != "watch" || s.St.Cost != "attention" || s.St.Overall != "attention" ||
		s.St.DDrift != s.T("d_good_drift", "k", 1) || s.St.DCost != s.T("d_good_cost", "usd", "$11") {
		t.Fatalf("%+v", s.St)
	}
	s = storyOf(t, nil, nil, judged(t, 4, 4)...)
	if s.St.Drift != "na" || s.St.Cost != "na" || s.St.Overall != "na" {
		t.Fatalf("too few: %+v", s.St)
	}
	if strings.Contains(Markdown(s.M), "Cost") && strings.Contains(Markdown(s.M), s.T("c_cost")) {
		t.Error("a drift-only report shows a cost row")
	}
}

// "Went against what you said" splits by where the instruction lived; a rule already in a file never gets
// "write it down" or a draft.
func TestSplitAndFileAdvice(t *testing.T) {
	s := storyOf(t, nil, nil,
		rec(t, "a", "claude", "p", ev{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "I3", IQuote: "Never edit generated files", Quote: "you edited the generated file again"}),
		rec(t, "b", "claude", "p", ev{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "I3", IQuote: "never edit generated files!", Quote: "the generated file was edited"}),
		rec(t, "c", "claude", "p", ev{Loc: "U4", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "U1", IQuote: "keep the table as it is", Quote: "I said keep the table"}))
	var file, chat *Finding
	for _, f := range append(s.Findings, s.Leftovers...) {
		switch f.Kind {
		case "BROKE_FILE":
			file = f
		case "BROKE_CHAT":
			chat = f
		}
	}
	if file == nil || chat == nil || file.Sessions != 2 || chat.Sessions != 1 || file.AdviceID != "drift.BROKE_SAID.file" || chat.AdviceID != "drift.BROKE_SAID.chat" {
		t.Fatalf("split: %+v / %+v", file, chat)
	}
	th := s.Thing(file)
	if th == nil || th.Mode != "repeat" || th.GS != 2 {
		t.Fatalf("same rule (case and punctuation differ) in two sessions should repeat: %+v", th)
	}
	if s.Draft(file, th) != nil || strings.Contains(s.Plain(file).Action, "Add what you had to repeat") {
		t.Error("a rule already in a file got 'write it down' advice")
	}
	if p := s.Plain(chat); p.Action != s.T("pl.BROKE_CHAT_EX.action") {
		t.Errorf("an unrepeated chat request should say restate it in a new session: %q", p.Action)
	}
}

// One session repeating a request twice is not repetition; the example follows the score; a quote that is a link
// is never chosen.
func TestThingExampleRules(t *testing.T) {
	s := storyOf(t, nil, nil,
		rec(t, "a", "claude", "p",
			ev{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "minor", ILoc: "U1", IQuote: "use the staging database", Quote: "staging, please"},
			ev{Loc: "U5", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "U1", IQuote: "use the staging database", Quote: "I told you to use staging"},
			ev{Loc: "U7", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "U6", IQuote: "https://example.com/spec", Quote: "read the spec link"}))
	f := s.Findings[0]
	th := s.Thing(f)
	if th == nil || th.Mode != "example" || th.Quote != "use the staging database" || th.E.Loc != "U5" {
		t.Fatalf("example: %+v", th)
	}
}

// A stored grouping (theme) that spans two of a finding's sessions names the thing, with its draft rule; the
// quote shown is the user's stored words; themes of events not in the report, or of one session, are ignored.
func TestThemesFromGrouping(t *testing.T) {
	recs := []py.Value{
		rec(t, "a", "claude", "p", ev{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "U1", IQuote: "숫자는 원문에서만 가져와", Quote: "숫자를 지어내지 마"}),
		rec(t, "b", "claude", "p", ev{Loc: "U3", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "U1", IQuote: "근거 없는 수치는 쓰지 마", Quote: "출처 없는 숫자가 또 있네"}),
	}
	g, _ := py.Loads(`{"tool": "claude", "group_version": "g2", "error": null, "ts": 1,
		"themes": [{"tool": "claude", "label": "수치는 출처에서만", "events": ["claude:a#0", "claude:b#0"], "draft_rule": "수치는 원문에 있는 것만 쓸 것."},
		           {"tool": "claude", "label": "gone", "events": ["claude:a#0", "claude:zzz#0"], "draft_rule": null}]}`)
	s := storyOf(t, nil, []py.Value{g}, recs...)
	if len(s.M.Themes) != 1 {
		t.Fatalf("themes: %d", len(s.M.Themes))
	}
	var chat *Finding
	for _, f := range s.Findings {
		if f.Kind == "BROKE_CHAT" {
			chat = f
		}
	}
	th := s.Thing(chat)
	if th == nil || th.Mode != "theme" || th.GS != 2 {
		t.Fatalf("theme mode: %+v", th)
	}
	lab, ctx := s.ThingText(chat, th)
	if lab != "수치는 출처에서만" || !strings.Contains(ctx, "2") {
		t.Errorf("label %q, context %q", lab, ctx)
	}
	if len(th.Quotes) != 2 || th.Quotes[0] != "숫자는 원문에서만 가져와" || th.Quotes[1] != "근거 없는 수치는 쓰지 마" {
		t.Errorf("quotes not the stored words: %q", th.Quotes)
	}
	if act, why, label, _ := s.FirstAction(); label != "수치는 출처에서만" || !strings.Contains(why, "Of those, 2 had the same request:") || strings.Contains(act, label) {
		t.Errorf("first screen in theme mode: %q / %q / %q", act, why, label)
	}
	if term := Terminal(s.M, "", ""); !strings.Contains(term, "Of those, 2 had the same request: 수치는 출처에서만") || strings.Contains(term, "“수치는 출처에서만”") {
		t.Errorf("terminal theme line:\n%s", term)
	}
	if h := s.HTML(); !strings.Contains(h, `class="bq theme"`) || !strings.Contains(h, s.T("th_by", "tool", "Claude Code", "draft", s.T("th_by_draft"))) {
		t.Error("themed fix card missing")
	}
	if d := s.Draft(chat, th); d == nil || d.Line != "수치는 원문에 있는 것만 쓸 것." {
		t.Errorf("draft: %+v", d)
	}
	if s.Plain(chat).Action != s.T("pl.BROKE_CHAT_TH.action", "file1", "CLAUDE.md") {
		t.Errorf("a theme across sessions should advise writing it down: %q", s.Plain(chat).Action)
	}
	// without groupings: example mode, as today
	if th := storyOf(t, nil, nil, recs...).Thing(chat); th != nil && th.Mode != "example" {
		// chat here belongs to the other story; recompute
	}
	s2 := storyOf(t, nil, nil, recs...)
	for _, f := range s2.Findings {
		if f.Kind == "BROKE_CHAT" {
			if th := s2.Thing(f); th == nil || th.Mode != "example" {
				t.Errorf("without grouping: %+v", th)
			}
		}
	}
}

// The fixes leave out agent mistakes, and the worse-rated axis goes first.
func TestFirstThree(t *testing.T) {
	recs := judged(t, 10, 1)
	recs = append(recs,
		withVerdict(rec(t, "x", "claude", "p", ev{Loc: "U1", Rule: "R2a", Cause: "agent_error", Size: "major", Quote: "wrong again, fix it"}), "minor_drift"),
		withVerdict(rec(t, "y", "claude", "p", ev{Loc: "U1", Rule: "R2a", Cause: "agent_error", Size: "major", Quote: "that is wrong too"}), "minor_drift"),
		withVerdict(rec(t, "z", "claude", "p", ev{Loc: "U1", Rule: "R6c", Cause: "agent_error", Size: "major", Quote: "it was not done"}), "minor_drift"))
	s := storyOf(t, leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 20, Count: 3}), nil, recs...)
	if len(s.Top) != 2 || s.Top[0].Side != "leak" || s.Top[1].Kind != "FALSE_DONE" {
		t.Fatalf("fixes: %v", sides(s.Top))
	}
}

func TestGoingWell(t *testing.T) {
	recs := judged(t, 10, 2)
	s := storyOf(t, leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 3, Count: 1}), nil, recs...)
	keys := ""
	for _, g := range s.Good {
		keys += g.Key + " "
	}
	if keys != "ok spend " {
		t.Fatalf("going well: %q", keys)
	}
	s = storyOf(t, leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 30, Count: 1}), nil, judged(t, 10, 5)...)
	if len(s.Good) != 0 || strings.Contains(s.HTML(), `id="s-good"`) {
		t.Error("nothing holds: the section must be left out")
	}
}

// Evidence: one case per session, readable quotes only.
func TestPickEvents(t *testing.T) {
	s := storyOf(t, nil, nil,
		rec(t, "a", "claude", "p", ev{Loc: "U1", Rule: "R2a", Cause: "agent_error", Size: "major", Quote: "https://example.com/x"},
			ev{Loc: "U2", Rule: "R2a", Cause: "agent_error", Size: "major", Quote: "please fix the failing test"},
			ev{Loc: "U3", Rule: "R2a", Cause: "agent_error", Size: "minor", Quote: "and the other failing test"}),
		rec(t, "b", "claude", "p", ev{Loc: "U1", Rule: "R2a", Cause: "agent_error", Size: "major", Quote: "docs/plan.md"}))
	evs := PickEvents(s.Findings[0], 3)
	if len(evs) != 1 || evs[0].Loc != "U2" {
		t.Fatalf("picked %d", len(evs))
	}
}

// One judge: one strip and no do-not-compare line; two: both. Left-out sessions are named with the flag.
func TestJudgesAndLeftOut(t *testing.T) {
	one := storyOf(t, nil, nil, judged(t, 5, 1)...)
	if h := one.HTML(); strings.Contains(h, one.T("judge_note")) || strings.Count(h, `class="jrow"`) != 1 {
		t.Error("one judge: no do-not-compare line, one strip")
	}
	two := storyOf(t, nil, nil, append(judged(t, 5, 1), withVerdict(rec(t, "c1", "codex", "p"), "as_intended"))...)
	if h := two.HTML(); !strings.Contains(h, E(two.T("judge_note"))) || strings.Count(h, `class="jrow"`) != 2 {
		t.Error("two judges: the line and both strips")
	}
	m := NewModel(Input{Records: judged(t, 5, 1), Advice: advice.Builtin(), Lang: "en", LeftOut: map[string]int{"codex": 7}})
	for name, out := range map[string]string{"html": NewStory(m).HTML(), "md": Markdown(m), "term": Terminal(m, "", "")} {
		if !strings.Contains(out, "7 Codex sessions were not judged") || !strings.Contains(out, "--tool all") {
			t.Errorf("%s lacks the left-out line", name)
		}
	}
	first := NewStory(m).HTML()
	if i := strings.Index(first, `id="s-verdict"`); i >= 0 && strings.Contains(first[i:strings.Index(first[i:], "</section>")+i], "--tool all") {
		t.Error("the left-out line belongs below the first screen")
	}
}

// The terminal summary stays short.
func TestTerminalLength(t *testing.T) {
	recs := judged(t, 10, 3)
	recs = append(recs, rec(t, "z", "claude", "p", ev{Loc: "U1", Rule: "R6c", Cause: "agent_error", Size: "major", Quote: "it was not done at all"}))
	m := NewModel(Input{Leak: leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 9, Count: 3}), Records: recs, Advice: advice.Builtin(), Lang: "ko"})
	if n := len(strings.Split(strings.TrimSpace(Terminal(m, "", "/tmp/r.html")), "\n")); n > 16 {
		t.Errorf("%d lines", n)
	}
}

// The validation rule for stored themes (§12.R7 item 2).
func TestThemeValidation(t *testing.T) {
	m := NewModel(Input{Records: []py.Value{
		rec(t, "a", "claude", "p", ev{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "U1", IQuote: "keep it", Quote: "kept?"}),
		rec(t, "b", "claude", "p", ev{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "I1", IQuote: "keep it always", Quote: "again"}),
		rec(t, "c", "codex", "p", ev{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "U1", IQuote: "keep", Quote: "x"}),
	}, Advice: advice.Builtin(), Lang: "en"})
	byID := map[string]*Event{}
	for _, r := range m.Records {
		for _, e := range r.Events {
			byID[r.Key+"#"+itoa(e.Idx)] = e
		}
	}
	th := func(js string) *Theme {
		v, err := py.Loads(js)
		if err != nil {
			t.Fatal(err)
		}
		return ValidTheme(v, byID)
	}
	if x := th(`{"tool": "claude", "label": "L", "events": ["claude:a#0", "claude:b#0", "claude:a#9"]}`); x == nil || len(x.Events) != 2 || !x.InFile || x.Sessions != 2 {
		t.Errorf("unknown id dropped, mixed chat and file gives in_file: %+v", x)
	}
	if x := th(`{"tool": "claude", "label": "L", "events": ["claude:a#0", "codex:c#0"]}`); x != nil {
		t.Error("a foreign-tool id must not count (one session left)")
	}
	if x := th(`{"tool": "claude", "label": "L", "events": ["claude:a#0", "claude:a#0"]}`); x != nil {
		t.Error("a one-session theme must be dropped")
	}
	// in_file: the already-in-the-file advice, never a draft
	g, _ := py.Loads(`{"tool": "claude", "group_version": "g2", "error": null, "ts": 1, "themes": [{"tool": "claude", "label": "Keep it", "events": ["claude:a#0", "claude:b#0"], "draft_rule": "Keep it."}]}`)
	s := NewStory(NewModel(Input{Records: []py.Value{
		rec(t, "a", "claude", "p", ev{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "U1", IQuote: "keep it", Quote: "kept?"}),
		rec(t, "b", "claude", "p", ev{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "U1", IQuote: "keep it as is", Quote: "again"}),
		rec(t, "d", "claude", "p", ev{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "I1", IQuote: "Keep it", Quote: "and again"}),
	}, Advice: advice.Builtin(), Lang: "en", Groups: []py.Value{g}}))
	for _, f := range s.Findings {
		if f.Kind == "BROKE_CHAT" {
			if th := s.Thing(f); th == nil || th.Mode != "theme" || s.Draft(f, th) == nil || s.Plain(f).Action != s.T("pl.BROKE_CHAT_TH.action", "file1", "CLAUDE.md") {
				t.Errorf("chat theme: %+v %q", th, s.Plain(f).Action)
			}
		}
	}
}

// Text budget of the first screen (docs/24 §12.R4 item 9, §18.1): at most 70 English words and 155 Hangul syllables
// of visible text, measured on the fixture reports (plain and themed); and the terminal stays at 16 lines or less.
func TestFirstScreenBudget(t *testing.T) {
	tag := regexp.MustCompile(`<[^>]+>`)
	enw := regexp.MustCompile(`[A-Za-z][A-Za-z'’.-]*`)
	ko := regexp.MustCompile(`[가-힣]`)
	for _, name := range []string{"report.en.html", "report.ko.html", "report-themed.en.html", "report-themed.ko.html"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "expected-go", name))
		if err != nil {
			t.Fatal(err)
		}
		h := string(b)
		i := strings.Index(h, `<section class="sec verdict"`)
		j := strings.Index(h[i:], "</section>")
		txt := html.UnescapeString(tag.ReplaceAllString(h[i:i+j], " "))
		if strings.HasPrefix(name, "report.en") || strings.HasPrefix(name, "report-themed.en") {
			if n := len(enw.FindAllString(txt, -1)); n > 70 {
				t.Errorf("%s: %d English words on the first screen", name, n)
			}
		} else if n := len(ko.FindAllString(txt, -1)); n > 155 {
			t.Errorf("%s: %d Hangul syllables on the first screen", name, n)
		}
		if strings.Contains(h[i:i+j], "“") && !strings.Contains(name, "themed") {
			t.Errorf("%s: the first screen quotes the user", name)
		}
	}
}

// No drift results in the period: every surface says the drift check has not been run, how to run it, and that
// the rating rests on cost only; nothing reads "0 of 0".
func TestDriftNotRunState(t *testing.T) {
	for _, lang := range []string{"en", "ko"} {
		m := NewModel(Input{Leak: leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 20, Count: 3}), Advice: advice.Builtin(), Lang: lang,
			Since: time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)})
		s := NewStory(m)
		cmd := s.DriftCmd()
		for name, out := range map[string]string{"html": s.HTML(), "md": Markdown(m), "term": Terminal(m, "", "/tmp/r.html")} {
			for _, want := range []string{s.T("drift_none_fact"), cmd, s.T("rating_cost_only")} {
				if name == "html" {
					want = E(want)
				}
				if !strings.Contains(out, want) {
					t.Errorf("%s %s lacks %q", lang, name, want)
				}
			}
			for _, bad := range []string{"0 of 0", "0개 중 0개", s.T("rating.na")} {
				if strings.Contains(out, bad) {
					t.Errorf("%s %s still says %q", lang, name, bad)
				}
			}
		}
		if len(s.Good) != 0 {
			t.Errorf("%s: going well claims something without judged sessions", lang)
		}
	}
}
