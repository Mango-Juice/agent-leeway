package leak

import (
	"fmt"
	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/Mango-Juice/agent-leeway/internal/logs"
	"github.com/Mango-Juice/agent-leeway/internal/pipeline"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// The reference run (research/oracle/make_expected.py) used this clock, zone and window.
var (
	fxNow   = time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
	fxSince = fxNow.Add(-30 * 24 * time.Hour)
)

func fxLoc(t *testing.T) *time.Location {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func expected(t *testing.T, name string) string {
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "expected", name))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if strings.HasSuffix(name, ".html") {
		return messages.FromReferenceHTML(text, config.Prices().Read, htmlEscape)
	}
	return messages.FromReference(text, config.Prices().Read)
}

func fixtureTables(t *testing.T) *pipeline.Tables {
	root, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "logs"))
	srcs := logs.Discover(logs.Discovery{CodexDirs: []string{filepath.Join(root, "codex")}, ClaudeDir: filepath.Join(root, "claude")})
	files, _ := logs.Load(srcs, 4, nil)
	tb := pipeline.FromFiles(files)
	tb.Build()
	return tb
}

type reports struct{ R, H, A, P, PH, PA *Report }

func fixtureReports(t *testing.T, tb *pipeline.Tables, tool string) reports {
	loc := fxLoc(t)
	E := LoadEvents(tb, tool)
	until := fxNow.Add(time.Minute)
	prev := fxSince.Add(-fxNow.Sub(fxSince))
	return reports{Compute(E, fxSince, until, tool, "", loc), Compute(E, fxSince, until, tool, "human", loc), Compute(E, fxSince, until, tool, "auto", loc),
		Compute(E, prev, fxSince, tool, "", loc), Compute(E, prev, fxSince, tool, "human", loc), Compute(E, prev, fxSince, tool, "auto", loc)}
}

// sameJSON compares two JSON values: same keys (any order), equal strings and booleans, numbers within tol.
func sameJSON(path string, a, b py.Value, tol float64, out *[]string) {
	fa, okA := a.Num()
	fb, okB := b.Num()
	if okA && okB && a.K != py.Bool && b.K != py.Bool {
		if math.Abs(fa-fb) > tol {
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
			sameJSON(path+"."+k, a.O.Vals[i], bv, tol, out)
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
			sameJSON(fmt.Sprintf("%s[%d]", path, i), a.A[i], b.A[i], tol, out)
		}
	default:
		if py.Repr(a) != py.Repr(b) {
			*out = append(*out, fmt.Sprintf("%s: reference %s, here %s", path, py.Repr(a), py.Repr(b)))
		}
	}
}

func diffText(t *testing.T, name, want, got string) {
	t.Helper()
	if want == got {
		return
	}
	wl, gl := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			t.Errorf("%s line %d:\n  reference: %q\n  here:      %q", name, i+1, w, g)
			return
		}
	}
}

// Gate 2: the leak check equals the reference on the fixtures: counts exactly, amounts within 1e-6,
// and the rendered text byte for byte.
func TestLeakMatchesReference(t *testing.T) {
	legacy(t) // the Python reference's prices (docs/17 section 4)
	tb := fixtureTables(t)
	loc := fxLoc(t)
	x := fixtureReports(t, tb, "")
	got := JSON(x.R, x.H, x.A, x.P, x.PH, x.PA)
	want, err := py.Loads(expected(t, "leak.json"))
	if err != nil {
		t.Fatal(err)
	}
	var diffs []string
	sameJSON("leak", want, got, 1e-6, &diffs)
	for _, d := range diffs {
		t.Error(d)
	}
	diffText(t, "leak.txt", expected(t, "leak.txt"), Render(x.R, 5, x.P, x.H, x.A, x.PH, x.PA, fxNow, loc)+"\n")
	diffText(t, "leak_noprev.txt", expected(t, "leak_noprev.txt"), Render(x.R, 5, nil, x.H, x.A, nil, nil, fxNow, loc)+"\n")
	for _, tool := range []string{"claude", "codex"} {
		y := fixtureReports(t, tb, tool)
		diffText(t, "leak_"+tool+".txt", expected(t, "leak_"+tool+".txt"), Render(y.R, 5, nil, nil, nil, nil, nil, fxNow, loc)+"\n")
	}
	// compare: previous window (with its split) -> this window, from the JSON documents
	prev := x.P.ToValue(false)
	prev.O.Set("by_origin", newObj().set("human", x.PH.ToValue(false)).set("auto", x.PA.ToValue(false)).v())
	diffText(t, "compare.txt", expected(t, "compare.txt"), withoutLengths(Compare(prev, got))+"\n")
	// compare also reads documents written by the reference
	wantPrev := want.Get("previous")
	diffText(t, "compare.txt (reference JSON)", expected(t, "compare.txt"), withoutLengths(Compare(wantPrev, want))+"\n")
}

func TestIdleMatchesReference(t *testing.T) {
	legacy(t) // the Python reference's prices (docs/17 section 4)
	root, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "logs"))
	want, err := py.Loads(expected(t, "idle.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range want.A {
		e := Estimate(filepath.Join(root, filepath.FromSlash(row.Get("file").S)), fxNow)
		w := row.Get("est")
		if w.IsNull() != (e == nil) {
			t.Errorf("%s: reference %s, here %+v", row.Get("file").S, py.Repr(w), e)
			continue
		}
		if e == nil {
			continue
		}
		num := func(k string) float64 { f, _ := w.Get(k).Num(); return f }
		if e.Tool != w.Get("tool").S || e.TS != w.Get("ts").S || e.ContextTokens != int64(num("context_tokens")) || e.Expired != w.Get("expired").B ||
			math.Abs(e.GapMin-num("gap_min")) > 1e-6 || math.Abs(e.ResumeCost-num("resume_cost")) > 1e-9 || logs.FromValue(w.Get("model")) != e.Model {
			t.Errorf("%s: reference %s, here %+v", row.Get("file").S, py.Repr(w), e)
		}
	}
}

// TestCompareLocalLeakJSON compares two `leak --json` files given by LEEWAY_CMP_WANT / LEEWAY_CMP_GOT
// (manual check on real logs; skipped otherwise).
func TestCompareLocalLeakJSON(t *testing.T) {
	a, b := os.Getenv("LEEWAY_CMP_WANT"), os.Getenv("LEEWAY_CMP_GOT")
	if a == "" || b == "" {
		t.Skip("set LEEWAY_CMP_WANT and LEEWAY_CMP_GOT")
	}
	load := func(p string) py.Value {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		v, err := py.Loads(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	var diffs []string
	sameJSON("leak", load(a), load(b), 1e-6, &diffs)
	for i, d := range diffs {
		if i < 30 {
			t.Error(d)
		}
	}
	t.Logf("%d differences", len(diffs))
}

// htmlEscape is Python's html.escape, as the report uses it.
func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#x27;").Replace(s)
}

// idle shows project labels and short keys, never a folder path or a full file name (docs/19 C8).
func TestIdleReportLabels(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "logs"))
	out := IdleReport(filepath.Join(root, "claude"), filepath.Join(root, "codex"), 3650, fxNow.Add(24*time.Hour))
	if !strings.Contains(out, "데모 프로젝트") || !strings.Contains(out, "proyecto") {
		t.Errorf("project labels missing:\n%s", out)
	}
	for _, bad := range []string{root, "-tmp-", ".jsonl", "rollout-"} {
		if strings.Contains(out, bad) {
			t.Errorf("idle output contains %q:\n%s", bad, out)
		}
	}
}

// withoutLengths drops the window-length line, which the reference did not print (docs/19 C9).
func withoutLengths(s string) string {
	var keep []string
	for _, l := range strings.Split(s, "\n") {
		if !strings.HasPrefix(l, "Window lengths: ") {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}

func TestCompareWarnsOnMismatchedWindows(t *testing.T) {
	doc := func(start, end string) py.Value {
		v, _ := py.Loads(`{"window_start":"` + start + `","window_end":"` + end + `","total_usd":1.0,"leak_usd":0.1,"causes":[],"habits":[]}`)
		return v
	}
	out := Compare(doc("2026-01-01T00:00:00+00:00", "2026-01-31T00:00:00+00:00"), doc("2026-01-24T00:00:00+00:00", "2026-01-31T00:00:00+00:00"))
	if !strings.Contains(out, "30.0 days -> 7.0 days") || !strings.Contains(out, "different lengths") || !strings.Contains(out, "overlap") {
		t.Errorf("no warnings:\n%s", out)
	}
	if !strings.Contains(out, "all sessions") {
		t.Error("the habit block must say it covers all sessions")
	}
}
