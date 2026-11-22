package report

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/advice"
	"github.com/Mango-Juice/agent-leeway/internal/leak"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// ev is a judged event for tests.
type ev struct {
	Loc, Rule, Cause, Size, Group, ILoc, IQuote, Quote, Fix, Summary string
}

func rec(t *testing.T, key, tool, project string, evs ...ev) py.Value {
	t.Helper()
	tier := map[string]string{"claude": "claude:opus", "codex": "codex:gpt-6.2-astra"}[tool]
	var es []map[string]any
	for _, e := range evs {
		m := map[string]any{"loc": e.Loc, "rule": e.Rule, "cause": e.Cause, "size": e.Size, "cause_group": e.Group, "resolution": "resolved",
			"summary": e.Summary, "evidence": []map[string]any{{"loc": e.Loc, "quote": e.Quote}}}
		if e.IQuote != "" {
			m["instruction"] = map[string]any{"loc": e.ILoc, "quote": e.IQuote}
		}
		if e.Fix != "" {
			m["fix_sentence"] = e.Fix
		}
		es = append(es, m)
	}
	r := map[string]any{"key": tool + ":" + key, "tool": tool, "tier": tier, "project": project, "derived": "agent_drift", "error": nil,
		"rubric_version": "v-test", "out": map[string]any{"events": es, "verdict": "agent_drift"}, "verify": map[string]any{"quotes": len(es), "bad_quotes": 0}}
	b, _ := json.Marshal(r)
	v, err := py.Loads(string(b))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func model(t *testing.T, R *leak.Report, recs ...py.Value) *Model {
	t.Helper()
	return NewModel(Input{Leak: R, Records: recs, Advice: advice.Builtin(), Lang: "en", Now: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC),
		Since: time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC), PricesRead: "2026-11-22", BundledRubric: "v-test"})
}

func find(fs []*Finding, kind string, repeated bool) *Finding {
	for _, f := range fs {
		if f.Kind == kind && (f.Repeated != nil) == repeated {
			return f
		}
	}
	return nil
}

// A cause_group repeated across 2 sessions is one tier-S finding, and its events leave the broad finding.
func TestRepeatedClusterBecomesItsOwnFinding(t *testing.T) {
	m := model(t, nil,
		rec(t, "a", "claude", "p", ev{Loc: "U2", Rule: "R2a", Cause: "agent_error", Size: "minor", Group: "skipped the tests", Quote: "run the tests"},
			ev{Loc: "U4", Rule: "R2a", Cause: "agent_error", Size: "minor", Group: "wrong file", Quote: "not that file"}),
		rec(t, "b", "claude", "p", ev{Loc: "U3", Rule: "R2a", Cause: "agent_error", Size: "minor", Group: "Skipped the tests.", Quote: "tests again"}))
	rep := find(m.Findings, "MISTAKE", true)
	if rep == nil || rep.Tier != "S" || rep.Sessions != 2 || rep.NEvents != 2 || rep.Repeated.From != "label" || rep.Repeated.Text != "skipped the tests" {
		t.Fatalf("repeated finding: %+v", rep)
	}
	if broad := find(append(m.Findings, m.Leftovers...), "MISTAKE", false); broad == nil || broad.NEvents != 1 {
		t.Fatalf("broad finding keeps only the other event: %+v", broad)
	}
}

// Undeterminable causes never become findings.
func TestUndeterminableIsNotAFinding(t *testing.T) {
	m := model(t, nil, rec(t, "a", "claude", "p", ev{Loc: "U1", Rule: "R2a", Cause: "undeterminable", Size: "major", Quote: "q"}))
	if len(m.Findings)+len(m.Leftovers) != 0 {
		t.Fatalf("findings: %v %v", m.Findings, m.Leftovers)
	}
}

func TestConfidenceAndCodexOnly(t *testing.T) {
	two := func(tool string) *Model {
		return model(t, nil,
			rec(t, "a", tool, "p", ev{Loc: "U1", Rule: "R6c", Cause: "agent_error", Size: "minor", Quote: "q1"}, ev{Loc: "U2", Rule: "R6c", Cause: "agent_error", Size: "minor", Quote: "q2"}),
			rec(t, "b", tool, "p", ev{Loc: "U1", Rule: "R6c", Cause: "agent_error", Size: "minor", Quote: "q3"}))
	}
	f := find(two("claude").Findings, "FALSE_DONE", false)
	if f == nil || f.Level != "firm" || len(f.Notes) != 0 {
		t.Fatalf("claude: %+v", f)
	}
	f = find(two("codex").Findings, "FALSE_DONE", false)
	if f == nil || f.Level != "likely" || len(f.Notes) != 1 || f.Notes[0] != "note_codex_only" {
		t.Fatalf("codex only drops one level: %+v", f)
	}
}

func leakReport(total float64, habits ...leak.Habit) *leak.Report {
	R := &leak.Report{TotalUSD: total, Habits: habits, WindowStart: time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC), WindowEnd: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)}
	for _, h := range habits {
		R.LeakUSD += h.USD
	}
	R.Sessions = []leak.SessionLeak{{Key: "claude:s", Tool: "claude", ByCause: map[string]float64{"expiry": 1, "oversize": 1, "reset_known": 1, "repeat": 1, "reread": 1}},
		{Key: "codex:s", Tool: "codex", ByCause: map[string]float64{"repeat": 3}}}
	return R
}

// The ranking rule: confidence first (measured and firm, then likely, then check), then size, with a repeated
// finding's size counted twice; at most 5 cards and 3 per side.
func TestTopRanking(t *testing.T) {
	// one session only: every drift finding is "check this" or "likely"
	R := leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 3, Count: 4})
	m := model(t, R,
		rec(t, "a", "claude", "p", ev{Loc: "U1", Rule: "R2a", Cause: "agent_error", Size: "major", Quote: "q"}, ev{Loc: "U2", Rule: "R2a", Cause: "agent_error", Size: "major", Quote: "q"}))
	if len(m.Top) != 2 || m.Top[0].Side != "leak" || m.Top[1].Level != "likely" {
		t.Fatalf("a measured leak (3%%) comes before a likely drift finding in 100%% of sessions: %v", sides(m.Top))
	}
	// a "check this" finding never takes the first place while a firm or measured one exists
	m = model(t, leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 0.6, Count: 1}),
		rec(t, "a", "codex", "p", ev{Loc: "U1", Rule: "R6a", Cause: "agent_error", Size: "minor", Quote: "q"}))
	if m.Top[0].Side != "leak" || m.Top[len(m.Top)-1].Level != "check" {
		t.Fatalf("check this first: %v", sides(m.Top))
	}
	// within one level, a repeated finding is lifted: 2 of 6 sessions repeated (x2 = 0.67) beats 3 of 6 broad (0.5)
	var rs []py.Value
	for _, k := range []string{"a", "b"} {
		rs = append(rs, rec(t, k, "claude", "p", ev{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "U1", IQuote: "keep the table", Quote: "q"},
			ev{Loc: "U3", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "U1", IQuote: "keep the table", Quote: "q"}))
	}
	for _, k := range []string{"c", "d", "e"} {
		rs = append(rs, rec(t, k, "claude", "p", ev{Loc: "U2", Rule: "R2a", Cause: "agent_error", Size: "major", Quote: "q"}))
	}
	rs = append(rs, rec(t, "f", "claude", "p"))
	m = model(t, nil, rs...)
	if m.Top[0].Repeated == nil || m.Top[0].Kind != "BROKE_SAID" || m.Top[1].Kind != "MISTAKE" {
		t.Fatalf("repeated lift within a level: %v", sides(m.Top))
	}
	if m.Top[0].Level != "firm" || m.Top[1].Level != "firm" {
		t.Fatalf("levels: %v", sides(m.Top))
	}
}

func TestTopCaps(t *testing.T) {
	var hs []leak.Habit
	for _, h := range []string{"long_break", "mid_switch", "let_repeat", "no_map", "big_output"} {
		hs = append(hs, leak.Habit{Habit: h, Causes: []string{"expiry"}, USD: 5, Count: 2})
	}
	var rs []py.Value
	for _, k := range []string{"a", "b"} {
		rs = append(rs, rec(t, k, "claude", "p", ev{Loc: "U1", Rule: "R6a", Cause: "agent_error", Size: "major", Quote: "q"},
			ev{Loc: "U2", Rule: "R3a", Cause: "agent_error", Size: "major", Quote: "q"}, ev{Loc: "U3", Rule: "R6c", Cause: "agent_error", Size: "major", Quote: "q"},
			ev{Loc: "U4", Rule: "R2a", Cause: "agent_error", Size: "major", Quote: "q"}))
	}
	m := model(t, leakReport(100, hs...), rs...)
	n := map[string]int{}
	for _, f := range m.Top {
		n[f.Side]++
	}
	if len(m.Top) != 5 || n["leak"] > 3 || n["drift"] > 3 || m.More == 0 {
		t.Fatalf("caps: %v, more %d", sides(m.Top), m.More)
	}
}

func sides(fs []*Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Side+":"+f.Kind+f.Habit+":"+f.Level)
	}
	return out
}

// Only standing rules are copyable: a single correction never; a requirement said in 2+ sessions (U#) yes; a
// repeated instruction-file rule (I#) no; a draft on a missing-instruction event yes, labelled.
func TestCopyBlocksStandingRulesOnly(t *testing.T) {
	m := model(t, nil, rec(t, "a", "claude", "p", ev{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "U1", IQuote: "never touch the schema", Quote: "I said no"}))
	for _, f := range append(m.Findings, m.Leftovers...) {
		for _, c := range f.Copy {
			for _, l := range c.Lines {
				if strings.Contains(l.Text, "schema") {
					t.Fatalf("a single correction is offered: %+v", l)
				}
			}
		}
	}
	chat := model(t, nil,
		rec(t, "a", "claude", "p", ev{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "U1", IQuote: "keep the table", Quote: "q"}),
		rec(t, "b", "claude", "p", ev{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "U1", IQuote: "Keep the table.", Quote: "q"}))
	f := find(chat.Findings, "BROKE_SAID", true)
	if f == nil || len(f.Copy) != 1 || f.Copy[0].Project != "p" || f.Copy[0].Lines[0].Kind != "repeat" || f.Copy[0].Lines[0].N != 2 || len(f.Copy[0].Lines[0].Refs) != 2 {
		t.Fatalf("repeated U# requirement: %+v", f)
	}
	file := model(t, nil,
		rec(t, "a", "claude", "p", ev{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "I3", IQuote: "keep the table", Quote: "q"}),
		rec(t, "b", "claude", "q", ev{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "I3", IQuote: "keep the table", Quote: "q"}))
	f = find(file.Findings, "BROKE_SAID", true)
	if f == nil || f.NCopy() != 0 || f.AdviceID != "drift.BROKE_SAID.file" || f.Repeated.From != "file" {
		t.Fatalf("repeated I# rule: %+v", f)
	}
	draft := model(t, nil,
		rec(t, "a", "claude", "p", ev{Loc: "U2", Rule: "R2a", Cause: "missing_or_ambiguous_instruction", Size: "minor", Quote: "q", Fix: "Always add a changelog entry."}),
		rec(t, "b", "claude", "r", ev{Loc: "U2", Rule: "R2a", Cause: "missing_or_ambiguous_instruction", Size: "minor", Quote: "q", Fix: "Always add a changelog entry"}))
	f = find(draft.Findings, "PROMPT", false)
	if f == nil || len(f.Copy) != 1 || f.Copy[0].Project != "" || len(f.Copy[0].Lines) != 1 || f.Copy[0].Lines[0].Kind != "draft" || len(f.Copy[0].Lines[0].Refs) != 2 {
		t.Fatalf("drafts de-duplicated into the user-level file: %+v", f)
	}
	// sourced instruction lines and settings
	rs := model(t, nil, rec(t, "a", "codex", "p", ev{Loc: "U1", Rule: "R6a", Cause: "agent_error", Size: "major", Quote: "q"}))
	f = find(rs.Findings, "RESERVED", false)
	if f == nil || len(f.Settings) != 1 || f.Settings[0].S.File != "~/.codex/rules/default.rules" {
		t.Fatalf("setting example: %+v", f)
	}
	fd := model(t, nil, rec(t, "a", "claude", "p", ev{Loc: "U1", Rule: "R6c", Cause: "agent_error", Size: "major", Quote: "q"}))
	f = find(fd.Findings, "FALSE_DONE", false)
	if f == nil || len(f.Copy) != 1 || f.Copy[0].Lines[0].Kind != "sourced" || f.Copy[0].Project != "" || f.Copy[0].Tool != "claude" {
		t.Fatalf("sourced line: %+v", f)
	}
}

// A finding shows the advice of the tools it came from (claude, then codex), and counter-evidence only for them.
func TestPerToolAdvice(t *testing.T) {
	m := model(t, nil,
		rec(t, "a", "codex", "p", ev{Loc: "U1", Rule: "R3a", Cause: "agent_error", Size: "major", Quote: "q"}),
		rec(t, "b", "claude", "p", ev{Loc: "U1", Rule: "R3a", Cause: "agent_error", Size: "major", Quote: "q"}))
	f := find(m.Findings, "ASKED_ONLY", false)
	if f == nil || len(f.Advice) != 2 || f.Advice[0].Tool != "claude" || f.Advice[1].Tool != "codex" {
		t.Fatalf("both tools: %+v", f)
	}
	m = model(t, nil, rec(t, "a", "codex", "p", ev{Loc: "U1", Rule: "R3a", Cause: "agent_error", Size: "major", Quote: "q"}))
	if f = find(m.Findings, "ASKED_ONLY", false); len(f.Advice) != 1 || f.Advice[0].Tool != "codex" {
		t.Fatalf("codex only: %+v", f)
	}
	// leak: tools with at least 10% of the habit's direct dollars; long_break's Claude-only counter-evidence
	R := leakReport(100, leak.Habit{Habit: "let_repeat", Causes: []string{"repeat", "fail"}, USD: 4, Count: 3}, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 4, Count: 3})
	m = model(t, R)
	for _, f := range m.Leak {
		switch f.Habit {
		case "let_repeat":
			if strings.Join(f.Tools, ",") != "claude,codex" {
				t.Errorf("let_repeat tools %v", f.Tools)
			}
		case "long_break":
			if strings.Join(f.Tools, ",") != "claude" || len(f.Against) != 1 {
				t.Errorf("long_break tools %v, against %d", f.Tools, len(f.Against))
			}
		}
	}
	R.Sessions[0].AutoShare = 1 // all of Claude's dollars are automation: not direct conversation
	m = model(t, R)
	for _, f := range m.Leak {
		if f.Habit == "let_repeat" && strings.Join(f.Tools, ",") != "codex" {
			t.Errorf("automation share not removed: %v", f.Tools)
		}
	}
}

func TestMaskingCount(t *testing.T) {
	secret := "ghp_" + strings.Repeat("a1B2c3D4e5", 4)
	m := model(t, nil, rec(t, "a", "claude", "p", ev{Loc: "U1", Rule: "R2a", Cause: "agent_error", Size: "major", Quote: "use " + secret, Summary: "pasted " + secret,
		ILoc: "U0", IQuote: "commit 3f786850e387550fdab836ed7e6dc881de23001b"}))
	if m.Masked != 2 {
		t.Fatalf("masked %d", m.Masked)
	}
	md := Markdown(m)
	if strings.Contains(md, secret) || !strings.Contains(md, "[masked github-token]") || !strings.Contains(md, "3f786850e387550fdab836ed7e6dc881de23001b") {
		t.Fatalf("masking in Markdown")
	}
	if !strings.Contains(md, "2 quote(s) had a key or token masked") {
		t.Error("masked count line missing")
	}
}

// Every warning slot appears when its fact is present, in Markdown and the terminal summary.
func TestWarningSlots(t *testing.T) {
	r := rec(t, "a", "claude", "p", ev{Loc: "U1", Rule: "R2a", Cause: "agent_error", Size: "major", Quote: "q"})
	r.O.Set("rubric_source", py.String("custom"))
	r.O.Set("judge_cli", py.String("claude 2.1.329"))
	r.O.Set("origin", py.String("uncertain"))
	p, _ := py.Loads(`{"omitted_chars": 41200, "total_chars": 201200}`)
	r.O.Set("partial", p)
	m := NewModel(Input{Records: []py.Value{r}, Advice: advice.Builtin(), Lang: "en", Now: time.Date(2027, 9, 1, 0, 0, 0, 0, time.UTC),
		Since: time.Date(2027, 8, 1, 0, 0, 0, 0, time.UTC), Until: time.Date(2027, 9, 1, 0, 0, 0, 0, time.UTC), PricesRead: "2026-11-22", BundledRubric: "v-bundled"})
	md, term := Markdown(m), Terminal(m, "", "")
	for _, want := range []string{"The price table was last checked 2026-11-22", "The advice catalogue was last checked 2026-11-22", "Checked with your edited rubric v-test",
		"Judge programs: claude 2.1.329.", "checked with 41,200 characters from the middle omitted", "could not be told apart", "partly checked", "origin unclear",
		"partly from sessions checked with the middle omitted"} {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown lacks %q", want)
		}
	}
	for _, want := range []string{"! The price table", "! The advice catalogue", "! Checked with your edited rubric"} {
		if !strings.Contains(term, want) {
			t.Errorf("terminal lacks %q", want)
		}
	}
	clean := model(t, nil, rec(t, "a", "claude", "p", ev{Loc: "U1", Rule: "R2a", Cause: "agent_error", Size: "major", Quote: "q"}))
	if md := Markdown(clean); strings.Contains(md, "> **!**") || strings.Contains(md, "masked") || strings.Contains(md, "middle omitted") || strings.Contains(md, "Judge programs") {
		t.Errorf("warnings without their facts:\n%s", md)
	}
}

func TestKorean(t *testing.T) {
	m := NewModel(Input{Records: []py.Value{rec(t, "a", "claude", "p", ev{Loc: "U1", Rule: "R6c", Cause: "agent_error", Size: "major", Quote: "q"})},
		Advice: advice.Builtin(), Lang: "ko", Now: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)})
	md := Markdown(m)
	for _, want := range []string{"먼저 고칠 것", "다 됐다고 하면 확인 결과를 보여 달라고 하세요", "확인했다거나 테스트가 통과했다고 말할 때는", "판정 모델 요약", "찾아보기"} {
		if !strings.Contains(md, want) {
			t.Errorf("Korean report lacks %q", want)
		}
	}
	for _, line := range strings.Split(Terminal(m, "a.md", "a.html"), "\n") {
		if Width(line) > TermWidth && strings.Contains(line, " ") {
			t.Errorf("line wider than %d columns: %q", TermWidth, line)
		}
	}
}

func TestWrapByDisplayWidth(t *testing.T) {
	got := Wrap("가나다 라마바 사아자", 14, "")
	if len(got) != 2 || got[0] != "가나다 라마바" {
		t.Fatalf("%q", got)
	}
}

// One self-contained file: no external script, stylesheet, font, image or @import; lower blocks are open in the
// markup and closed by the script (so they stay readable without scripts and open for print and links).
func TestHTMLSelfContainedAndCollapsible(t *testing.T) {
	R := leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 3, Count: 4})
	m := model(t, R, rec(t, "a", "claude", "p", ev{Loc: "U1", Rule: "R6c", Cause: "agent_error", Size: "major", Quote: `<img src="https://x/y.png">`}))
	h, err := HTML(m)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(h, "<img") || !strings.Contains(h, "&lt;img src=") {
		t.Error("judge-quoted HTML is not escaped")
	}
	for _, re := range []string{`(?i)<[^>]*\bsrc\s*=`, `(?i)url\(`, `(?i)@import`, `(?i)<link\b`, `(?i)<img\b`} {
		if regexp.MustCompile(re).MatchString(h) {
			t.Errorf("external resource pattern %s in the HTML", re)
		}
	}
	for _, href := range regexp.MustCompile(`href="([^"]*)"`).FindAllStringSubmatch(h, -1) {
		u := href[1]
		if !strings.HasPrefix(u, "#") && !strings.HasPrefix(u, "https://") {
			t.Errorf("unexpected href %q", u)
		}
	}
	if !strings.Contains(h, "Content-Security-Policy") || !strings.Contains(h, `<details data-fold`) || !strings.Contains(h, "x.open=false") ||
		!strings.Contains(h, "beforeprint") || !strings.Contains(h, "scroll-snap-type:y mandatory") || !strings.Contains(h, "showModal") {
		t.Error("folding, print, snapping or dialog behaviour missing")
	}
	if !strings.Contains(h, `href="#ev-f-drift-false-done" data-ev="ev-f-drift-false-done"`) || !strings.Contains(h, `<div class="ev6" id="ev-f-drift-false-done"`) {
		t.Error("the fix's button does not lead to its evidence (which is in the page without script)")
	}
	for _, bad := range []string{"?only=", "probe", "shot-only", "class=\"mock"} {
		if strings.Contains(h, bad) {
			t.Errorf("mock-up-only feature %q was ported", bad)
		}
	}
}

// Results judged before TaskRevision t2 carry English summaries, newer ones the user's language; a store with
// both renders each summary as written, under one neutral label.
func TestMixedSummaryLanguages(t *testing.T) {
	for _, lang := range []string{"en", "ko"} {
		m := NewModel(Input{Records: []py.Value{
			rec(t, "old", "claude", "p", ev{Loc: "U1", Rule: "R2a", Cause: "agent_error", Size: "major", Quote: "q", Summary: "The agent removed a table."}),
			rec(t, "new", "codex", "p", ev{Loc: "U1", Rule: "R2a", Cause: "agent_error", Size: "major", Quote: "q", Summary: "에이전트가   표를\n지웠다."}),
		}, Advice: advice.Builtin(), Lang: lang, Now: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)})
		md := Markdown(m)
		label := NewText(lang).T["judge_note"]
		for _, want := range []string{label + ": The agent removed a table.", label + ": 에이전트가 표를 지웠다."} {
			if !strings.Contains(md, want) {
				t.Errorf("%s: missing %q", lang, want)
			}
		}
		if strings.Contains(md, "English") || strings.Contains(md, "영어") {
			t.Errorf("%s: the report still says the summaries are English", lang)
		}
		if h, err := HTML(m); err != nil || !strings.Contains(h, "에이전트가 표를 지웠다.") {
			t.Errorf("%s: HTML summary: %v", lang, err)
		}
	}
}
