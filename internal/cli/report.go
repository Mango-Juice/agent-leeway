package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/advice"
	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/drift"
	"github.com/Mango-Juice/agent-leeway/internal/leak"
	"github.com/Mango-Juice/agent-leeway/internal/py"
	"github.com/Mango-Juice/agent-leeway/internal/report"
)

// bundledRubric is the version id of the bundled rubric the validation figures were measured with: without the
// task revision, because the validation ran on the judged text before it (TaskV21).
func bundledRubric() string {
	if rb, err := drift.LoadRubric(""); err == nil {
		v, _, _ := strings.Cut(rb.Version, "+")
		return v
	}
	return "?"
}

// newModel builds the report model from the window's leak report (nil for drift only) and judged records.
// groups are the stored grouping results (none: example mode); leftOut counts sessions of a tool that were not
// selected for judging.
func newModel(R *leak.Report, recs []py.Value, lang string, now, since time.Time, quoteChars int, groups []py.Value, leftOut map[string]int, errw io.Writer) *report.Model {
	cat, err := advice.Load(config.ConfigDir())
	if err != nil {
		fmt.Fprintf(errw, "Your advice.json could not be read (%v); using the built-in advice.\n", err)
	}
	return report.NewModel(report.Input{Leak: R, Records: recs, Advice: cat, Lang: lang, Now: now, Since: since, Until: now,
		PricesRead: config.Prices().Read, BundledRubric: bundledRubric(), QuoteChars: quoteChars, Groups: groups, LeftOut: leftOut})
}

// otherTool is the tool not chosen with --tool ("" when both are).
func otherTool(tool string) string {
	switch tool {
	case "claude":
		return "codex"
	case "codex":
		return "claude"
	}
	return ""
}
