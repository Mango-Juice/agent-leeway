package pipeline

import (
	"github.com/Mango-Juice/agent-leeway/internal/config"
	"sort"
	"strings"

	"github.com/Mango-Juice/agent-leeway/internal/logs"
)

// OriginRow says who started one response: the owner in a live conversation ("human") or an automation ("auto").
type OriginRow struct {
	ThreadID string
	Seq      int
	TS       int64
	Key      logs.NS
	Tool     string
	Trigger  logs.NS
	Origin   logs.NS
	Reason   logs.NS
}

// SessionOrigin is the session-level class.
type SessionOrigin struct {
	Key, Tool                   string
	SessionClass, SessionReason string
	Uncertain                   bool
	SessionOrigin               string
}

// OrcaMarkers are phrases of an orchestrator's dispatched-worker preamble.
var OrcaMarkers = []string{"You are a dispatched worker", "You are working inside Orca"}

var autoTriggers = map[string]string{"automation_heartbeat_scheduled": "auto:heartbeat", "goal": "auto:goal"}

// BuildOrigin classifies sessions and responses (origin.classify). Rules, first match wins:
// session: auto:orca_worker (worker preamble in a user prompt), auto:codex_exec, auto:orca_codex,
// auto:agent_opened (Codex session without a user root opened by an agent); response (human sessions only):
// auto:heartbeat, auto:goal; sub-agent threads take the class of the root turn running when they started.
func (t *Tables) BuildOrigin() {
	userThreads := map[string]bool{}
	type grp struct {
		key     string
		threads []logs.Thread
	}
	byKey := map[string]*grp{}
	var keys []string
	for _, x := range t.Threads {
		if !x.SessionID.OK {
			continue
		}
		if x.Source.OK && x.Source.S == "user" {
			userThreads[x.ThreadID] = true
		}
		k := x.Tool + ":" + x.SessionID.S
		g := byKey[k]
		if g == nil {
			g = &grp{key: k}
			byKey[k] = g
			keys = append(keys, k)
		}
		g.threads = append(g.threads, x)
	}
	rules := config.Origin()
	markers := rules.WorkerMarkers
	if config.Legacy {
		markers = OrcaMarkers
	}
	orca := map[string]bool{}
	for _, u := range t.Turns {
		if !userThreads[u.ThreadID] || !u.Key.OK {
			continue
		}
		for _, m := range markers {
			if m != "" && strings.Contains(u.Text, m) {
				orca[u.Key.S] = true
			}
		}
	}
	sort.Strings(keys)
	t.SessOrigin = nil
	cls := map[string]*SessionOrigin{}
	for _, k := range keys {
		g := byKey[k]
		var roots []logs.Thread
		for _, x := range g.threads {
			if x.Source.OK && x.Source.S == "user" {
				roots = append(roots, x)
			}
		}
		set := func(ts []logs.Thread, f func(logs.Thread) logs.NS) map[string]bool {
			m := map[string]bool{}
			for _, x := range ts {
				if v := f(x); v.OK {
					m[v.S] = true
				}
			}
			return m
		}
		src := roots
		if len(roots) == 0 {
			src = g.threads
		}
		orig := set(src, func(x logs.Thread) logs.NS { return x.Originator })
		ent := set(g.threads, func(x logs.Thread) logs.NS { return x.Entrypoint })
		sources := set(g.threads, func(x logs.Thread) logs.NS { return x.Source })
		tool := g.threads[0].Tool
		so := &SessionOrigin{Key: k, Tool: tool, SessionClass: "human", SessionReason: "interactive"}
		if config.Legacy {
			legacySessionClass(so, orca[k], orig, ent, sources, tool, len(roots))
		} else {
			typed := 0
			launch := set(src, func(x logs.Thread) logs.NS { return x.LaunchSource })
			for _, x := range g.threads {
				typed += x.TypedPrompts
			}
			classifySession(so, rules, orca[k], orig, ent, sources, launch, typed, len(roots))
		}
		cls[k] = so
		t.SessOrigin = append(t.SessOrigin, *so)
	}
	// response level
	out := make([]OriginRow, len(t.Steps))
	human := make([]bool, len(t.Steps))
	for i, s := range t.Steps {
		r := OriginRow{ThreadID: s.ThreadID, Seq: s.Seq, TS: s.TS, Key: s.Key, Tool: s.Tool, Trigger: s.Trigger}
		if s.Key.OK {
			if so := cls[s.Key.S]; so != nil {
				r.Origin, r.Reason = logs.Some(so.SessionClass), logs.Some(so.SessionReason)
				human[i] = so.SessionClass == "human"
			}
		}
		if human[i] && userThreads[s.ThreadID] && s.Trigger.OK {
			if a, ok := autoTriggers[s.Trigger.S]; ok {
				r.Origin, r.Reason = logs.Some("auto"), logs.Some(a)
			}
		}
		out[i] = r
	}
	// sub-agent threads: class of the root turn running when the thread started (merge_asof backward by key)
	type rs struct {
		ts             int64
		origin, reason logs.NS
	}
	byK := map[string][]rs{}
	var order []int
	for i, s := range t.Steps {
		if human[i] && userThreads[s.ThreadID] && s.TS != NaT {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(a, b int) bool { return t.Steps[order[a]].TS < t.Steps[order[b]].TS })
	for _, i := range order {
		k := t.Steps[i].Key.S
		byK[k] = append(byK[k], rs{t.Steps[i].TS, out[i].Origin, out[i].Reason})
	}
	subCls := map[string]rs{}
	for _, x := range t.Threads {
		if !x.SessionID.OK || userThreads[x.ThreadID] || x.Start == NaT {
			continue
		}
		list := byK[x.Tool+":"+x.SessionID.S]
		j := sort.Search(len(list), func(i int) bool { return list[i].ts > x.Start }) - 1
		if j >= 0 {
			subCls[x.ThreadID] = list[j]
		} else {
			subCls[x.ThreadID] = rs{}
		}
	}
	for i, s := range t.Steps {
		if !human[i] || userThreads[s.ThreadID] {
			continue
		}
		if c, ok := subCls[s.ThreadID]; ok && c.origin.OK && c.origin.S == "auto" {
			out[i].Origin, out[i].Reason = c.origin, c.reason
		}
	}
	t.Origin = out
	// session origin: mixed when its responses have more than one class
	mix := map[string]string{}
	first := map[string]string{}
	for _, r := range out {
		if !r.Key.OK || !r.Origin.OK {
			continue
		}
		k := r.Key.S
		if f, ok := first[k]; !ok {
			first[k] = r.Origin.S
			mix[k] = r.Origin.S
		} else if f != r.Origin.S {
			mix[k] = "mixed"
		}
	}
	for i := range t.SessOrigin {
		so := &t.SessOrigin[i]
		if m, ok := mix[so.Key]; ok {
			so.SessionOrigin = m
		} else {
			so.SessionOrigin = so.SessionClass
		}
	}
}

// legacySessionClass is the reference's session rule (one owner's setup), kept for the parity tests.
func legacySessionClass(so *SessionOrigin, worker bool, orig, ent, sources map[string]bool, tool string, nRoots int) {
	orca := map[string]bool{so.Key: worker}
	k := so.Key
	roots := make([]struct{}, nRoots)
	switch {
	case orca[k]:
		so.SessionClass, so.SessionReason = "auto", "auto:orca_worker"
	case orig["codex_exec"]:
		so.SessionClass, so.SessionReason = "auto", "auto:codex_exec"
	case orig["orca_desktop"]:
		so.SessionClass, so.SessionReason = "auto", "auto:orca_codex"
	case tool == "codex" && len(roots) == 0 && (sources["agent_created_thread"] || sources["agent_forked_thread"] || sources["guardian_review"] || sources["subagent"]):
		so.SessionClass, so.SessionReason = "auto", "auto:agent_opened"
	case tool == "claude" && (ent["sdk-ts"] || ent["orca"]):
		// launched programmatically but no worker preamble: could be a chat panel typed by the owner
		so.SessionReason, so.Uncertain = "sdk_without_worker_preamble", true
	case tool == "claude" && len(ent) == 0:
		so.SessionReason, so.Uncertain = "no_entrypoint", true
	}
}

func anyIn(set map[string]bool, list []string) (string, bool) {
	for _, v := range list {
		if set[v] {
			return v, true
		}
	}
	return "", false
}

func onlyIn(set map[string]bool, list []string) bool {
	if len(set) == 0 {
		return false
	}
	for v := range set {
		found := false
		for _, w := range list {
			if v == w {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// classifySession decides from what the log records about how the session was started (first match):
// a worker marker in a prompt; a launcher listed as automation (Claude Code entrypoint, Codex originator or
// session source); a Codex thread opened by an agent; a launcher listed as interactive, or a prompt typed in
// Claude Code's terminal UI. Anything else (an SDK program, an unknown app, no launcher recorded) is counted
// as a person's conversation but marked uncertain.
func classifySession(so *SessionOrigin, r config.OriginRules, worker bool, orig, ent, sources, launch map[string]bool, typed, nRoots int) {
	auto := func(reason string) { so.SessionClass, so.SessionReason = "auto", reason }
	if worker {
		auto("auto:worker_marker")
		return
	}
	if so.Tool == "codex" {
		if v, ok := anyIn(orig, r.AutoOriginators); ok {
			if v == "codex_exec" {
				auto("auto:codex_exec")
			} else {
				auto("auto:originator")
			}
			return
		}
		if _, ok := anyIn(launch, r.AutoSources); ok {
			auto("auto:codex_exec")
			return
		}
		if nRoots == 0 && (sources["agent_created_thread"] || sources["agent_forked_thread"] || sources["guardian_review"] || sources["subagent"]) {
			auto("auto:agent_opened")
			return
		}
		if onlyIn(orig, r.InteractiveOriginators) {
			return
		}
		so.Uncertain = true
		so.SessionReason = "unknown_launcher"
		if len(orig) == 0 {
			so.SessionReason = "no_launcher"
		}
		return
	}
	if _, ok := anyIn(ent, r.AutoEntrypoints); ok {
		auto("auto:headless")
		return
	}
	if typed > 0 || onlyIn(ent, r.InteractiveEntrypoints) {
		return
	}
	so.Uncertain = true
	so.SessionReason = "unknown_launcher"
	if len(ent) == 0 {
		so.SessionReason = "no_launcher"
	}
}
