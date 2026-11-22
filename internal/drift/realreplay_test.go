package drift

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/logs"
	"github.com/Mango-Juice/agent-leeway/internal/pipeline"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// TestReplayLocalCaches repeats research/oracle/export_replay.py on real cached judgments
// (LEEWAY_REPLAY=<its output>): recomputed verdicts and sizes, and for owner-sample sessions the re-rendered
// transcript (by hash) and the citation check on it. Local only; skipped otherwise.
func TestReplayLocalCaches(t *testing.T) {
	path := os.Getenv("LEEWAY_REPLAY")
	if path == "" {
		t.Skip("set LEEWAY_REPLAY")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rb, err := LoadRubric(validatedRubricPath())
	if err != nil {
		t.Fatal(err)
	}
	var R *Renderer
	render := map[string]Session{}
	n, mism, rendered, textDiff, verifyDiff := 0, 0, 0, 0, 0
	counts := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		rec, err := py.Loads(line)
		if err != nil {
			t.Fatal(err)
		}
		n++
		out := rec.Get("out")
		if got := DerivedVerdict(out, rb.Settings); got != rec.Get("derived").S || got != rec.Get("stored_derived").S {
			mism++
			t.Errorf("%s %s: verdict %s, reference %s, stored %s", rec.Get("id").S, rec.Get("tier").S, got, rec.Get("derived").S, rec.Get("stored_derived").S)
		}
		ev := listOf(out.Get("events"))
		sizes := EffectiveSizes(ev, rb.Settings.S1Threshold())
		for i, z := range sizes {
			if z != rec.Get("sizes").A[i].S {
				t.Errorf("%s: size %d %s, reference %s", rec.Get("id").S, i, z, rec.Get("sizes").A[i].S)
			}
		}
		counts["events"] += len(ev)
		counts["defects"] += len(listOf(out.Get("defect_reports")))
		counts["weak"] += len(listOf(out.Get("weak_signals")))
		if !rec.Has("key") {
			continue
		}
		if R == nil {
			srcs := logs.Discover(logs.Discovery{CodexDirs: []string{config.CodexDir()}, ClaudeDir: config.ClaudeDir()})
			files, _ := logs.Load(srcs, 0, nil)
			tb := pipeline.FromFiles(files)
			tb.Build()
			R = NewRenderer(tb)
		}
		key, ci := rec.Get("key").S, rec.Get("current_instr").B
		ck := key
		if ci {
			ck += "|ci"
		}
		s, ok := render[ck]
		if !ok {
			s = R.Session(key, ci)
			render[ck] = s
		}
		rendered++
		h := sha1.Sum([]byte(py.ReplaceSurrogates(s.Text, "?")))
		if hex.EncodeToString(h[:]) != rec.Get("text_sha1").S {
			textDiff++
			t.Errorf("%s: re-rendered transcript differs from the reference", rec.Get("id").S)
		}
		fresh, _ := py.Loads(py.Dumps(out))
		v := Verify(fresh, s.Locs)
		var diffs []string
		sameValue("verify", rec.Get("verify"), v, &diffs)
		if len(diffs) > 0 {
			verifyDiff++
			t.Errorf("%s: %v", rec.Get("id").S, diffs)
		}
	}
	t.Logf("%d cached judgments: %d verdict mismatches; %d re-rendered: %d transcript and %d citation-check differences; events %d, defects %d, weak %d",
		n, mism, rendered, textDiff, verifyDiff, counts["events"], counts["defects"], counts["weak"])
}
