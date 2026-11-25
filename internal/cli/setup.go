package cli

import (
	"math"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/drift"
	"github.com/Mango-Juice/agent-leeway/internal/pipeline"
	"github.com/Mango-Juice/agent-leeway/internal/report"
)

// setupItems are the "Your setup" rows (docs/33) from the main-thread sessions active in the window. Only counts,
// lengths, skill names and paths come from the logs; from the settings, only "skillOverrides" (Claude Code) and
// "project_doc_max_bytes" (Codex config.toml) are read.
//
// Only the sessions the rest of the report is about count (skill uses count every file active in the window): direct conversations with a request in the window (not
// automations, unless --include-auto), of the tools and the project asked for (--tool, --project).
func setupItems(o *driftOpts, tb *pipeline.Tables, since, now time.Time) []report.SetupItem {
	var tools []string
	if o.tool != "" {
		tools = []string{o.tool}
	}
	sel := map[string]string{}
	for _, s := range drift.Select(tb, since, now.Add(time.Minute), tools, o.project, o.includeAuto, 0) {
		sel[s.Key] = s.Project
	}
	// skill uses count from every log file of the tool active in the window (automations, sub-agent threads, other
	// projects): calling a used skill unused is the costly mistake, so uses are counted wide; the sessions that count
	// toward the list stay the report's
	from, to := since.UnixNano(), now.Add(time.Minute).UnixNano()
	useBy := map[string]report.SkillUse{}
	add := func(m *map[string]int, from map[string]int) {
		for k, n := range from {
			if *m == nil {
				*m = map[string]int{}
			}
			(*m)[k] += n
		}
	}
	var obs []report.SetupObs
	for _, r := range tb.Setups {
		if (o.tool == "" || o.tool == r.Tool) && r.End >= from && r.Start <= to {
			u := useBy[r.Tool]
			add(&u.Named, r.Setup.SkillUses)
			add(&u.Mentions, r.Setup.SkillMentions)
			useBy[r.Tool] = u
		}
		proj, ok := sel[r.Key.S]
		if !r.Key.OK || !ok {
			continue
		}
		obs = append(obs, report.SetupObs{Tool: r.Tool, Project: proj, Cwd: r.Cwd.S, Key: r.Key.S, S: r.Setup})
	}
	if len(obs) == 0 {
		return nil
	}
	nameOnly := map[string]map[string]bool{}
	return report.BuildSetup(obs, report.SetupOpts{
		NameOnly: func(cwd string) map[string]bool {
			if m, ok := nameOnly[cwd]; ok {
				return m
			}
			m := config.SkillNameOnly(cwd)
			nameOnly[cwd] = m
			return m
		},
		AgentsLimit: config.CodexDocMaxBytes(),
		Home:        config.Home(),
		Uses:        useBy["claude"].Named,
		UseBy:       useBy,
		Days:        int(math.Round(now.Sub(since).Hours() / 24)),
		Now:         now,
	})
}
