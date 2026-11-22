package report

import (
	"sort"
	"strconv"
	"strings"

	"github.com/Mango-Juice/agent-leeway/internal/drift"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// Theme is a stored grouping (drift.GroupAll; schema in docs/24 §12.R7 item 1) checked against this report's
// events: member ids that do not exist here or whose session belongs to another tool are dropped, the sessions
// are recomputed from the rest, and the theme is kept only when they are two or more. The report shows the
// member events' stored user quotes, never text from the grouping call except the label and the draft rule.
type Theme struct {
	Tool, Label, DraftRule string
	Events                 []*Event
	Sessions               int
	InFile                 bool // a member event cites a rule from an instruction file
}

// Themes maps the newest usable grouping of each tool onto the events of recs.
func Themes(groups []py.Value, recs []*Record) []*Theme {
	if len(groups) == 0 {
		return nil
	}
	byID := map[string]*Event{}
	for _, r := range recs {
		for _, e := range r.Events {
			byID[r.Key+"#"+strconv.Itoa(e.Idx)] = e
		}
	}
	latest := drift.LatestGroups(groups)
	var out []*Theme
	for _, tool := range []string{"claude", "codex"} {
		g, ok := latest[tool]
		if !ok {
			continue
		}
		for _, t := range g.Get("themes").A {
			th := ValidTheme(t, byID)
			if th != nil {
				out = append(out, th)
			}
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Sessions > out[b].Sessions })
	return out
}

// ValidTheme applies the validation rule to one stored theme (nil when it does not hold).
func ValidTheme(t py.Value, byID map[string]*Event) *Theme {
	th := &Theme{Tool: t.Get("tool").S, Label: strings.TrimSpace(t.Get("label").S), DraftRule: strings.TrimSpace(t.Get("draft_rule").S)}
	sess := map[string]bool{}
	seen := map[*Event]bool{}
	for _, id := range t.Get("events").A {
		e := byID[id.S]
		if e == nil || e.Rec.Tool != th.Tool || seen[e] {
			continue
		}
		seen[e] = true
		th.Events = append(th.Events, e)
		sess[e.Rec.Key] = true
		if strings.HasPrefix(e.InstrLoc, "I") {
			th.InFile = true
		}
	}
	th.Sessions = len(sess)
	if th.Sessions < 2 || th.Label == "" {
		return nil
	}
	return th
}
