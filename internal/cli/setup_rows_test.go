package cli

import (
	"strconv"
	"strings"
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

// Skill uses count from every log file of the tool active in the window, whatever the report's session choice: a
// Codex sub-agent thread without a session id, a Claude Code automation without a human turn. Calling a skill
// unused when it was used is the costly mistake; the sessions that count toward the list stay the report's.
func TestSetupUsesCountEveryFile(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	var files []*logs.File
	file := func(tool, id, src string, at int64, s logs.Setup, human bool) {
		th := logs.Thread{Tool: tool, ThreadID: id, SessionID: logs.Some(id), Source: logs.Some(src), Cwd: logs.Some("/nowhere/trail-planner"), // privacy-ok: made-up path
			Entrypoint: logs.Some("cli"), Originator: logs.Some("codex-tui"), TypedPrompts: 1, Start: at, End: at}
		for _, p := range []*int64{&s.SkillsAt, &s.InstrAt, &s.AgentsAt} {
			if *p == 0 {
				*p = logs.NaT
			}
		}
		f := &logs.File{Thread: th, Setup: &s, Steps: []logs.Step{{TS: at, Model: logs.Some("claude-opus-5-5"), Uncached: 10}}}
		if human {
			f.Turns = []logs.Turn{{TS: at, Kind: "human", Text: "please do it"}}
		}
		files = append(files, f)
	}
	list := []logs.SkillLine{{Name: "gif-maker", Chars: 40, Dir: "/x/skills/gif-maker"}, {Name: "notes", Chars: 40, Dir: "/x/skills/notes"}, {Name: "deploy", Chars: 40, Dir: "/x/skills/deploy"}}
	for i := 0; i < 22; i++ {
		at := now.Add(-time.Duration(i+1) * time.Hour).UnixNano()
		file("claude", "c"+strconv.Itoa(i), "user", at, logs.Setup{SkillsAt: at, Skills: 3, Listed: list, ListLen: 120}, true)
		file("codex", "x"+strconv.Itoa(i), "user", at, logs.Setup{SkillsAt: at, Skills: 3, Listed: list, ListLen: 120}, true)
	}
	at := now.Add(-2 * time.Hour).UnixNano()
	file("claude", "bot", "user", at, logs.Setup{SkillUses: map[string]int{"gif-maker": 1}}, false)                           // an automation: no human turn
	file("codex", "sub", "subagent", at, logs.Setup{SkillMentions: map[string]int{"skills/notes": 1}}, false)                 // a sub-agent thread of its own key
	file("codex", "old", "user", now.AddDate(0, 0, -60).UnixNano(), logs.Setup{SkillUses: map[string]int{"deploy": 1}}, true) // before the window
	tb := pipeline.FromFiles(files)
	tb.Build()
	unused := map[string]string{}
	for _, x := range setupItems(&driftOpts{}, tb, now.AddDate(0, 0, -30), now) {
		if x.Idle != nil {
			var ns []string
			for _, g := range x.Idle.Groups {
				ns = append(ns, g.Names...)
			}
			unused[x.Tool] = strings.Join(ns, ",")
		}
	}
	if unused["claude"] != "deploy,notes" {
		t.Errorf("claude: unused %q: the automation's use is lost", unused["claude"])
	}
	if unused["codex"] != "deploy,gif-maker" {
		t.Errorf("codex: unused %q: the sub-agent's use is lost, or a use before the window counted", unused["codex"])
	}
}

// A session that began before the window and ended in it counts its uses.
func TestSetupUsesSessionStraddlingTheWindow(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	since := now.AddDate(0, 0, -30)
	list := []logs.SkillLine{{Name: "gif-maker", Chars: 40}, {Name: "notes", Chars: 40}}
	var files []*logs.File
	add := func(id string, start, end int64, s logs.Setup, human bool) {
		for _, p := range []*int64{&s.SkillsAt, &s.InstrAt, &s.AgentsAt} {
			if *p == 0 {
				*p = logs.NaT
			}
		}
		f := &logs.File{Thread: logs.Thread{Tool: "claude", ThreadID: id, SessionID: logs.Some(id), Source: logs.Some("user"), Cwd: logs.Some("/nowhere/p"), // privacy-ok: made-up path
			Entrypoint: logs.Some("cli"), TypedPrompts: 1, Start: start, End: end}, Setup: &s, Steps: []logs.Step{{TS: end, Model: logs.Some("claude-opus-5-5"), Uncached: 10}}}
		if human {
			f.Turns = []logs.Turn{{TS: end, Kind: "human", Text: "go"}}
		}
		files = append(files, f)
	}
	for i := 0; i < 22; i++ {
		at := now.Add(-time.Duration(i+1) * time.Hour).UnixNano()
		add("c"+strconv.Itoa(i), at, at, logs.Setup{SkillsAt: at, Skills: 2, Listed: list, ListLen: 80}, true)
	}
	add("long", since.AddDate(0, 0, -5).UnixNano(), since.AddDate(0, 0, 1).UnixNano(), logs.Setup{SkillUses: map[string]int{"gif-maker": 1}}, false)
	tb := pipeline.FromFiles(files)
	tb.Build()
	for _, x := range setupItems(&driftOpts{}, tb, since, now) {
		if x.Idle != nil {
			var ns []string
			for _, g := range x.Idle.Groups {
				ns = append(ns, g.Names...)
			}
			if strings.Join(ns, ",") != "notes" {
				t.Errorf("unused %v, want [notes]", ns)
			}
		}
	}
}
