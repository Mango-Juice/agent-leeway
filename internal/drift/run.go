package drift

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"github.com/Mango-Juice/agent-leeway/internal/proc"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// ResultsPath is the local results file: one JSON record per judged session, appended; reruns skip
// sessions already judged with the same tier, rubric file and settings.
func ResultsPath(cache string) string { return filepath.Join(cache, "drift", "results.jsonl") }

// LoadResults reads every record (unreadable lines are skipped).
func LoadResults(cache string) []py.Value {
	var out []py.Value
	b, err := os.ReadFile(ResultsPath(cache))
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(py.DecodeBytes(b), "\n") {
		if v, err := py.Loads(line); err == nil && v.K == py.Obj {
			out = append(out, v)
		}
	}
	return out
}

// RunID identifies a judgment: session, tier, rubric version and a hash of the settings
// (json.dumps(settings, sort_keys=True, ensure_ascii=False), as the reference writes it).
func RunID(key, tier, rubricVer string, st Settings) string {
	s := py.DumpsOpt(st.Value(), py.DumpOpts{SortKeys: true})
	h := sha1.Sum([]byte(py.ReplaceSurrogates(s, "?")))
	return key + "|" + tier + "|" + rubricVer + "|" + hex.EncodeToString(h[:])[:8]
}

// EstimateLines is the estimate shown before judging; todo are the items not judged yet.
func EstimateLines(items []Item, done map[string]bool, rubricVer string, st Settings, budget float64) ([]string, []Item, float64) {
	var todo []Item
	for _, it := range items {
		if !done[RunID(it.Key, it.Tier, rubricVer, st)] {
			todo = append(todo, it)
		}
	}
	type agg struct {
		n   int
		usd float64
	}
	by := map[string]*agg{}
	var tiers []string
	for _, it := range todo {
		b := by[it.Tier]
		if b == nil {
			b = &agg{}
			by[it.Tier] = b
			tiers = append(tiers, it.Tier)
		}
		b.n++
		b.usd += it.EstUSD
	}
	var lines []string
	if len(items) != len(todo) {
		lines = append(lines, fmtMsg(messages.Drift["resumed"], "n", len(items)-len(todo)))
	}
	total := 0.0
	for _, t := range tiers {
		ref := ""
		if r := config.MeasuredUSDPerSession(t); r != 0 {
			ref = fmtMsg(messages.Drift["est_ref"], "ref", r)
		}
		if _, priced := config.JudgeUSD(t, config.Usage{}); !priced && config.MeasuredUSDPerSession(t) == 0 {
			lines = append(lines, fmtMsg(messages.Drift["est_unpriced"], "tier", t, "n", by[t].n))
			continue
		}
		lines = append(lines, fmtMsg(messages.Drift["est_line"], "tier", t, "n", by[t].n, "usd", by[t].usd, "ref", ref))
		total += by[t].usd
	}
	bud := ""
	if budget > 0 {
		bud = fmtMsg(messages.Drift["est_budget"], "b", budget)
	}
	lines = append(lines, fmtMsg(messages.Drift["est_total"], "usd", total, "budget", bud))
	return lines, todo, total
}

// DefaultMaxSessions is how many of the most recent sessions a drift run judges unless --max-sessions says
// otherwise (0 = all). A first check-up stays light, and 50 sessions give enough events to see which causes
// dominate. There is no default dollar budget: --budget is an optional limit.
const DefaultMaxSessions = 50

// BudgetFit is how many of the items, in judging order (the order Select returns: most recent first, ties by
// session key), fit within the budget by their estimates. The run itself checks the budget again before each
// call with the actual cost of finished calls, so this is a forecast.
func BudgetFit(todo []Item, budget float64) int {
	if budget <= 0 {
		return len(todo)
	}
	sum := 0.0
	for i, it := range todo {
		sum += it.EstUSD
		if sum > budget+1e-9 {
			return i
		}
	}
	return len(todo)
}

// PlanLines is the pre-run summary, sessions first: how many were selected, how many will be judged (one call
// each) and what that means on a subscription and with an API key, then one line per limit that cuts the run.
// total is the number of sessions selected before --max-sessions; maxSessions 0 means no such limit.
func PlanLines(total, pending, maxSessions int, todo []Item, budget float64) []string {
	m := BudgetFit(todo, budget)
	usd := 0.0
	for _, it := range todo[:m] {
		usd += it.EstUSD
	}
	lines := []string{fmtMsg(messages.Drift["plan_line"], "n", total, "m", m, "usd", usd)}
	if maxSessions > 0 && pending > maxSessions {
		lines = append(lines, fmtMsg(messages.Drift["cut_max"], "k", maxSessions, "total", pending))
	}
	if m < len(todo) {
		lines = append(lines, fmtMsg(messages.Drift["cut_budget"], "b", budget, "m", m, "todo", len(todo)))
	}
	return lines
}

func fmtMsg(tmpl string, kv ...any) string {
	m := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return py.Format(tmpl, m)
}

// Options of a judging run.
type Options struct {
	Concurrency int           // calls in flight at once (default 3)
	Timeout     time.Duration // per call (default 30 min)
	Budget      float64       // 0 = no cap
	Log         func(string)
	// Now stamps records (Unix seconds; default the clock).
	Now func() float64
	// CLIVersions is the preflight-checked version of each judge CLI, recorded with every judgment.
	CLIVersions map[string]string
}

// Outcome of a run.
type Outcome struct {
	Done          int
	Spent         float64 // list price of finished calls; a failed call without usage counts at its estimate
	StoppedReason string
	Failed        bool // stopped by failures, a usage limit, a configuration error or a cancel
}

// JudgeAll judges items, Concurrency at a time, appending one record per finished call.
//
// Stopping rules (also in docs/17):
//   - Budget: before a call starts, the actual cost of finished calls plus the estimates of calls in flight
//     plus this call's estimate must stay within the budget; otherwise no new call starts.
//   - Limit: when a provider answers with a usage-limit or rate-limit message, no new call starts; calls in
//     flight finish and are recorded, then the run ends.
//   - Failures: when two sessions next to each other in the judging order have both failed, no new call starts;
//     calls in flight finish and are recorded. (With one call at a time this is "two failures in a row", as in
//     the reference; with several in flight it does not depend on which call happens to finish first.)
//   - Cancel (ctx): every child process tree is ended; calls in flight are not recorded (a rerun judges them).
//   - Timeout: a call that runs longer than Timeout is ended (with its process tree) and recorded as failed.
func JudgeAll(ctx context.Context, cache string, todo []Item, rb *Rubric, o Options) Outcome {
	if o.Concurrency <= 0 {
		o.Concurrency = 3
	}
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.Log == nil {
		o.Log = func(string) {}
	}
	if o.Now == nil {
		o.Now = func() float64 { return float64(time.Now().UnixNano()) / 1e9 }
	}
	system := BuildSystem(rb.Body, rb.Settings)
	_ = os.MkdirAll(filepath.Dir(ResultsPath(cache)), 0o700)
	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	var out Outcome
	inFlight, reserved := 0, 0.0
	state := make([]int8, len(todo)) // 0 pending, 1 ok, 2 failed
	stop := ""
	budgetStop := false
	for idx, it := range todo {
		mu.Lock()
		for inFlight >= o.Concurrency && stop == "" && ctx.Err() == nil {
			cond.Wait()
		}
		if stop == "" && ctx.Err() != nil {
			stop = messages.Drift["stop_cancel"]
		}
		if stop == "" && o.Budget > 0 && out.Spent+reserved+it.EstUSD > o.Budget {
			stop = fmtMsg(messages.Drift["stop_budget"], "b", o.Budget)
			budgetStop = true
		}
		if stop != "" {
			mu.Unlock()
			break
		}
		inFlight++
		reserved += it.EstUSD
		mu.Unlock()
		go func(idx int, it Item) {
			rec, err := judgeOne(ctx, it, system, rb, o)
			mu.Lock()
			defer func() {
				inFlight--
				reserved -= it.EstUSD
				cond.Broadcast()
				mu.Unlock()
			}()
			if errors.Is(err, context.Canceled) {
				return
			}
			var lim *ErrLimit
			if errors.As(err, &lim) {
				if stop == "" {
					stop = fmtMsg(messages.Drift["stop_limit"], "msg", py.Head(lim.Msg, 120))
				}
				return
			}
			var cfg *ErrConfig
			if errors.As(err, &cfg) {
				if stop == "" {
					stop = fmtMsg(messages.Drift["stop_config"], "msg", py.Head(cfg.Msg, 300))
				}
				return
			}
			appendRecord(cache, rec)
			if u, ok := rec.Get("usd").Num(); ok {
				out.Spent += u
			} else {
				out.Spent += it.EstUSD // a failed or timed-out call may still have been billed: count its estimate
			}
			if e := rec.Get("error"); e.Truthy() {
				state[idx] = 2
				o.Log(fmtMsg(messages.Drift["failed"], "tier", it.Tier, "err", py.Head(e.S, 80)))
				if ((idx > 0 && state[idx-1] == 2) || (idx+1 < len(state) && state[idx+1] == 2)) && stop == "" {
					stop = messages.Drift["stop_fail"]
				}
				return
			}
			state[idx] = 1
			out.Done++
			usd, _ := rec.Get("usd").Num()
			v := rec.Get("derived").S
			if l, ok := messages.Verdict[v]; ok {
				v = l
			}
			o.Log(fmtMsg(messages.Drift["progress"], "i", out.Done, "n", len(todo), "tool", it.Tool, "project", it.Project, "verdict", v, "usd", usd))
		}(idx, it)
	}
	mu.Lock()
	for inFlight > 0 {
		cond.Wait()
	}
	if ctx.Err() != nil && stop == "" {
		stop = messages.Drift["stop_cancel"]
	}
	out.StoppedReason = stop
	out.Failed = stop != "" && !budgetStop
	mu.Unlock()
	return out
}

var recMu sync.Mutex

func appendRecord(cache string, rec py.Value) {
	recMu.Lock()
	defer recMu.Unlock()
	f, err := os.OpenFile(ResultsPath(cache), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(py.DumpsOpt(rec, py.DumpOpts{}) + "\n")
}

func numV(x float64) py.Value {
	if x == math.Trunc(x) && math.Abs(x) < 1e15 {
		return py.Value{K: py.Int, I: int64(x)}
	}
	return py.Value{K: py.Float, F: x}
}

// judgeOne calls the judge and builds the record (a limit or cancel error returns no record).
func judgeOne(ctx context.Context, it Item, system string, rb *Rubric, o Options) (py.Value, error) {
	text, u, sec, err := Call(ctx, it.Tier, system, it.Body, o.Timeout)
	var lim *ErrLimit
	var cfg *ErrConfig
	if errors.As(err, &lim) || errors.As(err, &cfg) || errors.Is(err, context.Canceled) {
		return py.Value{}, err
	}
	var out py.Value
	errText := ""
	haveUsage := false
	if err != nil {
		if errors.Is(err, proc.ErrTimeout) {
			errText = "timeout after " + o.Timeout.String()
		} else {
			errText = py.Head(err.Error(), 300)
		}
		text, sec = "", 0
	} else {
		haveUsage = true
		if v, ok := Parse(text); ok {
			out = v // an empty object is kept as the (empty) output but counts as a parse error, as in the reference
		}
		if !out.Truthy() {
			errText = "PARSE_ERROR"
		}
	}
	rec := &py.Object{}
	set := func(k string, v py.Value) { rec.Set(k, v) }
	str := func(s string) py.Value { return py.String(s) }
	null := py.Value{}
	set("run_id", str(RunID(it.Key, it.Tier, rb.Version, rb.Settings)))
	set("key", str(it.Key))
	set("tool", str(it.Tool))
	set("project", str(it.Project))
	set("tier", str(it.Tier))
	usage, usd, model := null, null, null
	if haveUsage {
		uo := &py.Object{}
		uo.Set("input", numV(u.Input))
		uo.Set("cache_read", numV(u.CacheRead))
		uo.Set("cache_write", numV(u.CacheWrite))
		uo.Set("output", numV(u.Output))
		uo.Set("model", str(u.Model))
		if len(u.Extra) > 0 { // other models the CLI used in the same call; priced into "usd"
			var ex []py.Value
			for _, e := range u.Extra {
				eo := &py.Object{}
				eo.Set("model", str(e.Model))
				eo.Set("input", numV(e.Input))
				eo.Set("cache_read", numV(e.CacheRead))
				eo.Set("cache_write", numV(e.CacheWrite))
				eo.Set("output", numV(e.Output))
				ex = append(ex, py.Value{K: py.Obj, O: eo})
			}
			uo.Set("other_models", py.Value{K: py.Arr, A: ex})
		}
		usage = py.Value{K: py.Obj, O: uo}
		if x, ok := config.JudgeUSD(it.Tier, u); ok {
			usd = py.Value{K: py.Float, F: x}
		}
		model = str(u.Model)
	} else {
		usage = py.Value{K: py.Obj, O: &py.Object{}}
	}
	set("model", model)
	set("rubric_version", str(rb.Version))
	set("settings", rb.Settings.Value())
	if errText != "" {
		set("error", str(errText))
	} else {
		set("error", null)
	}
	set("seconds", py.Value{K: py.Float, F: py.Round(sec, 1)})
	set("usd", usd)
	set("usage", usage)
	verify := py.Value{K: py.Obj, O: &py.Object{}}
	kept, dropped := out, 0
	derived, problems, verdict := null, null, null
	if out.Truthy() {
		verify = Verify(out, it.Locs)
		kept, dropped = DropUngrounded(out)
		derived = str(DerivedVerdict(kept, rb.Settings))
		var ps []py.Value
		for _, p := range Validate(out) {
			ps = append(ps, str(p))
		}
		if ps == nil {
			ps = []py.Value{}
		}
		problems = py.Value{K: py.Arr, A: ps}
		verdict = out.Get("verdict")
	}
	set("verify", verify)
	set("dropped_events", py.Value{K: py.Int, I: int64(dropped)})
	set("verdict", verdict)
	set("derived", derived)
	set("schema_problems", problems)
	if it.Warning != "" {
		set("instr_warning", str(it.Warning))
	} else {
		set("instr_warning", null)
	}
	set("n_user", py.Value{K: py.Int, I: int64(it.NUser)})
	set("out", kept)
	// facts about how this judgment was made (not part of the judge input)
	if it.OmittedChars > 0 {
		po := &py.Object{}
		po.Set("omitted_chars", py.Value{K: py.Int, I: int64(it.OmittedChars)})
		po.Set("total_chars", py.Value{K: py.Int, I: int64(it.TotalChars)})
		set("partial", py.Value{K: py.Obj, O: po})
	} else {
		set("partial", null)
	}
	origin := "direct"
	if it.OriginUncertain {
		origin = "uncertain"
	}
	set("origin", str(origin))
	set("rubric_source", str(rb.Source()))
	if v, ok := o.CLIVersions[strings.SplitN(it.Tier, ":", 2)[0]]; ok {
		set("judge_cli", str(v))
	} else {
		set("judge_cli", null)
	}
	set("ts", py.Value{K: py.Float, F: o.Now()})
	return py.Value{K: py.Obj, O: rec}, nil
}

// DoneIDs are run ids of successful records (resuming skips them).
func DoneIDs(records []py.Value) map[string]bool {
	m := map[string]bool{}
	for _, r := range records {
		if !r.Get("error").Truthy() {
			m[r.Get("run_id").S] = true
		}
	}
	return m
}

// WriteInputs writes, for each session, exactly what its judge would read on stdin, and the system prompt,
// into dir (created private). Nothing is sent. Returns the number of sessions written.
func WriteInputs(dir string, items []Item, rb *Rubric) (int, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return 0, err
	}
	write := func(name, text string) error {
		return os.WriteFile(filepath.Join(dir, name), []byte(py.ReplaceSurrogates(text, "\uFFFD")), 0o600)
	}
	if err := write("system-prompt.md", BuildSystem(rb.Body, rb.Settings)); err != nil {
		return 0, err
	}
	for i, it := range items {
		key := it.Key
		if _, id, ok := strings.Cut(key, ":"); ok {
			key = py.Tail(id, 12)
		}
		name := fmt.Sprintf("%03d-%s-%s.txt", i+1, it.Tool, safeName.ReplaceAllString(key, "_"))
		if err := write(name, Stdin(it.Tier, it.Body)); err != nil {
			return i, err
		}
	}
	return len(items), nil
}

var safeName = regexp.MustCompile(`[^A-Za-z0-9_.-]`)
