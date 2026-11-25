package logs

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

// The setup records are undocumented. These fixtures copy the shapes seen in real logs (Claude Code 2.1.3xx,
// Codex 0.171): numbers, names and paths are made up.

func jl(t *testing.T, recs ...any) []byte {
	t.Helper()
	var b strings.Builder
	for _, r := range recs {
		x, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(x)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func ccLine(typ string, extra map[string]any) map[string]any {
	m := map[string]any{"type": typ, "timestamp": "2026-11-20T10:00:00Z", "sessionId": "s1", "cwd": "/home/u/trail-planner"} // privacy-ok: made-up path
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func listing(initial bool, names []string, content string) map[string]any {
	return ccLine("attachment", map[string]any{"attachment": map[string]any{"type": "skill_listing", "isInitial": initial, "skillCount": len(names),
		"names": names, "content": content}})
}

func TestClaudeSkillListing(t *testing.T) {
	dir := t.TempDir()
	names := []string{"svg-icons", "pdf-forms", "plug:deploy", "plug:notes", "gif-maker"}
	content := "- svg-icons: Make icons\n- pdf-forms\n- plug:deploy: Ship it: carefully\n- plug:notes\n- gif-maker"
	p := filepath.Join(dir, "a.jsonl")
	writeFile(t, p, jl(t,
		listing(true, names, content),
		listing(false, []string{"late-one"}, "- late-one"), // a later listing names only what was added
		ccLine("assistant", map[string]any{"message": map[string]any{"id": "m1", "model": "claude-opus-5-5", "usage": map[string]any{"input_tokens": 1},
			"content": []any{map[string]any{"type": "tool_use", "id": "t1", "name": "Skill", "input": map[string]any{"skill": "gif-maker"}}}}}),
		ccLine("attachment", map[string]any{"attachment": map[string]any{"type": "instructions", "files": []any{
			map[string]any{"path": "/home/u/.claude/CLAUDE.md", "type": "User", "content": strings.Repeat("x\n", 40)},                           // privacy-ok: made-up path
			map[string]any{"path": "/home/u/trail-planner/CLAUDE.md", "type": "Project", "content": strings.Repeat("y\n", 236)},                 // privacy-ok: made-up path
			map[string]any{"path": "/home/u/.claude/projects/x/memory/MEMORY.md", "type": "AutoMem", "content": strings.Repeat("z\n", 900)}}}}), // privacy-ok: made-up path
		ccLine("attachment", map[string]any{"attachment": map[string]any{"type": "instructions", "changed": true, "reason": "session_start", "files": []any{
			map[string]any{"path": "/home/u/trail-planner/CLAUDE.md", "type": "Project", "content": strings.Repeat("y\n", 180) + "last"}}}}), // privacy-ok: made-up path
	))
	f, err := ParseClaude(p)
	if err != nil {
		t.Fatal(err)
	}
	s := f.Setup
	if s == nil || s.Skills != 5 || strings.Join(s.Bare, ",") != "pdf-forms,plug:notes,gif-maker" {
		t.Fatalf("listing: %+v", s)
	}
	if s.SkillUses["gif-maker"] != 1 {
		t.Errorf("skill use: %v", s.SkillUses)
	}
	if len(s.Instr) != 2 || s.Instr[1].Lines != 181 || s.Instr[0].Lines != 40 {
		t.Errorf("instruction files (CLAUDE.md only, a later attachment replaces a file): %+v", s.Instr)
	}

	// a listing shaped differently (a name with no line of its own) is not used: unknown, not "within"
	writeFile(t, p, jl(t, listing(true, []string{"a", "b"}, "Skills:\n a, b")))
	if f, _ := ParseClaude(p); f.Setup != nil {
		t.Errorf("unknown shape must leave the listing unknown: %+v", f.Setup)
	}
	// a sub-agent's listing is its parent's: only its skill use counts
	sub := filepath.Join(dir, "s1", "subagents", "agent-1.jsonl")
	writeFile(t, sub, jl(t, listing(true, names, content)))
	if f, _ := ParseClaude(sub); f.Setup != nil {
		t.Errorf("sub-agent: %+v", f.Setup)
	}
}

func cxLine(typ string, payload map[string]any) map[string]any {
	return map[string]any{"type": typ, "timestamp": "2026-11-20T10:00:00Z", "payload": payload}
}

func entries(shown ...bool) map[string]any {
	m := map[string]any{}
	for i, s := range shown {
		m[`["host","host","/home/u/.agents/skills/s`+string(rune('a'+i))+`/SKILL.md"]`] = map[string]any{"fingerprint": "f", "shown": s} // privacy-ok: made-up path
	}
	return m
}

func codexFile(t *testing.T, p string, recs ...any) {
	t.Helper()
	head := []any{cxLine("session_meta", map[string]any{"id": "thread-one", "cwd": "/home/u/invoice-sync", "source": "cli"})} // privacy-ok: made-up path
	b := jl(t, append(head, recs...)...)
	if strings.HasSuffix(p, ".zst") {
		enc, _ := zstd.NewWriter(nil)
		b = enc.EncodeAll(b, nil)
	}
	writeFile(t, p, b)
}

func TestCodexWorldState(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "rollout-2026-11-20T10-00-00-thread-one.jsonl.zst")
	text := strings.Repeat("r", 32772) // what Codex logged with three 15 KiB AGENTS.md files: cut at 32 KiB, joined by blank lines
	codexFile(t, p,
		cxLine("world_state", map[string]any{"full": true, "state": map[string]any{"host_skills": map[string]any{"version": 1, "entries": entries(true, false, true)},
			"agents_md": map[string]any{"directory": "/home/u/invoice-sync", "text": text}}}), // privacy-ok: made-up path
		// a skill added later in the session is not shown yet: that is not the budget
		cxLine("world_state", map[string]any{"full": false, "state": map[string]any{"host_skills": map[string]any{"entries": map[string]any{
			`["host","host","/home/u/.agents/skills/new/SKILL.md"]`: map[string]any{"fingerprint": "g", "shown": false}}}}}), // privacy-ok: made-up path
	)
	f, err := ParseCodex(p)
	if err != nil {
		t.Fatal(err)
	}
	if s := f.Setup; s == nil || s.AgentsBytes != 32772 || s.AgentsEndsNL {
		t.Fatalf("compressed rollout: %+v", f.Setup)
	}
	// the skill list is not read: a skill left out for the budget cannot be told from one turned off
	q := filepath.Join(dir, "rollout-2026-11-20T10-00-00-thread-two.jsonl")
	codexFile(t, q, cxLine("world_state", map[string]any{"full": true, "state": map[string]any{"host_skills": map[string]any{"body": "x", "includeInstructions": true}}}))
	if f, err := ParseCodex(q); err != nil || f.Setup != nil {
		t.Errorf("unknown shape: %+v %v", f.Setup, err)
	}
	// no world_state at all: nothing, and no error
	codexFile(t, q, cxLine("turn_context", map[string]any{"model": "gpt-6.2-astra"}))
	if f, err := ParseCodex(q); err != nil || f.Setup != nil {
		t.Errorf("no record: %+v %v", f.Setup, err)
	}
}

func TestRelPath(t *testing.T) {
	for _, c := range [][4]string{
		{"/home/u/trail-planner/CLAUDE.md", "/home/u/trail-planner", "/home/u", "CLAUDE.md"},         // privacy-ok: made-up path
		{"/home/u/trail-planner/web/CLAUDE.md", "/home/u/trail-planner", "/home/u", "web/CLAUDE.md"}, // privacy-ok: made-up path
		{"/home/u/.claude/CLAUDE.md", "/home/u/trail-planner", "/home/u", "~/.claude/CLAUDE.md"},     // privacy-ok: made-up path
		{"/etc/claude/CLAUDE.md", "/home/u/x", "/home/u", "claude/CLAUDE.md"},                        // privacy-ok: made-up path
	} {
		if got := RelPath(c[0], c[1], c[2]); got != c[3] {
			t.Errorf("%s: %s", c[0], got)
		}
	}
}

// Each listed skill keeps the characters its entry takes (a description may run over several lines); a skill
// counts as used when the agent calls it or the user types it (/name, or "<command-message>" first), with or
// without its plugin prefix as logged; built-in commands are counted under their own names and match no skill.
func TestClaudeSkillCharsAndTypedUse(t *testing.T) {
	dir := t.TempDir()
	names := []string{"svg-icons", "pdf-forms", "plug:deploy"}
	content := "Skills:\n- svg-icons: Make icons\n  and small logos\n- pdf-forms\n- plug:deploy: Ship it"
	typed := func(text string) map[string]any {
		return ccLine("user", map[string]any{"origin": map[string]any{"kind": "human"}, "message": map[string]any{"role": "user", "content": text}})
	}
	p := filepath.Join(dir, "a.jsonl")
	writeFile(t, p, jl(t,
		listing(true, names, content),
		typed("<command-message>pdf-forms</command-message>\n<command-name>/pdf-forms</command-name>\n<command-args>fill it</command-args>"),
		typed("<command-name>/plug:deploy</command-name>"),
		typed("<command-name>/model</command-name>"),
		typed("<command-name>workflow-x</command-name>"),
		typed("please run <command-name>/svg-icons</command-name>"), // not a typed command: words come first
	))
	f, err := ParseClaude(p)
	if err != nil {
		t.Fatal(err)
	}
	s := f.Setup
	if s == nil || len(s.Listed) != 3 || s.ListLen != len([]rune(content)) {
		t.Fatalf("listing: %+v", s)
	}
	want := []SkillLine{{Name: "svg-icons", Chars: len("- svg-icons: Make icons\n  and small logos\n")}, {Name: "pdf-forms", Chars: len("- pdf-forms\n")},
		{Name: "plug:deploy", Chars: len("- plug:deploy: Ship it"), Source: "plug"}}
	for i, w := range want {
		if s.Listed[i] != w {
			t.Errorf("chars %d: %+v, want %+v", i, s.Listed[i], w)
		}
	}
	if s.SkillUses["pdf-forms"] != 1 || s.SkillUses["plug:deploy"] != 1 || s.SkillUses["workflow-x"] != 1 || s.SkillUses["svg-icons"] != 0 || s.SkillUses["model"] != 1 {
		t.Errorf("typed uses: %v", s.SkillUses)
	}
}

func cxDev(text string) map[string]any {
	return cxLine("response_item", map[string]any{"type": "message", "role": "developer", "content": []any{map[string]any{"type": "input_text", "text": text}}})
}

func cxList(items ...string) string {
	return "<skills_instructions>\n## Skills\n### Skill roots\n- `r0` = `/home/u/.agents/skills`\n- `r1` = `/home/u/.codex/plugins/cache/mkt/atlas/1.2/skills`\n### Available skills\n" + // privacy-ok: made-up path
		strings.Join(items, "\n") + "\n### How to use skills\n- Read the SKILL.md.\n</skills_instructions>"
}

// Codex (docs/33 for Codex): the list from the developer message and world_state (patched), root aliases expanded,
// a plugin's skills grouped by plugin; uses from a pick and from commands that read SKILL.md or run a script, by
// folder through the session's own list, not from searches or writes; an uncertain list is flagged, not counted.
func TestCodexSkillListAndUse(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "rollout-2026-11-20T10-00-00-thread-one.jsonl.zst")
	cmd := func(c string) map[string]any {
		return cxLine("event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "CommandExecution", "id": "c", "command": []any{"/bin/zsh", "-lc", c},
			"cwd": "/home/u/invoice-sync", "exit_code": 0}}) // privacy-ok: made-up path
	}
	codexFile(t, p,
		cxDev(cxList("- pdf-forms: Fill forms (file: r0/pdf-forms/SKILL.md)", "- atlas:maps: Draw maps (file: r1/maps/SKILL.md)",
			"- gif-maker: Make GIFs (file: r0/gif-maker/SKILL.md)", "- notes: Keep notes (file: r0/notes/SKILL.md)", "- deploy: Ship (file: r0/deploy/SKILL.md)")),
		cxLine("world_state", map[string]any{"full": true, "state": map[string]any{"host_skills": map[string]any{"includeInstructions": true,
			"body": cxList("- pdf-forms: Fill forms (file: r0/pdf-forms/SKILL.md)")}}}),
		// a patch adds a list with one more skill
		cxLine("world_state", map[string]any{"full": false, "state": map[string]any{"host_skills": map[string]any{
			"body": cxList("- pdf-forms: Fill forms (file: r0/pdf-forms/SKILL.md)", "- late-one: New (file: r0/late-one/SKILL.md)")}}}),
		cxLine("response_item", map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text",
			"text": "<skill>\n<name>pdf-forms</name>\n<path>/home/u/.agents/skills/pdf-forms/SKILL.md</path>\nbody</skill>"}}}), // privacy-ok: made-up path
		cmd("cat '/home/u/.codex/plugins/cache/mkt/atlas/1.2/skills/maps/SKILL.md' && ls /home/u/.agents/skills"),            // privacy-ok: made-up path
		cmd("rg -n deploy /home/u/.agents/skills/gif-maker/SKILL.md; sed -i 's/a/b/' /home/u/.agents/skills/notes/SKILL.md"), // privacy-ok: made-up path
		cmd("cd /home/u && python3 .agents/skills/deploy/scripts/run.py"),                                                    // privacy-ok: made-up path
	)
	f, err := ParseCodex(p)
	if err != nil {
		t.Fatal(err)
	}
	s := f.Setup
	if s == nil || s.SkillsAt == NaT || s.SkillsUncertain || len(s.Listed) != 6 {
		t.Fatalf("list: %+v", s)
	}
	var names []string
	for _, l := range s.Listed {
		names = append(names, l.Name+"="+l.Source)
	}
	if strings.Join(names, ",") != "pdf-forms=,atlas:maps=atlas,gif-maker=,notes=,deploy=,late-one=" {
		t.Errorf("names and sources: %v", names)
	}
	// a search or an edit that names a skill's folder counts too: only "used" may be wrong, never "unused"
	if s.SkillUses["pdf-forms"] != 1 || strings.Join(mentioned(s), ",") != "atlas:maps,deploy,gif-maker,notes,pdf-forms" {
		t.Errorf("uses: picked %v, mentioned %v", s.SkillUses, mentioned(s))
	}
	// a relative script path counts from the command's folder
	codexFile(t, p, cxDev(cxList("- deploy: Ship (file: r0/deploy/SKILL.md)")),
		cxLine("event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "CommandExecution", "id": "c",
			"command": []any{"/bin/zsh", "-lc", "python3 .agents/skills/deploy/scripts/run.py"}, "cwd": "/home/u", "exit_code": 0}})) // privacy-ok: made-up path
	if f, _ := ParseCodex(p); f.Setup == nil || strings.Join(mentioned(f.Setup), ",") != "deploy" {
		t.Errorf("relative script path: %+v", f.Setup)
	}
	// commands: after cd, under ~ or $HOME, with 2>/dev/null, a write, a Windows path: the folder is named, so used
	for c, want := range map[string]string{
		"cd /home/u/.agents/skills/notes && cat SKILL.md":                  "notes",
		"cat ~/.agents/skills/notes/SKILL.md":                              "notes",
		`bash "$HOME/.agents/skills/deploy/scripts/run.sh" 2>&1 | tail -5`: "deploy",
		"cat /home/u/.agents/skills/notes/SKILL.md 2>/dev/null":            "notes",
		"python3 /home/u/.agents/skills/deploy/scripts/x.py > out.json":    "deploy",
		"echo '# done' >> /home/u/.agents/skills/notes/SKILL.md":           "notes",
		"cat > /home/u/.agents/skills/notes/SKILL.md":                      "notes",
		`type C:\Users\u\.agents\skills\notes\SKILL.md`:                    "notes",
	} { // privacy-ok: made-up paths
		codexFile(t, p, cxDev(cxList("- notes: Keep notes (file: r0/notes/SKILL.md)", "- deploy: Ship (file: r0/deploy/SKILL.md)")),
			cxLine("event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "CommandExecution", "id": "c",
				"command": []any{"/bin/zsh", "-lc", c}, "cwd": "/work", "exit_code": 0}}))
		f, _ := ParseCodex(p)
		got := mentioned(f.Setup)
		if strings.Join(got, ",") != want {
			t.Errorf("%q: used %v, want %q", c, got, want)
		}
	}
	// a Windows list and command
	codexFile(t, p, cxDev("<skills_instructions>\n### Skill roots\n- `r0` = `C:\\Users\\u\\.agents\\skills`\n### Available skills\n- notes: Keep notes (file: r0/notes/SKILL.md)\n</skills_instructions>"),
		cxLine("event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "CommandExecution", "id": "c",
			"command": `Get-Content C:\Users\u\.agents\skills\notes\SKILL.md`, "cwd": `C:\work`, "exit_code": 0}})) // privacy-ok: made-up path
	if f, _ := ParseCodex(p); f.Setup == nil || strings.Join(mentioned(f.Setup), ",") != "notes" || f.Setup.Listed[0].Dir != "c:/Users/u/.agents/skills/notes" {
		t.Errorf("windows: %+v", f.Setup)
	}
	// uncertain lists: hidden by config, every description dropped, or the "omitted" body
	for name, rec := range map[string]map[string]any{
		"hidden":  cxLine("world_state", map[string]any{"full": true, "state": map[string]any{"host_skills": map[string]any{"includeInstructions": false, "body": ""}}}),
		"bare":    cxDev(cxList("- pdf-forms: (file: r0/pdf-forms/SKILL.md)", "- notes: (file: r0/notes/SKILL.md)")),
		"omitted": cxLine("world_state", map[string]any{"full": true, "state": map[string]any{"host_skills": map[string]any{"body": "Host skills are available but omitted because the skills context budget was exceeded."}}}),
	} {
		codexFile(t, p, rec)
		if f, _ := ParseCodex(p); f.Setup == nil || !f.Setup.SkillsUncertain {
			t.Errorf("%s: not flagged: %+v", name, f.Setup)
		}
	}
}

// A Codex sub-agent thread: its inherited list only tells which folder is which skill; its own use counts, its list
// does not (the session is counted once, from the parent).
func TestCodexSubAgentSkillUse(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "rollout-2026-11-20T10-00-00-thread-sub.jsonl")
	meta := cxLine("session_meta", map[string]any{"id": "thread-sub", "cwd": "/home/u/invoice-sync", "source": map[string]any{"subagent": "x"}, // privacy-ok: made-up path
		"parent_thread_id": "thread-one", "subagent_history_start_ordinal": 2})
	dev := cxDev(cxList("- notes: Keep notes (file: r0/notes/SKILL.md)"))
	dev["ordinal"] = 1
	run := cxLine("event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "CommandExecution", "id": "c",
		"command": []any{"/bin/zsh", "-lc", "cat /home/u/.agents/skills/notes/SKILL.md"}, "cwd": "/home/u", "exit_code": 0}}) // privacy-ok: made-up path
	run["ordinal"] = 3
	writeFile(t, p, jl(t, meta, dev, run))
	f, err := ParseCodex(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Setup == nil || f.Setup.SkillMentions["skills/notes"] == 0 || len(f.Setup.Listed) != 0 || f.Setup.SkillsAt != NaT {
		t.Errorf("sub-agent: %+v", f.Setup)
	}
}

// A folder whose name holds "scripts" (transcripts, npm-scripts, a scripts folder above the skills) is still the
// skill's folder: SKILL.md and scripts/ count only as whole path parts.
func TestCodexScriptsInFolderName(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "rollout-2026-11-20T10-00-00-thread-one.jsonl")
	list := "<skills_instructions>\n### Skill roots\n- `r0` = `/home/u/.agents/skills`\n- `r1` = `/home/u/dev/scripts/skills`\n### Available skills\n" + // privacy-ok: made-up path
		"- transcripts: Clean transcripts (file: r0/transcripts/SKILL.md)\n- npm-scripts: Run npm scripts (file: r0/npm-scripts/SKILL.md)\n" +
		"- deploy: Ship (file: r1/deploy/SKILL.md)\n</skills_instructions>"
	for c, want := range map[string]string{
		"cat /home/u/.agents/skills/transcripts/SKILL.md":            "transcripts",
		"sed -n 1,40p '/home/u/.agents/skills/npm-scripts/SKILL.md'": "npm-scripts",
		"python3 /home/u/dev/scripts/skills/deploy/scripts/run.py":   "deploy",
		"cat /home/u/dev/scripts/skills/deploy/SKILL.md":             "deploy",
		"cat /home/u/.agents/skills/transcripts/README.md":           "transcripts", // the folder is named
	} { // privacy-ok: made-up paths
		codexFile(t, p, cxDev(list), cxLine("event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "CommandExecution", "id": "c",
			"command": []any{"/bin/zsh", "-lc", c}, "cwd": "/work", "exit_code": 0}}))
		f, _ := ParseCodex(p)
		got := mentioned(f.Setup)
		if strings.Join(got, ",") != want {
			t.Errorf("%q: used %v, want %q", c, got, want)
		}
	}
}

// A listing whose names hold a non-string is shaped differently: no empty name becomes a listed skill.
func TestClaudeListingOddName(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.jsonl")
	writeFile(t, p, jl(t, ccLine("attachment", map[string]any{"attachment": map[string]any{"type": "skill_listing", "isInitial": true,
		"names": []any{"svg-icons", 7, "pdf-forms"}, "content": "- svg-icons: Make icons\n- pdf-forms"}})))
	f, err := ParseClaude(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Setup != nil {
		for _, l := range f.Setup.Listed {
			if l.Name == "" {
				t.Errorf("an empty name was listed: %+v", f.Setup.Listed)
			}
		}
	}
}

// Codex's delta form: an entry shown names a listed skill even when no list text names it (its folder's name then);
// a folder a list text names keeps that name; an entry never shown (left out for the budget) is not listed; one shown
// and later removed was listed at some point, like a skill in an earlier list text.
func TestCodexDeltaEntries(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "rollout-2026-11-20T10-00-00-thread-one.jsonl")
	key := func(dir string) string { return `["host","host","` + dir + `/SKILL.md"]` }
	codexFile(t, p,
		cxLine("world_state", map[string]any{"full": true, "state": map[string]any{"host_skills": map[string]any{"version": 1, "entries": map[string]any{
			key("/home/u/.agents/skills/notes"):   map[string]any{"fingerprint": "a", "shown": true},  // privacy-ok: made-up path
			key("/home/u/.agents/skills/gif"):     map[string]any{"fingerprint": "b", "shown": true},  // privacy-ok: made-up path
			key("/home/u/.agents/skills/hidden"):  map[string]any{"fingerprint": "c", "shown": false}, // privacy-ok: made-up path
			key("/home/u/.agents/skills/removed"): map[string]any{"fingerprint": "d", "shown": true},  // privacy-ok: made-up path
		}}}}),
		cxLine("world_state", map[string]any{"full": false, "state": map[string]any{"host_skills": map[string]any{"entries": map[string]any{
			key("/home/u/.agents/skills/removed"): nil,                          // privacy-ok: made-up path
			key("/home/u/.agents/skills/gif"):     map[string]any{"grant": "x"}, // privacy-ok: made-up path; no "shown": still shown
		}}}}),
		cxDev(cxList("- gif-maker: Make GIFs (file: r0/gif/SKILL.md)")),
		cxLine("event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "CommandExecution", "id": "c",
			"command": []any{"/bin/zsh", "-lc", "cat /home/u/.agents/skills/notes/SKILL.md"}, "cwd": "/work", "exit_code": 0}})) // privacy-ok: made-up path
	f, err := ParseCodex(p)
	if err != nil {
		t.Fatal(err)
	}
	s := f.Setup
	var names []string
	for _, l := range s.Listed {
		names = append(names, l.Name)
	}
	sort.Strings(names)
	if s.SkillsAt == NaT || strings.Join(names, ",") != "gif-maker,notes,removed" || strings.Join(mentioned(s), ",") != "notes" {
		t.Errorf("delta: listed %v, mentioned %v", names, mentioned(s))
	}
}

// Review round 4: a sub-agent thread whose list is only inherited (world_state or compacted, not a developer
// message) still keeps the folders its commands touched; a code-mode exec call with no item_completed record is
// read for its commands; a skill typed in a scheduled (non-human) prompt counts.
func TestSkillUsesRound4(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "rollout-2026-11-20T10-00-00-thread-sub.jsonl")
	meta := cxLine("session_meta", map[string]any{"id": "thread-sub", "cwd": "/home/u/invoice-sync", "source": map[string]any{"subagent": "x"}, // privacy-ok: made-up path
		"parent_thread_id": "thread-one", "subagent_history_start_ordinal": 2})
	run := cxLine("event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "CommandExecution", "id": "c",
		"command": []any{"/bin/zsh", "-lc", "cat ~/.agents/skills/cloud-kit/SKILL.md"}, "cwd": "/home/u", "exit_code": 0}})
	run["ordinal"] = 3
	writeFile(t, p, jl(t, meta, run))
	f, err := ParseCodex(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Setup == nil || f.Setup.SkillMentions["skills/cloud-kit"] == 0 {
		t.Errorf("sub-agent without a list of its own: %+v", f.Setup)
	}
	// code-mode exec without item_completed records
	q := filepath.Join(dir, "rollout-2026-11-20T10-00-00-thread-exec.jsonl")
	codexFile(t, q, cxLine("response_item", map[string]any{"type": "custom_tool_call", "name": "exec", "call_id": "e1",
		"input": `text(await tools.exec_command({cmd:"cat /home/u/.agents/skills/notes/SKILL.md\nls","max_output_tokens":4000}));`})) // privacy-ok: made-up path
	if f, _ := ParseCodex(q); f.Setup == nil || f.Setup.SkillMentions["skills/notes"] == 0 {
		t.Errorf("code-mode exec: %+v", f.Setup)
	}
	// Claude Code: a scheduled prompt that types a skill
	r := filepath.Join(dir, "a.jsonl")
	writeFile(t, r, jl(t, ccLine("user", map[string]any{"origin": map[string]any{"kind": "scheduled"}, "message": map[string]any{"role": "user",
		"content": "<command-message>pdf-forms</command-message>\n<command-name>/pdf-forms</command-name>"}})))
	if f, _ := ParseClaude(r); f.Setup == nil || f.Setup.SkillUses["pdf-forms"] != 1 {
		t.Errorf("scheduled prompt: %+v", f.Setup)
	}
}

// Claude desktop logs a typed skill as a meta user record ("<command-message>…<command-name>name</command-name>
// <skill-format>true</skill-format>"): it is still a use.
func TestClaudeTypedSkillMeta(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.jsonl")
	writeFile(t, p, jl(t, ccLine("user", map[string]any{"isMeta": true, "message": map[string]any{"role": "user",
		"content": "<command-message>flow-writer</command-message>\n<command-name>flow-writer</command-name>\n<skill-format>true</skill-format>"}})))
	if f, _ := ParseClaude(p); f.Setup == nil || f.Setup.SkillUses["flow-writer"] != 1 {
		t.Errorf("meta record: %+v", f.Setup)
	}
}

// mentioned are the listed skills whose folder a tool call named (and the picked ones), sorted.
func mentioned(s *Setup) []string {
	var out []string
	if s == nil {
		return nil
	}
	for _, l := range s.Listed {
		hit := s.SkillUses[l.Name] > 0
		for _, k := range SkillKeys(l.Dir, "") {
			hit = hit || s.SkillMentions[k] > 0
		}
		if hit {
			out = append(out, l.Name)
		}
	}
	sort.Strings(out)
	return out
}

// Claude Code uses: a typed skill in any text block (an opened-file note may come first), a Skill call whose input
// is {"command": …}, and a Skill call in a sub-agent's file or a sidechain record.
func TestClaudeSkillUseShapes(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.jsonl")
	skill := func(inp map[string]any, extra map[string]any) map[string]any {
		m := map[string]any{"message": map[string]any{"id": "m", "model": "claude-opus-5-5", "usage": map[string]any{"input_tokens": 1},
			"content": []any{map[string]any{"type": "tool_use", "id": "t", "name": "Skill", "input": inp}}}}
		for k, v := range extra {
			m[k] = v
		}
		return ccLine("assistant", m)
	}
	writeFile(t, p, jl(t,
		ccLine("user", map[string]any{"origin": map[string]any{"kind": "human"}, "message": map[string]any{"role": "user", "content": []any{
			map[string]any{"type": "text", "text": "<ide_opened_file>The user opened notes.md</ide_opened_file>"},
			map[string]any{"type": "text", "text": "<command-message>alpha</command-message>\n<command-name>/alpha</command-name>"}}}}),
		skill(map[string]any{"command": "beta"}, nil),
		skill(map[string]any{"skill": "gamma"}, map[string]any{"isSidechain": true}),
	))
	f, err := ParseClaude(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Setup == nil || f.Setup.SkillUses["alpha"] != 1 || f.Setup.SkillUses["beta"] != 1 || f.Setup.SkillUses["gamma"] != 1 {
		t.Errorf("uses: %+v", f.Setup)
	}
	sub := filepath.Join(dir, "s1", "subagents", "agent-1.jsonl")
	writeFile(t, sub, jl(t, skill(map[string]any{"skill": "delta"}, map[string]any{"isSidechain": true})))
	if f, _ := ParseClaude(sub); f.Setup == nil || f.Setup.SkillUses["delta"] != 1 {
		t.Errorf("sub-agent file: %+v", f.Setup)
	}
}

// normArg folds a tool call's input to one comparable form, in one place.
func TestNormArg(t *testing.T) {
	for in, want := range map[string]string{
		`C:\\Users\\U\\.agents\\skills\\foo\\SKILL.md`: "c:/users/u/.agents/skills/foo/skill.md",
		`C:\Users\U\.agents\skills\foo`:                "c:/users/u/.agents/skills/foo",
		`\u002fhome\u002fu\u002fskills\u002ffoo`:       "/home/u/skills/foo",
		`skills%2Ffoo%2fSKILL.md`:                      "skills/foo/skill.md",
		`/home/u//.agents///skills/foo`:                "/home/u/.agents/skills/foo",
		`$HOME/.agents/skills/foo`:                     "~/.agents/skills/foo",
		`${HOME}/x`:                                    "~/x",
		`%USERPROFILE%\.agents`:                        "~/.agents",
		`$env:USERPROFILE\.agents`:                     "~/.agents",
		`$CODEX_HOME/skills/x`:                         "$codex_home/skills/x",
		`${CODEX_HOME}/skills/x`:                       "$codex_home/skills/x",
		`%CODEX_HOME%\skills\x`:                        "$codex_home/skills/x",
		`$env:CODEX_HOME\skills\x`:                     "$codex_home/skills/x",
	} {
		if got := normArg(in); got != want {
			t.Errorf("normArg(%q) = %q, want %q", in, got, want)
		}
	}
}

// A Skill call or a pick whose name carries arguments ("name args") counts under the name.
func TestSkillNameWithArgs(t *testing.T) {
	s := &Setup{}
	s.countUse("pdf-forms fill the March form")
	s.countUse("  notes  ")
	if s.SkillUses["pdf-forms"] != 1 || s.SkillUses["notes"] != 1 {
		t.Errorf("uses: %v", s.SkillUses)
	}
}

// The whole path from a call's raw input: decoded once as it was written (JSON, nested JSON, or a script's strings),
// then normalized. A decoded Windows path keeps "\notes" as a folder, not a newline.
func TestArgPipeline(t *testing.T) {
	for in, want := range map[string]string{
		`"\ud55c/skills/\uba54\ubaa8"`: "한/skills/메모",
		`"cat \"a b/SKILL.md\""`:       `cat "a b/skill.md"`,
		`{"command":"Get-Content C:\\Users\\U\\.agents\\skills\\notes\\SKILL.md"}`:        "get-content c:/users/u/.agents/skills/notes/skill.md",
		`{"cmd":"{\"path\":\"C:\\\\Users\\\\U\\\\skills\\\\foo\"}"}`:                      "c:/users/u/skills/foo",
		`text(await tools.exec_command({cmd:"cat C:\\Users\\U\\skills\\foo\\SKILL.md"}))`: `text(await tools.exec_command({cmd:"cat c:/users/u/skills/foo/skill.md"}))` + "\n" + `text(await tools.exec_command({cmd:"cat c:/users/u/skills/foo/skill.md"}))`, // as written and unescaped
		`{"path":"\u002fhome\u002fu\u002fskills\u002ffoo"}`:                               "/home/u/skills/foo",
	} {
		if got := normArg(argTextRaw(in)); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	if got := normArg(`C:\Users\U\.agents\skills\notes`); got != "c:/users/u/.agents/skills/notes" {
		t.Errorf("a decoded Windows path: %q", got)
	}
}

// A large tool input (a 1 MB base64 blob) is read in linear time, and the folder hashes stay within their bound.
func TestMentionsLargeInput(t *testing.T) {
	blob := make([]byte, 1<<20)
	for i := range blob {
		blob[i] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"[(i*7919+i/3)%64]
	}
	s := &Setup{}
	start := time.Now()
	s.mentions("data:" + string(blob) + " /home/u/.agents/skills/foo/SKILL.md") // privacy-ok: made-up path
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("1 MB input took %v", d)
	}
	if s.SkillMentions["skills/foo"] == 0 {
		t.Error("the path after the blob was lost")
	}
	if s.hashes > cxMaxHashes {
		t.Errorf("hashes %d over the bound", s.hashes)
	}
}

// Plain-text keys kept in the cache hold only folder-name characters (letters, digits, . _ - + @, an escaped space),
// never command text; matching is unchanged (the tokens inside a quoted command still match). Skill names too.
func TestPlainKeysHoldNoCommandText(t *testing.T) {
	s := &Setup{}
	s.mentions(argTextRaw(`{"cmd":"bash -lc \"sed -n '1,40p' ~/.agents/skills/foo/SKILL.md && TOKEN=s3cr3t ./skills/x y/z && cd /tmp/skills/key=ab%20cd/q && cat 'skills/a b; rm -rf ~'/SKILL.md\""}`))
	s.mentions(`cat /home/u/.agents/skills/my\ notes/SKILL.md`) // privacy-ok: made-up path
	s.countUse("pdf-forms fill it")
	s.countUse("x;rm -rf ~")
	s.countUse("$(whoami)")
	ok := regexp.MustCompile(`^[\p{L}\p{N}._+@ -]{1,100}$`)
	for k := range s.SkillMentions {
		if strings.HasPrefix(k, "#") {
			continue
		}
		parent, folder, _ := strings.Cut(k, "/")
		if (parent != "skills" && parent != ".system" && parent != "*") || !ok.MatchString(folder) {
			t.Errorf("plain key with command text: %q", k)
		}
	}
	for n := range s.SkillUses {
		if !regexp.MustCompile(`^[\p{L}\p{N}._+@:-]{1,100}$`).MatchString(n) {
			t.Errorf("skill name with command text: %q", n)
		}
	}
	if s.SkillMentions["skills/foo"] == 0 || s.SkillMentions["*/foo"] == 0 || s.SkillMentions["skills/my notes"] == 0 || s.SkillUses["pdf-forms"] != 1 {
		t.Errorf("matching lost: %v %v", s.SkillMentions, s.SkillUses)
	}
}

// The bound on folder hashes counts distinct keys, not repeats: thousands of calls in one folder leave room for a
// later custom root.
func TestHashBoundCountsKeys(t *testing.T) {
	s := &Setup{}
	for i := 0; i < cxMaxHashes+5000; i++ {
		s.mentions(`{"cmd":"ls","workdir":"/work/a/b/c/d/e"}`)
	}
	s.mentions("python /opt/team-kit/foo/run.py")
	if s.SkillMentions[folderHash("/opt/team-kit/foo")] == 0 {
		t.Errorf("a later custom root was dropped: %d hashes kept", s.hashes)
	}
}

// A tool's output is not a use: only calls' inputs count.
func TestCodexOutputIsNoUse(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rollout-2026-11-20T10-00-00-thread-one.jsonl")
	codexFile(t, p, cxLine("response_item", map[string]any{"type": "function_call_output", "call_id": "c",
		"output": "see /home/u/.agents/skills/foo/SKILL.md and skills/bar/scripts/run.py"})) // privacy-ok: made-up path
	if f, _ := ParseCodex(p); f.Setup != nil && len(f.Setup.SkillMentions) > 0 {
		t.Errorf("output counted: %v", f.Setup.SkillMentions)
	}
}

// A new delta entry without "shown" is not shown (not listed).
func TestCodexDeltaNewEntryWithoutShown(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rollout-2026-11-20T10-00-00-thread-one.jsonl")
	codexFile(t, p, cxLine("world_state", map[string]any{"full": true, "state": map[string]any{"host_skills": map[string]any{"entries": map[string]any{
		`["host","host","/home/u/.agents/skills/quiet/SKILL.md"]`: map[string]any{"fingerprint": "a"}}}}})) // privacy-ok: made-up path
	if f, _ := ParseCodex(p); f.Setup != nil && len(f.Setup.Listed) > 0 {
		t.Errorf("an entry without shown was listed: %+v", f.Setup.Listed)
	}
}

// A file's own listed folders keep their hash keys after the bound is full: a custom-root skill named late in a
// long session still matches.
func TestListedFolderHashPastTheBound(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rollout-2026-11-20T10-00-00-thread-one.jsonl")
	recs := []any{cxDev("<skills_instructions>\n### Available skills\n- kit: Team kit (file: /opt/team-kit/kit/SKILL.md)\n</skills_instructions>")}
	for i := 0; i < cxMaxHashes/3+100; i++ {
		recs = append(recs, cxLine("event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "CommandExecution", "id": "c",
			"command": "ls /w/a" + strconv.Itoa(i) + "/b/c", "cwd": "/w", "exit_code": 0}}))
	}
	recs = append(recs, cxLine("event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "CommandExecution", "id": "c",
		"command": "python /opt/team-kit/kit/tool.py", "cwd": "/w", "exit_code": 0}}))
	codexFile(t, p, recs...)
	f, err := ParseCodex(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Setup.SkillMentions[folderHash("/opt/team-kit/kit")] == 0 {
		t.Errorf("the listed folder's hash was dropped past the bound (%d hashes)", len(f.Setup.SkillMentions))
	}
}

// Review of PR #15: a skill shown only by a delta host_skills entry (no list text) gets the same list hashes as a
// listed one: a relative path under a custom root joins it, and its folder's hash is kept past the bound.
func TestCodexDeltaEntryListHashes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rollout-2026-11-20T10-00-00-thread-one.jsonl")
	recs := []any{cxLine("world_state", map[string]any{"full": true, "state": map[string]any{"host_skills": map[string]any{"version": 1, "entries": map[string]any{
		`["host","host","/opt/team-kit/kit/SKILL.md"]`: map[string]any{"fingerprint": "a", "shown": true},
	}}}})}
	for i := 0; i < cxMaxHashes/3+100; i++ {
		recs = append(recs, cxLine("event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "CommandExecution", "id": "c",
			"command": "ls /w/a" + strconv.Itoa(i) + "/b/c", "cwd": "/w", "exit_code": 0}}))
	}
	recs = append(recs, cxLine("event_msg", map[string]any{"type": "item_completed", "item": map[string]any{"type": "CommandExecution", "id": "c",
		"command": "cd /opt/team-kit && python kit/tool.py", "cwd": "/w", "exit_code": 0}}))
	codexFile(t, p, recs...)
	f, err := ParseCodex(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Setup.SkillMentions[folderHash("/opt/team-kit/kit")] == 0 {
		t.Errorf("a delta-only skill under a custom root, past the bound: not matched (%d hashes)", len(f.Setup.SkillMentions))
	}
}

// Joined relative-path hashes have a budget of their own: a file that reaches the bound keeps every folder hash the
// paths themselves name, as before the joining existed.
func TestJoinedHashesKeepTheBound(t *testing.T) {
	t.Setenv("HOME", "/home/u") // privacy-ok: made-up path
	s := &Setup{}
	for i := 0; i < 3000; i++ {
		s.mentions("cd ~ && ls a" + strconv.Itoa(i) + "/")
	}
	for j := 0; j < (cxMaxHashes-10)/3; j++ {
		s.mentions("ls /w/b" + strconv.Itoa(j) + "/c/d")
	}
	s.mentions("ls /w/last/c/d")
	if s.SkillMentions[folderHash("/w/last/c/d")] == 0 {
		t.Errorf("a path's own hash was pushed out by joined hashes (%d keys)", len(s.SkillMentions))
	}
}
