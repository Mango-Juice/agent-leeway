package report

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/advice"
	"github.com/Mango-Juice/agent-leeway/internal/leak"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// The short source label and the why line follow the finding's tool: a Codex-only report never names the Claude
// Code docs (or Claude) in them, and a Claude-only report never names Codex or OpenAI docs; leak findings of both
// tools get a neutral label. Checked on every finding and on the rendered HTML, Markdown and terminal text, in
// English and Korean.
func TestSourceLabelFollowsTool(t *testing.T) {
	kinds := func(tool string) []py.Value {
		var rs []py.Value
		evs := []ev{
			{Loc: "U2", Rule: "R6a", Cause: "instruction_violation", Size: "major", Quote: "you pushed it"},
			{Loc: "U2", Rule: "R6c", Cause: "agent_error", Size: "major", Quote: "it was not done"},
			{Loc: "U2", Rule: "R3a", Cause: "agent_error", Size: "major", Quote: "I only asked"},
			{Loc: "U2", Rule: "R1a", Cause: "missing_or_ambiguous_instruction", Size: "minor", Quote: "not what I meant"},
			{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "U1", IQuote: "keep the table", Quote: "again the table"},
			{Loc: "U2", Rule: "R2a", Cause: "instruction_violation", Size: "major", ILoc: "I1", IQuote: "never use tabs", Quote: "tabs again"},
			{Loc: "U2", Rule: "R5a", Cause: "agent_error", Size: "minor", Quote: "that is wrong"},
		}
		for i, e := range evs {
			rs = append(rs, rec(t, tool+itoa(i), tool, "p", e), rec(t, tool+"b"+itoa(i), tool, "p", e))
		}
		return rs
	}
	other := map[string][]string{"codex": {"Claude", "code.claude.com", "CLAUDE.md"}, "claude": {"Codex", "OpenAI", "openai.com", "AGENTS.md"}}
	for _, tool := range []string{"codex", "claude"} {
		for _, lang := range []string{"en", "ko"} {
			s := NewStory(NewModel(Input{Records: kinds(tool), Advice: advice.Builtin(), Lang: lang, Now: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC),
				Since: time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC), Until: time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)}))
			if len(s.Findings) < 6 {
				t.Fatalf("%s: only %d findings", tool, len(s.Findings))
			}
			var lines []string
			for _, f := range append(append([]*Finding{}, s.Findings...), s.Leftovers...) {
				p := s.Plain(f)
				lines = append(lines, p.SrcName, p.SrcURL, p.Why)
			}
			// rendered surfaces: every "Source: <name>" label and every why line of the HTML; the Markdown's source
			// labels and the terminal summary (the detail panels list each tool's sources in full and are not checked)
			h := s.HTML()
			labels := regexp.MustCompile(`>(?:Source|출처): ([^<]*)<`).FindAllStringSubmatch(h, -1)
			whys := regexp.MustCompile(`<p class="why">([^<]*)`).FindAllStringSubmatch(h, -1)
			if len(labels) < 3 || len(whys) < 3 {
				t.Fatalf("%s %s: found %d labels and %d why lines in the HTML", tool, lang, len(labels), len(whys))
			}
			for _, m := range append(labels, whys...) {
				lines = append(lines, m[1])
			}
			for _, m := range regexp.MustCompile(`\[([^\]]*)\]\((https?://[^)]*)\)`).FindAllStringSubmatch(Markdown(s.M), -1) {
				if strings.Contains(m[2], "docs") || strings.Contains(m[2], "trychroma") {
					lines = append(lines, m[1], m[2])
				}
			}
			lines = append(lines, Terminal(s.M, "", ""))
			for _, l := range lines {
				for _, bad := range other[tool] {
					if strings.Contains(l, bad) {
						i := strings.Index(l, bad)
						t.Errorf("%s-only report (%s): %q in %q", tool, lang, bad, l[max(0, i-80):min(len(l), i+60)])
					}
				}
			}
		}
	}
	// leak findings: one tool gets that tool's source; both get the neutral label
	s := storyOf(t, nil, nil)
	for _, h := range []string{"long_break", "mid_switch", "let_repeat", "no_map", "big_output", "automation"} {
		for _, tools := range [][]string{{"codex"}, {"claude"}, {"claude", "codex"}} {
			f := &Finding{Side: "leak", Habit: h, Tools: tools}
			Advise(f, advice.Builtin())
			p := s.Plain(f)
			switch {
			case len(tools) == 2 && p.SrcName != s.T("src_both") && p.SrcName != s.T("src_mixed") && p.SrcName != s.T("src_ours"):
				t.Errorf("%s, both tools: label %q", h, p.SrcName)
			case len(tools) == 1:
				for _, bad := range other[tools[0]] {
					if strings.Contains(p.SrcName+p.SrcURL+p.Why, bad) {
						t.Errorf("%s, %s only: %q in %q / %q / %q", h, tools[0], bad, p.SrcName, p.SrcURL, p.Why)
					}
				}
			}
		}
	}
	_ = leak.Habits
}
