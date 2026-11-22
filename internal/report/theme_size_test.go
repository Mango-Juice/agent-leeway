package report

import (
	"strings"
	"testing"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/advice"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// The owner's case (first real report): a larger finding whose theme is small is ranked above a smaller finding
// whose theme is larger, and every surface says so in the right order: the finding's size first, then the theme
// inside it. A finding with two themes shows the one with more of its sessions (ties: more events, then label
// order) and lists the other under it.
func TestThemedFixShowsFindingSize(t *testing.T) {
	var recs []py.Value
	broke := func(q string) ev {
		return ev{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "U1", IQuote: q, Quote: "again: " + q}
	}
	prompt := func(q string) ev {
		return ev{Loc: "U2", Rule: "R1a", Cause: "missing_or_ambiguous_instruction", Size: "minor", Quote: q, Fix: "Say: " + q}
	}
	// "broke what I already said" in 6 of 10 sessions (a1-a6), "request left something out" in 4 (b1-b4)
	recs = append(recs, rec(t, "a1", "claude", "p", broke("keep the table"), broke("keep the table as it is")))
	recs = append(recs, rec(t, "a2", "claude", "p", broke("do not touch the table")))
	recs = append(recs, rec(t, "a3", "claude", "p", broke("ask before deleting files")))
	recs = append(recs, rec(t, "a4", "claude", "p", broke("ask me before you delete")))
	recs = append(recs, rec(t, "a5", "claude", "p", broke("use tabs")))
	recs = append(recs, rec(t, "a6", "claude", "p", broke("write in Korean")))
	for _, k := range []string{"b1", "b2", "b3", "b4"} {
		recs = append(recs, rec(t, k, "claude", "q", prompt("the tests must pass too, "+k)))
	}
	g, _ := py.Loads(`{"tool": "claude", "group_version": "g2", "error": null, "ts": 1, "themes": [
		{"tool": "claude", "label": "Keep existing tables", "events": ["claude:a1#0", "claude:a1#1", "claude:a2#0"], "draft_rule": "Do not change existing tables."},
		{"tool": "claude", "label": "Ask before deleting", "events": ["claude:a3#0", "claude:a4#0"], "draft_rule": "Ask before deleting files."},
		{"tool": "claude", "label": "Tests must pass", "events": ["claude:b1#0", "claude:b2#0", "claude:b3#0"], "draft_rule": "Run the tests before saying done."}]}`)

	for _, lang := range []string{"en", "ko"} {
		s := NewStory(NewModel(Input{Records: recs, Advice: advice.Builtin(), Lang: lang, Now: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC),
			Since: time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC), Groups: []py.Value{g}}))
		if len(s.Top) < 2 || s.Top[0].Kind != "BROKE_CHAT" || s.Top[1].Kind != "PROMPT" {
			var ks []string
			for _, f := range s.Top {
				ks = append(ks, f.Kind)
			}
			t.Fatalf("%s: order %v (the larger finding must come first)", lang, ks)
		}
		a, b := s.Thing(s.Top[0]), s.Thing(s.Top[1])
		if a == nil || a.Mode != "theme" || a.Theme.Label != "Keep existing tables" || a.GS != 2 || a.GE != 3 ||
			len(a.Others) != 1 || a.Others[0].Theme.Label != "Ask before deleting" {
			t.Fatalf("%s: theme of the larger finding: %+v", lang, a)
		}
		if b == nil || b.Mode != "theme" || b.GS != 3 {
			t.Fatalf("%s: theme of the smaller finding: %+v", lang, b)
		}
		want := map[string][]string{
			"en": {"In 6 of 10 sessions it ignored what you had already said.", "Of those, 2 had the same request:", "Keep existing tables",
				"In 4 of 10 sessions the request left out something you wanted.", "Of those, 3 had the same request:", "Tests must pass",
				"Also grouped within this problem: Ask before deleting (2 sessions)"},
			"ko": {"세션 10개 중 6개에서 이미 한 말을 어겼습니다.", "그중 같은 요청이 세션 2개에서 반복됐습니다:", "Keep existing tables",
				"세션 10개 중 4개에서 요청에 빠진 내용이 있었습니다.", "그중 같은 요청이 세션 3개에서 반복됐습니다:", "Tests must pass",
				"이 문제 안에서 함께 묶인 것: Ask before deleting (세션 2개)"},
		}[lang]
		inOrder := func(surface, text string, parts ...string) {
			t.Helper()
			at := 0
			for _, p := range parts {
				i := strings.Index(text[at:], p)
				if i < 0 {
					t.Errorf("%s %s: %q missing or out of order", lang, surface, p)
					return
				}
				at += i + len(p)
			}
		}
		h := strings.ReplaceAll(s.HTML(), "&#39;", "'")
		i := strings.Index(h, `<div class="panel7">`)
		panel := h[i : i+strings.Index(h[i:], `</div></div>`)]
		inOrder("first screen", panel, want[0], want[1], "<strong>"+want[2]+"</strong>")
		fix1 := h[strings.Index(h, `id="s-fix1"`):strings.Index(h, `id="s-fix2"`)]
		inOrder("fix 1", fix1, `<p class="big">6 / 10</p>`, want[0], `<p class="tin">`+want[1], want[2], want[6])
		fix2 := h[strings.Index(h, `id="s-fix2"`):]
		inOrder("fix 2", fix2, `<p class="big">4 / 10</p>`, want[3], `<p class="tin">`+want[4], want[5])
		term := Terminal(s.M, "", "")
		inOrder("terminal", term, want[0], want[1]+" "+want[2])
		md := Markdown(s.M)
		inOrder("markdown", md, want[0]+" "+want[1]+" **"+want[2]+"**", "**"+want[0]+"**", want[1], want[2], want[6], "**"+want[3]+"**", want[4], want[5])
	}
}

// Ties between themes of one finding: more events first, then label order; the choice does not depend on the
// order the grouping stored them in.
func TestThemeChoiceIsDeterministic(t *testing.T) {
	broke := func(q string) ev {
		return ev{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "U1", IQuote: q, Quote: q}
	}
	recs := []py.Value{rec(t, "a", "claude", "p", broke("x one"), broke("y one")), rec(t, "b", "claude", "p", broke("x two"), broke("y two"))}
	for _, order := range []string{
		`{"tool": "claude", "label": "Zeta", "events": ["claude:a#0", "claude:b#0"]}, {"tool": "claude", "label": "Alpha", "events": ["claude:a#1", "claude:b#1"]}`,
		`{"tool": "claude", "label": "Alpha", "events": ["claude:a#1", "claude:b#1"]}, {"tool": "claude", "label": "Zeta", "events": ["claude:a#0", "claude:b#0"]}`,
	} {
		g, _ := py.Loads(`{"tool": "claude", "group_version": "g2", "error": null, "ts": 1, "themes": [` + order + `]}`)
		s := storyOf(t, nil, []py.Value{g}, recs...)
		th := s.Thing(s.Top[0])
		if th == nil || th.Theme.Label != "Alpha" || len(th.Others) != 1 || th.Others[0].Theme.Label != "Zeta" {
			t.Errorf("tie: %+v", th)
		}
	}
}
