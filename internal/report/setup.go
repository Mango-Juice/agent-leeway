package report

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/logs"
)

// The setup checks (docs/33): the skill list and the instruction file of each tool, against the line the tool's
// own docs publish. Counted from the logs, offline; only counts, lengths, skill names and file paths are kept.

// Vendor lines.
const (
	ClaudeMDLines   = 200       // "target under 200 lines per CLAUDE.md file"
	AgentsMDDefault = 32 * 1024 // project_doc_max_bytes default (32 KiB)
)

// SetupItem is one row of "Your setup": one tool and kind, from the newest main session of the project shown.
type SetupItem struct {
	Tool    string // claude | codex
	Kind    string // skills (Claude Code) | instr
	N, K    int    // skills: listed and dropped; instr: lines (CLAUDE.md) or bytes (AGENTS.md)
	Limit   int    // instr: the line (200 lines, or the AGENTS.md limit in bytes)
	Over    bool
	Project string   // the project of the session shown ("" for your user-level CLAUDE.md)
	File    string   // CLAUDE.md: the longest file, relative to the project (or under ~)
	Names   []string // Claude Code skills whose description was dropped, used least first
	OverIn  []string // other projects over the line
	SeenAt  time.Time
	Share   float64 // how far over, as a share of the line (ranks setup fixes of one kind)
}

// SetupObs is one main-thread log file's setup facts, with its project.
type SetupObs struct {
	Tool, Project, Cwd string
	S                  logs.Setup
}

// SetupOpts are what the logs do not hold.
type SetupOpts struct {
	NameOnly    func(cwd string) map[string]bool // Claude Code skills set to "name-only" in skillOverrides (user, project, local)
	AgentsLimit int                              // project_doc_max_bytes (0: the default)
	Home        string                           // shown as "~" in file paths
	Uses        map[string]int                   // Skill tool calls per name in the window
}

// BuildSetup makes the rows: per tool and kind, the newest session of each project that recorded the fact, and of
// those the one furthest over (or, when none is over, the largest). A tool or kind nothing was read for gets no row.
func BuildSetup(obs []SetupObs, o SetupOpts) []SetupItem {
	limit := o.AgentsLimit
	if limit <= 0 {
		limit = AgentsMDDefault
	}
	type pick struct {
		at  int64
		obs *SetupObs
	}
	newest := func(tool string, at func(s *logs.Setup) int64) []*SetupObs {
		by := map[string]pick{}
		for i := range obs {
			x := &obs[i]
			if x.Tool != tool {
				continue
			}
			t := at(&x.S)
			if t == logs.NaT {
				continue
			}
			if p, ok := by[x.Project]; !ok || t > p.at {
				by[x.Project] = pick{t, x}
			}
		}
		var out []*SetupObs
		for _, k := range sortedKeys(by) {
			out = append(out, by[k].obs)
		}
		return out
	}
	when := func(ns int64) time.Time { return time.Unix(0, ns).UTC() }
	worst := func(items []SetupItem) *SetupItem {
		if len(items) == 0 {
			return nil
		}
		sort.SliceStable(items, func(i, j int) bool {
			a, b := items[i], items[j]
			if a.Over != b.Over {
				return a.Over
			}
			if a.Share != b.Share {
				return a.Share > b.Share
			}
			if a.K != b.K {
				return a.K > b.K
			}
			if a.N != b.N {
				return a.N > b.N
			}
			return a.SeenAt.After(b.SeenAt)
		})
		w := items[0]
		for _, x := range items[1:] {
			if x.Over && x.Project != w.Project {
				w.OverIn = append(w.OverIn, x.Project)
			}
		}
		return &w
	}
	var out []SetupItem
	add := func(x *SetupItem) {
		if x != nil {
			out = append(out, *x)
		}
	}
	// Claude Code skill list: names listed without a description, less those set to "name-only" on purpose
	var items []SetupItem
	for _, x := range newest("claude", func(s *logs.Setup) int64 { return s.SkillsAt }) {
		var off map[string]bool
		if o.NameOnly != nil {
			off = o.NameOnly(x.Cwd)
		}
		var names []string
		for _, n := range x.S.Bare {
			if !off[n] {
				names = append(names, n)
			}
		}
		sort.SliceStable(names, func(i, j int) bool { return o.Uses[names[i]] < o.Uses[names[j]] })
		it := SetupItem{Tool: "claude", Kind: "skills", N: x.S.Skills, K: len(names), Over: len(names) > 0, Project: x.Project, Names: names, SeenAt: when(x.S.SkillsAt)}
		if it.N > 0 {
			it.Share = float64(it.K) / float64(it.N)
		}
		items = append(items, it)
	}
	add(worst(items))
	// Claude Code: the longest CLAUDE.md file. A project's own files come from its newest session; your user-level
	// file is the same in every project, so it is one candidate on its own (shown once). A project whose newest
	// session loaded no CLAUDE.md of its own (only AGENTS.md, say) adds no project file.
	items = nil
	user := map[string]SetupItem{}
	for _, x := range newest("claude", func(s *logs.Setup) int64 { return s.InstrAt }) {
		var best *logs.InstrFile
		for i := range x.S.Instr {
			f := &x.S.Instr[i]
			if f.Lines == 0 {
				continue
			}
			if userFile(f, o.Home) {
				if u, ok := user[f.Path]; !ok || when(x.S.InstrAt).After(u.SeenAt) {
					user[f.Path] = SetupItem{Tool: "claude", Kind: "instr", N: f.Lines, Limit: ClaudeMDLines, Over: f.Lines > ClaudeMDLines,
						File: logs.RelPath(f.Path, "", o.Home), SeenAt: when(x.S.InstrAt), Share: float64(f.Lines-ClaudeMDLines) / ClaudeMDLines}
				}
				continue
			}
			if best == nil || f.Lines > best.Lines {
				best = f
			}
		}
		if best == nil {
			continue
		}
		it := SetupItem{Tool: "claude", Kind: "instr", N: best.Lines, Limit: ClaudeMDLines, Over: best.Lines > ClaudeMDLines, Project: x.Project,
			File: logs.RelPath(best.Path, x.Cwd, o.Home), SeenAt: when(x.S.InstrAt)}
		it.Share = float64(it.N-ClaudeMDLines) / ClaudeMDLines
		items = append(items, it)
	}
	for _, k := range sortedKeys(user) {
		items = append(items, user[k])
	}
	add(worst(items))
	// Codex AGENTS.md: the text Codex sent. Codex cuts the files it reads at project_doc_max_bytes and joins them with
	// a blank line, so a text that reached the limit is at least the limit long and, cut mid-line, ends without a
	// newline or, with more than one file, carries the separators past it. A text longer than the limit plus about
	// 1 KiB means the limit assumed here is not the one Codex used (a profile, -c, another CODEX_HOME): no row.
	items = nil
	for _, x := range newest("codex", func(s *logs.Setup) int64 { return s.AgentsAt }) {
		n := x.S.AgentsBytes
		if n == 0 || n > limit+agentsSlack {
			continue
		}
		over := n >= limit && (!x.S.AgentsEndsNL || n >= limit+len(agentsSep))
		it := SetupItem{Tool: "codex", Kind: "instr", N: n, Limit: limit, Over: over, Project: x.Project, SeenAt: when(x.S.AgentsAt)}
		it.Share = float64(n-limit) / float64(limit)
		items = append(items, it)
	}
	add(worst(items))
	return out
}

// agentsSep joins Codex's AGENTS.md files; agentsSlack is how far past the limit a text may run (separators).
const (
	agentsSep   = "\n\n"
	agentsSlack = 1024
)

// userFile is your own CLAUDE.md (loaded in every project), as opposed to a project's.
func userFile(f *logs.InstrFile, home string) bool {
	if f.Type == "User" {
		return true
	}
	return home != "" && strings.HasPrefix(filepath.ToSlash(f.Path), strings.TrimRight(filepath.ToSlash(home), "/")+"/.claude/")
}

// KiB is a byte count in whole KiB (at least 1).
func KiB(n int) int { return max(1, (n+512)/1024) }
