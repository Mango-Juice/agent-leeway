package report

import (
	"fmt"
	"math"
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
	Share   float64    // how far over, as a share of the line (ranks setup fixes of one kind)
	Idle    *SkillIdle // skills: the listed skills not used in the period (nil: too few sessions to say)

	listed  []logs.SkillLine // skills: the list shown, while the rows are built
	listLen int
}

// SkillUse is one tool's skill uses in the window, per skill name: named (the agent's call, a typed command or a
// pick) and inferred (Codex: a command read the skill's SKILL.md or ran its scripts). Both count as a use.
type SkillUse struct {
	Named    map[string]int
	Mentions map[string]int // Codex: skill folders tool calls named (logs.SkillKeys), matched to the list's folders
}

func (o SetupOpts) useFor(tool string) SkillUse {
	if u, ok := o.UseBy[tool]; ok {
		return u
	}
	if tool == "claude" {
		return SkillUse{Named: o.Uses}
	}
	return SkillUse{}
}

// IdleMinSessions is how many sessions in the period must have recorded the tool's skill list, and must have
// listed a skill, before a skill is called unused. A skill started in one session out of ten goes unused in 20 such
// sessions by chance about one time in eight (0.9^20 = 12%); with fewer sessions "never used" says more about the
// sample than the skill.
const IdleMinSessions = 20

// SkillIdle is one tool's skills listed but never used in the period, against the listing of the session the row
// shows. A use is the agent's call of the skill or the user's typed command. Kept per tool (SetupItem.Tool), so
// another tool's list can get its own row.
type SkillIdle struct {
	Days      int // the period, in days
	Sessions  int // sessions in the period that recorded the list
	Listed    int // skills in the list shown
	Unused    int // of those, listed in at least IdleMinSessions sessions and never used
	New       int // listed in fewer sessions than that: not counted either way
	Inferred  int // of the skills counted as used, those used only by inference (Codex)
	Uncertain int // sessions whose list was not counted: hidden by config, or cut by the budget (Codex)
	Chars     int // the list's characters
	Idle      int // the unused skills' characters in it
	Groups    []SkillGroup
}

// Share is the unused skills' part of the list's characters.
func (x *SkillIdle) Share() float64 {
	if x == nil || x.Chars == 0 {
		return 0
	}
	return float64(x.Idle) / float64(x.Chars)
}

// SkillGroup is the unused skills of one source: a plugin (names "plugin:skill") or, with Source "", the rest.
type SkillGroup struct {
	Source string
	Listed int      // the source's skills in the list
	Names  []string // its unused skills
}

// All says every listed skill of a plugin went unused.
func (g SkillGroup) All() bool { return g.Source != "" && len(g.Names) == g.Listed }

// SetupObs is one main-thread log file's setup facts, with its project.
type SetupObs struct {
	Tool, Project, Cwd string
	Key                string // the session ("" counts each file as one)
	S                  logs.Setup
}

// SetupOpts are what the logs do not hold.
type SetupOpts struct {
	NameOnly    func(cwd string) map[string]bool // Claude Code skills set to "name-only" in skillOverrides (user, project, local)
	AgentsLimit int                              // project_doc_max_bytes (0: the default)
	Home        string                           // shown as "~" in file paths
	Uses        map[string]int                   // Claude Code skill uses per name in the window, for "used least first"
	UseBy       map[string]SkillUse              // skill uses per tool, for "not used" (nil: Uses, as Claude Code's)
	Days        int                              // the window, in days
	Now         time.Time                        // the window's end (zero: the window is all logged)
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
		it.listed, it.listLen = x.S.Listed, x.S.ListLen
		items = append(items, it)
	}
	if w := worst(items); w != nil {
		w.Idle = idleSkills(obs, "claude", w, o)
		w.listed = nil
		add(w)
	}
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
	// Codex: no skill-list row (a budget cut cannot be told apart in the default form), only unused skills, against
	// the newest certain list of the project with the longest one
	items = nil
	for _, x := range newest("codex", func(s *logs.Setup) int64 {
		if s.SkillsUncertain {
			return logs.NaT
		}
		return s.SkillsAt
	}) {
		items = append(items, SetupItem{Tool: "codex", Kind: "idle", N: len(x.S.Listed), Project: x.Project, SeenAt: when(x.S.SkillsAt),
			listed: x.S.Listed, listLen: x.S.ListLen})
	}
	if w := worst(items); w != nil {
		if w.Idle = idleSkills(obs, "codex", w, o); w.Idle != nil {
			w.listed, w.OverIn = nil, nil
			add(w)
		}
	}
	return out
}

// idleSkills counts the skills of the row's list that no session in the period used, when enough sessions recorded
// the tool's list (IdleMinSessions). A skill listed in fewer sessions (added lately, or one project's own) is not
// counted either way.
func idleSkills(obs []SetupObs, tool string, it *SetupItem, o SetupOpts) *SkillIdle {
	if len(it.listed) == 0 {
		return nil
	}
	// per session (a session's files share its key), the names it listed; and the oldest list, so the period said is
	// no longer than the logs reach
	lists, oldest, unsure := map[string]map[string]bool{}, int64(0), map[string]bool{}
	for i := range obs {
		x := &obs[i]
		k := x.Key
		if k == "" {
			k = fmt.Sprintf("#%d", i)
		}
		if x.Tool == tool && x.S.SkillsUncertain {
			unsure[k] = true
			continue
		}
		if x.Tool != tool || x.S.SkillsAt == logs.NaT {
			continue
		}
		if lists[k] == nil {
			lists[k] = map[string]bool{}
		}
		for _, l := range x.S.Listed {
			lists[k][l.Name] = true
		}
		if oldest == 0 || x.S.SkillsAt < oldest {
			oldest = x.S.SkillsAt
		}
	}
	if len(lists) < IdleMinSessions {
		return nil
	}
	in := map[string]int{}
	for _, l := range lists {
		for n := range l {
			in[n]++
		}
	}
	days := o.Days
	if !o.Now.IsZero() {
		if d := int(math.Ceil(o.Now.Sub(time.Unix(0, oldest)).Hours() / 24)); d < days || days == 0 {
			days = d
		}
	}
	use := o.useFor(tool)
	// a name, or the name without the plugin prefix ("tiles" for "atlas:tiles", and for every listed skill ending so)
	named := func(name string) bool {
		if use.Named[name] > 0 {
			return true
		}
		_, s, ok := strings.Cut(name, ":")
		return ok && use.Named[s] > 0
	}
	// every folder a listed name had in the period (a plugin may rename a skill's folder between versions)
	dirsOf := map[string]map[string]bool{}
	for i := range obs {
		if obs[i].Tool != tool {
			continue
		}
		for _, l := range obs[i].S.Listed {
			if l.Dir != "" {
				if dirsOf[l.Name] == nil {
					dirsOf[l.Name] = map[string]bool{}
				}
				dirsOf[l.Name][l.Dir] = true
			}
		}
	}
	mentioned := func(l logs.SkillLine) bool {
		dirs := dirsOf[l.Name]
		if l.Dir != "" {
			if dirs == nil {
				dirs = map[string]bool{}
			}
			dirs[l.Dir] = true
		}
		for dir := range dirs {
			for _, k := range logs.SkillKeys(dir, o.Home) {
				if use.Mentions[k] > 0 {
					return true
				}
			}
		}
		return false
	}
	r := &SkillIdle{Days: days, Sessions: len(lists), Listed: len(it.listed), Chars: it.listLen}
	for k := range unsure {
		if lists[k] == nil {
			r.Uncertain++
		}
	}
	by := map[string]*SkillGroup{}
	var order []string
	for _, l := range it.listed {
		src := l.Source
		g := by[src]
		if g == nil {
			g = &SkillGroup{Source: src}
			by[src] = g
			order = append(order, src)
		}
		g.Listed++
		switch {
		case named(l.Name):
		case mentioned(l):
			r.Inferred++
		case in[l.Name] < IdleMinSessions:
			r.New++
		default:
			r.Unused++
			r.Idle += l.Chars
			g.Names = append(g.Names, l.Name)
		}
	}
	for _, src := range order {
		g := by[src]
		if len(g.Names) == 0 {
			continue
		}
		sort.Strings(g.Names)
		r.Groups = append(r.Groups, *g)
	}
	// a plugin whose every skill went unused first, then more unused first; the skills of no plugin last
	sort.SliceStable(r.Groups, func(i, j int) bool {
		a, b := r.Groups[i], r.Groups[j]
		if (a.Source == "") != (b.Source == "") {
			return b.Source == ""
		}
		if a.All() != b.All() {
			return a.All()
		}
		if len(a.Names) != len(b.Names) {
			return len(a.Names) > len(b.Names)
		}
		return a.Source < b.Source
	})
	return r
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
