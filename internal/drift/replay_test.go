package drift

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/py"
	"github.com/Mango-Juice/agent-leeway/internal/testfake"
)

// The test binary doubles as the fake judge (LEEWAY_FAKE_MODE set), see internal/testfake.
func TestMain(m *testing.M) {
	if mode := os.Getenv("LEEWAY_FAKE_MODE"); mode != "" {
		os.Exit(testfake.Main(mode))
	}
	os.Exit(m.Run())
}

func useFake(t *testing.T, mode string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(exe, " \t") {
		t.Skip("test binary path contains spaces")
	}
	judge, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "judge"))
	t.Setenv("LEEWAY_JUDGE_CMD", exe)
	t.Setenv("LEEWAY_FAKE_MODE", mode)
	t.Setenv("LEEWAY_CACHE", filepath.Join(t.TempDir(), "cache")) // judge working folders go here, not in ~/.cache
	t.Setenv("FIXTURE_JUDGE_DIR", judge)
}

func fixtureItems(t *testing.T) ([]Item, *Rubric) {
	tb := fixtureTables(t)
	rb, err := LoadRubric(validatedRubricPath())
	if err != nil {
		t.Fatal(err)
	}
	sel := Select(tb, fxSince, fxNow.Add(time.Minute), nil, "", true, 0)
	return Prepare(NewRenderer(tb), sel, rb, map[string]string{"claude": "claude:opus", "codex": "codex:gpt-6.2-astra"}, false), rb
}

func stripVolatile(r py.Value) py.Value {
	o := &py.Object{}
	for i, k := range r.O.Keys {
		if k != "ts" && k != "seconds" && !goOnlyRecordFields[k] {
			o.Set(k, r.O.Vals[i])
		}
	}
	return py.Value{K: py.Obj, O: o}
}

func sameValue(path string, a, b py.Value, out *[]string) {
	fa, okA := a.Num()
	fb, okB := b.Num()
	if okA && okB && a.K != py.Bool && b.K != py.Bool {
		if d := fa - fb; d > 1e-9 || d < -1e-9 {
			*out = append(*out, fmt.Sprintf("%s: reference %v, here %v", path, fa, fb))
		}
		return
	}
	if a.K != b.K {
		*out = append(*out, fmt.Sprintf("%s: reference %s, here %s", path, py.Repr(a), py.Repr(b)))
		return
	}
	switch a.K {
	case py.Obj:
		for i, k := range a.O.Keys {
			bv, ok := b.O.Get(k)
			if !ok {
				*out = append(*out, path+"."+k+": missing here")
				continue
			}
			sameValue(path+"."+k, a.O.Vals[i], bv, out)
		}
		for _, k := range b.O.Keys {
			if !a.Has(k) {
				*out = append(*out, path+"."+k+": not in the reference")
			}
		}
	case py.Arr:
		if len(a.A) != len(b.A) {
			*out = append(*out, fmt.Sprintf("%s: %d items in the reference, %d here", path, len(a.A), len(b.A)))
			return
		}
		for i := range a.A {
			sameValue(fmt.Sprintf("%s[%d]", path, i), a.A[i], b.A[i], out)
		}
	default:
		if py.Repr(a) != py.Repr(b) {
			*out = append(*out, fmt.Sprintf("%s: reference %s, here %s", path, py.Repr(a), py.Repr(b)))
		}
	}
}

// Gate 3: replaying the stored judge answers gives the reference's records (verdicts, events, defects,
// citations, schema problems), progress log and summary.
func TestReplayMatchesReference(t *testing.T) {
	legacy(t) // the Python reference's prices (docs/17 section 4)
	useFake(t, "ok")
	items, rb := fixtureItems(t)
	cache := t.TempDir()
	var log []string
	res := JudgeAll(context.Background(), cache, items, rb, Options{Concurrency: 1, Log: func(s string) { log = append(log, s) }})
	wantLog := strings.Split(strings.TrimRight(expected(t, "drift_log.txt"), "\n"), "\n")
	gotLog := append(log, fmt.Sprintf(`{"done": %d, "spent": %s, "stopped_reason": null}`, res.Done, py.FloatRepr(res.Spent, true)))
	if strings.Join(wantLog, "\n") != strings.Join(gotLog, "\n") {
		t.Errorf("log:\nreference:\n%s\nhere:\n%s", strings.Join(wantLog, "\n"), strings.Join(gotLog, "\n"))
	}
	got := LoadResults(cache)
	var want []py.Value
	for _, l := range strings.Split(strings.TrimSpace(expected(t, "drift_results.jsonl")), "\n") {
		v, err := py.Loads(l)
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, v)
	}
	if len(got) != len(want) {
		t.Fatalf("%d records, reference %d", len(got), len(want))
	}
	var diffs []string
	for i := range want {
		sameValue("record["+want[i].Get("key").S+"]", stripVolatile(want[i]), stripVolatile(got[i]), &diffs)
	}
	for _, d := range diffs {
		t.Error(d)
	}
	D := Summarize(got, 60)
	for _, k := range []string{"partial_sessions", "omitted_chars", "custom_rubric_sessions", "origin_uncertain_sessions", "judge_clis", "masked_quotes"} {
		if !D.Has(k) {
			t.Errorf("summary lacks %s", k)
		}
	}
	Dref := &py.Object{} // compare with the reference without the Go-only facts
	for i, k := range D.O.Keys {
		switch k {
		case "partial_sessions", "omitted_chars", "custom_rubric_sessions", "origin_uncertain_sessions", "judge_clis", "masked_quotes":
		default:
			Dref.Set(k, D.O.Vals[i])
		}
	}
	wantD, _ := py.Loads(expected(t, "drift_summary.json"))
	diffs = nil
	sameValue("summary", wantD, py.Value{K: py.Obj, O: Dref}, &diffs)
	for _, d := range diffs {
		t.Error(d)
	}
	// The report text is no longer compared with the reference: the report was redesigned (docs/24) and is
	// checked by its own golden files in internal/report. The data it is built from (records and summary) is
	// compared above (docs/17 section 4).
	// resuming: a second run finds nothing to do
	lines, todo, _ := EstimateLines(items, DoneIDs(got), rb.Version, rb.Settings, 0)
	if len(todo) != 2 || !strings.Contains(lines[0], "9 sessions already judged") { // the two parse errors are judged again
		t.Errorf("resume: %d to do, %q", len(todo), lines)
	}
	wantEst := strings.TrimRight(expected(t, "drift_estimate.txt"), "\n")
	gotEst, _, _ := EstimateLines(items, nil, rb.Version, rb.Settings, 5)
	if strings.Join(gotEst, "\n") != wantEst {
		t.Errorf("estimate:\nreference:\n%s\nhere:\n%s", wantEst, strings.Join(gotEst, "\n"))
	}
}

// goOnlyRecordFields are facts the Go tool adds to each record (the reference did not record them).
var goOnlyRecordFields = map[string]bool{"partial": true, "origin": true, "rubric_source": true, "judge_cli": true}
