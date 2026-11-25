package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/logs"
	"github.com/Mango-Juice/agent-leeway/internal/pipeline"
)

// The Codex skill list of the tests below: own skills, a system one, a plugin's.
var mentionList = []logs.SkillLine{
	{Name: "foo", Dir: "/home/u/.agents/skills/foo"},                                                     // privacy-ok: made-up path
	{Name: "bar", Dir: "/home/u/.agents/skills/bar"},                                                     // privacy-ok: made-up path
	{Name: "transcripts", Dir: "/home/u/.agents/skills/transcripts"},                                     // privacy-ok: made-up path
	{Name: "sys", Dir: "/home/u/.codex/skills/.system/sys"},                                              // privacy-ok: made-up path
	{Name: "atlas:maps", Source: "atlas", Dir: "/home/u/.codex/plugins/cache/mkt/atlas/1.3/skills/maps"}, // privacy-ok: made-up path
}

// codexUnusedAfter runs the report's rows over 20 sessions with mentionList and one rollout holding recs; it
// returns the skills called unused.
func codexUnusedAfter(t *testing.T, recs ...map[string]any) []string {
	t.Helper()
	return codexUnusedWith(t, func(int) []logs.SkillLine { return mentionList }, recs...)
}

// codexUnusedWith is codexUnusedAfter with the list of session i (0 is the newest) given.
func codexUnusedWith(t *testing.T, listOf func(i int) []logs.SkillLine, recs ...map[string]any) []string {
	t.Helper()
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	at := now.Add(-time.Hour)
	p := filepath.Join(t.TempDir(), "rollout-2026-11-20T11-00-00-thread-use.jsonl")
	var b strings.Builder
	for _, r := range append([]map[string]any{{"type": "session_meta", "payload": map[string]any{"id": "thread-use", "cwd": "/home/u/p", "source": "cli"}}}, recs...) { // privacy-ok: made-up path
		r["timestamp"] = at.Format(time.RFC3339)
		x, _ := json.Marshal(r)
		b.Write(x)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	used, err := logs.ParseCodex(p)
	if err != nil {
		t.Fatal(err)
	}
	files := []*logs.File{used}
	for i := 0; i < 22; i++ {
		ts := now.Add(-time.Duration(i+2) * time.Hour).UnixNano()
		id := "x" + string(rune('a'+i))
		files = append(files, &logs.File{Thread: logs.Thread{Tool: "codex", ThreadID: id, SessionID: logs.Some(id), Source: logs.Some("user"),
			Cwd: logs.Some("/home/u/p"), Originator: logs.Some("codex-tui"), Start: ts, End: ts}, // privacy-ok: made-up path
			Setup: &logs.Setup{SkillsAt: ts, InstrAt: logs.NaT, AgentsAt: logs.NaT, Skills: len(listOf(i)), Listed: listOf(i), ListLen: 500},
			Steps: []logs.Step{{TS: ts, Model: logs.Some("gpt-5.4"), Uncached: 10}}, Turns: []logs.Turn{{TS: ts, Kind: "human", Text: "go"}}})
	}
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	tb := pipeline.FromFiles(files)
	tb.Build()
	var out []string
	for _, x := range setupItems(&driftOpts{}, tb, now.AddDate(0, 0, -30), now) {
		if x.Tool == "codex" && x.Idle != nil {
			for _, g := range x.Idle.Groups {
				out = append(out, g.Names...)
			}
		}
	}
	sort.Strings(out)
	return out
}

func cmdDone(c string) map[string]any {
	return map[string]any{"type": "event_msg", "payload": map[string]any{"type": "item_completed", "item": map[string]any{"type": "CommandExecution",
		"id": "c", "command": []any{"/bin/zsh", "-lc", c}, "cwd": "/home/u/p", "exit_code": 0}}} // privacy-ok: made-up path
}

func call(name, args string) map[string]any {
	return map[string]any{"type": "response_item", "payload": map[string]any{"type": "function_call", "name": name, "arguments": args, "call_id": "f"}}
}

// A Codex skill counts as used when any tool call's input names its folder, however the command is written (every
// case a review reproduced is here); the list text itself never counts, and a search of the skills root without a
// skill folder names none.
func TestCodexUseByMention(t *testing.T) {
	all := []string{"atlas:maps", "bar", "foo", "sys", "transcripts"}
	for _, c := range []struct {
		name string
		recs []map[string]any
		used string
	}{
		{"bash -lc", []map[string]any{cmdDone(`bash -lc 'cat ~/.agents/skills/foo/SKILL.md'`)}, "foo"},
		{"subshell cd", []map[string]any{cmdDone("(cd ~/.agents/skills/foo && cat SKILL.md)")}, "foo"},
		{"cd then script", []map[string]any{cmdDone("cd ~/.agents/skills/foo && python3 scripts/run.py")}, "foo"},
		{"python -c", []map[string]any{cmdDone(`python3 -c "print(open('/home/u/.agents/skills/foo/SKILL.md').read())"`)}, "foo"}, // privacy-ok: made-up path
		{"pushd", []map[string]any{cmdDone("pushd ~/.agents/skills/foo; cat SKILL.md; popd")}, "foo"},
		{"env -C", []map[string]any{cmdDone("env -C ~/.agents/skills/foo cat SKILL.md")}, "foo"},
		{"pipe", []map[string]any{cmdDone("cat ~/.agents/skills/foo/SKILL.md | head -5")}, "foo"},
		{"redirect", []map[string]any{cmdDone("bash ~/.agents/skills/foo/scripts/run.sh 2>&1 > out.txt")}, "foo"},
		{"cd root then folder", []map[string]any{cmdDone("cd ~/.agents/skills && cat foo/SKILL.md")}, "foo"},
		{"folder named transcripts", []map[string]any{cmdDone("cat /home/u/.agents/skills/transcripts/SKILL.md")}, "transcripts"}, // privacy-ok: made-up path
		{"$CODEX_HOME", []map[string]any{cmdDone("cat $CODEX_HOME/skills/.system/sys/SKILL.md")}, "sys"},
		{"${CODEX_HOME}", []map[string]any{cmdDone("cat ${CODEX_HOME}/skills/.system/sys/SKILL.md")}, "sys"},
		{"%USERPROFILE%", []map[string]any{cmdDone(`type %USERPROFILE%\.agents\skills\foo\SKILL.md`)}, "foo"},
		{"$env:CODEX_HOME", []map[string]any{cmdDone(`Get-Content $env:CODEX_HOME\skills\.system\sys\SKILL.md`)}, "sys"},
		{"Windows case", []map[string]any{cmdDone(`Get-Content C:\Users\U\.Agents\Skills\FOO\SKILL.md`)}, "foo"},
		{"another plugin version and marketplace", []map[string]any{cmdDone("cat /home/u/.codex/plugins/cache/other/atlas/1.2/skills/maps/SKILL.md")}, "atlas:maps"}, // privacy-ok: made-up path
		{"code-mode exec beside item_completed", []map[string]any{cmdDone("ls"),
			{"type": "response_item", "payload": map[string]any{"type": "custom_tool_call", "name": "exec", "call_id": "e",
				"input": `text(await tools.exec_command({cmd:"cat ~/.agents/skills/foo/SKILL.md"}));`}}}, "foo"},
		{"any tool's arguments", []map[string]any{call("read_file", `{"path":"/home/u/.agents/skills/foo/SKILL.md"}`)}, "foo"}, // privacy-ok: made-up path
		{"an edit names the folder", []map[string]any{cmdDone("sed -i 's/a/b/' ~/.agents/skills/foo/SKILL.md")}, "foo"},
		{"a search of the root names none", []map[string]any{cmdDone("rg -n deploy ~/.agents/skills"), cmdDone("ls skills/")}, ""},
		{"the list text is not a use", []map[string]any{
			{"type": "response_item", "payload": map[string]any{"type": "message", "role": "developer", "content": []any{map[string]any{"type": "input_text",
				"text": "<skills_instructions>\n### Skill roots\n- `r0` = `/home/u/.agents/skills`\n### Available skills\n- foo: Foo (file: r0/foo/SKILL.md)\n- bar: Bar (file: /home/u/.agents/skills/bar/SKILL.md)\n</skills_instructions>"}}}}, // privacy-ok: made-up path
			{"type": "world_state", "payload": map[string]any{"full": true, "state": map[string]any{"host_skills": map[string]any{
				"body": "### Available skills\n- foo: Foo (file: /home/u/.agents/skills/foo/SKILL.md)"}}}}}, ""}, // privacy-ok: made-up path
	} {
		got := codexUnusedAfter(t, c.recs...)
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

// Review round 5: the arguments are normalized once (JSON escapes, URL escapes, backslashes, doubled slashes, home
// notations, case) before matching, and every listed folder of the period counts.
func TestCodexUseByMentionRound5(t *testing.T) {
	all := []string{"atlas:maps", "bar", "foo", "sys", "transcripts"}
	for _, c := range []struct {
		name string
		recs []map[string]any
		used string
	}{
		{"escaped Windows path in arguments", []map[string]any{call("shell_command", `{"command":"Get-Content C:\\Users\\U\\.agents\\skills\\foo\\SKILL.md"}`)}, "foo"},
		{"Windows workdir", []map[string]any{call("shell_command", `{"command":"Get-Content SKILL.md","workdir":"C:\\Users\\U\\.agents\\skills\\foo"}`)}, "foo"},
		{"\\u002f escapes", []map[string]any{call("read_file", `{"path":"\\u002fhome\\u002fu\\u002f.agents\\u002fskills\\u002ffoo\\u002fSKILL.md"}`)}, "foo"},
		{"URL-encoded", []map[string]any{call("open_url", `{"url":"file:///home/u/.agents/skills%2Ffoo%2FSKILL.md"}`)}, "foo"},
		{"doubled slashes", []map[string]any{cmdDone("cat /home/u/.agents/skills//foo//SKILL.md")}, "foo"},
		{"workdir JSON", []map[string]any{call("exec_command", `{"cmd":"cat SKILL.md","workdir":"/home/u/.agents/skills/foo"}`)}, "foo"},
	} {
		got := codexUnusedAfter(t, c.recs...)
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

	// non-ASCII and spaced folder names through local-shell and MCP arguments, a custom root outside skills/
	odd := []logs.SkillLine{
		{Name: "memo", Dir: "/home/u/.agents/skills/메모"},        // privacy-ok: made-up path
		{Name: "notes", Dir: "/home/u/.agents/skills/my notes"}, // privacy-ok: made-up path
		{Name: "kit", Dir: "/opt/team-kit/kit"},                 // privacy-ok: made-up path
		{Name: "idle", Dir: "/home/u/.agents/skills/idle"},      // privacy-ok: made-up path
	}
	for _, c := range []struct {
		name string
		recs []map[string]any
		used string
	}{
		{"local shell, non-ASCII", []map[string]any{{"type": "response_item", "payload": map[string]any{"type": "local_shell_call", "call_id": "l",
			"action": map[string]any{"type": "exec", "command": []any{"cat", "/home/u/.agents/skills/메모/SKILL.md"}}}}}, "memo"}, // privacy-ok: made-up path
		{"MCP, non-ASCII", []map[string]any{{"type": "event_msg", "payload": map[string]any{"type": "item_completed", "item": map[string]any{"type": "McpToolCall",
			"id": "m", "server": "fs", "tool": "read", "arguments": map[string]any{"path": "/home/u/.agents/skills/메모/SKILL.md"}}}}}, "memo"}, // privacy-ok: made-up path
		{"a space in the folder name", []map[string]any{cmdDone(`cat "/home/u/.agents/skills/my notes/SKILL.md"`)}, "notes"}, // privacy-ok: made-up path
		{"a custom root, references/", []map[string]any{cmdDone("cat /opt/team-kit/kit/references/guide.md")}, "kit"},
		{"a custom root, a tool script", []map[string]any{cmdDone("python3 /opt/team-kit/kit/tool.py")}, "kit"},
	} {
		got := codexUnusedWith(t, func(int) []logs.SkillLine { return odd }, c.recs...)
		var want []string
		for _, n := range []string{"idle", "kit", "memo", "notes"} {
			if n != c.used {
				want = append(want, n)
			}
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: unused %v, want %v", c.name, got, want)
		}
	}

	// a plugin skill whose folder was renamed between versions: a use through the older folder counts
	renamed := func(i int) []logs.SkillLine {
		d := "/home/u/.codex/plugins/cache/mkt/atlas/2.0/skills/maps-v2" // privacy-ok: made-up path
		if i > 5 {
			d = "/home/u/.codex/plugins/cache/mkt/atlas/1.0/skills/setup" // privacy-ok: made-up path
		}
		return []logs.SkillLine{{Name: "atlas:maps", Source: "atlas", Dir: d}, {Name: "idle", Dir: "/home/u/.agents/skills/idle"}} // privacy-ok: made-up path
	}
	if got := codexUnusedWith(t, renamed, cmdDone("cat ~/.codex/plugins/cache/mkt/atlas/1.0/skills/setup/SKILL.md")); strings.Join(got, ",") != "idle" {
		t.Errorf("renamed folder: unused %v, want [idle]", got)
	}
}

// Review round 6: a quoted string holding a whole command, punctuation after a folder, "./" parts, a raw patch with
// Windows paths, Unicode forms, escaped spaces and brace lists.
func TestCodexUseByMentionRound6(t *testing.T) {
	all := []string{"atlas:maps", "bar", "foo", "sys", "transcripts"}
	for _, c := range []struct {
		name string
		recs []map[string]any
		used string
	}{
		{"a quoted command with cd", []map[string]any{call("exec_command", `{"cmd":"bash -lc \"cd ~/.codex/skills/foo && ./scripts/run.sh\""}`)}, "foo"},
		{"single quotes inside double", []map[string]any{call("exec_command", `{"cmd":"bash -lc \"cat 'x' && ls ~/.agents/skills/foo\""}`)}, "foo"},
		{"a colon after the folder", []map[string]any{cmdDone("echo see ~/.agents/skills/foo: done")}, "foo"},
		{"a period after the folder", []map[string]any{cmdDone("Read ~/.agents/skills/foo.")}, "foo"},
		{"./ parts", []map[string]any{cmdDone("cat ~/.agents/skills/./foo/SKILL.md")}, "foo"},
		{"a raw patch with Windows paths", []map[string]any{{"type": "response_item", "payload": map[string]any{"type": "custom_tool_call", "name": "apply_patch", "call_id": "p",
			"input": "*** Begin Patch\n*** Update File: C:\\Users\\u\\.agents\\skills\\transcripts\\todo.md\n@@\n-a\n+b\n*** End Patch"}}}, "transcripts"},
		{"a brace list", []map[string]any{cmdDone("cat ~/.agents/skills/{foo,zzz}/SKILL.md")}, "foo"},
		{"an escaped space", []map[string]any{cmdDone(`cat ~/.agents/skills/foo/SKILL.md\ extra`)}, "foo"},
	} {
		got := codexUnusedAfter(t, c.recs...)
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
	// Unicode forms (macOS writes NFD), and a space written as "\ " or %20 inside a folder name
	nfc, nfd := "/home/u/.agents/skills/메모", "/home/u/.agents/skills/\u1106\u1166\u1106\u1169"                                                                     // privacy-ok: made-up paths
	odd := []logs.SkillLine{{Name: "memo", Dir: nfc}, {Name: "notes", Dir: "/home/u/.agents/skills/my notes"}, {Name: "idle", Dir: "/home/u/.agents/skills/idle"}} // privacy-ok: made-up path
	for _, c := range []struct {
		name, cmd, used string
	}{
		{"NFD use of an NFC folder", "cat " + nfd + "/SKILL.md", "memo"},
		{"escaped space", `cat /home/u/.agents/skills/my\ notes/SKILL.md`, "notes"}, // privacy-ok: made-up path
		{"%20", "open file:///home/u/.agents/skills/my%20notes/SKILL.md", "notes"},  // privacy-ok: made-up path
	} {
		got := codexUnusedWith(t, func(int) []logs.SkillLine { return odd }, cmdDone(c.cmd))
		var want []string
		for _, n := range []string{"idle", "memo", "notes"} {
			if n != c.used {
				want = append(want, n)
			}
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s: unused %v, want %v", c.name, got, want)
		}
	}
}

// A custom root under home, named from home ("~/team/kit/tool.py"), matches the listed folder's whole path.
func TestCodexHomeRootByHash(t *testing.T) {
	t.Setenv("HOME", "/home/u")                                                                                // privacy-ok: made-up path
	t.Setenv("USERPROFILE", "/home/u")                                                                         // privacy-ok: made-up path; Windows reads the home folder here
	list := []logs.SkillLine{{Name: "kit", Dir: "/home/u/team/kit"}, {Name: "idle", Dir: "/home/u/team/idle"}} // privacy-ok: made-up paths
	if got := codexUnusedWith(t, func(int) []logs.SkillLine { return list }, cmdDone("python ~/team/kit/tool.py")); strings.Join(got, ",") != "idle" {
		t.Errorf("unused %v, want [idle]", got)
	}
}

// Relative paths within one call: a skill root named in the call (cd, workdir) plus a relative path whose first part
// is a listed skill's folder; a file:/// URL to a custom root; folder names a plain-text key cannot hold. And the
// reverse: the root alone, another folder under the root, a folder whose name only shares a prefix.
func TestCodexUseRelativeInOneCall(t *testing.T) {
	t.Setenv("HOME", "/home/u")        // privacy-ok: made-up path
	t.Setenv("USERPROFILE", "/home/u") // privacy-ok: made-up path
	list := []logs.SkillLine{
		{Name: "foo", Dir: "/home/u/.codex/skills/foo"},           // privacy-ok: made-up path
		{Name: "foo-bar", Dir: "/home/u/.codex/skills/foo-bar"},   // privacy-ok: made-up path
		{Name: "my-skill", Dir: "/home/u/.codex/skills/My Skill"}, // privacy-ok: made-up path
		{Name: "odd", Dir: "/home/u/.codex/skills/odd (v2) #1"},   // privacy-ok: made-up path
		{Name: "kit", Dir: "/opt/team-kit/kit"},                   // privacy-ok: made-up path
		{Name: "idle", Dir: "/home/u/.codex/skills/idle"},         // privacy-ok: made-up path
	}
	all := []string{"foo", "foo-bar", "idle", "kit", "my-skill", "odd"}
	for _, c := range []struct {
		name string
		recs []map[string]any
		used []string
	}{
		{"cd root, relative script", []map[string]any{cmdDone("cd ~/.codex/skills && python foo/run.py")}, []string{"foo"}},
		{"workdir root, relative SKILL.md", []map[string]any{call("exec_command", `{"cmd":"cat foo/SKILL.md","workdir":"/home/u/.codex/skills"}`)}, []string{"foo"}},            // privacy-ok: made-up path
		{"quoted relative with a space", []map[string]any{call("exec_command", `{"cmd":"cat \"My Skill/SKILL.md\"","workdir":"/home/u/.codex/skills"}`)}, []string{"my-skill"}}, // privacy-ok: made-up path
		{"odd characters, relative", []map[string]any{call("exec_command", `{"cmd":"cat 'odd (v2) #1/notes.md'","workdir":"/home/u/.codex/skills"}`)}, []string{"odd"}},         // privacy-ok: made-up path
		{"file URL to a custom root", []map[string]any{call("open", `{"url":"file:///opt/team-kit/kit/references/guide.md"}`)}, []string{"kit"}},
		{"the root alone", []map[string]any{cmdDone("ls ~/.codex/skills")}, nil},
		{"another folder under the root", []map[string]any{cmdDone("cd ~/.codex/skills && cat other/README.md")}, nil},
		{"a longer name sharing a prefix", []map[string]any{cmdDone("cd ~/.codex/skills && python foo-bar/run.py")}, []string{"foo-bar"}},
		{"a relative path with no root in the call", []map[string]any{cmdDone("python foo/run.py")}, nil},
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

// Review of the relative-path change: a Windows file URL, a bare ~ or $CODEX_HOME as the folder a relative path is
// under, a root two levels above the skill, a relative folder with only a trailing slash; and an install command
// run elsewhere (the skills folder is not where it runs, and not its only folder) is no use.
func TestCodexUseRelativeReview(t *testing.T) {
	t.Setenv("HOME", "/home/u")        // privacy-ok: made-up path
	t.Setenv("USERPROFILE", "/home/u") // privacy-ok: made-up path
	list := []logs.SkillLine{
		{Name: "foo", Dir: "/home/u/.codex/skills/foo"},         // privacy-ok: made-up path
		{Name: "odd", Dir: "/home/u/.codex/skills/odd (v2) #1"}, // privacy-ok: made-up path
		{Name: "win", Dir: "c:/Users/u/kit/win"},                // privacy-ok: made-up path
		{Name: "deep", Dir: "/opt/team-kit/odd kit"},            // privacy-ok: made-up path
		{Name: "idle", Dir: "/home/u/.codex/skills/idle"},       // privacy-ok: made-up path
	}
	all := []string{"deep", "foo", "idle", "odd", "win"}
	for _, c := range []struct {
		name, cmd, used string
	}{
		{"Windows file URL", "open file:///C:/Users/u/kit/win/x.md", "win"},
		{"bare $CODEX_HOME root", `cd $CODEX_HOME && cat 'skills/odd (v2) #1/SKILL.md'`, "odd"},
		{"bare ~ root", `cd ~ && cat '.codex/skills/odd (v2) #1/notes.md'`, "odd"},
		{"root two levels up", `cd /opt && cat 'team-kit/odd kit/x.md'`, "deep"},
		{"trailing slash only", "cd ~/.codex/skills && ls foo/", "foo"},
		{"an install command", "cp -r ./foo ~/.codex/skills/", ""},
	} {
		recs := []map[string]any{cmdDone(c.cmd)}
		if c.used == "deep" { // a custom root two levels up counts through the session's own list
			recs = append([]map[string]any{{"type": "response_item", "payload": map[string]any{"type": "message", "role": "developer",
				"content": []any{map[string]any{"type": "input_text", "text": "<skills_instructions>\n### Available skills\n- deep: Deep kit (file: /opt/team-kit/odd kit/SKILL.md)\n</skills_instructions>"}}}}}, recs...) // privacy-ok: made-up path
		}
		got := codexUnusedWith(t, func(int) []logs.SkillLine { return list }, recs...)
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
