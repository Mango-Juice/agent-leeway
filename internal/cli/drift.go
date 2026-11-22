package cli

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
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
	register(command{name: "drift", help: "drift check: judge sessions with the rubric through each provider's official CLI (asks first)", run: func(a []string, o, e io.Writer) int { return cmdDrift(a, o, e, false) }})
	register(command{name: "report", help: "combined leak + drift report (terminal summary + Markdown/HTML files)", run: cmdReport})
	register(command{name: "rubric", help: "show the rubric file and settings S1..S10, or copy it for editing", run: cmdRubric})
}

type driftOpts struct {
	logOpts
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
	noGroup     bool
	groupOnly   bool
	grouping    string
}

func addDrift(fs *flag.FlagSet, o *driftOpts) {
	fs.StringVar(&o.project, "project", "", "only projects whose name contains this text")
	fs.BoolVar(&o.includeAuto, "include-auto", false, "also judge automation sessions (default: direct conversations only)")
	fs.IntVar(&o.maxSessions, "max-sessions", drift.DefaultMaxSessions, "judge at most this many sessions, most recent first (0 = all sessions in the window)")
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
	for _, t := range tools {
		tier := drift.JudgeTier(t, ov)
		if p, _, _ := strings.Cut(tier, ":"); p != t {
			say(errw, fmtT(T["wrong_provider"], "tool", t, "tier", tier))
			return 2
		}
		if strings.Contains(strings.ToLower(tier), "fable") {
			say(errw, T["fable_refused"])
			return 2
		}
		if !drift.Available(t) {
			say(errw, fmtT(T["no_cli"], "tool", t))
		}
		if config.JudgeLower[tier] {
			say(errw, fmtT(T["lower_tier"], "tier", tier, "msg", messages.LowerTierWarning))
		}
		tiers[t] = tier
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
	all := drift.Select(tb, since, now.Add(time.Minute), tools, o.project, o.includeAuto, 0)
	selectedTotal := len(all)
	if o.maxSessions > 0 && len(all) > o.maxSessions {
		all = all[:o.maxSessions]
	}
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
		versions[t] = v
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
	done := drift.DoneIDs(drift.LoadResults(cache))
	lines, todo, _ := drift.EstimateLines(items, done, rb.Version, rb.Settings, o.budget)
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
	say(errw, fmtT(T["head"], "n", len(items), "scope", scope, "limit", limit, "ver", rb.Version))
	both := drift.Select(tb, since, now.Add(time.Minute), nil, o.project, o.includeAuto, 0)
	perTool := map[string]int{}
	for _, s := range both {
		perTool[s.Tool]++
	}
	say(errw, fmtT(T["by_tool"], "claude", perTool["claude"], "codex", perTool["codex"]))
	leftOut := map[string]int{}
	if ot := otherTool(o.tool); ot != "" && perTool[ot] > 0 {
		leftOut[ot] = perTool[ot]
		say(errw, fmtT(T["left_out"], "n", perTool[ot], "tool", messages.Tool[ot]))
	}
	for _, l := range drift.PlanLines(selectedTotal, o.maxSessions, todo, o.budget) {
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
		if len(todo) > 0 {
			res = drift.JudgeAll(ctx, cache, todo, rb, drift.Options{Concurrency: o.concurrency, Timeout: o.timeout, Budget: o.budget, CLIVersions: versions,
				Log: func(s string) { say(errw, s) }})
			reason := ""
			if res.StoppedReason != "" {
				reason = fmtT(T["stopped"], "reason", res.StoppedReason)
			}
			say(errw, fmtT(T["judged"], "n", res.Done, "usd", res.Spent, "reason", reason))
			if res.StoppedReason != "" {
				say(errw, T["resume_hint"])
			}
			if res.Failed {
				rc = 1
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
}

func fmtT(tmpl string, kv ...any) string { return leak.F(tmpl, kv...) }

func cmdReport(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	fs.SetOutput(errw)
	var o driftOpts
	addCommon(fs, &o.logOpts)
	addDrift(fs, &o)
	runDriftFirst := fs.Bool("run-drift", false, "run the drift check first (default: use results already judged)")
	dir := fs.String("out", "", "report folder (default: "+config.ReportDir()+")")
	quoteChars := fs.Int("quote-chars", 160, "maximum length of evidence excerpts")
	if err := fs.Parse(args); err != nil {
		return helpRC(err)
	}
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
	var recs []py.Value
	for _, r := range drift.LoadResults(o.cacheDir()) {
		if keys[r.Get("key").S] {
			recs = append(recs, r)
		}
	}
	// sessions left out: the other tool's with --tool; without it, a tool none of whose sessions was judged
	leftOut := map[string]int{}
	judgedTool := map[string]bool{}
	for _, r := range recs {
		judgedTool[r.Get("tool").S] = true
	}
	for _, t := range []string{"claude", "codex"} {
		if (o.tool != "" && t == o.tool) || (o.tool == "" && judgedTool[t]) || (o.tool == "" && len(recs) == 0) {
			continue
		}
		if n := len(drift.Select(tb, since, now.Add(time.Minute), []string{t}, o.project, o.includeAuto, 0)); n > 0 {
			leftOut[t] = n
		}
	}
	groupOn, err := groupingState(&o)
	if err != nil {
		say(errw, err.Error())
		return 2
	}
	m := newModel(x.R, recs, config.Language(o.lang), now, since, *quoteChars, shownGroups(drift.LoadGroups(o.cacheDir()), groupOn), leftOut, errw)
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
	fmt.Fprint(out, py.ReplaceSurrogates(report.Terminal(m, pm, ph), "�"))
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
