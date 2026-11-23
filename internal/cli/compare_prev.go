package cli

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/drift"
	"github.com/Mango-Juice/agent-leeway/internal/leak"
	"github.com/Mango-Juice/agent-leeway/internal/pipeline"
	"github.com/Mango-Juice/agent-leeway/internal/py"
	"github.com/Mango-Juice/agent-leeway/internal/report"
)

// keepDays bounds the records: older ones are dropped when a record is written.
const keepDays = 400

// compareWithPrevious is "compared with" in the report (docs/how-it-works.md).
//   - Cost: the previous window of the same length from the logs when they cover it; otherwise the most recent
//     earlier report of the same length that saved its numbers (at most 60 days old, its window ending before this
//     one starts), labelled with its date; otherwise nothing. This report's numbers are saved, per tool, when the
//     logs cover its whole window.
//   - In-depth: this window's checked sessions (one judge, one result per session, through this report's own
//     filters) against the most recent earlier check record with the same selection, scope, window length, judging
//     model family and rubric, at most 60 days old and sharing no session.
//
// Windows under a day: nil. all is every cached result, read once by the caller.
func compareWithPrevious(o *driftOpts, tb *pipeline.Tables, x leakSet, m *report.Model, since, now time.Time, tools []string, inWin map[string]int, curKeys map[string]bool, all []py.Value) *report.Compare {
	span := now.Sub(since)
	if span < report.CmpMinHours*time.Hour {
		return nil
	}
	hours := int(math.Round(span.Hours()))
	prevSince := since.Add(-span)
	c := &report.Compare{Hours: hours, CostArgs: map[string]any{}, DriftArgs: map[string]any{}}
	until := now.Add(time.Minute)
	ev := map[string]*leak.Events{"claude": x.E.ForTool("claude"), "codex": x.E.ForTool("codex")}
	agg := func(tool string, s, u time.Time) report.Agg {
		return report.AggOf(leak.Compute(ev[tool], s, u, tool, "", time.Local))
	}
	src := snapSources(&o.logOpts)
	claudeNow, claudePrev := claudeCovers(tb, since, now), claudeCovers(tb, prevSince, now)
	cache := o.cacheDir()
	nowAggs := map[string]report.Agg{"claude": agg("claude", since, until), "codex": agg("codex", since, until)}
	saving := claudeNow && nowAggs["claude"].Total > 0 // this report's Claude Code numbers will be a later baseline
	// the baseline is chosen before this report's own numbers are saved
	c.Cost, c.CostWhy = costCompare(cache, src, claudePrev, saving, report.AggOf(x.R), report.AggOf(x.P), since, now, hours, c.CostArgs)
	saveSnapshots(cache, src, since, now, hours, nowAggs, map[string]bool{"claude": claudeNow, "codex": true})

	if o.project != "" {
		c.DriftArgs["project"] = o.project
	}
	c.Drift, c.DriftWhy = driftCompare(cache, src, tb, m.Records, all, now, hours, inWin, scopeOf(cache, o.project, o.includeAuto), c.DriftArgs)
	return c
}

// costCompare is the cost side. saving: this report's numbers are saved, so a first visit can say the next report
// compares with it.
func costCompare(cache string, src map[string]string, claudePrev, saving bool, nowAll, prevAll report.Agg, since, now time.Time, hours int, args map[string]any) (*report.CmpSide, string) {
	few := func(prev report.Agg) string {
		if nowAll.Sessions < report.CmpMinSessions && prev.Total >= report.CmpMinSpend && prev.Sessions >= report.CmpMinSessions {
			return "cmp_why_cost_few_now"
		}
		return "cmp_why_cost_few"
	}
	if claudePrev {
		if c := report.CostSide(nowAll, prevAll); c != nil {
			c.Base = report.BasePrior
			return c, ""
		}
		return nil, few(prevAll)
	}
	snaps := readSnapshots(cache, "claude", src["claude"])
	if len(snaps) == 0 {
		if saving {
			return nil, "cmp_first"
		}
		return nil, ""
	}
	var best *snapshot
	older := false // a record that would do but counted another set of causes
	newest := snaps[0]
	for i := range snaps {
		s := &snaps[i]
		if s.end().After(newest.end()) {
			newest = *s
		}
		if s.Hours != hours || s.end().After(since.Add(report.CmpSessionSlack)) || now.Sub(s.end()) > report.CmpMaxAgeDays*24*time.Hour {
			continue
		}
		if s.causes() != leak.CauseSet {
			older = true
			continue
		}
		if best == nil || s.end().After(best.end()) {
			best = s
		}
	}
	if best == nil && older {
		return nil, "cmp_why_cost_causes"
	}
	if best == nil {
		if now.Sub(newest.end()) > report.CmpMaxAgeDays*24*time.Hour {
			args["date"] = newest.end()
			return nil, "cmp_why_cost_old"
		}
		return nil, "cmp_why_cost_gone"
	}
	base := best.agg()
	for _, s := range readSnapshots(cache, "codex", src["codex"]) { // the same report's Codex numbers (none: no Codex use then)
		if s.Start == best.Start && s.Hours == best.Hours && s.causes() == best.causes() {
			base = base.Add(s.agg())
		}
	}
	c := report.CostSide(nowAll, base)
	if c == nil {
		return nil, few(base)
	}
	c.Base, c.BaseAt, c.BaseFrom, c.BaseTo = report.BaseReport, best.end(), best.start(), best.end()
	return c, ""
}

// driftCompare is the in-depth side: per tool, the current checked sessions against the baseline check. Two tools are
// combined when both compare with one check run; otherwise the one that can be compared is named (no card line) and
// one line says why the other is not.
func driftCompare(cache string, src map[string]string, tb *pipeline.Tables, cur []*report.Record, all []py.Value, now time.Time, hours int, inWin map[string]int, scope string, args map[string]any) (*report.CmpSide, string) {
	recs := loadChecks(cache)
	var tools []string
	for _, t := range []string{"claude", "codex"} {
		if hasTool(cur, t) {
			tools = append(tools, t)
		}
	}
	if len(tools) == 0 {
		return nil, ""
	}
	type res struct {
		cur, base *report.CheckAgg
		side      *report.CmpSide
		why       string
		args      map[string]any
	}
	got := map[string]res{}
	for _, t := range tools {
		recs = deriveChecks(cache, src[t], t, tb, all, recs) // records an earlier version's checks left only in the cache
		var mine []*report.Record
		for _, r := range cur {
			if r.Tool == t {
				mine = append(mine, r)
			}
		}
		cr, why, a := currentCheck(t, src[t], mine, recs, hours, inWin[t], scope)
		if cr == nil {
			got[t] = res{why: why, args: a}
			continue
		}
		b, why, a := report.PickCheck(cr, recs, now)
		r := res{cur: cr, base: b, why: why, args: a}
		if b != nil {
			if r.side = report.DriftCompare(cr, b); r.side == nil {
				r.why = "cmp_why_few"
			}
		}
		got[t] = r
	}
	// two tools whose baselines come from one check run: their totals are compared (the thresholds apply to the totals)
	if len(tools) == 2 && got["claude"].base != nil && got["codex"].base != nil &&
		absDur(got["claude"].base.CheckedAt.Sub(got["codex"].base.CheckedAt)) < 2*time.Hour {
		a, b := got["claude"], got["codex"]
		side := report.DriftCompare(report.Combine(a.cur, b.cur), report.Combine(a.base, b.base))
		if side == nil {
			return nil, "cmp_why_few"
		}
		side.Updated = a.side != nil && a.side.Updated || b.side != nil && b.side.Updated
		side.OtherModel = a.side != nil && a.side.OtherModel || b.side != nil && b.side.OtherModel
		side.Pill = side.Pill && !side.Updated && !side.OtherModel
		return side, ""
	}
	var ok []string
	for _, t := range tools {
		if got[t].side != nil {
			ok = append(ok, t)
		}
	}
	if len(ok) == 1 && len(tools) == 1 {
		return got[ok[0]].side, ""
	}
	if len(ok) > 0 { // one tool compared: no card line, named in the detail, with why the other is not
		side, other := got[ok[0]].side, ""
		for _, t := range tools {
			if t != ok[0] {
				other = t
			}
		}
		side.Tool, side.Pill, side.Other = ok[0], false, other
		o := got[other]
		side.OtherWhy, side.OtherArgs = o.why, o.args
		if o.side != nil || o.why == "" { // both comparable, but against checks made on different days
			side.OtherWhy = "cmp_why_other_day"
		}
		return side, ""
	}
	r := got[tools[0]]
	for k, v := range r.args {
		args[k] = v
	}
	return nil, r.why
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// currentCheck is this report's checked sessions of one tool as a check, taken from the check record that made them
// (the most recent record sharing a session with them): only that record's judge and rubric count, one result per
// session, through this report's own filters. nil, with a reason, when this report cannot stand for that check: no
// record ("" reason: nothing was stored), sessions checked with more than one model family, another scope or window.
func currentCheck(tool, src string, mine []*report.Record, recs []report.CheckAgg, hours, inWindow int, scope string) (*report.CheckAgg, string, map[string]any) {
	keys := map[string]bool{}
	families := map[string]bool{}
	for _, r := range mine {
		keys[r.Key] = true
		families[report.ModelFamily(r.Tier, r.Model)] = true
	}
	var made *report.CheckAgg
	for i := range recs {
		r := &recs[i]
		if r.Tool != tool || r.Source != src {
			continue
		}
		for _, k := range r.Keys {
			if keys[k] {
				if made == nil || r.CheckedAt.After(made.CheckedAt) {
					made = r
				}
				break
			}
		}
	}
	switch {
	case made == nil:
		return nil, "", nil
	case len(families) > 1:
		return nil, "cmp_why_mixed", nil
	case made.WindowHours != hours:
		return nil, "cmp_why_report_window", map[string]any{"then": made.WindowHours / 24, "now": hours / 24}
	case made.Scope != scope:
		return nil, "cmp_why_report_scope", nil
	}
	judged := oneJudge(mine, made.Tier, made.Rubric)
	n, serious, kinds := report.AggChecks(judged)
	cur := *made
	cur.Checked, cur.Serious, cur.Kinds, cur.Keys = n, serious, kinds, nil
	for _, r := range judged {
		cur.Keys = append(cur.Keys, r.Key)
	}
	cur.InWindow = max(inWindow, n)
	return &cur, "", nil
}

// scopeOf is a check's scope: "all", or "project:" and a salted hash of the --project text (the salt is random per
// install, kept in the cache, so the hash cannot be matched against guessed names); "+auto" when automations count.
func scopeOf(cache, project string, includeAuto bool) string {
	s := "all"
	if project != "" {
		sum := sha256.Sum256([]byte(installSalt(cache) + "\x00" + strings.ToUpper(project)))
		s = "project:" + hex.EncodeToString(sum[:])[:12]
	}
	if includeAuto {
		s += "+auto"
	}
	return s
}

// installSalt is a random value made once per cache folder (compare/salt).
func installSalt(cache string) string {
	p := filepath.Join(snapDir(cache), "salt")
	if b, err := os.ReadFile(p); err == nil && len(strings.TrimSpace(string(b))) >= 16 {
		return strings.TrimSpace(string(b))
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	s := hex.EncodeToString(b)
	_ = os.MkdirAll(snapDir(cache), 0o700)
	_ = os.WriteFile(p, []byte(s+"\n"), 0o600)
	return s
}

// ---- check records: cache/compare/checks.jsonl, one line per tool per in-depth check

func checksPath(cache string) string { return filepath.Join(snapDir(cache), "checks.jsonl") }

// loadChecks reads the records. A damaged or over-long line is skipped without losing the lines after it; a missing
// or unreadable file is no records. It never fails.
func loadChecks(cache string) []report.CheckAgg {
	f, err := os.Open(checksPath(cache))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []report.CheckAgg
	rd := bufio.NewReaderSize(f, 64<<10)
	for {
		line, err := readLine(rd, 4<<20)
		if line != nil {
			var r report.CheckAgg
			if json.Unmarshal(line, &r) == nil && r.Tool != "" && r.Checked >= 0 && r.Serious >= 0 && r.Serious <= r.Checked {
				out = append(out, r)
			}
		}
		if err != nil {
			return out
		}
	}
}

// readLine is the next line without its newline; nil for a line longer than limit (it is read past and skipped).
func readLine(rd *bufio.Reader, limit int) ([]byte, error) {
	var buf []byte
	long := false
	for {
		part, isPrefix, err := rd.ReadLine()
		if !long {
			buf = append(buf, part...)
			if len(buf) > limit {
				long, buf = true, nil
			}
		}
		if err != nil {
			if err == io.EOF && (len(buf) > 0 || long) {
				err = nil
				if long {
					return nil, io.EOF
				}
				return buf, io.EOF
			}
			return nil, err
		}
		if !isPrefix {
			if long {
				return nil, nil
			}
			return buf, nil
		}
	}
}

// appendChecks adds records, keeping only those of the last keepDays days (the file is rewritten when an old one is
// dropped; otherwise appended, starting a new line if the last one was left unfinished).
func appendChecks(cache string, now time.Time, recs ...report.CheckAgg) {
	if len(recs) == 0 {
		return
	}
	_ = os.MkdirAll(snapDir(cache), 0o700)
	p := checksPath(cache)
	old := loadChecks(cache)
	var keep []report.CheckAgg
	for _, r := range old {
		if now.Sub(r.CheckedAt) <= keepDays*24*time.Hour {
			keep = append(keep, r)
		}
	}
	if len(keep) < len(old) { // rewrite without the old ones
		var b strings.Builder
		for _, r := range append(keep, recs...) {
			j, _ := json.Marshal(r)
			b.Write(j)
			b.WriteByte('\n')
		}
		tmp := p + ".tmp"
		if os.WriteFile(tmp, []byte(b.String()), 0o600) == nil {
			_ = os.Rename(tmp, p)
		}
		return
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() > 0 {
		last := make([]byte, 1)
		if _, err := f.ReadAt(last, st.Size()-1); err == nil && last[0] != '\n' {
			_, _ = f.WriteAt([]byte{'\n'}, st.Size())
		}
	}
	_, _ = f.Seek(0, io.SeekEnd)
	for _, r := range recs {
		b, _ := json.Marshal(r)
		_, _ = f.Write(append(b, '\n'))
	}
}

// saveCheckRecords writes, after an in-depth check, one record per tool that this run judged at least one session
// of (a result written since runStart): the window's checked sessions in numbers (counts, kinds, ids), the window,
// the selection (the N most recent not checked yet), the scope and the judge.
func saveCheckRecords(o *driftOpts, tb *pipeline.Tables, since, now time.Time, tiers map[string]string, rb *drift.Rubric, runStart float64) {
	cache := o.cacheDir()
	src := snapSources(&o.logOpts)
	results := drift.LoadResults(cache)
	scope := scopeOf(cache, o.project, o.includeAuto)
	var out []report.CheckAgg
	for _, tool := range []string{"claude", "codex"} {
		tier, ok := tiers[tool]
		if !ok {
			continue
		}
		judgedNow := false
		for _, r := range results {
			ts, _ := r.Get("ts").Or(py.Value{K: py.Int}).Num()
			if r.Get("tool").S == tool && ts >= runStart && !r.Get("error").Truthy() && r.Get("out").Truthy() {
				judgedNow = true
				break
			}
		}
		if !judgedNow {
			continue
		}
		sel := drift.Select(tb, since, now.Add(time.Minute), []string{tool}, o.project, o.includeAuto, 0)
		last := map[string]time.Time{}
		for _, s := range sel {
			last[s.Key] = time.Unix(0, s.Last)
		}
		var raw []py.Value
		for _, r := range results {
			if _, in := last[r.Get("key").S]; in {
				raw = append(raw, r)
			}
		}
		recs, _ := report.Records(raw, 0)
		mine := oneJudge(recs, tier, rb.Version)
		if len(mine) == 0 {
			continue
		}
		n, serious, kinds := report.AggChecks(mine)
		r := report.CheckAgg{Tool: tool, CheckedAt: now.UTC(), WindowStart: since.UTC(), WindowHours: int(math.Round(now.Sub(since).Hours())),
			Checked: n, Serious: serious, Kinds: kinds, InWindow: len(sel), Tier: tier, Rubric: rb.Version, Rule: o.maxSessions, Scope: scope, Source: src[tool]}
		models := map[string]bool{}
		for _, x := range mine {
			r.Keys = append(r.Keys, x.Key)
			t := last[x.Key]
			if r.First.IsZero() || t.Before(r.First) {
				r.First = t.UTC()
			}
			if t.After(r.Last) {
				r.Last = t.UTC()
			}
			models[x.Model] = true
			if x.RequestedTier != "" { // the CLI answered with another model: that model's family
				r.Tier = x.Tier
			}
		}
		if len(models) == 1 {
			for k := range models {
				r.Model = k
			}
		}
		r.Family = report.ModelFamily(r.Tier, r.Model)
		out = append(out, r)
	}
	appendChecks(cache, now, out...)
}

// oneJudge keeps one record per session: the one judged with this tier (or answered for it) and rubric.
func oneJudge(recs []*report.Record, tier, rubric string) []*report.Record {
	by := map[string]*report.Record{}
	var keys []string
	for _, r := range recs {
		if (r.Tier != tier && r.RequestedTier != tier) || r.RubricVersion != rubric {
			continue
		}
		if _, ok := by[r.Key]; !ok {
			keys = append(keys, r.Key)
		}
		by[r.Key] = r
	}
	out := make([]*report.Record, 0, len(keys))
	for _, k := range keys {
		out = append(out, by[k])
	}
	return out
}

// deriveChecks builds check records for in-depth checks made by an earlier version (0.1.5, 0.1.6), which kept their
// results but no record. It does so only when nothing has to be guessed: a run (results of one tool judged without a
// pause of more than 6 hours) counts as a default check when the logs of its 30-day window are still here and its
// sessions are exactly the min(50, N) most recent of the N sessions of that window, none checked before it, with one
// judge and one rubric. Anything else (another --since or --max-sessions, the rest of a continued check, results
// from other log folders or a made-up clock) is left: that user simply starts fresh.
func deriveChecks(cache, src, tool string, tb *pipeline.Tables, all []py.Value, recs []report.CheckAgg) []report.CheckAgg {
	known := map[string]bool{}
	for _, r := range recs {
		if r.Tool == tool && r.Source == src {
			for _, k := range r.Keys {
				known[k] = true
			}
		}
	}
	type item struct {
		v  py.Value
		ts float64
	}
	var items []item
	for _, r := range all {
		if r.Get("tool").S != tool || r.Get("error").Truthy() || !r.Get("out").Truthy() {
			continue
		}
		ts, _ := r.Get("ts").Or(py.Value{K: py.Int}).Num()
		items = append(items, item{r, ts})
	}
	if len(items) == 0 {
		return recs
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ts < items[j].ts })
	var runs [][]item
	for i, it := range items {
		if i == 0 || it.ts-items[i-1].ts > 6*3600 {
			runs = append(runs, nil)
		}
		runs[len(runs)-1] = append(runs[len(runs)-1], it)
	}
	checkedBefore := map[string]bool{}
	var add []report.CheckAgg
	for _, run := range runs {
		var raw []py.Value
		keys, shared := map[string]bool{}, false
		for _, it := range run {
			raw = append(raw, it.v)
			k := it.v.Get("key").S
			keys[k] = true
			shared = shared || known[k]
		}
		prior := checkedBefore
		checkedBefore = map[string]bool{}
		for k := range prior {
			checkedBefore[k] = true
		}
		for k := range keys {
			checkedBefore[k] = true
		}
		if shared {
			continue
		}
		at := time.Unix(int64(run[len(run)-1].ts), 0).UTC()
		start := time.Unix(int64(run[0].ts), 0).UTC()
		sel := drift.Select(tb, start.Add(-30*24*time.Hour), start.Add(time.Minute), []string{tool}, "", false, 0) // most recent first
		if len(sel) == 0 {
			continue
		}
		want := map[string]bool{}
		touchedBefore := false
		for _, s := range sel {
			touchedBefore = touchedBefore || prior[s.Key]
			if len(want) < 50 {
				want[s.Key] = true
			}
		}
		same := len(want) == len(keys) && !touchedBefore
		for k := range keys {
			same = same && want[k]
		}
		if !same {
			continue
		}
		rs, _ := report.Records(raw, 0)
		tiers, models, rubrics := map[string]bool{}, map[string]bool{}, map[string]bool{}
		for _, r := range rs {
			tiers[r.Tier], models[r.Model], rubrics[r.RubricVersion] = true, true, true
		}
		if len(rs) == 0 || len(tiers) != 1 || len(models) != 1 || len(rubrics) != 1 {
			continue
		}
		n, serious, kinds := report.AggChecks(rs)
		r := report.CheckAgg{Tool: tool, CheckedAt: at, WindowStart: start.Add(-30 * 24 * time.Hour), WindowHours: 720, Checked: n, Serious: serious,
			Kinds: kinds, InWindow: len(sel), Tier: rs[0].Tier, Model: rs[0].Model, Rubric: rs[0].RubricVersion, Rule: 50, Scope: "all", Source: src, Derived: true}
		r.Family = report.ModelFamily(r.Tier, r.Model)
		last := map[string]time.Time{}
		for _, s := range sel {
			last[s.Key] = time.Unix(0, s.Last).UTC()
		}
		for _, x := range rs {
			r.Keys = append(r.Keys, x.Key)
			// the session dates, as a live record keeps them: each session's last turn in the window
			if t, ok := last[x.Key]; ok {
				if r.First.IsZero() || t.Before(r.First) {
					r.First = t
				}
				if t.After(r.Last) {
					r.Last = t
				}
			}
		}
		add = append(add, r)
	}
	appendChecks(cache, time.Now(), add...)
	return append(recs, add...)
}

func hasTool(rs []*report.Record, tool string) bool {
	for _, r := range rs {
		if r.Tool == tool {
			return true
		}
	}
	return false
}

// claudeCovers says whether the Claude Code logs read reach back to start: there are none (Claude Code is not
// used), or the oldest transcript is at most a day after start, or Claude Code's retention setting reaches back past
// start and has not changed since before start (a raised setting does not bring back logs deleted before).
func claudeCovers(tb *pipeline.Tables, start, now time.Time) bool {
	if tb.OldestClaude == 0 || tb.OldestClaude <= start.Add(24*time.Hour).UnixNano() {
		return true
	}
	days, changed := config.ClaudeCleanupDays()
	return !now.AddDate(0, 0, -days).After(start) && changed.Before(start)
}

// ---- report records (cost): a window's cost in numbers only, saved so a later report can compare with it when the
// logs of the previous window are gone (Claude Code deletes transcripts after 30 days by default).

type snapshot struct {
	Start    string             `json:"start"` // RFC 3339, UTC
	Hours    int                `json:"hours"`
	End      string             `json:"end,omitempty"` // the report's time (earlier versions: start + hours)
	TotalUSD float64            `json:"total_usd"`
	LeakUSD  float64            `json:"leak_usd"`
	MaxLeak  float64            `json:"max_session_leak_usd"`
	MaxShare float64            `json:"max_session_share,omitempty"`
	Sessions int                `json:"sessions"`
	Habits   map[string]float64 `json:"habits_usd"`
	Causes   int                `json:"cause_set,omitempty"` // leak.CauseSet the numbers were counted with (absent: 1)
}

// causes is the set of causes the numbers were counted with (records before 0.1.8 have none: set 1).
func (s snapshot) causes() int { return max(s.Causes, 1) }

func (s snapshot) start() time.Time { t, _ := time.Parse(time.RFC3339, s.Start); return t }

func (s snapshot) end() time.Time {
	if t, err := time.Parse(time.RFC3339, s.End); err == nil {
		return t
	}
	return s.start().Add(time.Duration(s.Hours) * time.Hour)
}

func (s snapshot) agg() report.Agg {
	return report.Agg{Total: s.TotalUSD, Leak: s.LeakUSD, MaxLeak: s.MaxLeak, Sessions: s.Sessions, Habits: s.Habits}
}

func snapDir(cache string) string { return filepath.Join(cache, "compare") }

// snapSources names, per tool, where the numbers come from: a short hash of the log folders read, so runs on other
// folders never mix with the real ones. A fixed clock (LEEWAY_NOW) adds a marker: runs at a made-up time never mix
// with real runs (but do with each other, as a simulation needs). Without LEEWAY_NOW the hash is the one 0.1.5 used.
func snapSources(o *logOpts) map[string]string {
	claude, _, codexDirs := o.dirs()
	clock := "now="
	if os.Getenv("LEEWAY_NOW") != "" {
		clock = "now=fixed-clock"
	}
	h := func(parts ...string) string {
		for i, p := range parts {
			if a, err := filepath.Abs(p); err == nil {
				parts[i] = a
			}
		}
		sum := sha256.Sum256([]byte(strings.Join(append(parts, clock), "\x00")))
		return hex.EncodeToString(sum[:])[:10]
	}
	return map[string]string{"claude": h(claude), "codex": h(codexDirs...)}
}

// snapName is a record's file name. Records of cause set 2 or later carry it in the name ("…-720h.c2.json"), so a
// 0.1.7 binary, which reads only "…h.json", never compares them with its own numbers as if equal.
func snapName(tool, src string, start time.Time, hours int) string {
	return snapNameSet(tool, src, start, hours, leak.CauseSet)
}

func snapNameSet(tool, src string, start time.Time, hours, set int) string {
	if set <= 1 {
		return fmt.Sprintf("%s-%s-%s-%dh.json", tool, src, start.UTC().Format("2006-01-02"), hours)
	}
	return fmt.Sprintf("%s-%s-%s-%dh.c%d.json", tool, src, start.UTC().Format("2006-01-02"), hours, set)
}

// saveSnapshots writes each tool's window whose logs cover it (ok) and that has any spend.
func saveSnapshots(cache string, src map[string]string, since, now time.Time, hours int, aggs map[string]report.Agg, ok map[string]bool) {
	pruneSnapshots(cache, now)
	for tool, a := range aggs {
		if !ok[tool] || a.Total <= 0 {
			continue
		}
		_ = os.MkdirAll(snapDir(cache), 0o700)
		s := snapshot{Start: since.UTC().Format(time.RFC3339), Hours: hours, End: now.UTC().Format(time.RFC3339), TotalUSD: a.Total, LeakUSD: a.Leak,
			MaxLeak: a.MaxLeak, Sessions: a.Sessions, Habits: a.Habits, Causes: leak.CauseSet}
		if a.Total > 0 {
			s.MaxShare = a.MaxLeak / a.Total
		}
		b, _ := json.MarshalIndent(s, "", " ")
		_ = os.WriteFile(filepath.Join(snapDir(cache), snapName(tool, src[tool], since, hours)), b, 0o600)
	}
}

// readSnapshots is every saved report window of a tool from these log folders whose file name agrees with its fields
// and whose numbers can be right.
func readSnapshots(cache, tool, src string) []snapshot {
	files, _ := filepath.Glob(filepath.Join(snapDir(cache), fmt.Sprintf("%s-%s-*h.json", tool, src)))
	more, _ := filepath.Glob(filepath.Join(snapDir(cache), fmt.Sprintf("%s-%s-*h.c*.json", tool, src)))
	files = append(files, more...)
	var out []snapshot
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var s snapshot
		if json.Unmarshal(b, &s) != nil || s.Hours <= 0 {
			continue
		}
		st, err := time.Parse(time.RFC3339, s.Start)
		if err != nil || filepath.Base(f) != snapNameSet(tool, src, st, s.Hours, s.causes()) || !snapSane(s) {
			continue
		}
		out = append(out, s)
	}
	return out
}

func snapSane(s snapshot) bool {
	const eps = 1e-6
	if s.TotalUSD < 0 || s.LeakUSD < 0 || s.LeakUSD > s.TotalUSD+eps || s.Sessions < 0 || s.MaxLeak < 0 || s.MaxLeak > s.LeakUSD+eps {
		return false
	}
	for _, v := range s.Habits {
		if v < 0 || v > s.LeakUSD+eps {
			return false
		}
	}
	return true
}

// pruneSnapshots removes saved report windows that ended more than keepDays days ago.
func pruneSnapshots(cache string, now time.Time) {
	files, _ := filepath.Glob(filepath.Join(snapDir(cache), "*h.json"))
	more, _ := filepath.Glob(filepath.Join(snapDir(cache), "*h.c*.json"))
	for _, f := range append(files, more...) {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var s snapshot
		if json.Unmarshal(b, &s) == nil && s.Hours > 0 && now.Sub(s.end()) > keepDays*24*time.Hour {
			_ = os.Remove(f)
		}
	}
}
