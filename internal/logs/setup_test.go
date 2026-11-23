package logs

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

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
