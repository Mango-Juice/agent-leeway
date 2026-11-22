package pipeline

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Mango-Juice/agent-leeway/internal/logs"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// Dump writes every table as JSONL in the format of research/oracle/agent_leeway/dump.py, for comparison
// with the Python reference: timestamps as integer nanoseconds, NaN/NaT/None as null, thread ids replaced by
// "T:" + the file path relative to root (with "/"), file paths relative to root.
func (t *Tables) Dump(root, out string) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	tid := map[string]string{}
	for _, x := range t.Threads {
		tid[x.ThreadID] = "T:" + rel(root, x.File)
	}
	T := func(id string) any {
		if v, ok := tid[id]; ok {
			return v
		}
		return id
	}
	var rows []map[string]any
	w := func(name string) error {
		err := writeRows(filepath.Join(out, name+".jsonl"), rows)
		rows = nil
		return err
	}
	for _, x := range t.Threads {
		rows = append(rows, map[string]any{"bytes": x.Bytes, "cost_usd": val(x.CostUSD), "cwd": ns(x.Cwd), "end": ts(x.End),
			"entrypoint": ns(x.Entrypoint), "file": rel(root, x.File), "has_final_msg": x.HasFinalMsg, "lines_added": val(x.LinesAdded),
			"model": ns(x.Model), "n_aborted": x.NAborted, "n_calls": x.NCalls, "n_compactions": x.NCompactions, "n_human_turns": x.NHumanTurns,
			"n_steps": x.NSteps, "originator": ns(x.Originator), "parent_id": ns(x.ParentID), "session_id": ns(x.SessionID),
			"source": ns(x.Source), "start": ts(x.Start), "thread_id": T(x.ThreadID), "tool": x.Tool, "version": ns(x.Version)})
	}
	if err := w("threads"); err != nil {
		return err
	}
	step := func(s StepRow, priced bool) map[string]any {
		m := map[string]any{"after_compaction": s.AfterCompaction, "cache_write": s.CacheWrite, "cached": s.Cached, "input_total": s.InputTotal,
			"output": s.Output, "reasoning": s.Reasoning, "seq": s.Seq, "session_id": ns(s.SessionID), "thread_id": T(s.ThreadID), "tool": s.Tool,
			"trigger": ns(s.Trigger), "ts": ts(s.TS), "uncached": s.Uncached}
		if priced {
			m["key"], m["model"], m["p_in"], m["p_out"], m["p_read"], m["p_write"], m["wcost"] = ns(s.Key), ns(s.ThreadModel), s.PIn, s.POut, s.PRead, s.PWrite, f(s.WCost)
		}
		return m
	}
	for _, s := range t.Steps {
		rows = append(rows, step(s, false))
	}
	if err := w("steps"); err != nil {
		return err
	}
	for _, s := range t.Steps {
		rows = append(rows, step(s, true))
	}
	if err := w("steps_priced"); err != nil {
		return err
	}
	for _, c := range t.Calls {
		var inChars any = c.InChars
		if c.InChars < 0 {
			inChars = nil
		}
		var cwd any
		if c.HasCwd {
			cwd = ns(c.Cwd)
		}
		rows = append(rows, map[string]any{"call_id": ns(c.CallID), "cwd": cwd, "failed": tri(c.Failed), "in_chars": inChars, "kind": c.Kind,
			"name": ns(c.Name), "out_chars": c.OutChars, "path": ns(c.Path), "session_id": ns(c.SessionID), "step": c.Step, "target": ns(c.Target),
			"thread_id": T(c.ThreadID), "tool": c.Tool, "ts": ts(c.TS)})
	}
	if err := w("calls"); err != nil {
		return err
	}
	turn := func(u TurnRow, withKey bool) map[string]any {
		m := map[string]any{"kind": u.Kind, "session_id": ns(u.SessionID), "text": py.ReplaceSurrogates(u.Text, "�"), "thread_id": T(u.ThreadID), "tool": u.Tool, "ts": ts(u.TS)}
		if withKey {
			m["key"] = ns(u.Key)
		}
		return m
	}
	for _, u := range t.Turns {
		rows = append(rows, turn(u, false))
	}
	if err := w("turns"); err != nil {
		return err
	}
	for _, u := range t.TurnsHuman {
		rows = append(rows, turn(u, true))
	}
	if err := w("turns_human"); err != nil {
		return err
	}
	for _, c := range t.Commits {
		rows = append(rows, map[string]any{"branch": c.Branch, "cwd": ns(c.Cwd), "session_id": ns(c.SessionID), "sha": c.SHA, "thread_id": T(c.ThreadID), "tool": c.Tool, "ts": ts(c.TS)})
	}
	if err := w("log_commits"); err != nil {
		return err
	}
	for _, s := range t.Sessions {
		rows = append(rows, map[string]any{"cwd": ns(s.Cwd), "end": ts(s.End), "exclude": s.Exclude, "key": s.Key, "model": ns(s.Model), "n_steps": s.NSteps,
			"n_threads": s.NThreads, "project": s.Project, "root_source": ns(s.RootSource), "session_id": s.SessionID, "start": ts(s.Start), "tool": s.Tool, "wcost": f(s.WCost)})
	}
	if err := w("sessions"); err != nil {
		return err
	}
	for _, r := range t.Waste {
		rows = append(rows, map[string]any{"after_compaction": r.AfterCompaction, "c_fail": r.CFail, "c_repeat": r.CRepeat, "c_reread": r.CReread,
			"call_waste": r.CallWaste, "fixed": r.Fixed, "key": r.Key, "miss_cost": r.MissCost, "miss_expiry": r.MissExpiry, "n_expiry": r.NExpiry,
			"n_fail": r.NFail, "n_midmiss": r.NMidmiss, "n_oversize": r.NOversize, "n_repeat": r.NRepeat, "n_reread": r.NReread, "oversize": r.Oversize,
			"seq": r.Seq, "sub": r.Sub, "thread_id": T(r.ThreadID), "tool": r.Tool, "ts": ts(r.TS), "wcost": f(r.WCost)})
	}
	if err := w("step_waste"); err != nil {
		return err
	}
	for _, r := range t.Origin {
		rows = append(rows, map[string]any{"key": ns(r.Key), "origin": ns(r.Origin), "reason": ns(r.Reason), "seq": r.Seq, "thread_id": T(r.ThreadID),
			"tool": r.Tool, "trigger": ns(r.Trigger), "ts": ts(r.TS)})
	}
	if err := w("step_origin"); err != nil {
		return err
	}
	for _, s := range t.SessOrigin {
		rows = append(rows, map[string]any{"key": s.Key, "session_class": s.SessionClass, "session_origin": s.SessionOrigin, "session_reason": s.SessionReason,
			"tool": s.Tool, "uncertain": s.Uncertain})
	}
	if err := w("session_origin"); err != nil {
		return err
	}
	for _, e := range t.ResetEv {
		m := map[string]any{"cls": e.Cls, "frac": f(e.Frac), "key": e.Key, "thread_id": T(e.ThreadID), "tool": e.Tool, "ts": ts(e.TS), "usd": e.USD}
		for _, k := range Priority {
			m[k] = e.Flags[k]
		}
		rows = append(rows, m)
	}
	if err := w("reset_events"); err != nil {
		return err
	}
	for _, r := range t.ResetSteps {
		m := step(*r.StepRow, true)
		m["cls"], m["frac"], m["mid"], m["miss_cost"], m["miss_expiry"], m["miss_tok"], m["prev_in"], m["prev_ts"] =
			r.Cls, f(r.Frac), r.Mid, r.MissCost, r.MissExpiry, r.MissTok, f(r.PrevIn), ts(r.PrevTS)
		rows = append(rows, m)
	}
	return w("reset_steps")
}

func rel(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(r)
}

func ns(n logs.NS) any {
	if !n.OK {
		return nil
	}
	return py.ReplaceSurrogates(n.S, "�")
}

func ts(t int64) any {
	if t == NaT {
		return nil
	}
	return t
}

func f(x float64) any {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return nil
	}
	return x
}

func tri(t logs.Tri) any {
	switch t {
	case logs.TriTrue:
		return true
	case logs.TriFalse:
		return false
	}
	return nil
}

func val(v py.Value) any {
	switch v.K {
	case py.Null:
		return nil
	case py.Bool:
		return v.B
	case py.Int:
		if v.Big != "" {
			return json.Number(v.Big)
		}
		return v.I
	case py.Float:
		return f(v.F)
	case py.Str:
		return v.S
	}
	return py.PyStr(v)
}

func writeRows(path string, rows []map[string]any) error {
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(fh)
	enc := json.NewEncoder(bw)
	enc.SetEscapeHTML(false)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			fh.Close()
			return err
		}
	}
	if err := bw.Flush(); err != nil {
		fh.Close()
		return err
	}
	return fh.Close()
}

// ---------------------------------------------------------------- comparison

// tableKeys are the columns rows are matched on (rows with equal keys keep their order).
var tableKeys = map[string][]string{
	"threads": {"thread_id"}, "steps": {"thread_id", "seq"}, "steps_priced": {"thread_id", "seq"}, "calls": {"thread_id"},
	"turns": {"thread_id"}, "turns_human": {"thread_id"}, "log_commits": {"thread_id"}, "sessions": {"key"},
	"step_waste": {"thread_id", "seq"}, "step_origin": {"thread_id", "seq"}, "session_origin": {"key"},
	"reset_events": {"thread_id", "ts"}, "reset_steps": {"thread_id", "seq"},
}

// TableNames lists the dumped tables.
func TableNames() []string {
	var n []string
	for k := range tableKeys {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

// CompareDumps compares two dump folders table by table: same rows (matched by key columns), same columns,
// equal values; numbers may differ by tol (absolute, or relative for large values). Returns readable differences.
func CompareDumps(want, got string, tol float64, limit int) []string {
	return CompareDumpsExcept(want, got, tol, limit, nil)
}

// CompareDumpsExcept is CompareDumps ignoring rows of the given files (relative paths, as in the dump) and of
// their sessions: files still being written while the two runs happened.
func CompareDumpsExcept(want, got string, tol float64, limit int, files []string) []string {
	var diffs []string
	skipT, skipS := map[string]bool{}, map[string]bool{}
	for _, f := range files {
		skipT["T:"+f] = true
	}
	if th, err := readRows(filepath.Join(want, "threads.jsonl")); err == nil {
		for _, r := range th {
			if skipT[fmt.Sprint(r["thread_id"])] {
				skipS[fmt.Sprint(r["session_id"])] = true
			}
		}
	}
	drop := func(rows []map[string]any) []map[string]any {
		var out []map[string]any
		for _, r := range rows {
			if skipT[fmt.Sprint(r["thread_id"])] || skipS[fmt.Sprint(r["session_id"])] {
				continue
			}
			if k, ok := r["key"].(string); ok {
				if i := strings.IndexByte(k, ':'); i >= 0 && skipS[k[i+1:]] {
					continue
				}
			}
			out = append(out, r)
		}
		return out
	}
	add := func(s string) {
		if len(diffs) < limit {
			diffs = append(diffs, s)
		}
	}
	for _, name := range TableNames() {
		a, errA := readRows(filepath.Join(want, name+".jsonl"))
		b, errB := readRows(filepath.Join(got, name+".jsonl"))
		if errA != nil || errB != nil {
			add(fmt.Sprintf("%s: cannot read (%v / %v)", name, errA, errB))
			continue
		}
		a, b = drop(a), drop(b)
		if len(a) != len(b) {
			add(fmt.Sprintf("%s: %d rows in the reference, %d here", name, len(a), len(b)))
		}
		sortRows(a, tableKeys[name])
		sortRows(b, tableKeys[name])
		for i := 0; i < len(a) && i < len(b); i++ {
			for _, k := range unionKeys(a[i], b[i]) {
				if !sameValue(a[i][k], b[i][k], tol) {
					add(fmt.Sprintf("%s row %d (%v) column %s: reference %v, here %v", name, i, rowID(a[i], tableKeys[name]), k, short(a[i][k]), short(b[i][k])))
				}
			}
		}
	}
	return diffs
}

func short(v any) string {
	s := fmt.Sprint(v)
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}

func rowID(r map[string]any, keys []string) string {
	var p []string
	for _, k := range keys {
		p = append(p, fmt.Sprint(r[k]))
	}
	return strings.Join(p, " ")
}

func readRows(path string) ([]map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	for dec.More() {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func sortRows(rows []map[string]any, keys []string) {
	sort.SliceStable(rows, func(i, j int) bool {
		for _, k := range keys {
			a, b := fmt.Sprint(rows[i][k]), fmt.Sprint(rows[j][k])
			if na, ok := rows[i][k].(json.Number); ok {
				if nb, ok2 := rows[j][k].(json.Number); ok2 {
					x, _ := na.Float64()
					y, _ := nb.Float64()
					if x != y {
						return x < y
					}
					continue
				}
			}
			if a != b {
				return a < b
			}
		}
		return false
	})
}

func unionKeys(a, b map[string]any) []string {
	m := map[string]bool{}
	for k := range a {
		m[k] = true
	}
	for k := range b {
		m[k] = true
	}
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sameValue(a, b any, tol float64) bool {
	na, okA := a.(json.Number)
	nb, okB := b.(json.Number)
	if okA && okB {
		x, _ := na.Float64()
		y, _ := nb.Float64()
		d := math.Abs(x - y)
		return d <= tol || d <= tol*math.Max(math.Abs(x), math.Abs(y))
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}
