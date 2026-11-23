package cli

import (
	"testing"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/logs"
	"github.com/Mango-Juice/agent-leeway/internal/pipeline"
)

// A6, A7: the setup rows follow the report's sessions: --tool, --project, and direct conversations only unless
// --include-auto.
func TestSetupRowsFollowReportScope(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	at := now.Add(-time.Hour).UnixNano()
	file := func(tool, id, cwd string, th logs.Thread, s logs.Setup) *logs.File {
		th.Tool, th.ThreadID, th.SessionID, th.Source, th.Cwd = tool, id, logs.Some(id), logs.Some("user"), logs.Some(cwd)
		th.Start, th.End = at, at
		for _, p := range []*int64{&s.SkillsAt, &s.InstrAt, &s.AgentsAt} {
			if *p == 0 {
				*p = logs.NaT
			}
		}
		return &logs.File{Thread: th, Setup: &s, Steps: []logs.Step{{TS: at, Model: logs.Some("claude-opus-5-5"), Uncached: 10}},
			Turns: []logs.Turn{{TS: at, Kind: "human", Text: "please do it"}}}
	}
	over := logs.Setup{SkillsAt: at, Skills: 70, Bare: []string{"a", "b"}}
	tb := pipeline.FromFiles([]*logs.File{
		file("claude", "p1", "/nowhere/trail-planner", logs.Thread{Entrypoint: logs.Some("cli"), TypedPrompts: 1}, logs.Setup{SkillsAt: at, Skills: 40}),               // privacy-ok: made-up path
		file("claude", "bot", "/nowhere/nightly", logs.Thread{Entrypoint: logs.Some("sdk-cli")}, over),                                                                 // privacy-ok: made-up path
		file("codex", "x1", "/nowhere/invoice-sync", logs.Thread{Originator: logs.Some("codex-tui")}, logs.Setup{AgentsAt: at, AgentsBytes: 5000, AgentsEndsNL: true}), // privacy-ok: made-up path
	})
	tb.Build()
	rows := func(o driftOpts) map[string]int {
		m := map[string]int{}
		for _, x := range setupItems(&o, tb, now.AddDate(0, 0, -30), now) {
			m[x.Tool+"/"+x.Kind] = x.K
		}
		return m
	}
	if got := rows(driftOpts{}); got["claude/skills"] != 0 || len(got) != 2 {
		t.Errorf("automation left out: %v", got)
	}
	if got := rows(driftOpts{includeAuto: true}); got["claude/skills"] != 2 {
		t.Errorf("--include-auto: %v", got)
	}
	if got := rows(driftOpts{logOpts: logOpts{tool: "codex"}}); len(got) != 1 || got["codex/instr"] != 0 {
		t.Errorf("--tool codex: %v", got)
	}
	if got := rows(driftOpts{project: "invoice"}); len(got) != 1 {
		t.Errorf("--project: %v", got)
	}
}
