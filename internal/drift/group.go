package drift

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"github.com/Mango-Juice/agent-leeway/internal/proc"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// The grouping step (docs/24 §17.11, owner-approved): after judging, one call per provider groups that
// provider's judged problems by meaning ("you asked for this in 4 sessions"). Events from Claude Code sessions go
// only to the claude CLI and events from Codex sessions only to the codex CLI, with the same lock-down command
// line, preflight, timeout and budget as the judge. The model only names groups and lists member ids: the code
// checks every id, keeps an event in at most one theme, drops themes that span fewer than two sessions, and the
// report shows the stored user quotes, never text from this call. `report` reads stored groupings and never calls.

// GroupVersion identifies the grouping prompt (GroupSystem). Pinned by TestGroupPromptIsPinned.
// g2 (docs/27 §9): a theme must be a request the person stated in their own words, and each member carries the
// words that state it, checked in code; g1 themes stored before are no longer used.
const GroupVersion = "g2"

// GroupSystem is the grouping instruction.
const GroupSystem = `You group problems that were found in one person's sessions with an AI coding agent.
Each item is one problem a reviewer found: its id, the session it came from, the reviewer's rule and cause codes,
the instruction the agent went against (if any), what the person wrote at that point, and the reviewer's
one-sentence summary.

A group is one request that the person themselves stated, or had to state again, in their own words: the same
instruction, rule or expectation, possibly worded differently. Decide only from the person's words (person_said
and instruction). Do not decide from the summaries or the codes. Do not group items that only share a topic, a
file, a cause or a kind of agent mistake: for example several cases of the agent claiming something without
checking, about different facts, are not a request the person made, unless the person's own words ask for it each
time. Leave every other item ungrouped. An item belongs to at most one group. A group is useful only when its
items come from at least two different sessions.

For each group give:
- label: a short name for what the person's words ask for, at most 8 words, in the language the person mainly
  writes in; never a general principle the person did not state;
- events: the ids of its items, copied exactly from the list;
- stated: for every id in events, the person's words that state the request, copied character for character
  from that item's person_said or instruction (leave the item out if it has no such words);
- rule: one sentence the person could add to their instruction file so they would not have to say it again, in
  the language the person mainly writes in; null when no standing rule fits.

Use only ids from the list. Answer with one JSON object only, using exactly these field names:
{"themes": [{"label": "...", "events": ["e1", "e4"], "stated": {"e1": "...", "e4": "..."}, "rule": "..." or null}]}
If nothing should be grouped, answer {"themes": []}.`

// Limits on what the model may return (longer text is dropped, not cut).
const (
	MaxThemeLabel = 80  // characters
	MaxThemeRule  = 240 // characters
	MaxThemes     = 30
)

// GroupEvent is one judged problem sent to the grouping call.
type GroupEvent struct {
	ID, Session      string // short ids used in the prompt ("e3", "s2")
	Key, Tier        string // the judged record
	Idx              int    // position of the event in the record's events
	Loc, Rule, Cause string
	Instruction      string   // the cited instruction quote, as the judge saw it
	Quotes           []string // the person's quotes (evidence at U# locations), as the judge saw them
	Summary          string
}

// isProblem says whether a judged event can become a finding (the report's kinds); undeterminable causes are
// left out.
func isProblem(rule, cause string) bool {
	switch rule {
	case "R6a", "R6b", "R6c", "R3a":
		return true
	}
	switch cause {
	case "missing_or_ambiguous_instruction", "instruction_violation", "agent_error":
		return true
	}
	return false
}

// GroupInput collects one provider's judged problems from the latest successful record of each session,
// in a stable order (session key, then event order).
func GroupInput(records []py.Value, tool string) []GroupEvent {
	var recs []py.Value
	for _, r := range Latest(records) {
		if r.Get("tool").S == tool {
			recs = append(recs, r)
		}
	}
	sort.SliceStable(recs, func(a, b int) bool {
		if ka, kb := recs[a].Get("key").S, recs[b].Get("key").S; ka != kb {
			return ka < kb
		}
		return recs[a].Get("tier").S < recs[b].Get("tier").S
	})
	var out []GroupEvent
	sess := map[string]string{}
	for _, r := range recs {
		key := r.Get("key").S
		for i, e := range listOf(r.Get("out").Get("events")) {
			rule, cause := e.Get("rule").S, e.Get("cause").S
			if !isProblem(rule, cause) {
				continue
			}
			sid, ok := sess[key]
			if !ok {
				sid = "s" + itoa(len(sess)+1)
				sess[key] = sid
			}
			ge := GroupEvent{ID: "e" + itoa(len(out)+1), Session: sid, Key: key, Tier: r.Get("tier").S, Idx: i, Loc: e.Get("loc").S,
				Rule: rule, Cause: cause, Instruction: e.Get("instruction").Get("quote").S, Summary: e.Get("summary").S}
			for _, q := range listOf(e.Get("evidence")) {
				if s := q.Get("quote").S; s != "" && strings.HasPrefix(q.Get("loc").S, "U") { // the person's words only, not the agent's
					ge.Quotes = append(ge.Quotes, s)
				}
			}
			out = append(out, ge)
		}
	}
	return out
}

func itoa(n int) string { return py.PyStr(py.Value{K: py.Int, I: int64(n)}) }

// Groupable says whether a call can find anything: at least two problems from at least two sessions.
func Groupable(evs []GroupEvent) bool {
	s := map[string]bool{}
	for _, e := range evs {
		s[e.Session] = true
	}
	return len(evs) >= 2 && len(s) >= 2
}

// GroupPrompt is the grouping call's input: the items as one JSON object.
func GroupPrompt(evs []GroupEvent) string {
	var items []py.Value
	for _, e := range evs {
		o := &py.Object{}
		o.Set("id", py.String(e.ID))
		o.Set("session", py.String(e.Session))
		o.Set("rule", py.String(e.Rule))
		o.Set("cause", py.String(e.Cause))
		if e.Instruction != "" {
			o.Set("instruction", py.String(e.Instruction))
		} else {
			o.Set("instruction", py.Value{})
		}
		var qs []py.Value
		for _, q := range e.Quotes {
			qs = append(qs, py.String(q))
		}
		o.Set("person_said", arrOf(qs))
		o.Set("summary", py.String(e.Summary))
		items = append(items, py.Value{K: py.Obj, O: o})
	}
	o := &py.Object{}
	o.Set("items", arrOf(items))
	return py.DumpsOpt(py.Value{K: py.Obj, O: o}, py.DumpOpts{Indent: 1})
}

// GroupHash keys the cache: the prompt version and text, the judging tier and the input.
func GroupHash(tier, prompt string) string {
	h := sha256.Sum256([]byte(GroupVersion + "\x00" + GroupSystem + "\x00" + tier + "\x00" + prompt))
	return hex.EncodeToString(h[:])[:16]
}

// Theme is a validated group.
type Theme struct {
	Label  string
	Rule   string
	Events []string // ids
}

// Dropped is a theme the checks removed, with why.
type Dropped struct {
	Label, Reason string
}

// CheckThemes validates the model's answer against the input: ids must exist; an event is in at most one theme
// (later themes lose it); a theme must span at least two sessions; label and rule have length limits. ok is
// false when the answer is not a JSON object with a themes list.
func CheckThemes(text string, evs []GroupEvent) (themes []Theme, dropped []Dropped, ok bool) {
	v, parsed := Parse(text)
	if !parsed || v.K != py.Obj || v.Get("themes").K != py.Arr {
		return nil, nil, false
	}
	byID := map[string]GroupEvent{}
	for _, e := range evs {
		byID[e.ID] = e
	}
	used := map[string]bool{}
	for i, t := range v.Get("themes").A {
		label := strings.Join(strings.Fields(t.Get("label").S), " ")
		drop := func(reason string) { dropped = append(dropped, Dropped{label, reason}) }
		if i >= MaxThemes {
			drop("more than " + itoa(MaxThemes) + " themes")
			continue
		}
		if label == "" {
			drop("no label")
			continue
		}
		if utf8.RuneCountInString(label) > MaxThemeLabel {
			drop("label longer than " + itoa(MaxThemeLabel) + " characters")
			continue
		}
		ids := listOf(t.Get("events"))
		var keep []string
		bad, dup := "", false
		for _, x := range ids {
			id := strings.TrimSpace(x.S)
			if _, ok := byID[id]; !ok || x.K != py.Str {
				bad = py.PyStr(x)
				break
			}
			if used[id] {
				dup = true
				continue
			}
			if !contains(keep, id) {
				keep = append(keep, id)
			}
		}
		if bad != "" {
			drop("unknown event id " + py.Head(bad, 20))
			continue
		}
		// g2: keep only members whose stated words are found in that member's own person-side text
		stated := t.Get("stated")
		var verified []string
		unstated := 0
		for _, id := range keep {
			if StatedIn(stated.Get(id).S, byID[id]) {
				verified = append(verified, id)
			} else {
				unstated++
			}
		}
		sess := map[string]bool{}
		for _, id := range verified {
			sess[byID[id].Session] = true
		}
		if len(sess) < 2 {
			switch {
			case unstated > 0:
				drop("not stated in the person's own words in two sessions (" + itoa(unstated) + " member(s) without matching words)")
			case dup:
				drop("its events are already in an earlier theme")
			default:
				drop("fewer than two sessions")
			}
			continue
		}
		if unstated > 0 {
			dropped = append(dropped, Dropped{label, itoa(unstated) + " member(s) removed: their stated words are not in their own quotes"})
		}
		keep = verified
		rule := strings.Join(strings.Fields(t.Get("rule").S), " ")
		if utf8.RuneCountInString(rule) > MaxThemeRule {
			dropped = append(dropped, Dropped{label, "rule longer than " + itoa(MaxThemeRule) + " characters (theme kept without it)"})
			rule = ""
		}
		for _, id := range keep {
			used[id] = true
		}
		themes = append(themes, Theme{Label: label, Rule: rule, Events: keep})
	}
	return themes, dropped, true
}

// GroupsPath is where grouping results are stored (one JSON line per call).
func GroupsPath(cache string) string { return filepath.Join(cache, "drift", "groups.jsonl") }

// LoadGroups reads the stored grouping records.
func LoadGroups(cache string) []py.Value {
	b, err := os.ReadFile(GroupsPath(cache))
	if err != nil {
		return nil
	}
	var out []py.Value
	for _, line := range strings.Split(py.DecodeBytes(b), "\n") {
		if v, err := py.Loads(line); err == nil && v.K == py.Obj {
			out = append(out, v)
		}
	}
	return out
}

// ThemeValue is a theme as stored (docs/24 §12.R7 item 1): the tool, the label, the member events as
// "<session key>#<index in out.events>", the sessions they span, and the optional draft rule.
func ThemeValue(tool string, t Theme, byID map[string]GroupEvent) py.Value {
	o := &py.Object{}
	o.Set("tool", py.String(tool))
	o.Set("label", py.String(t.Label))
	var ids, ss []py.Value
	seen := map[string]bool{}
	for _, id := range t.Events {
		e := byID[id]
		ids = append(ids, py.String(e.Key+"#"+itoa(e.Idx)))
		if !seen[e.Key] {
			seen[e.Key] = true
			ss = append(ss, py.String(e.Key))
		}
	}
	o.Set("events", arrOf(ids))
	o.Set("sessions", arrOf(ss))
	o.Set("n_sessions", py.Value{K: py.Int, I: int64(len(ss))})
	if t.Rule != "" {
		o.Set("draft_rule", py.String(t.Rule))
	} else {
		o.Set("draft_rule", py.Value{})
	}
	return py.Value{K: py.Obj, O: o}
}

// LatestGroups is the newest usable grouping record per tool (no error, current prompt version).
func LatestGroups(recs []py.Value) map[string]py.Value {
	out := map[string]py.Value{}
	for _, r := range recs {
		if r.Get("error").Truthy() || r.Get("group_version").S != GroupVersion {
			continue
		}
		p := r.Get("tool").S
		if old, ok := out[p]; !ok || num(r.Get("ts")) >= num(old.Get("ts")) {
			out[p] = r
		}
	}
	return out
}

// GroupEstimateUSD is the estimated cost of one grouping call for n judged sessions: about 450 tokens of input
// per session (a few problems, each with quotes and a summary) plus the instruction, and up to 1,500 tokens out.
func GroupEstimateUSD(tier string, nSessions int) float64 {
	tokIn := float64(py.Len(GroupSystem))/CharsPerTokenEst + 450*float64(nSessions)
	u := EstimateUsage(tier, tokIn)
	u.Output = 1500
	usd, _ := config.JudgeUSD(tier, u)
	return usd
}

// GroupOutcome is what the grouping step did per provider.
type GroupOutcome struct {
	Lines []string // one line per provider, for the run log
	Spent float64
	Stop  string // a limit or configuration problem: the run should say so
}

// GroupAll runs the grouping step for each provider with judged problems in records. spent is what the run has
// already cost (for the budget check). A provider whose input is unchanged since its last stored grouping is not
// called again.
func GroupAll(ctx context.Context, cache string, records []py.Value, tiers map[string]string, o Options, spent float64) GroupOutcome {
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.Now == nil {
		o.Now = func() float64 { return float64(time.Now().UnixNano()) / 1e9 }
	}
	var out GroupOutcome
	stored := LoadGroups(cache)
	for _, tool := range []string{"claude", "codex"} {
		tier, ok := tiers[tool]
		if !ok {
			continue
		}
		evs := GroupInput(records, tool)
		name := messages.Tool[tool]
		if !Groupable(evs) {
			out.Lines = append(out.Lines, fmtMsg(messages.Drift["group_nothing"], "tool", name))
			continue
		}
		prompt := GroupPrompt(evs)
		hash := GroupHash(tier, prompt)
		cached := false
		for _, r := range stored {
			if r.Get("input_hash").S == hash && !r.Get("error").Truthy() {
				cached = true
			}
		}
		if cached {
			out.Lines = append(out.Lines, fmtMsg(messages.Drift["group_unchanged"], "tool", name))
			continue
		}
		sessions := map[string]bool{}
		for _, e := range evs {
			sessions[e.Session] = true
		}
		est := GroupEstimateUSD(tier, len(sessions))
		if o.Budget > 0 && spent+out.Spent+est > o.Budget {
			out.Lines = append(out.Lines, fmtMsg(messages.Drift["group_budget"], "tool", name, "b", o.Budget))
			continue
		}
		text, u, sec, err := Call(ctx, tier, GroupSystem, prompt, o.Timeout)
		var lim *ErrLimit
		var cfg *ErrConfig
		switch {
		case errors.Is(err, context.Canceled):
			out.Stop = "cancelled"
			return out
		case errors.As(err, &lim):
			out.Stop = "usage limit: " + py.Head(lim.Msg, 120)
			out.Lines = append(out.Lines, fmtMsg(messages.Drift["group_limit"], "tool", name))
			return out
		case func() bool { _, ok := IsErrModel(err); return ok }():
			// the grouping model was refused: skip grouping for this tool (no switch); judged results stay
			out.Lines = append(out.Lines, fmtMsg(messages.Drift["group_refused"], "tool", name, "model", ModelName(tier)))
			continue
		case errors.As(err, &cfg):
			out.Stop = py.Head(cfg.Msg, 300)
			out.Lines = append(out.Lines, fmtMsg(messages.Drift["group_not_run"], "tool", name, "msg", out.Stop))
			return out
		}
		rec := &py.Object{}
		rec.Set("tool", py.String(tool))
		rec.Set("tier", py.String(tier))
		rec.Set("group_version", py.String(GroupVersion))
		rec.Set("input_hash", py.String(hash))
		rec.Set("n_events", py.Value{K: py.Int, I: int64(len(evs))})
		errText := ""
		var themes []Theme
		var dropped []Dropped
		usd := py.Value{}
		if err != nil {
			if errors.Is(err, proc.ErrTimeout) {
				errText = "timeout after " + o.Timeout.String()
			} else {
				errText = py.Head(err.Error(), 300)
			}
			out.Spent += est // a failed call may still have been billed: count its estimate
		} else {
			if x, ok := config.JudgeUSD(tier, u); ok {
				usd = py.Value{K: py.Float, F: x}
				out.Spent += x
			} else {
				out.Spent += est
			}
			var okParse bool
			themes, dropped, okParse = CheckThemes(text, evs)
			if !okParse {
				errText = "PARSE_ERROR"
			}
		}
		byID := map[string]GroupEvent{}
		for _, e := range evs {
			byID[e.ID] = e
		}
		var tv, dv []py.Value
		for _, t := range themes {
			tv = append(tv, ThemeValue(tool, t, byID))
		}
		for _, d := range dropped {
			do := &py.Object{}
			do.Set("label", py.String(d.Label))
			do.Set("reason", py.String(d.Reason))
			dv = append(dv, py.Value{K: py.Obj, O: do})
		}
		rec.Set("themes", arrOf(tv))
		rec.Set("dropped", arrOf(dv))
		if errText != "" {
			rec.Set("error", py.String(errText))
		} else {
			rec.Set("error", py.Value{})
		}
		uo := &py.Object{}
		uo.Set("input", numV(u.Input))
		uo.Set("cache_read", numV(u.CacheRead))
		uo.Set("cache_write", numV(u.CacheWrite))
		uo.Set("output", numV(u.Output))
		uo.Set("model", py.String(u.Model))
		rec.Set("usage", py.Value{K: py.Obj, O: uo})
		rec.Set("usd", usd)
		rec.Set("seconds", py.Value{K: py.Float, F: py.Round(sec, 1)})
		if v, ok := o.CLIVersions[tool]; ok {
			rec.Set("judge_cli", py.String(v))
		} else {
			rec.Set("judge_cli", py.Value{})
		}
		rec.Set("ts", py.Value{K: py.Float, F: o.Now()})
		appendGroup(cache, py.Value{K: py.Obj, O: rec})
		switch {
		case errText != "":
			out.Lines = append(out.Lines, fmtMsg(messages.Drift["group_failed"], "tool", name, "msg", py.Head(errText, 80)))
		default:
			if len(dropped) > 0 {
				out.Lines = append(out.Lines, fmtMsg(messages.Drift["group_done_dropped"], "tool", name, "n", len(themes), "d", len(dropped)))
			} else {
				out.Lines = append(out.Lines, fmtMsg(messages.Drift["group_done"], "tool", name, "n", len(themes)))
			}
		}
	}
	return out
}

func appendGroup(cache string, rec py.Value) {
	recMu.Lock()
	defer recMu.Unlock()
	_ = os.MkdirAll(filepath.Dir(GroupsPath(cache)), 0o700)
	f, err := os.OpenFile(GroupsPath(cache), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(py.DumpsOpt(rec, py.DumpOpts{}) + "\n")
}

// StatedIn says whether words the model returned as stating the request occur in the event's person-side text
// (the cited instruction or the person's quotes), after collapsing whitespace and ignoring case and the
// surrounding quote marks. At least 4 characters are required.
func StatedIn(words string, e GroupEvent) bool {
	w := strings.ToLower(Norm(py.String(words)))
	if utf8.RuneCountInString(w) < 4 {
		return false
	}
	for _, src := range append([]string{e.Instruction}, e.Quotes...) {
		if src != "" && strings.Contains(strings.ToLower(Norm(py.String(src))), w) {
			return true
		}
	}
	return false
}
