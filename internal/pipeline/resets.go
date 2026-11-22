package pipeline

import (
	"math"
	"time"
)

// ResetThreshold: a response is a cache *reset* when at least this share of the prefix the previous
// response already sent was not read from cache (gap below the TTL, not after a compaction).
const ResetThreshold = 0.20

// Priority of events that can explain a reset (the first present one is the primary cause).
var Priority = []string{"model_change", "effort_change", "config_update", "tools_change", "shape_change", "developer_msg",
	"world_state_full", "settings_applied", "new_user_turn", "subagent_activity", "gap_over_5min"}

// ResetStep is a response considered for the reset table (valid previous response, no expiry).
type ResetStep struct {
	*StepRow
	MissCost, MissExpiry float64
	PrevIn               float64
	PrevTS               int64
	MissTok, Frac, Mid   float64
	Cls                  string // reset | partial | clean
}

// ResetEvent is one response of a thread that has at least one reset, with the events before it.
type ResetEvent struct {
	Tool, ThreadID, Key string
	TS                  int64
	Cls                 string
	USD, Frac           float64
	Flags               map[string]bool
}

// Primary is the highest-priority event before a reset, or "no_event_found".
func (e ResetEvent) Primary() string {
	for _, k := range Priority {
		if e.Flags[k] {
			return k
		}
	}
	return "no_event_found"
}

// BuildResets builds the reset table (cache_causes.reset_table).
func (t *Tables) BuildResets() {
	t.ResetSteps, t.ResetEv = nil, nil
	var valid []ResetStep
	for _, w := range t.Waste {
		if math.IsNaN(w.prevIn) || w.step.AfterCompaction || w.MissExpiry > 0 {
			continue
		}
		s := w.step
		r := ResetStep{StepRow: s, MissCost: w.MissCost, MissExpiry: w.MissExpiry, PrevIn: w.prevIn, PrevTS: w.prevTS}
		r.MissTok = math.Max(math.Min(w.prevIn, float64(s.InputTotal))-float64(s.Cached), 0)
		r.Frac = r.MissTok / w.prevIn
		r.Mid = math.Max(w.MissCost-w.MissExpiry, 0)
		switch {
		case r.Frac >= ResetThreshold:
			r.Cls = "reset"
		case r.Mid > 1e-9:
			r.Cls = "partial"
		default:
			r.Cls = "clean"
		}
		valid = append(valid, r)
	}
	t.ResetSteps = valid
	// threads with a reset, in order of appearance
	var threads []string
	has := map[string]bool{}
	for _, r := range valid {
		if r.Cls == "reset" && !has[r.ThreadID] {
			has[r.ThreadID] = true
			threads = append(threads, r.ThreadID)
		}
	}
	tool := map[string]string{}
	for _, x := range t.Threads {
		if _, ok := tool[x.ThreadID]; !ok {
			tool[x.ThreadID] = x.Tool
		}
	}
	byThread := map[string][]ResetStep{}
	for _, r := range valid {
		if has[r.ThreadID] {
			byThread[r.ThreadID] = append(byThread[r.ThreadID], r)
		}
	}
	for _, tid := range threads {
		ev := t.Events[tid]
		ts := make([]int64, len(ev))
		for i, e := range ev {
			ts[i] = e.TS
		}
		for _, r := range byThread[tid] {
			lo, hi := r.PrevTS, r.TS // NaT compares as the smallest int64, as pandas' .value does
			flags := map[string]bool{}
			for i, e := range ev {
				if ts[i] > lo && ts[i] <= hi {
					flags[e.Kind] = true
				}
			}
			if r.TS != NaT && r.PrevTS != NaT && r.TS-r.PrevTS > int64(5*time.Minute) {
				flags["gap_over_5min"] = true
			}
			for k := range flags {
				if !inPriority(k) {
					delete(flags, k)
				}
			}
			t.ResetEv = append(t.ResetEv, ResetEvent{Tool: tool[tid], ThreadID: tid, Key: r.Key.S, TS: r.TS, Cls: r.Cls, USD: r.Mid, Frac: r.Frac, Flags: flags})
		}
	}
}

func inPriority(k string) bool {
	for _, p := range Priority {
		if p == k {
			return true
		}
	}
	return false
}
