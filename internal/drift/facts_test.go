package drift

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/assets"
	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// Task item 2: a session judged with its middle omitted is recorded as partial, with how much was left out,
// and counted in the summary; the judge input itself is built exactly as before.
func TestPartialJudgingIsRecorded(t *testing.T) {
	old := MaxBody
	MaxBody = 600
	defer func() { MaxBody = old }()
	tb := fixtureTables(t)
	rb, _ := LoadRubric("")
	sel := Select(tb, fxSince, fxNow.Add(time.Minute), nil, "", true, 0)
	items := Prepare(NewRenderer(tb), sel, rb, map[string]string{"claude": "claude:opus", "codex": "codex:gpt-6.2-astra"}, false)
	R := NewRenderer(tb)
	partial := 0
	for _, it := range items {
		full := R.Session(it.Key, false).Text
		if py.Len(full) > MaxBody {
			partial++
			want := py.Head(full, MaxBody/2) + messages.Drift["truncated"] + py.Tail(full, MaxBody/2)
			if it.Body != want || it.OmittedChars != py.Len(full)-MaxBody || it.TotalChars != py.Len(full) {
				t.Errorf("%s: omitted %d of %d", it.Key, it.OmittedChars, it.TotalChars)
			}
		} else if it.OmittedChars != 0 {
			t.Errorf("%s: short session marked partial", it.Key)
		}
	}
	if partial == 0 {
		t.Fatal("no fixture session is long enough")
	}
	_, recs := runWith(t, "ok", items, rb, Options{Concurrency: 1})
	n := 0
	for _, r := range recs {
		if r.Get("partial").K == py.Obj {
			n++
		}
	}
	D := Summarize(recs, 60)
	if got, _ := D.Get("partial_sessions").Num(); n == 0 || int(got) > n || got == 0 {
		t.Errorf("partial records %d, summary %v", n, got)
	}
}

// Task item 4: results and summary carry whether the bundled rubric or a custom one was used.
func TestCustomRubricIsMarked(t *testing.T) {
	dir := t.TempDir()
	same := filepath.Join(dir, "copy.md")
	_ = os.WriteFile(same, assets.Rubric, 0o644)
	edited := filepath.Join(dir, "edited.md")
	_ = os.WriteFile(edited, []byte(strings.Replace(string(assets.Rubric), "| S1 |", "| S1 |", 1)+"\nmy note\n"), 0o644)
	for p, want := range map[string]string{"": "bundled", same: "bundled", edited: "custom"} {
		rb, err := LoadRubric(p)
		if err != nil {
			t.Fatal(err)
		}
		if rb.Source() != want {
			t.Errorf("%q: %s, want %s", p, rb.Source(), want)
		}
	}
	items, _ := fixtureItems(t)
	rb, _ := LoadRubric(edited)
	_, recs := runWith(t, "ok", items[:2], rb, Options{Concurrency: 1})
	for _, r := range recs {
		if r.Get("rubric_source").S != "custom" || !strings.HasPrefix(r.Get("rubric_version").S, rb.Version[:4]) {
			t.Errorf("record: %s %s", r.Get("rubric_source").S, r.Get("rubric_version").S)
		}
	}
	if n, _ := Summarize(recs, 60).Get("custom_rubric_sessions").Num(); n == 0 {
		t.Error("summary does not count custom-rubric sessions")
	}
}

// Task item 3: the preflight finds every lock-down flag in the CLIs' help (no model call), returns the
// version, and refuses a CLI that lacks one, naming it; a judge call never runs without it.
func TestPreflightChecksLockDownFlags(t *testing.T) {
	fakeCLI(t, "claude-cli")
	resetPreflight()
	v, err := Preflight("claude")
	if err != nil || v != "2.1.329 (Claude Code)" {
		t.Fatalf("claude preflight: %q %v", v, err)
	}
	t.Setenv("LEEWAY_FAKE_MODE", "codex-cli")
	if v, err := Preflight("codex"); err != nil || v != "codex-cli 0.171.0" {
		t.Fatalf("codex preflight: %q %v", v, err)
	}
	for mode, drop := range map[string]string{"claude-cli": "--restricted", "codex-cli": "hooks"} {
		resetPreflight()
		t.Setenv("LEEWAY_FAKE_MODE", mode)
		t.Setenv("LEEWAY_FAKE_DROP", drop)
		prov := strings.TrimSuffix(mode, "-cli")
		if _, err := Preflight(prov); err == nil || !strings.Contains(err.Error(), drop) {
			t.Errorf("%s without %s: %v", prov, drop, err)
		}
		if _, _, _, err := Call(context.Background(), prov+":x", "s", "p", time.Minute); err == nil || !strings.Contains(err.Error(), drop) {
			t.Errorf("%s: a judge call ran without %s: %v", prov, drop, err)
		}
	}
	t.Setenv("LEEWAY_FAKE_DROP", "")
	resetPreflight()
	// the version goes into every record
	items, rb := fixtureItems(t)
	_, recs := runWith(t, "ok", items[:1], rb, Options{Concurrency: 1, CLIVersions: map[string]string{"claude": "2.1.329 (Claude Code)", "codex": "codex-cli 0.171.0"}})
	if len(recs) != 1 || !strings.Contains(recs[0].Get("judge_cli").S, ".") {
		t.Errorf("judge_cli not recorded: %s", py.Dumps(recs[0].Get("judge_cli")))
	}
}

// Task item 5: a quote holding a pasted secret is masked in the summary (report, terminal), while the stored
// judgment and the citation check keep the original, so the masked quote still counts as found.
func TestSecretsMaskedInDisplayOnly(t *testing.T) {
	secret := "ghp_" + strings.Repeat("a1B2c3D4e5", 4)
	transcript := map[string]string{"U1": "토큰은 " + secret + " 이에요. 이걸로 배포해 줘."}
	out, _ := py.Loads(`{"events":[{"loc":"U1","rule":"R2a","cause":"agent_error","size":"major","irreversible_harm":"no","resolution":"unknown",` +
		`"evidence":[{"loc":"U1","quote":"토큰은 ` + secret + ` 이에요"}],"summary":"used ` + secret + `"}],"verdict":"agent_drift"}`)
	v := Verify(out, transcript)
	if b, _ := v.Get("bad_quotes").Num(); b != 0 {
		t.Fatalf("citation check failed on the original quote: %s", py.Dumps(v))
	}
	rec := &py.Object{}
	for k, val := range map[string]py.Value{"key": py.String("claude:x"), "tier": py.String("claude:opus"), "out": out, "verify": v} {
		rec.Set(k, val)
	}
	D := Summarize([]py.Value{{K: py.Obj, O: rec}}, 60)
	text := py.Dumps(D)
	if strings.Contains(text, "a1B2c3D4e5") || !strings.Contains(text, "[masked github-token]") {
		t.Errorf("secret shown in summary data: %s", text)
	}
	if !strings.Contains(py.Dumps(out), secret) {
		t.Error("the stored judgment must keep the original quote")
	}
	if !strings.Contains(D.Get("sessions").A[0].Get("events").A[0].Get("excerpt").S, "토큰은") {
		t.Error("the text around the secret must stay")
	}
}
