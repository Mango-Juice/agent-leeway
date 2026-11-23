// Package pipeline turns parsed log files into the tables the leak and drift checks use: sessions, priced
// responses, per-response waste, who started each response (owner or automation) and the cache-rewrite table.
// It follows research/oracle/agent_leeway/pipeline/{sessions,waste,origin,cache_causes}.py, keeping pandas'
// missing-value rules (NaN/NaT are skipped by sums and never equal anything).
package pipeline

import (
	"math"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/logs"
)

// NaT is a missing timestamp.
const NaT = logs.NaT

// NaN is a missing number.
var NaN = math.NaN()

// StepRow is one model response with its thread and price.
type StepRow struct {
	logs.Step
	Tool      string
	ThreadID  string
	SessionID logs.NS
	Key       logs.NS
	// priced (steps_priced). ThreadModel is the thread's first model (the reference prices by it);
	// the product prices each response by Step.Model.
	ThreadModel              logs.NS
	PIn, PRead, PWrite, POut float64
	WCost                    float64
	// TTL is the cache lifetime that applies before this response (0 = the legacy per-tool lifetime).
	TTL time.Duration
	// Assumed names the model when its price came from a fallback row ("" = priced from its own row).
	Assumed string
}

// CallRow is one tool call with its thread.
type CallRow struct {
	logs.Call
	Tool      string
	ThreadID  string
	SessionID logs.NS
	Key       logs.NS
}

// TurnRow is one user turn with its thread.
type TurnRow struct {
	logs.Turn
	Tool      string
	ThreadID  string
	SessionID logs.NS
	Key       logs.NS
}

// CommitRow is one printed commit.
type CommitRow struct {
	logs.Commit
	Tool      string
	ThreadID  string
	SessionID logs.NS
}

// SetupRow is what one log file says about the setup sent with every request (docs/33).
type SetupRow struct {
	Tool, ThreadID string
	Key            logs.NS // the session
	Cwd            logs.NS
	Start, End     int64
	Setup          logs.Setup
}

// Tables holds every table, in the reference's row order.
type Tables struct {
	// OldestClaude is the oldest Claude Code transcript's time (UnixNano; 0 when none was read): Claude Code
	// deletes transcripts after 30 days by default, so earlier windows can lack its logs.
	OldestClaude int64
	Threads      []logs.Thread
	Steps        []StepRow // steps (normalize order); priced fields filled by Sessions()
	Calls        []CallRow
	Turns        []TurnRow
	Commits      []CommitRow
	Events       map[string][]logs.Event // per thread id
	TurnsHuman   []TurnRow
	Sessions     []Session
	Waste        []WasteRow
	Origin       []OriginRow
	SessOrigin   []SessionOrigin
	ResetEv      []ResetEvent
	ResetSteps   []ResetStep
	Stats        logs.Stats
	Setups       []SetupRow // files with setup facts, in discovery order
	Files        int
	Skipped      int
}

func keyOf(tool string, sid logs.NS) logs.NS {
	if !sid.OK {
		return logs.NS{}
	}
	return logs.Some(tool + ":" + sid.S)
}

// FromFiles concatenates parsed files (in discovery order) into the base tables.
func FromFiles(files []*logs.File) *Tables {
	t := &Tables{Events: map[string][]logs.Event{}}
	for _, f := range files {
		th := f.Thread
		t.Threads = append(t.Threads, th)
		k := keyOf(th.Tool, th.SessionID)
		for _, s := range f.Steps {
			t.Steps = append(t.Steps, StepRow{Step: s, Tool: th.Tool, ThreadID: th.ThreadID, SessionID: th.SessionID, Key: k})
		}
		for _, c := range f.Calls {
			t.Calls = append(t.Calls, CallRow{Call: c, Tool: th.Tool, ThreadID: th.ThreadID, SessionID: th.SessionID, Key: k})
		}
		for _, u := range f.Turns {
			t.Turns = append(t.Turns, TurnRow{Turn: u, Tool: th.Tool, ThreadID: th.ThreadID, SessionID: th.SessionID, Key: k})
		}
		for _, c := range f.Commits {
			t.Commits = append(t.Commits, CommitRow{Commit: c, Tool: th.Tool, ThreadID: th.ThreadID, SessionID: th.SessionID})
		}
		t.Events[th.ThreadID] = f.Events
		if f.Setup != nil {
			t.Setups = append(t.Setups, SetupRow{Tool: th.Tool, ThreadID: th.ThreadID, Key: k, Cwd: th.Cwd, Start: th.Start, End: th.End, Setup: *f.Setup})
		}
		t.Stats.Add(f.Stats)
	}
	t.Files = len(files)
	return t
}

// Build runs every stage after reading.
func (t *Tables) Build() {
	t.BuildSessions()
	t.BuildWaste()
	t.BuildOrigin()
	t.BuildResets()
}

// kahan is pandas' groupby sum (compensated summation; NaN skipped).
type kahan struct{ sum, c float64 }

func (k *kahan) add(x float64) {
	if math.IsNaN(x) {
		return
	}
	y := x - k.c
	t := k.sum + y
	k.c = (t - k.sum) - y
	k.sum = t
}
