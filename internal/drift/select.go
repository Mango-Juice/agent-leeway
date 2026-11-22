package drift

import (
	"sort"
	"strings"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"github.com/Mango-Juice/agent-leeway/internal/pipeline"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// MaxBody is the longest judge input (code points); a longer session is judged with its middle omitted.
// A variable only so tests can shorten it.
var MaxBody = 160_000

// Judging input limits and estimate constants.
const (
	OutTokensEst     = 2500 // typical judge output (validation runs)
	CharsPerTokenEst = 3.0  // conservative for mixed-script text
)

// Selected is one session chosen for judging.
type Selected struct {
	Key, Tool, Project string
	Last               int64
	NReq               int
	OriginUncertain    bool // the log does not tell whether a person or a program started the session
}

// Select picks sessions with at least one user request in [since, until), direct conversations unless
// includeAuto, most recent first.
func Select(t *pipeline.Tables, since, until time.Time, tools []string, project string, includeAuto bool, limit int) []Selected {
	lo, hi := since.UnixNano(), until.UnixNano()
	type agg struct {
		last int64
		n    int
	}
	A := map[string]*agg{}
	var keys []string
	for _, u := range t.TurnsHuman {
		if u.Kind != "human" || !u.Key.OK || u.TS == pipeline.NaT || u.TS < lo || u.TS >= hi {
			continue
		}
		a := A[u.Key.S]
		if a == nil {
			a = &agg{last: u.TS}
			A[u.Key.S] = a
			keys = append(keys, u.Key.S)
		}
		a.n++
		if u.TS > a.last {
			a.last = u.TS
		}
	}
	sort.Strings(keys)
	sess := map[string]pipeline.Session{}
	for _, s := range t.Sessions {
		if _, ok := sess[s.Key]; !ok {
			sess[s.Key] = s
		}
	}
	auto, uncertain := map[string]bool{}, map[string]bool{}
	for _, so := range t.SessOrigin {
		if so.SessionClass == "auto" {
			auto[so.Key] = true
		} else if so.Uncertain {
			uncertain[so.Key] = true
		}
	}
	var out []Selected
	for _, k := range keys {
		s, ok := sess[k]
		tool, proj := "", ""
		if ok {
			tool, proj = s.Tool, s.Project
		}
		if !includeAuto && auto[k] {
			continue
		}
		if len(tools) > 0 && !contains(tools, tool) {
			continue
		}
		if project != "" && !strings.Contains(py.ToUpper(proj), py.ToUpper(project)) {
			continue
		}
		out = append(out, Selected{Key: k, Tool: tool, Project: proj, Last: A[k].last, NReq: A[k].n, OriginUncertain: uncertain[k]})
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Last > out[b].Last })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// JudgeTier picks the judging tier for a tool: --model override, then settings.json, then the default.
func JudgeTier(tool string, overrides map[string]string) string {
	if v := overrides[tool]; v != "" {
		return v
	}
	if v := config.UserSettings().Judge[tool]; v != "" {
		return v
	}
	return config.JudgeDefault[tool]
}

// Item is a session ready to judge.
type Item struct {
	Key, Tool, Project, Tier string
	Body                     string
	Locs                     map[string]string
	NUser                    int
	EstUSD                   float64
	Warning                  string
	// OmittedChars is how much of the middle of a long session was left out of the judge input (0 = judged
	// whole); TotalChars is the rendered session's full length (code points).
	OmittedChars, TotalChars int
	OriginUncertain          bool
}

// EstimateUsage is the token usage assumed before a call. It is an upper estimate for the input: nothing is
// assumed to come from the provider's prompt cache, and Claude input is priced as a fresh 1-hour cache write
// (what the Claude Code CLI does on a cold call), so a re-run soon after the same sessions costs less. Output is
// the typical judge output. Under the reference's frozen prices the reference's plain-input estimate is kept.
func EstimateUsage(tier string, tokIn float64) config.Usage {
	if !config.Legacy && strings.HasPrefix(tier, "claude:") {
		return config.Usage{CacheWrite: tokIn, CacheWrite1h: tokIn, Output: OutTokensEst}
	}
	return config.Usage{Input: tokIn, Output: OutTokensEst}
}

// Prepare renders every selected session once and estimates its judging cost.
func Prepare(R *Renderer, sel []Selected, rb *Rubric, tiers map[string]string, currentInstr bool) []Item {
	system := BuildSystem(rb.Body, rb.Settings)
	var items []Item
	for _, s := range sel {
		tier := tiers[s.Tool]
		r := R.Session(s.Key, currentInstr && s.Tool == "claude")
		body := r.Text
		total := py.Len(body)
		omitted := 0
		if total > MaxBody {
			body = py.Head(body, MaxBody/2) + messages.Drift["truncated"] + py.Tail(body, MaxBody/2)
			omitted = total - 2*(MaxBody/2)
		}
		tokIn := float64(py.Len(system)+py.Len(body)) / CharsPerTokenEst
		est, _ := config.JudgeUSD(tier, EstimateUsage(tier, tokIn))
		if m := config.MeasuredUSDPerSession(tier); m > est {
			est = m // length alone misses reasoning and cache writes
		}
		w := ""
		if r.InstrCurrent {
			w = messages.Drift["instr_current_warning"]
		}
		items = append(items, Item{Key: s.Key, Tool: s.Tool, Project: s.Project, Tier: tier, Body: body, Locs: r.Locs, NUser: r.NUser, EstUSD: est, Warning: w,
			OmittedChars: omitted, TotalChars: total, OriginUncertain: s.OriginUncertain})
	}
	return items
}
