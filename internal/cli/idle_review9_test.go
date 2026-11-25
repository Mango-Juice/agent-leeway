package cli

import (
	"strings"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/logs"
)

// Last review of PR #15 (8d5cdda): a relative path joins only a folder the call is said to run in (its workdir or
// cwd, a cd, a -C); the shapes as Codex writes them.
func TestCodexUseRelativeReview9(t *testing.T) {
	t.Setenv("HOME", "/home/u")        // privacy-ok: made-up path
	t.Setenv("USERPROFILE", "/home/u") // privacy-ok: made-up path
	list := []logs.SkillLine{
		{Name: "foo", Dir: "/home/u/.codex/skills/foo"},       // privacy-ok: made-up path
		{Name: "review", Dir: "/home/u/.codex/skills/review"}, // privacy-ok: made-up path
		{Name: "docs", Dir: "/home/u/.codex/skills/docs"},     // privacy-ok: made-up path
	}
	all := []string{"docs", "foo", "review"}
	// CommandExecution as Codex records it: the command as an argv array, the cwd as a file URL
	exec := func(cmd, cwd string) map[string]any {
		return map[string]any{"type": "event_msg", "payload": map[string]any{"type": "item_completed", "item": map[string]any{"type": "CommandExecution",
			"id": "c", "command": []any{"/bin/zsh", "-lc", cmd}, "cwd": cwd, "exit_code": 0}}}
	}
	code := func(js string) map[string]any {
		return map[string]any{"type": "response_item", "payload": map[string]any{"type": "custom_tool_call", "name": "exec", "call_id": "e1", "input": js}}
	}
	for _, c := range []struct {
		name string
		rec  map[string]any
		used []string
	}{
		{"S1: file URL cwd at the root", exec("python foo/run.py", "file:///home/u/.codex/skills"), []string{"foo"}}, // privacy-ok: made-up path
		{"S1: file URL cwd, escaped", exec("python foo/run.py", "file:///home/u/.codex/skills%2F"), []string{"foo"}}, // privacy-ok: made-up path
		{"S1: the shell is no root", exec("ls review/notes.md", "file:///home/u/p"), nil},                            // privacy-ok: made-up path
		{"S2: search the root without running in it", code(`text(await tools.exec_command({cmd:"rg x ~/.codex/skills review/notes.md docs/a.md"}));`), nil},
		{"S2: copy into the root", code(`text(await tools.exec_command({cmd:"cp -r ./review/ ~/.codex/skills/"}));`), nil},
		{"N1: ls -C", exec("ls -C ~/.codex/skills review/y.md", "file:///home/u/p"), nil},                                   // privacy-ok: made-up path
		{"N1: wc -c", exec("wc -c ~/.codex/skills/x review/y.md", "file:///home/u/p"), nil},                                 // privacy-ok: made-up path
		{"N1: bash -c", exec(`bash -c "ls ~/.codex/skills review/y.md"`, "file:///home/u/p"), nil},                          // privacy-ok: made-up path
		{"N1: git -C still runs there", exec("git -C ~/.codex/skills log review/", "file:///home/u/p"), []string{"review"}}, // privacy-ok: made-up path
	} {
		got := codexUnusedWith(t, func(int) []logs.SkillLine { return list }, c.rec)
		var want []string
		for _, n := range all {
			used := false
			for _, u := range c.used {
				used = used || u == n
			}
			if !used {
				want = append(want, n)
			}
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: unused %v, want %v", c.name, got, want)
		}
	}
}
