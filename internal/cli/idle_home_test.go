package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/logs"
	"github.com/Mango-Juice/agent-leeway/internal/report"
)

// A Codex command that names a skill's SKILL.md from a home variable counts as a use of the listed skill, end to end:
// the rollout is parsed, its folders kept as written, and the report matches them to the list's folder.
func TestCodexHomeNotations(t *testing.T) {
	listed := map[string]string{"unix": "/home/u/.codex/skills/foo", "win": "c:/Users/u/.codex/skills/foo"} // privacy-ok: made-up paths
	cases := []struct{ list, cmd string }{
		{"unix", "cat $CODEX_HOME/skills/foo/SKILL.md"},
		{"unix", "cat ${CODEX_HOME}/skills/foo/SKILL.md"},
		{"unix", "cat \"$CODEX_HOME/skills/foo/SKILL.md\""},
		{"unix", "cat $HOME/.codex/skills/foo/SKILL.md"},
		{"unix", "cat ${HOME}/.codex/skills/foo/SKILL.md"},
		{"unix", "cat ~/.codex/skills/foo/SKILL.md"},
		{"win", `Get-Content %USERPROFILE%\.codex\skills\foo\SKILL.md`},
		{"win", `Get-Content $env:USERPROFILE\.codex\skills\foo\SKILL.md`},
		{"win", `Get-Content $env:CODEX_HOME\skills\foo\SKILL.md`},
	}
	for _, c := range cases {
		p := filepath.Join(t.TempDir(), "rollout-2026-11-20T10-00-00-thread-one.jsonl")
		rec := map[string]any{"type": "event_msg", "timestamp": "2026-11-20T10:00:00Z", "payload": map[string]any{"type": "item_completed",
			"item": map[string]any{"type": "CommandExecution", "id": "c", "command": c.cmd, "cwd": "/home/u/p", "exit_code": 0}}} // privacy-ok: made-up path
		b, _ := json.Marshal(rec)
		if err := os.WriteFile(p, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		f, err := logs.ParseCodex(p)
		if err != nil {
			t.Fatal(err)
		}
		var dirs map[string]int
		if f.Setup != nil {
			dirs = f.Setup.SkillMentions
		}
		list := []logs.SkillLine{{Name: "foo", Dir: listed[c.list]}, {Name: "bar", Dir: strings.Replace(listed[c.list], "foo", "bar", 1)}}
		var obs []report.SetupObs
		at := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
		for i := 0; i < report.IdleMinSessions; i++ {
			s := logs.Setup{SkillsAt: at.Add(time.Duration(-i) * time.Hour).UnixNano(), InstrAt: logs.NaT, AgentsAt: logs.NaT, Skills: 2, Listed: list, ListLen: 100}
			obs = append(obs, report.SetupObs{Tool: "codex", Project: "p", Key: "k" + string(rune('a'+i)), S: s})
		}
		var unused []string
		for _, x := range report.BuildSetup(obs, report.SetupOpts{UseBy: map[string]report.SkillUse{"codex": {Mentions: dirs}}, Days: 30}) {
			if x.Tool == "codex" && x.Idle != nil {
				for _, g := range x.Idle.Groups {
					unused = append(unused, g.Names...)
				}
			}
		}
		if strings.Join(unused, ",") != "bar" {
			t.Errorf("%q: unused %v (folders kept %v), want only bar", c.cmd, unused, dirs)
		}
	}
}
