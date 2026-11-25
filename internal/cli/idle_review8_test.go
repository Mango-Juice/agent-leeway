package cli

import (
	"strings"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/logs"
)

// Re-check of PR #15 (f13638b): narrowing a relative path to the folder the call runs in must keep every use the
// earlier rule (5e1eb1b) counted. The folder a call runs in is a cd/pushd/Set-Location target (also after
// "cd --"), an env/make/git -C or env --chdir target, the call's workdir or cwd (a JSON field, or a key in a
// code-mode script), or a string leaf that is one absolute path.
func TestCodexUseRelativeShapes(t *testing.T) {
	t.Setenv("HOME", "/home/u")        // privacy-ok: made-up path
	t.Setenv("USERPROFILE", "/home/u") // privacy-ok: made-up path
	list := []logs.SkillLine{
		{Name: "foo", Dir: "/home/u/.codex/skills/foo"},           // privacy-ok: made-up path
		{Name: "odd", Dir: "/home/u/.codex/skills/odd (v2)"},      // privacy-ok: made-up path
		{Name: "idle", Dir: "/home/u/.codex/skills/idle"},         // privacy-ok: made-up path
		{Name: "win", Dir: "c:/Users/u/.codex/skills/win"},        // privacy-ok: made-up path
		{Name: "make-kit", Dir: "/home/u/.codex/skills/make-kit"}, // privacy-ok: made-up path
	}
	all := []string{"foo", "idle", "make-kit", "odd", "win"}
	code := func(js string) map[string]any {
		return map[string]any{"type": "response_item", "payload": map[string]any{"type": "custom_tool_call", "name": "exec", "call_id": "e1", "input": js}}
	}
	for _, c := range []struct {
		name string
		rec  map[string]any
		used string
	}{
		{"cd root, a second absolute path", cmdDone("cd ~/.codex/skills && python foo/run.py --out /tmp/o.json"), "foo"},
		{"bash -lc", cmdDone(`bash -lc "cd ~/.codex/skills && python foo/run.py --out /tmp/o.json"`), "foo"},
		{"workdir root, an absolute argument", call("exec_command", `{"cmd":"python foo/run.py --out /tmp/o.json","workdir":"/home/u/.codex/skills"}`), "foo"}, // privacy-ok: made-up path
		{"project workdir, cd root", call("exec_command", `{"cmd":"cd ~/.codex/skills && python foo/run.py","workdir":"/home/u/p"}`), "foo"},                   // privacy-ok: made-up path
		{"parentheses", cmdDone("(cd ~/.codex/skills && python foo/run.py --out /tmp/o.json)"), "foo"},
		{"pushd", cmdDone("pushd ~/.codex/skills && python foo/run.py --out /tmp/o.json; popd"), "foo"},
		{"env -C", cmdDone("env -C ~/.codex/skills python foo/run.py --out /tmp/o.json"), "foo"},
		{"make -C", cmdDone("make -C ~/.codex/skills -f make-kit/Makefile OUT=/tmp/o"), "make-kit"},
		{"git -C", cmdDone("git -C ~/.codex/skills log foo/ /tmp/x"), "foo"},
		{"cd into the skill folder", cmdDone("cd ~/.codex/skills/foo && python scripts/run.py --out /tmp/o.json"), "foo"},
		{"cd chain", cmdDone("cd /tmp && cd ~/.codex/skills && python foo/run.py"), "foo"},
		{"quoted $HOME", cmdDone(`cd "$HOME/.codex/skills" && python foo/run.py --out /tmp/o.json`), "foo"},
		{"code-mode cd", code(`text(await tools.exec_command({cmd:"cd ~/.codex/skills && python foo/run.py --out /tmp/o.json"}));`), "foo"},
		{"code-mode workdir, double quotes", code(`text(await tools.exec_command({cmd:"python foo/run.py --out /tmp/o.json", workdir:"~/.codex/skills"}));`), "foo"},
		{"code-mode workdir, single quotes", code(`text(await tools.exec_command({cmd:'python foo/run.py --out /tmp/o.json', workdir:'/home/u/.codex/skills'}));`), "foo"}, // privacy-ok: made-up path
		{"code-mode workdir, odd folder", code(`text(await tools.exec_command({cmd:"cat 'odd (v2)/notes.md' /tmp/x", workdir:"~/.codex/skills"}));`), "odd"},
		{"code-mode cwd key", code(`text(await tools.exec_command({cmd:"python foo/run.py /tmp/x", cwd:'~/.codex/skills'}));`), "foo"},
		{"JSON cwd with a space", call("shell", `{"command":["python","foo/run.py","/tmp/x"],"cwd": "/home/u/.codex/skills"}`), "foo"}, // privacy-ok: made-up path
		{"cd --", cmdDone("cd -- ~/.codex/skills && python foo/run.py --out /tmp/o.json"), "foo"},
		{"env --chdir=", cmdDone("env --chdir=/home/u/.codex/skills python foo/run.py --out /tmp/o.json"), "foo"}, // privacy-ok: made-up path
		{"semicolon", cmdDone("cd ~/.codex/skills; python foo/run.py /tmp/x"), "foo"},
		{"Set-Location", call("shell_command", `{"command":"Set-Location C:\\Users\\u\\.codex\\skills; python win\\run.py C:\\tmp\\x"}`), "win"},
		{"cd $CODEX_HOME/skills", cmdDone("cd $CODEX_HOME/skills && python foo/run.py /tmp/x"), "foo"},
		{"make --directory=", cmdDone("make --directory=/home/u/.codex/skills -f make-kit/Makefile /tmp/x"), "make-kit"}, // privacy-ok: made-up path
	} {
		got := codexUnusedWith(t, func(int) []logs.SkillLine { return list }, c.rec)
		var want []string
		for _, n := range all {
			if n != c.used {
				want = append(want, n)
			}
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: unused %v, want %v", c.name, got, want)
		}
	}
}
