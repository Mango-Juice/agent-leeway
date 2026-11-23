package cli

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/assets"
	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/drift"
	"github.com/Mango-Juice/agent-leeway/internal/leak"
	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"github.com/Mango-Juice/agent-leeway/internal/py"
	"github.com/Mango-Juice/agent-leeway/internal/report"
)

func init() {
	register(command{name: "drift", help: "in-depth check: judge sessions with the rubric through each provider's official CLI (asks first)", run: func(a []string, o, e io.Writer) int { return cmdDrift(a, o, e, false) }})
	register(command{name: "report", help: "combined leak + drift report (terminal summary + Markdown/HTML files)", run: cmdReport})
	register(command{name: "rubric", help: "show the rubric file and settings S1..S10, or copy it for editing", run: cmdRubric})
}

type driftOpts struct {
	logOpts
	setFlags    [][2]string // the flags given on the command line (name, value; "" for a bool), for a follow-up command
	recheck     bool
	project     string
	includeAuto bool
	maxSessions int
	budget      float64
	model       string
	rubric      string
	settings    string
	noCurrent   bool
	yes         bool
	dryRun      bool
	concurrency int
	timeout     time.Duration
	writeInputs string
	debugLog    bool
	noGroup     bool
	groupOnly   bool
	grouping    string
}

func addDrift(fs *flag.FlagSet, o *driftOpts) {
	fs.StringVar(&o.project, "project", "", "only projects whose name contains this text")
	fs.BoolVar(&o.includeAuto, "include-auto", false, "also judge automation sessions (default: direct conversations only)")
	fs.IntVar(&o.maxSessions, "max-sessions", drift.DefaultMaxSessions, "judge at most this many sessions not judged yet, most recent first (0 = all sessions in the window)")
	fs.Float64Var(&o.budget, "budget", 0, "optional spending cap for this run (USD at list prices), checked before each call starts; unset = no dollar limit")
	fs.StringVar(&o.model, "model", "", "judging models, e.g. claude=opus,codex=gpt-6.2-astra (a session is judged only by its own provider)")
	fs.StringVar(&o.rubric, "rubric", "", "rubric file (default: settings.json \"rubric\", else the bundled rubric)")
	fs.StringVar(&o.settings, "settings", "", "JSON file overriding S1..S10")
	fs.BoolVar(&o.noCurrent, "no-current-instructions", false, "do not add the current instruction files to Claude sessions")
	fs.BoolVar(&o.yes, "yes", false, "skip the cost confirmation")
	fs.BoolVar(&o.dryRun, "dry-run", false, "show the estimate only; judge nothing")
	fs.IntVar(&o.concurrency, "concurrency", 3, "judge calls running at the same time")
	fs.BoolVar(&o.groupOnly, "group-only", false, "judge nothing; run only the grouping step on the sessions already judged in the window")
	fs.StringVar(&o.grouping, "grouping", "", "turn the grouping step on or off per tool, e.g. claude=off or codex=off (default: on for both; settings.json \"grouping\")")
	fs.BoolVar(&o.noGroup, "no-grouping", false, "skip the grouping step (after judging, one call per provider that groups the problems by meaning)")
	fs.BoolVar(&o.recheck, "recheck", false, "check sessions again with this model even if another model already checked them under the same rubric (by default a session is checked once, whatever model)")
	fs.BoolVar(&o.debugLog, "debug-log", false, "write a diagnostics file of the judge calls (CLI version, arguments, exit status, stderr, event types; no prompt or session text) under the cache folder; also LEEWAY_DEBUG=1")
	fs.StringVar(&o.writeInputs, "write-inputs", "", "write the exact judge input of each session (and the system prompt) to this folder, to read before agreeing")
	fs.DurationVar(&o.timeout, "timeout", drift.DefaultTimeout, "time limit per judge call (the call and everything it started are ended)")
}

func parseModels(s string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if !strings.Contains(v, ":") {
			v = k + ":" + v
		}
		out[k] = v
	}
	return out
}

func loadRubric(o *driftOpts) (*drift.Rubric, error) {
	rb, err := drift.LoadRubric(drift.RubricPath(o.rubric))
	if err != nil {
		return nil, err
	}
	rb.Settings = rb.Settings.ApplyOverrides(config.UserSettings().RubricSettings)
	if o.settings != "" {
		ov, err := drift.LoadOverrides(o.settings)
		if err != nil {
			return nil, err
		}
		rb.Settings = rb.Settings.ApplyOverrides(ov)
	}
	return rb, nil
}

func say(w io.Writer, s string) { fmt.Fprintln(w, py.ReplaceSurrogates(s, "�")) }

func verdictCounts(D py.Value) string {
	v := D.Get("verdicts")
	var parts []string
	for i, k := range v.O.Keys {
		l := k
		if x, ok := messages.Verdict[k]; ok {
			l = x
		}
		parts = append(parts, l+" "+py.PyStr(v.O.Vals[i]))
	}
	return strings.Join(parts, ", ")
}

func cmdDrift(args []string, out, errw io.Writer, quietSummary bool) int {
	fs := flag.NewFlagSet("drift", flag.ContinueOnError)
	fs.SetOutput(errw)
	var o driftOpts
	addCommon(fs, &o.logOpts)
	addDrift(fs, &o)
	if err := fs.Parse(args); err != nil {
		return helpRC(err)
	}
	o.setFlags = givenFlags(fs, nil)
	if !o.checkTool(errw) {
		return 2
	}
	return runDrift(&o, out, errw, quietSummary)
}

func runDrift(o *driftOpts, out, errw io.Writer, quietSummary bool) int {
	rc := 0
	messages.SetLang(config.Language(o.lang))
	T := messages.Drift
	now := config.Now()
	since, err := parseSince(o.since, now)
	if err != nil {
		say(errw, err.Error())
		return 2
	}
	lo := o.logOpts
	lo.tool = "" // both tools' logs are read, so the sessions left out by --tool can be counted
	tb, err := loadTables(&lo, since, errw)
	if err != nil {
		say(errw, err.Error())
		return 1
	}
	tools := []string{"claude", "codex"}
	if o.tool != "" {
		tools = []string{o.tool}
	}
	ov := parseModels(o.model)
	tiers := map[string]string{}
	userSet := map[string]bool{} // --model or settings.json "judge" chose the model: no automatic choice
	for _, t := range tools {
		tiers[t] = drift.JudgeTier(t, ov)
		userSet[t] = ov[t] != "" || config.UserSettings().Judge[t] != ""
		if !drift.Available(t) {
			say(errw, fmtT(T["no_cli"], "tool", t))
		}
	}
	checkTiers := func() int { // on the final tiers
		for _, t := range tools {
			tier := tiers[t]
			if p, _, _ := strings.Cut(tier, ":"); p != t {
				say(errw, fmtT(T["wrong_provider"], "tool", t, "tier", tier))
				return 2
			}
			if strings.Contains(strings.ToLower(tier), "fable") {
				say(errw, T["fable_refused"])
				return 2
			}
			if config.JudgeLower[tier] {
				say(errw, fmtT(T["lower_tier"], "tier", tier, "msg", messages.LowerTierWarning))
			}
		}
		return 0
	}
	groupOn, err := groupingState(o)
	if err != nil {
		say(errw, err.Error())
		return 2
	}
	rb, err := loadRubric(o)
	if err != nil {
		say(errw, err.Error())
		return 1
	}
	cache := o.cacheDir()
	// --max-sessions counts sessions not judged yet (most recent first), so a second run continues with older
	// sessions instead of finding the most recent ones already judged; the cut is made after the estimate below
	all := drift.Select(tb, since, now.Add(time.Minute), tools, o.project, o.includeAuto, 0)
	selectedTotal := len(all)
	var sel []drift.Selected
	missing := map[string]int{}
	for _, s := range all {
		if drift.Available(s.Tool) {
			sel = append(sel, s)
		} else {
			missing[s.Tool]++
		}
	}
	for _, t := range []string{"claude", "codex"} {
		if missing[t] > 0 && len(sel) > 0 {
			say(errw, fmtT(T["no_cli_skipped"], "n", missing[t], "tool", t))
		}
	}
	if len(all) == 0 {
		say(errw, T["none"])
		return 0
	}
	if len(sel) == 0 {
		say(errw, fmtT(T["no_cli_any"], "n", len(all)))
		return 1
	}
	if os.Getenv("LEEWAY_JUDGE_CMD") != "" {
		say(errw, fmtT(T["judge_cmd_set"], "cmd", os.Getenv("LEEWAY_JUDGE_CMD")))
	}
	versions := map[string]string{}
	expected := map[string]drift.Candidate{}
	for _, t := range []string{"claude", "codex"} {
		need := false
		for _, s := range sel {
			need = need || s.Tool == t
		}
		if !need {
			continue
		}
		v, err := drift.Preflight(t)
		if err != nil {
			say(errw, fmtT(T["preflight_failed"], "msg", err.Error()))
			return 1
		}
		older := drift.OlderThanTested(t, v)
		switch {
		case older && t == "codex":
			// below 0.171.0 the CLI has no catalog entry for GPT-6.2 Astra: it still asks for that model, with
			// default settings ("Model metadata ... not found", seen with 0.169.0); the validated model is kept
			say(errw, fmtT(T["cli_older_codex"], "v", drift.BareVersion(v), "tested", drift.BareVersion(drift.TestedCLIs[t])))
		case older:
			say(errw, fmtT(T["cli_older"], "tool", messages.Tool[t], "v", drift.BareVersion(v), "tested", drift.BareVersion(drift.TestedCLIs[t])))
		}
		versions[t] = v
		if !userSet[t] && os.Getenv("LEEWAY_JUDGE_CMD") == "" { // the expected judging model (judges.json)
			cands := expectedCandidates(t, v, older, cache)
			if len(cands) == 0 {
				say(errw, fmtT(T["no_model_left"], "tool", messages.Tool[t]))
				return 1
			}
			tiers[t] = cands[0].Tier
			expected[t] = cands[0]
		}
	}
	if rc := checkTiers(); rc != 0 {
		return rc
	}
	items := drift.Prepare(drift.NewRenderer(tb), sel, rb, tiers, !o.noCurrent)
	if o.budget > 0 {
		for _, t := range tiers {
			if _, ok := config.JudgeUSD(t, config.Usage{}); !ok && config.MeasuredUSDPerSession(t) == 0 {
				say(errw, fmtT(T["budget_unpriced"], "tier", t, "b", o.budget))
				return 2
			}
		}
	}
	done := drift.DoneIDs(drift.LoadResults(cache)) // checked by any model of the tool counts (no paying twice)
	if o.recheck {
		done = drift.DoneIDsExact(drift.LoadResults(cache))
	}
	lines, todo, _ := drift.EstimateLines(items, done, rb.Version, rb.Settings, o.budget)
	pendingTotal := len(todo)
	if o.maxSessions > 0 && len(todo) > o.maxSessions {
		keep := map[string]bool{}
		for _, it := range todo[:o.maxSessions] {
			keep[it.Key] = true
		}
		pending := map[string]bool{}
		for _, it := range todo {
			pending[it.Key] = true
		}
		var items2 []drift.Item
		var sel2 []drift.Selected
		for i, it := range items {
			if !pending[it.Key] || keep[it.Key] { // judged ones stay (counted as skipped); pending ones up to the limit
				items2, sel2 = append(items2, it), append(sel2, sel[i])
			}
		}
		items, sel = items2, sel2
		lines, todo, _ = drift.EstimateLines(items, done, rb.Version, rb.Settings, o.budget)
	}
	if o.groupOnly {
		if o.noGroup {
			say(errw, T["group_only_conflict"])
			return 2
		}
		lines, todo = nil, nil
	}
	scope, limit := T["scope_all"], ""
	if !o.includeAuto {
		scope = T["scope_direct"]
	}
	if o.maxSessions > 0 {
		limit = fmtT(T["limit"], "n", o.maxSessions)
	}
	nHead := len(items)
	if !o.groupOnly {
		nHead = len(todo) // the sessions this run would judge (judged ones are skipped, said below)
	}
	say(errw, fmtT(T["head"], "n", nHead, "scope", scope, "limit", limit, "ver", rb.Version))
	both := drift.Select(tb, since, now.Add(time.Minute), nil, o.project, o.includeAuto, 0)
	perTool := map[string]int{}
	for _, s := range both {
		perTool[s.Tool]++
	}
	say(errw, fmtT(T["by_tool"], "claude", perTool["claude"], "codex", perTool["codex"]))
	for _, t := range []string{"claude", "codex"} { // the model expected to judge: name and checked status only
		tier, ok := tiers[t]
		if !ok || os.Getenv("LEEWAY_JUDGE_CMD") != "" || countTool(todo, t) == 0 {
			continue
		}
		key := "judge_model"
		if countTool(todo, "claude") > 0 && countTool(todo, "codex") > 0 { // --tool all: say which tool
			key = "judge_model_tool"
		}
		say(errw, fmtT(T[key], "tool", messages.Tool[t], "name", modelLabel(t, tier, expected[t])))
	}
	leftOut := map[string]int{}
	if ot := otherTool(o.tool); ot != "" && perTool[ot] > 0 {
		leftOut[ot] = perTool[ot]
		say(errw, fmtT(T["left_out"], "n", perTool[ot], "tool", messages.Tool[ot]))
	}
	for _, l := range drift.PlanLines(selectedTotal, pendingTotal, o.maxSessions, todo, o.budget) {
		say(errw, l)
	}
	listSessions(errw, sel, items, todo)
	say(errw, T["includes_instructions"])
	for _, l := range lines {
		say(errw, l)
	}
	if len(todo) > 0 {
		say(errw, T["est_upper"])
	}
	if len(todo) > 20 {
		say(errw, T["start_smaller"])
	}
	if note := stalePricesNote(now); note != "" {
		say(errw, note)
	}
	// the grouping step: one call per provider after judging, under the same agreement
	windowRecs := func() []py.Value {
		keys := map[string]bool{}
		for _, s := range sel {
			keys[s.Key] = true
		}
		var recs []py.Value
		for _, r := range drift.LoadResults(cache) {
			if keys[r.Get("key").S] {
				recs = append(recs, r)
			}
		}
		return recs
	}
	needGroup := false
	gTiers := map[string]string{} // the judged tools whose grouping is on
	for t, tier := range tiers {
		if groupOn[t] {
			gTiers[t] = tier
		}
	}
	if len(tiers) > 0 {
		var st []string
		for _, t := range []string{"claude", "codex"} {
			if _, ok := tiers[t]; ok {
				st = append(st, messages.Tool[t]+" "+map[bool]string{true: T["state_on"], false: T["state_off"]}[groupOn[t]])
			}
		}
		say(errw, fmtT(T["grouping_state"], "state", strings.Join(st, ", ")))
	}
	if o.groupOnly && len(gTiers) == 0 {
		say(errw, T["grouping_none_on"])
		return 0
	}
	if len(gTiers) > 0 {
		perTool := map[string]int{}
		for _, s := range sel {
			perTool[s.Tool]++
		}
		est, names := 0.0, []string{}
		for _, t := range []string{"claude", "codex"} {
			if perTool[t] > 0 && groupOn[t] {
				est += drift.GroupEstimateUSD(tiers[t], perTool[t])
				names = append(names, messages.Tool[t])
			}
		}
		if len(names) > 0 {
			say(errw, fmtT(T["grouping_plan"], "n", len(names), "tools", strings.Join(names, ", "), "usd", est))
		}
		if len(todo) == 0 {
			stored := drift.LoadGroups(cache)
			recs := windowRecs()
			for _, t := range []string{"claude", "codex"} {
				evs := drift.GroupInput(recs, t)
				if perTool[t] == 0 || !groupOn[t] || !drift.Groupable(evs) {
					continue
				}
				h := drift.GroupHash(tiers[t], drift.GroupPrompt(evs))
				hit := false
				for _, r := range stored {
					hit = hit || (r.Get("input_hash").S == h && !r.Get("error").Truthy())
				}
				needGroup = needGroup || !hit
			}
		}
	}
	say(errw, messages.Privacy)
	if o.writeInputs != "" {
		n, err := drift.WriteInputs(o.writeInputs, todo, rb)
		if err != nil {
			say(errw, err.Error())
			return 1
		}
		say(errw, fmtT(T["inputs_written"], "n", n, "dir", o.writeInputs))
	}
	switch {
	case len(todo) == 0 && !needGroup:
		say(errw, T["nothing"])
	case o.dryRun:
		say(errw, T["dry_run"])
		say(errw, T["scope_change"])
		return 0
	default:
		if !o.yes {
			if !isTerminal(os.Stdin) {
				say(errw, T["need_yes"])
				return 2
			}
			fmt.Fprint(errw, T["ask"])
			ans, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil && strings.TrimSpace(ans) == "" {
				say(errw, T["need_yes"])
				return 2
			}
			if a := strings.ToLower(strings.TrimSpace(ans)); a != "y" && a != "yes" {
				say(errw, T["cancelled"])
				return 0
			}
		}
		ctx, stop := notifyContext()
		var res drift.Outcome
		if o.debugLog || os.Getenv("LEEWAY_DEBUG") == "1" {
			if p, err := drift.EnableDebug(filepath.Join(cache, "debug")); err == nil {
				say(errw, fmtT(T["debug_on"], "path", p))
			}
		}
		runStart := float64(time.Now().UnixNano()) / 1e9 // results written from here on are this run's
		if len(todo) > 0 {
			res = drift.JudgeAll(ctx, cache, todo, rb, drift.Options{Concurrency: o.concurrency, Timeout: o.timeout, Budget: o.budget, CLIVersions: versions,
				Log: func(s string) { say(errw, s) }})
			reason := ""
			if res.StoppedReason != "" {
				reason = fmtT(T["stopped"], "reason", res.StoppedReason)
			}
			say(errw, fmtT(T["judged"], "n", res.Done, "usd", res.Spent, "reason", reason))
			if res.Done > 0 && !config.Legacy { // a numbers-only record of this check, for later comparisons
				judgedTiers := map[string]string{}
				for _, it := range todo {
					if t, ok := tiers[it.Tool]; ok {
						judgedTiers[it.Tool] = t
					}
				}
				saveCheckRecords(o, tb, since, now, judgedTiers, rb, runStart)
			}
			if res.StoppedReason != "" {
				say(errw, T["resume_hint"])
			}
			if res.Failed {
				rc = 1
			}
			if res.Refused != "" { // the model was refused: remember it, name the next one, ask before using it
				t, _, _ := strings.Cut(res.Refused, ":")
				drift.MarkUnavailable(cache, t, versions[t], res.Refused)
				cands := expectedCandidates(t, versions[t], drift.OlderThanTested(t, versions[t]), cache)
				stop()
				if len(cands) == 0 {
					say(errw, fmtT(T["no_model_left"], "tool", messages.Tool[t]))
					return 4
				}
				_, nm, _ := strings.Cut(cands[0].Tier, ":")
				model := joinModels(o.model, t+"="+nm)
				say(errw, fmtT(T["model_next"], "name", modelLabel(t, cands[0].Tier, cands[0]), "done", res.Done, "cmd", followUp(o, model)))
				if !o.yes && isTerminal(os.Stdin) { // the dry run of the next model, then the usual approval question
					o2 := *o
					o2.model = model
					return runDrift(&o2, out, errw, quietSummary)
				}
				return 4
			}
			if res.Failed || res.StoppedReason != "" {
				if p := drift.DebugPath(); p != "" {
					say(errw, fmtT(T["debug_at"], "path", p))
				} else {
					say(errw, T["debug_hint"])
				}
			}
		} else {
			say(errw, T["nothing"])
		}
		// grouping runs unless it is switched off or judging stopped for a limit, a setup problem or a cancel
		if len(gTiers) > 0 && !res.Failed && ctx.Err() == nil {
			g := drift.GroupAll(ctx, cache, windowRecs(), gTiers, drift.Options{Timeout: o.timeout, Budget: o.budget, CLIVersions: versions}, res.Spent)
			for _, l := range g.Lines {
				say(errw, l)
			}
		}
		stop()
	}
	if !quietSummary {
		keys := map[string]bool{}
		for _, s := range sel {
			keys[s.Key] = true
		}
		var recs []py.Value
		for _, r := range drift.LoadResults(cache) {
			if keys[r.Get("key").S] {
				recs = append(recs, r)
			}
		}
		D := drift.Summarize(recs, 60)
		if D.Get("n_sessions").Truthy() {
			say(out, fmtT(T["result"], "n", D.Get("n_sessions"), "verdicts", verdictCounts(D), "defects", D.Get("n_defects"),
				"models", strings.Join(D.Get("models").O.Keys, ", "), "extra", summaryExtra(D)))
			if !config.Legacy {
				m := newModel(nil, recs, config.Language(o.lang), now, since, 0, shownGroups(drift.LoadGroups(cache), groupOn), leftOut, errw)
				say(out, "")
				fmt.Fprint(out, py.ReplaceSurrogates(report.Terminal(m, "", ""), "�"))
			} else {
				say(out, T["see_report"])
			}
		} else {
			say(out, T["none_judged"])
		}
	}
	return rc
}

// listSessions shows what the run would send: date, tool, project and approximate size of each session.
func listSessions(w io.Writer, sel []drift.Selected, items []drift.Item, todo []drift.Item) {
	pending := map[string]bool{}
	for _, it := range todo {
		pending[it.Key] = true
	}
	n := 0
	for i, it := range items {
		if !pending[it.Key] {
			continue
		}
		if n == 40 {
			say(w, fmtT(messages.Drift["list_more"], "n", len(todo)-40))
			break
		}
		n++
		chars := py.Len(it.Body)
		say(w, fmtT(messages.Drift["list_line"], "date", time.Unix(0, sel[i].Last).In(time.Local).Format("2006-01-02"), "tool", messages.Tool[it.Tool],
			"project", it.Project, "ktok", float64(chars)/drift.CharsPerTokenEst/1000, "kb", float64(len(it.Body))/1024))
	}
	// every session to judge counted by project (the list above stops at 40), so an agent can summarise all of them
	by := map[string]int{}
	for _, it := range todo {
		by[it.Project]++
	}
	if len(by) == 0 {
		return
	}
	names := make([]string, 0, len(by))
	for p := range by {
		names = append(names, p)
	}
	sort.Slice(names, func(a, b int) bool {
		if by[names[a]] != by[names[b]] {
			return by[names[a]] > by[names[b]]
		}
		return names[a] < names[b]
	})
	var parts []string
	rest, restN := 0, 0
	for i, p := range names {
		if i >= 8 && len(names) > 9 {
			rest, restN = rest+by[p], restN+1
			continue
		}
		parts = append(parts, fmtT(messages.Drift["by_project_one"], "project", p, "n", by[p]))
	}
	if restN > 0 {
		parts = append(parts, fmtT(messages.Drift["by_project_rest"], "k", restN, "n", rest))
	}
	say(w, fmtT(messages.Drift["by_project"], "n", len(todo), "list", strings.Join(parts, " · ")))
}

func fmtT(tmpl string, kv ...any) string { return leak.F(tmpl, kv...) }

func cmdReport(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(errw)
	var o driftOpts
	addCommon(fs, &o.logOpts)
	addDrift(fs, &o)
	runDriftFirst := fs.Bool("run-drift", false, "run the in-depth check first (default: use results already checked)")
	dir := fs.String("out", "", "report folder (default: "+config.ReportDir()+")")
	quoteChars := fs.Int("quote-chars", 160, "maximum length of evidence excerpts")
	asJSON := fs.Bool("json", false, "print the report paths and, per tool, the sessions in the window, judged and not judged, as JSON instead of the summary")
	if err := fs.Parse(args); err != nil {
		return helpRC(err)
	}
	o.setFlags = givenFlags(fs, map[string]bool{"run-drift": true, "out": true, "quote-chars": true, "json": true})
	if !o.checkTool(errw) {
		return 2
	}
	messages.SetLang(config.Language(o.lang))
	now := config.Now()
	since, err := parseSince(o.since, now)
	if err != nil {
		say(errw, err.Error())
		return 2
	}
	if *runDriftFirst {
		if rc := runDrift(&o, out, errw, true); rc != 0 {
			return rc
		}
	}
	// the leak side always reads both tools; --tool limits the drift side (which sessions are judged and shown)
	lo := o.logOpts
	lo.tool = ""
	x, tb, err := leakReports(&lo, since, now, true, errw)
	if err != nil {
		say(errw, err.Error())
		return 1
	}
	var tools []string
	if o.tool != "" {
		tools = []string{o.tool}
	}
	sel := drift.Select(tb, since, now.Add(time.Minute), tools, o.project, o.includeAuto, 0)
	keys := map[string]bool{}
	for _, s := range sel {
		keys[s.Key] = true
	}
	all := drift.LoadResults(o.cacheDir()) // read once: the window, the per-tool counts and the comparison use it
	var recs []py.Value
	for _, r := range all {
		if keys[r.Get("key").S] {
			recs = append(recs, r)
		}
	}
	// Per tool: sessions in the window, and how many have a judged result. A report asked for one tool shows
	// only that tool's drift results and says how many of the other tool's are kept out; the summary ends with
	// the asked-for tools' sessions that were never judged (an agent offers the drift check from it).
	inWin, judged := map[string]int{}, map[string]int{}
	for _, t := range []string{"claude", "codex"} {
		ks := map[string]bool{}
		for _, s := range drift.Select(tb, since, now.Add(time.Minute), []string{t}, o.project, o.includeAuto, 0) {
			ks[s.Key] = true
		}
		inWin[t] = len(ks)
		var rs []py.Value
		for _, r := range all {
			if ks[r.Get("key").S] {
				rs = append(rs, r)
			}
		}
		recsT, _ := report.Records(rs, 0)
		judged[t] = len(recsT)
	}
	unjudged, otherJudged := map[string]int{}, map[string]int{}
	for _, t := range []string{"claude", "codex"} {
		if o.tool == "" || t == o.tool {
			if n := inWin[t] - judged[t]; n > 0 {
				unjudged[t] = n
			}
		} else if judged[t] > 0 {
			otherJudged[t] = judged[t]
		}
	}
	groupOn, err := groupingState(&o)
	if err != nil {
		say(errw, err.Error())
		return 2
	}
	m := newModel(x.R, recs, config.Language(o.lang), now, since, *quoteChars, shownGroups(drift.LoadGroups(o.cacheDir()), groupOn), nil, errw)
	m.Tool, m.Unjudged, m.OtherJudged = o.tool, unjudged, otherJudged
	m.Setup = setupItems(&o, tb, since, now)
	m.Compare = compareWithPrevious(&o, tb, x, m, since, now, tools, inWin, keys, all)
	md, html, err := report.Build(m)
	if err != nil {
		say(errw, err.Error())
		return 1
	}
	pm, ph, err := report.Write(md, html, *dir, now)
	if err != nil {
		say(errw, err.Error())
		return 1
	}
	if *asJSON {
		st := report.NewStory(m)
		return reportJSON(out, o.tool, pm, ph, m.From, m.To, inWin, judged, st.NotJudgedLine(), st.PartsLine(), st.AskLine())
	}
	fmt.Fprint(out, py.ReplaceSurrogates(report.Terminal(m, pm, ph), "�"))
	return 0
}

// reportJSON is `report --json`: the report files and, per asked-for tool, the sessions in the window, judged and
// not judged, so an agent can tell "nothing judged for this tool" from "judged" without reading the summary.
func reportJSON(out io.Writer, tool, md, html, from, to string, inWin, judged map[string]int, next, parts, ask string) int {
	type counts struct {
		Sessions int `json:"sessions"`
		Judged   int `json:"judged"`
		Unjudged int `json:"unjudged"`
	}
	tools := map[string]counts{}
	for _, t := range []string{"claude", "codex"} {
		if tool == "" || t == tool {
			tools[t] = counts{inWin[t], judged[t], inWin[t] - judged[t]}
		}
	}
	// the check-up's two parts: "cost" (money lost to habits) is always done; "asked" (did the agent do what you
	// asked) is done, partial or not_started for the asked-for tools
	j, u := 0, 0
	for _, c := range tools {
		j, u = j+c.Judged, u+c.Unjudged
	}
	asked := "done"
	switch {
	case u > 0 && j == 0:
		asked = "not_started"
	case u > 0:
		asked = "partial"
	}
	v := struct {
		Markdown  string            `json:"markdown"`
		HTML      string            `json:"html"`
		From      string            `json:"from"`
		To        string            `json:"to"`
		Parts     map[string]string `json:"parts"`
		PartsLine string            `json:"parts_line,omitempty"`
		Tools     map[string]counts `json:"tools"`
		NotJudged string            `json:"not_judged,omitempty"`
		First     string            `json:"message_first,omitempty"` // line ①: relay word for word
		Last      string            `json:"message_last,omitempty"`  // line ⑤: the last line of the message
	}{md, html, from, to, map[string]string{"cost": "done", "asked": asked}, parts, tools, next, parts, ask}
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Fprintln(out, string(b))
	return 0
}

func cmdRubric(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("rubric", flag.ContinueOnError)
	fs.SetOutput(errw)
	path := fs.String("rubric", "", "rubric file (default: the bundled rubric)")
	copyTo := fs.String("copy-to", "", "copy the rubric to this file for editing")
	force := fs.Bool("force", false, "with --copy-to: replace an existing file (it is backed up first)")
	if err := fs.Parse(args); err != nil {
		return helpRC(err)
	}
	p := drift.RubricPath(*path)
	if *copyTo != "" {
		b := assets.Rubric
		if p != "" {
			var err error
			if b, err = os.ReadFile(p); err != nil {
				say(errw, err.Error())
				return 1
			}
		}
		if _, err := os.Lstat(*copyTo); err == nil {
			if !*force {
				say(errw, fmtT(messages.Cli["rubric_exists"], "path", *copyTo))
				return 1
			}
			bak := *copyTo + ".bak-" + time.Now().Format("20060102-150405")
			if err := os.Rename(*copyTo, bak); err != nil {
				say(errw, err.Error())
				return 1
			}
			say(out, fmtT(messages.Cli["rubric_backed_up"], "path", bak))
		}
		f, err := os.OpenFile(*copyTo, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			_, err = f.Write(b)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
		}
		if err != nil {
			say(errw, err.Error())
			return 1
		}
		say(out, fmtT(messages.Cli["rubric_copied"], "path", *copyTo))
		return 0
	}
	rb, err := drift.LoadRubric(p)
	if err != nil {
		say(errw, err.Error())
		return 1
	}
	name := p
	if name == "" {
		name = "(bundled)"
	}
	say(out, fmtT(messages.Cli["rubric_file"], "path", name, "ver", rb.Version))
	v := rb.Settings.Value()
	for i, k := range v.O.Keys {
		say(out, "  "+k+": "+v.O.Vals[i].Get("name").S+" = "+v.O.Vals[i].Get("value").S)
	}
	return 0
}

// summaryExtra adds the short notes to the summary line: partly judged sessions and a custom rubric.
func summaryExtra(D py.Value) string {
	x := ""
	if n, _ := D.Get("partial_sessions").Num(); n > 0 {
		x += fmtT(messages.Drift["result_partial"], "n", int(n))
	}
	if n, _ := D.Get("custom_rubric_sessions").Num(); n > 0 {
		x += messages.Drift["result_custom"]
	}
	return x
}

// groupingState is the grouping step's state per tool for this run (defaults, settings.json, --grouping;
// --no-grouping turns every tool off).
func groupingState(o *driftOpts) (map[string]bool, error) {
	on, err := config.Grouping(o.grouping)
	if err != nil {
		return nil, err
	}
	if o.noGroup {
		for t := range on {
			on[t] = false
		}
	}
	return on, nil
}

// shownGroups keeps the stored groupings of the tools whose grouping is on: themes from a tool whose grouping is
// off are never shown, even if they were stored earlier.
func shownGroups(recs []py.Value, on map[string]bool) []py.Value {
	var out []py.Value
	for _, r := range recs {
		if on[r.Get("tool").S] {
			out = append(out, r)
		}
	}
	return out
}

// stalePricesNote is config.StalePricesNote in the interface language.
func stalePricesNote(now time.Time) string {
	t := config.Prices()
	if days, stale := config.Staleness(t.Read, now); stale {
		return fmtT(messages.Drift["stale_prices"], "days", days, "read", t.Read)
	}
	if id, end := t.EndedPromotion(now); id != "" {
		return fmtT(messages.Drift["ended_promo"], "model", id, "date", end)
	}
	return ""
}

// countTool counts the items of one tool.
func countTool(items []drift.Item, tool string) int {
	n := 0
	for _, it := range items {
		if it.Tool == tool {
			n++
		}
	}
	return n
}

// expectedCandidates are the judging models to try for a tool, best first (judges.json order), from what can
// be known without a model call: the Codex catalog (`codex debug models`, not read for a CLI older than the
// tested one, whose bundled list lacks newer models) and the models this CLI version refused before.
func expectedCandidates(tool, version string, older bool, cache string) []drift.Candidate {
	var cat []string
	def := ""
	if tool == "codex" {
		cat, def = drift.CodexCatalog()
		if older { // an older CLI's list lacks newer models: only its default is used
			cat = nil
		}
	}
	return drift.Candidates(tool, cat, def, drift.Unavailable(cache, tool, version))
}

// modelLabel is the readable name of the model expected to judge ("GPT-6.2 Astra", "the Codex default model").
// The checked status is not shown here (only in --json and in the report's "checked by" line).
func modelLabel(tool, tier string, c drift.Candidate) string {
	if strings.HasSuffix(tier, ":default") {
		return fmtT(messages.Drift["model_cli_default"], "tool", messages.Tool[tool])
	}
	return drift.ModelName(tier)
}

// joinModels adds or replaces one tool=model entry of --model.
func joinModels(cur, add string) string {
	t, _, _ := strings.Cut(add, "=")
	var keep []string
	for _, p := range strings.Split(cur, ",") {
		if k, _, _ := strings.Cut(p, "="); p != "" && k != t {
			keep = append(keep, p)
		}
	}
	return strings.Join(append(keep, add), ",")
}

// givenFlags lists the flags set on a command line, leaving out those in skip.
func givenFlags(fs *flag.FlagSet, skip map[string]bool) [][2]string {
	var out [][2]string
	fs.Visit(func(f *flag.Flag) {
		if skip[f.Name] {
			return
		}
		v := f.Value.String()
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			if v != "true" {
				return
			}
			v = ""
		}
		out = append(out, [2]string{f.Name, v})
	})
	return out
}

// followUp is the dry run to repeat with another model: every option the user gave, --model merged, without
// --yes (the user agrees after seeing it).
func followUp(o *driftOpts, model string) string {
	a := []string{"agent-leeway", "drift", "--dry-run"}
	for _, f := range o.setFlags {
		switch f[0] {
		case "yes", "dry-run", "model":
			continue
		}
		a = append(a, "--"+f[0])
		if f[1] != "" {
			v := f[1]
			if strings.ContainsAny(v, " \t\"'") {
				v = "'" + strings.ReplaceAll(v, "'", "'\\''") + "'"
			}
			a = append(a, v)
		}
	}
	return strings.Join(append(a, "--model", model), " ")
}
