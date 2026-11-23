package cli

import (
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
// Only the sessions the rest of the report is about count: direct conversations with a request in the window (not
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
	uses := map[string]int{}
	var obs []report.SetupObs
	for _, r := range tb.Setups {
		proj, ok := sel[r.Key.S]
		if !r.Key.OK || !ok {
			continue
		}
		for k, n := range r.Setup.SkillUses {
			uses[k] += n
		}
		obs = append(obs, report.SetupObs{Tool: r.Tool, Project: proj, Cwd: r.Cwd.S, S: r.Setup})
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
		Uses:        uses,
	})
}
