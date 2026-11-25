package cli

import (
	"strings"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/logs"
)

// Review of PR #15 (5e1eb1b): a relative path joins only a folder the call runs in (its working folder, a
// `cd <root>`), a leading ".." is resolved against that folder, and any percent escape in a file: URL is decoded.
func TestCodexUseRelativeReview7(t *testing.T) {
	t.Setenv("HOME", "/home/u")        // privacy-ok: made-up path
	t.Setenv("USERPROFILE", "/home/u") // privacy-ok: made-up path
	list := []logs.SkillLine{
		{Name: "foo", Dir: "/home/u/.codex/skills/foo"},       // privacy-ok: made-up path
		{Name: "docs", Dir: "/home/u/.codex/skills/docs"},     // privacy-ok: made-up path
		{Name: "review", Dir: "/home/u/.codex/skills/review"}, // privacy-ok: made-up path
		{Name: "idle", Dir: "/home/u/.codex/skills/idle"},     // privacy-ok: made-up path
		{Name: "kit", Dir: "/opt/team-kit/my kit"},            // privacy-ok: made-up path
	}
	all := []string{"docs", "foo", "idle", "kit", "review"}
	for _, c := range []struct {
		name string
		recs []map[string]any
		used []string
	}{
		{"copy out of a skill to a relative folder", []map[string]any{cmdDone("cp ~/.codex/skills/foo/t.md docs/")}, []string{"foo"}},
		{"search the root and a relative file", []map[string]any{cmdDone("rg x ~/.codex/skills review/notes.md")}, nil},
		{"leading .. from a skill folder", []map[string]any{cmdDone("cd ~/.codex/skills/idle && python ../foo/run.py")}, []string{"foo", "idle"}},
		{"file URL with other escapes", []map[string]any{call("open", `{"url":"file:///opt/team-kit/my%20kit/r%C3%A9f.md"}`)}, []string{"kit"}},
		{"file URL with a bad escape", []map[string]any{call("open", `{"url":"file:///opt/team-kit/my%20kit/%zz%4.md"}`)}, []string{"kit"}},
		{"file URL, encoded name", []map[string]any{call("open", `{"url":"file:///home/u/.codex/skills/%66oo/SKILL.md"}`)}, []string{"foo"}}, // privacy-ok: made-up path
		// unchanged: the cases the relative-path change caught
		{"cd root, relative script", []map[string]any{cmdDone("cd ~/.codex/skills && python foo/run.py")}, []string{"foo"}},
		{"workdir root", []map[string]any{call("exec_command", `{"cmd":"cat foo/SKILL.md","workdir":"/home/u/.codex/skills"}`)}, []string{"foo"}}, // privacy-ok: made-up path
		{"an install command run elsewhere", []map[string]any{cmdDone("cp -r ./foo ~/.codex/skills/")}, nil},
		{"a folder named, not run in (no workdir)", []map[string]any{call("exec_command", `{"cmd":"ls ~/.codex/skills foo/"}`)}, nil},
		{"git -C the root", []map[string]any{cmdDone("git -C ~/.codex/skills log foo/")}, []string{"foo"}},
	} {
		got := codexUnusedWith(t, func(int) []logs.SkillLine { return list }, c.recs...)
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
