package logs

import (
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// Setup is what one main-thread log file says about the setup sent with every request (docs/33 §5): counts,
// lengths, skill names and instruction-file paths, never their text. The record types read here are not
// documented by either vendor; a record that is missing or shaped differently leaves its part unknown (At = NaT),
// so the report shows nothing for it rather than a wrong "within".
type Setup struct {
	// Claude Code: the newest initial skill listing (attachment skill_listing, isInitial true)
	SkillsAt int64
	Skills   int         // skills in the listing
	Bare     []string    // names listed without a description
	Listed   []SkillLine // every listed skill, in the listing's order, with the characters it takes there
	ListLen  int         // the listing's characters (runes)
	// Claude Code: the instruction files as last loaded (attachment instructions; later ones replace a file by path)
	InstrAt int64
	Instr   []InstrFile
	// Codex: the AGENTS.md text in effect (world_state agents_md.text), in bytes, and whether it ends with a newline
	// (Codex cuts the text at its limit, usually mid-line). The Codex skill list is not read: a skill left out for the
	// budget cannot be told from one the user turned off.
	AgentsAt     int64
	AgentsBytes  int
	AgentsEndsNL bool
	// Skill uses per skill name, as the user or agent named it (Claude Code: Skill tool calls and skills started by
	// typing /name; Codex: the <skill><name> block a pick adds)
	SkillUses map[string]int
	// Codex: the skill folders tool calls named (keys as SkillKeys gives them), matched to any session's list
	SkillMentions map[string]int
	// Codex: the list cannot be taken as all that was listed (hidden by config, every description dropped, or the
	// "omitted" body); the session's list is not counted
	SkillsUncertain bool
	hashes          int // folder hashes kept in SkillMentions (bounded)
	joined          int // of those, hashes of a relative path joined to the folder its call runs in (bounded apart)

	cx *codexSkills // Codex: the list being read (not kept)
}

// SkillLine is one skill of the listing: its name and the characters of its entry (its line, with any lines of
// description that follow, and the newline).
type SkillLine struct {
	Name   string
	Chars  int
	Source string // the skills' group: a plugin's name, or "" for a skill of its own
	Dir    string // Codex: the skill's folder (its SKILL.md's), to match uses by path
}

// InstrFile is one CLAUDE.md file as loaded: its path, the loader's type (User, Project, Local, ...) and lines.
type InstrFile struct {
	Path, Type string
	Lines      int
}

func newSetup() *Setup { return &Setup{SkillsAt: NaT, InstrAt: NaT, AgentsAt: NaT} }

// Empty says nothing was read.
func (s *Setup) Empty() bool {
	return s == nil || (s.SkillsAt == NaT && s.InstrAt == NaT && s.AgentsAt == NaT && len(s.SkillUses) == 0 && len(s.SkillMentions) == 0 && !s.SkillsUncertain)
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
		chars := map[string]int{}
		cur := "" // the skill whose entry the line belongs to ("" before the first)
		lines := strings.Split(content.S, "\n")
		for li, line := range lines {
			n := utf8.RuneCountInString(line)
			if li < len(lines)-1 {
				n++ // its newline
			}
			if rest, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "- "); ok {
				if set[rest] {
					bare[rest], cur = true, rest
				} else {
					// "- name: description" (a name may hold a colon itself: match the longest listed name)
					for i := len(rest) - 1; i > 0; i-- {
						if rest[i] == ':' && i+1 < len(rest) && rest[i+1] == ' ' && set[rest[:i]] {
							described[rest[:i]], cur = true, rest[:i]
							break
						}
					}
				}
			}
			if cur != "" {
				chars[cur] += n
			}
		}
		if len(bare)+len(described) < len(set) {
			return // a name with no line of its own: the listing is shaped differently than expected
		}
		s.SkillsAt, s.Skills, s.Bare, s.Listed = ts, len(set), nil, nil
		s.ListLen = utf8.RuneCountInString(content.S)
		seen := map[string]bool{}
		for _, n := range names.A {
			if bare[n.S] && !described[n.S] {
				s.Bare = append(s.Bare, n.S)
			}
			if !seen[n.S] {
				seen[n.S] = true
				src := ""
				if i := strings.Index(n.S, ":"); i > 0 { // a plugin's skill: "plugin:skill"
					src = n.S[:i]
				}
				s.Listed = append(s.Listed, SkillLine{Name: n.S, Chars: chars[n.S], Source: src})
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
	if n := inp.Get("skill").Or(inp.Get("command")); n.K == py.Str {
		s.countUse(n.S)
	}
}

// typedUse counts a command typed as a message by its name: the message starts with "<command-name>/name" or, for
// a skill, "<command-message>name</command-message>" followed by it. Built-in commands (/model, /compact) are
// counted too; they never match a listed skill, so they never count as one.
func (s *Setup) typedUse(text string) {
	if !strings.HasPrefix(text, "<command-name>") && !strings.HasPrefix(text, "<command-message>") {
		return
	}
	_, rest, ok := strings.Cut(text, "<command-name>")
	if !ok {
		return
	}
	name, _, ok := strings.Cut(rest, "</command-name>")
	if !ok {
		return
	}
	s.countUse(strings.TrimPrefix(strings.TrimSpace(name), "/"))
}

func (s *Setup) countUse(name string) {
	f := strings.Fields(strings.TrimPrefix(strings.TrimSpace(name), "/")) // "name args": the name
	if len(f) == 0 {
		return
	}
	name = strings.TrimPrefix(f[0], "/")
	if !plainName(name, "._-+@:", 200) { // a skill's name ("plugin:skill" may be long), never command text
		return
	}
	if s.SkillUses == nil {
		s.SkillUses = map[string]int{}
	}
	s.SkillUses[name]++
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
