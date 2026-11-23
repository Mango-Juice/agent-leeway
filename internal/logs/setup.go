package logs

import (
	"path/filepath"
	"strings"

	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// Setup is what one main-thread log file says about the setup sent with every request (docs/33 §5): counts,
// lengths, skill names and instruction-file paths, never their text. The record types read here are not
// documented by either vendor; a record that is missing or shaped differently leaves its part unknown (At = NaT),
// so the report shows nothing for it rather than a wrong "within".
type Setup struct {
	// Claude Code: the newest initial skill listing (attachment skill_listing, isInitial true)
	SkillsAt int64
	Skills   int      // skills in the listing
	Bare     []string // names listed without a description
	// Claude Code: the instruction files as last loaded (attachment instructions; later ones replace a file by path)
	InstrAt int64
	Instr   []InstrFile
	// Codex: the AGENTS.md text in effect (world_state agents_md.text), in bytes, and whether it ends with a newline
	// (Codex cuts the text at its limit, usually mid-line). The Codex skill list is not read: a skill left out for the
	// budget cannot be told from one the user turned off.
	AgentsAt     int64
	AgentsBytes  int
	AgentsEndsNL bool
	// Skill tool calls per skill name (Claude Code), for "used least first"
	SkillUses map[string]int
}

// InstrFile is one CLAUDE.md file as loaded: its path, the loader's type (User, Project, Local, ...) and lines.
type InstrFile struct {
	Path, Type string
	Lines      int
}

func newSetup() *Setup { return &Setup{SkillsAt: NaT, InstrAt: NaT, AgentsAt: NaT} }

// Empty says nothing was read.
func (s *Setup) Empty() bool {
	return s == nil || (s.SkillsAt == NaT && s.InstrAt == NaT && s.AgentsAt == NaT && len(s.SkillUses) == 0)
}

// isClaudeMD is a file the "under 200 lines per CLAUDE.md file" line is about.
func isClaudeMD(path string) bool {
	b := path
	if i := strings.LastIndexAny(b, `/\`); i >= 0 {
		b = b[i+1:]
	}
	return b == "CLAUDE.md" || b == "CLAUDE.local.md"
}

// countLines counts lines as an editor shows them: a final line without a newline counts.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

// claudeAttachment reads the skill listing and the instruction files of one attachment record.
func (s *Setup) claudeAttachment(a py.Value, ts int64) {
	if a.K != py.Obj {
		return
	}
	switch {
	case isStr(a.Get("type"), "skill_listing"):
		if !(a.Get("isInitial").K == py.Bool && a.Get("isInitial").Truthy()) {
			return // a later listing names only the skills added since
		}
		content, names := a.Get("content"), a.Get("names")
		if content.K != py.Str || names.K != py.Arr || len(names.A) == 0 {
			return
		}
		bare, described := map[string]bool{}, map[string]bool{}
		set := map[string]bool{}
		for _, n := range names.A {
			if n.K != py.Str || n.S == "" {
				return
			}
			set[n.S] = true
		}
		for _, line := range strings.Split(content.S, "\n") {
			rest, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "- ")
			if !ok {
				continue
			}
			if set[rest] {
				bare[rest] = true
				continue
			}
			// "- name: description" (a name may hold a colon itself: match the longest listed name)
			for i := len(rest) - 1; i > 0; i-- {
				if rest[i] == ':' && i+1 < len(rest) && rest[i+1] == ' ' && set[rest[:i]] {
					described[rest[:i]] = true
					break
				}
			}
		}
		if len(bare)+len(described) < len(set) {
			return // a name with no line of its own: the listing is shaped differently than expected
		}
		s.SkillsAt, s.Skills, s.Bare = ts, len(set), nil
		for _, n := range names.A {
			if bare[n.S] && !described[n.S] {
				s.Bare = append(s.Bare, n.S)
			}
		}
	case isStr(a.Get("type"), "instructions"):
		files := a.Get("files")
		if files.K != py.Arr {
			return
		}
		got := map[string]int{}
		for i, f := range s.Instr {
			got[f.Path] = i
		}
		seen := false
		for _, f := range files.A {
			p, c := f.Get("path"), f.Get("content")
			if p.K != py.Str || c.K != py.Str {
				continue
			}
			seen = true
			if !isClaudeMD(p.S) {
				continue
			}
			x := InstrFile{Path: p.S, Type: py.PyStr(f.Get("type")), Lines: countLines(c.S)}
			if i, ok := got[p.S]; ok {
				s.Instr[i] = x
			} else {
				got[p.S] = len(s.Instr)
				s.Instr = append(s.Instr, x)
			}
		}
		if seen {
			s.InstrAt = ts
		}
	}
}

// codexWorldState reads the AGENTS.md size of one world_state payload (a full snapshot or a change).
func (s *Setup) codexWorldState(p py.Value, ts int64) {
	st := p.Get("state")
	if st.K != py.Obj {
		return
	}
	if t := st.Get("agents_md").Get("text"); t.K == py.Str {
		s.AgentsAt, s.AgentsBytes, s.AgentsEndsNL = ts, len(t.S), strings.HasSuffix(t.S, "\n")
	} else if p.Get("full").K == py.Bool && p.Get("full").Truthy() {
		s.AgentsAt = NaT
	}
}

// skillUse counts a Skill tool call by the skill's name.
func (s *Setup) skillUse(inp py.Value) {
	n := inp.Get("skill")
	if n.K != py.Str || n.S == "" {
		return
	}
	if s.SkillUses == nil {
		s.SkillUses = map[string]int{}
	}
	s.SkillUses[n.S]++
}

// RelPath shows an instruction file's path relative to the project folder, or under "~" for the user's own file;
// otherwise its last two parts.
func RelPath(path, cwd, home string) string {
	p := filepath.ToSlash(path)
	for _, base := range []string{cwd, home} {
		if base == "" {
			continue
		}
		b := strings.TrimRight(filepath.ToSlash(base), "/") + "/"
		if rest, ok := strings.CutPrefix(p, b); ok {
			if base == home {
				return "~/" + rest
			}
			return rest
		}
	}
	parts := strings.Split(p, "/")
	if len(parts) > 2 {
		parts = parts[len(parts)-2:]
	}
	return strings.Join(parts, "/")
}
