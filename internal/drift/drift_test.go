package drift

import (
	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/Mango-Juice/agent-leeway/internal/logs"
	"github.com/Mango-Juice/agent-leeway/internal/pipeline"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

var (
	fxNow   = time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
	fxSince = fxNow.Add(-30 * 24 * time.Hour)
)

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

var unsafe = regexp.MustCompile(`[^A-Za-z0-9_.-]`)

func firstDiff(a, b string) string {
	ra, rb := []rune(a), []rune(b)
	for i := 0; i < len(ra) && i < len(rb); i++ {
		if ra[i] != rb[i] {
			lo := max(0, i-60)
			return "reference: " + string(ra[lo:min(len(ra), i+60)]) + "\nhere:      " + string(rb[lo:min(len(rb), i+60)])
		}
	}
	return "lengths differ"
}

func selectedJSON(sel []Selected) py.Value {
	var a []py.Value
	for _, s := range sel {
		o := &py.Object{}
		o.Set("key", py.String(s.Key))
		o.Set("tool", py.String(s.Tool))
		o.Set("project", py.String(s.Project))
		o.Set("last", py.Value{K: py.Int, I: s.Last})
		o.Set("n_req", py.Value{K: py.Int, I: int64(s.NReq)})
		a = append(a, py.Value{K: py.Obj, O: o})
	}
	return py.Value{K: py.Arr, A: a}
}

// Gate 3: the rendered judging input of every selected session is byte-identical to the reference.
func TestTranscriptsMatchReference(t *testing.T) {
	legacy(t) // the Python reference's prices (docs/17 section 4)
	tb := fixtureTables(t)
	until := fxNow.Add(time.Minute)
	for _, inc := range []bool{false, true} {
		name := "drift_select_direct.json"
		if inc {
			name = "drift_select_all.json"
		}
		want, _ := py.Loads(expected(t, name))
		if got := selectedJSON(Select(tb, fxSince, until, nil, "", inc, 0)); py.Dumps(got) != py.Dumps(want) {
			t.Errorf("%s:\n reference %s\n here      %s", name, py.Dumps(want), py.Dumps(got))
		}
	}
	rb, err := LoadRubric(validatedRubricPath())
	if err != nil {
		t.Fatal(err)
	}
	if v := strings.TrimSpace(expected(t, "rubric_version.txt")); rb.Version != v {
		t.Errorf("rubric version %s, reference %s", rb.Version, v)
	}
	if got := BuildSystem(rb.Body, rb.Settings); got != expected(t, "system_prompt.txt") {
		t.Errorf("system prompt differs: %s", firstDiff(expected(t, "system_prompt.txt"), got))
	}
	sel := Select(tb, fxSince, until, nil, "", true, 0)
	tiers := map[string]string{"claude": "claude:opus", "codex": "codex:gpt-6.2-astra"}
	items := Prepare(NewRenderer(tb), sel, rb, tiers, false)
	wantItems, _ := py.Loads(expected(t, "drift_items.json"))
	if len(items) != len(wantItems.A) {
		t.Fatalf("%d items, reference %d", len(items), len(wantItems.A))
	}
	for i, it := range items {
		w := wantItems.A[i]
		want := expected(t, filepath.Join("transcripts", unsafe.ReplaceAllString(it.Key, "_")+".txt"))
		if it.Body != want {
			t.Errorf("%s transcript differs:\n%s", it.Key, firstDiff(want, it.Body))
		}
		wl, _ := py.Loads(expected(t, filepath.Join("transcripts", unsafe.ReplaceAllString(it.Key, "_")+".locs.json")))
		if len(wl.O.Keys) != len(it.Locs) {
			t.Errorf("%s: %d locations, reference %d", it.Key, len(it.Locs), len(wl.O.Keys))
		}
		for j, k := range wl.O.Keys {
			if it.Locs[k] != wl.O.Vals[j].S {
				t.Errorf("%s location %s differs:\n%s", it.Key, k, firstDiff(wl.O.Vals[j].S, it.Locs[k]))
			}
		}
		est, _ := w.Get("est_usd").Num()
		if d := est - it.EstUSD; d > 1e-9 || d < -1e-9 || it.NUser != int(w.Get("n_user").I) || it.Tier != w.Get("tier").S {
			t.Errorf("%s: est %v n_user %d tier %s; reference %s", it.Key, it.EstUSD, it.NUser, it.Tier, py.Dumps(w))
		}
	}
}

// htmlEscape is Python's html.escape, as the report uses it.
func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#x27;").Replace(s)
}
