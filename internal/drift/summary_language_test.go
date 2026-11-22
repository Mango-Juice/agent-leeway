package drift

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// judgeWith runs the fake judge on the fixture sessions with answers from dir (testdata/judge or judge-ko).
func judgeWith(t *testing.T, dir string) []py.Value {
	t.Helper()
	items, rb := fixtureItems(t)
	useFake(t, "ok")
	t.Setenv("FIXTURE_JUDGE_DIR", dir)
	cache := t.TempDir()
	JudgeAll(t.Context(), cache, items, rb, Options{Concurrency: 1})
	return LoadResults(cache)
}

// stripSummaries drops the judge's free-text summaries and the volatile fields, keeping everything the code
// derives (citation check, schema problems, verdicts, sizes, dropped events).
func stripSummaries(v py.Value) py.Value {
	switch v.K {
	case py.Obj:
		o := &py.Object{}
		for i, k := range v.O.Keys {
			switch k {
			case "summary", "verdict_reason", "ts", "seconds":
				continue
			}
			o.Set(k, stripSummaries(v.O.Vals[i]))
		}
		return py.Value{K: py.Obj, O: o}
	case py.Arr:
		var a []py.Value
		for _, x := range v.A {
			a = append(a, stripSummaries(x))
		}
		return py.Value{K: py.Arr, A: a}
	}
	return v
}

// Since TaskRevision t2 the summaries come in the user's language. The fixtures in testdata/judge-ko are the
// English answers with only summary and verdict_reason in Korean; the citation check, schema validation,
// verdict recomputation and summary data must come out the same.
func TestSummaryLanguageDoesNotChangeChecks(t *testing.T) {
	en, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "judge"))
	ko, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "judge-ko"))
	a, b := judgeWith(t, en), judgeWith(t, ko)
	if len(a) != len(b) || len(a) == 0 {
		t.Fatalf("%d and %d records", len(a), len(b))
	}
	byKey := map[string]py.Value{}
	for _, r := range b {
		byKey[r.Get("key").S] = r
	}
	korean := 0
	for _, r := range a {
		k := r.Get("key").S
		var diffs []string
		sameValue("record["+k+"]", stripSummaries(r), stripSummaries(byKey[k]), &diffs)
		for _, d := range diffs {
			t.Error(d)
		}
		for _, e := range listOf(byKey[k].Get("out").Get("events")) {
			if strings.Contains(e.Get("summary").S, "에이전트") || strings.Contains(e.Get("summary").S, "사용자") {
				korean++
			}
		}
	}
	if korean < 5 {
		t.Fatalf("only %d records carry Korean summaries", korean)
	}
	Da, Db := Summarize(a, 60), Summarize(b, 60)
	for _, k := range []string{"verdicts", "cause_classes", "causes", "sizes", "n_defects", "citation_check", "masked_quotes"} {
		if py.Dumps(Da.Get(k)) != py.Dumps(Db.Get(k)) {
			t.Errorf("summary %s: %s vs %s", k, py.Dumps(Da.Get(k)), py.Dumps(Db.Get(k)))
		}
	}
	for _, s := range Db.Get("sessions").A {
		for _, e := range s.Get("events").A {
			if sum := e.Get("summary").S; sum != "" && !strings.ContainsAny(sum, "가나다라마바사아자차카타파하에사리") {
				t.Errorf("summary not kept as written: %q", sum)
			}
		}
	}
}

// A secret inside a Korean summary is masked in the displayed copy and counted.
func TestKoreanSummaryIsMasked(t *testing.T) {
	ko, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "judge-ko"))
	dir := t.TempDir()
	token := "ghp_" + strings.Repeat("a1B2c3D4e5", 4) // fake
	ents, _ := os.ReadDir(ko)
	for _, e := range ents {
		b, _ := os.ReadFile(filepath.Join(ko, e.Name()))
		s := string(b)
		if e.Name() == "c_ko.txt" {
			s = strings.Replace(s, "사용자가 그대로 두라고 한 표를", "토큰 "+token+" 을 붙여 넣은 뒤 사용자가 그대로 두라고 한 표를", 1)
		}
		_ = os.WriteFile(filepath.Join(dir, e.Name()), []byte(s), 0o600)
	}
	recs := judgeWith(t, dir)
	D := Summarize(recs, 60)
	if n, _ := D.Get("masked_quotes").Num(); n != 1 {
		t.Fatalf("masked_quotes %v", n)
	}
	out := py.Dumps(D)
	if strings.Contains(out, token) || !strings.Contains(out, "[masked github-token]") {
		t.Error("the token is shown")
	}
	stored := false
	for _, r := range recs {
		stored = stored || strings.Contains(py.Dumps(r), token)
	}
	if !stored {
		t.Error("the stored result must keep the original text")
	}
}
