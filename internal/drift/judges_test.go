package drift

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// judges.json order: the checked version first (a catalog without it is only a hint), then a newer version of
// the family (tier variants such as -mini excluded), then the CLI's default; refused models are skipped for 7 days.
func TestJudgeCandidates(t *testing.T) {
	c := Candidates("codex", []string{"gpt-6.2-sol", "gpt-6.2-astra"}, "gpt-6.2-sol", nil)
	if c[0].Tier != "codex:gpt-6.2-astra" || !c[0].Checked {
		t.Errorf("checked model first: %+v", c)
	}
	// account list without the checked model: it is still tried first, then a newer astra, then the default
	c = Candidates("codex", []string{"gpt-6.2-sol", "gpt-6.3-astra-mini", "gpt-6.3-astra"}, "gpt-6.2-sol", nil)
	if c[0].Tier != "codex:gpt-6.2-astra" || c[1].Tier != "codex:gpt-6.3-astra" || c[1].Checked || c[len(c)-1].Tier != "codex:gpt-6.2-sol" || !c[len(c)-1].Default {
		t.Errorf("hint only, then newer astra, then default: %+v", c)
	}
	for _, x := range c {
		if strings.Contains(x.Tier, "mini") {
			t.Errorf("a tier variant was taken: %+v", x)
		}
	}
	c = Candidates("codex", nil, "", nil)
	if c[0].Tier != "codex:gpt-6.2-astra" || c[len(c)-1].Tier != "codex:default" || CLIModel("codex:default") != "" {
		t.Errorf("no catalog: %+v", c)
	}
	c = Candidates("claude", nil, "", nil)
	if c[0].Tier != "claude:opus" || CLIModel("claude:opus") != "claude-opus-5-5" || c[1].Tier != "claude:opus@latest" || CLIModel(c[1].Tier) != "opus" ||
		c[len(c)-1].Tier != "claude:default" || CLIModel("claude:default") != "default" || CLIModel("claude:sonnet") != "claude-sonnet-5-5" {
		t.Errorf("claude: %+v", c)
	}
	cache := t.TempDir()
	MarkUnavailable(cache, "claude", "2.1.329 (Claude Code)", "claude:opus")
	if c = Candidates("claude", nil, "", Unavailable(cache, "claude", "2.1.329 (Claude Code)")); c[0].Tier != "claude:opus@latest" {
		t.Errorf("refused model not skipped: %+v", c)
	}
	if len(Unavailable(cache, "claude", "2.1.330 (Claude Code)")) != 0 {
		t.Error("refusals are per CLI version")
	}
	// a refusal older than 7 days is tried again
	old := map[string]map[string]string{"claude 2.1.329 (Claude Code)": {"claude:opus": time.Now().Add(-8 * 24 * time.Hour).UTC().Format(time.RFC3339)}}
	b, _ := json.Marshal(old)
	_ = os.WriteFile(modelsFile(cache), b, 0o600)
	if len(Unavailable(cache, "claude", "2.1.329 (Claude Code)")) != 0 {
		t.Error("an expired refusal is still used")
	}
	// every candidate refused: an empty list (the CLI says so and exits non-zero)
	all := map[string]bool{"claude:opus": true, "claude:opus@latest": true, "claude:default": true}
	if c = Candidates("claude", nil, "", all); len(c) != 0 {
		t.Errorf("all refused: %+v", c)
	}
	for in, want := range map[string]string{"codex:gpt-6.2-astra": "GPT-6.2 Astra", "claude:opus": "Claude Opus 5.5", "claude-opus-5-6": "Claude Opus 5.6"} {
		if got := ModelName(in); got != want {
			t.Errorf("ModelName(%s) = %q, want %q", in, got, want)
		}
	}
	if !Checked("claude:opus") || !Checked("codex:gpt-6.2-astra") || Checked("codex:gpt-6.3-astra") || Checked("claude:opus@latest") {
		t.Error("checked status")
	}
}

// A model the CLI refuses (recorded wording) stops the run: nothing is recorded and no other model judges.
func TestRefusedModelStopsTheRun(t *testing.T) {
	items, rb := fixtureItems(t)
	var codex []Item
	for _, it := range items {
		if it.Tool == "codex" {
			codex = append(codex, it)
		}
	}
	fakeCLI(t, "codex-cli")
	resetPreflight()
	t.Setenv("LEEWAY_FAKE_CODEX", "refuse")
	_, _, _, err := Call(context.Background(), "codex:gpt-6.2-astra", "s", "p", time.Minute)
	if me, ok := IsErrModel(err); !ok || me.Tier != "codex:gpt-6.2-astra" {
		t.Fatalf("refusal not recognised: %v", err)
	}
	cache := t.TempDir()
	res := JudgeAll(context.Background(), cache, codex, rb, Options{Concurrency: 1})
	if res.Refused != "codex:gpt-6.2-astra" || !res.Failed || res.Done != 0 || len(LoadResults(cache)) != 0 {
		t.Errorf("run: %+v, %d records", res, len(LoadResults(cache)))
	}
	// a notice that the request was rerouted to another model is a model change: stop, nothing recorded
	t.Setenv("LEEWAY_FAKE_CODEX", "reroute")
	t.Setenv("LEEWAY_FAKE_TEXT", `{"verdict": "as_intended", "events": [], "summary": "ok"}`)
	res = JudgeAll(context.Background(), cache, codex, rb, Options{Concurrency: 1})
	if res.Refused != "codex:gpt-6.2-astra" || res.Done != 0 || len(LoadResults(cache)) != 0 || !strings.Contains(res.StoppedReason, "rerouted") {
		t.Errorf("reroute: %+v", res)
	}
	// an unrelated failure of a dot-less, unchecked model id is a normal failure, not a model refusal
	t.Setenv("LEEWAY_FAKE_CODEX", "unauthorized-dotless")
	_, _, _, err = Call(context.Background(), "codex:gpt-7-astra", "s", "p", time.Minute)
	if _, ok := IsErrModel(err); ok || err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("an unrelated 401 was taken for a model refusal: %v", err)
	}
	// the metadata notice of an older CLI is not a refusal
	t.Setenv("LEEWAY_FAKE_CODEX", "notice-ok")
	if _, _, _, err := Call(context.Background(), "codex:gpt-6.2-astra", "s", "p", time.Minute); err != nil {
		t.Errorf("metadata notice treated as an error: %v", err)
	}
}

// An alias tier records the model that answered (the main one, never a helper); an exact tier that is answered
// by another model is a model change, not a recorded switch.
func TestAliasTierRecordsTheMainModel(t *testing.T) {
	out := `{"result":"{}","num_turns":1,"usage":{"input_tokens":10,"output_tokens":5},"modelUsage":{"claude-haiku-5-5":{"inputTokens":50,"outputTokens":20},"claude-opus-5-6":{"inputTokens":900,"outputTokens":500}}}`
	_, u, err := parseClaude(procResultOut(0, out), "opus@latest")
	if err != nil || u.Model != "claude-opus-5-6" || !u.Fallback || len(u.Extra) != 1 || u.Extra[0].Model != "claude-haiku-5-5" {
		t.Errorf("alias: %+v %v", u, err)
	}
	_, _, err = parseClaude(procResultOut(0, `{"result":"{}","num_turns":1,"usage":{},"modelUsage":{"claude-sonnet-5-5":{"outputTokens":5}}}`), "claude-opus-5-5")
	if _, ok := IsErrModel(err); !ok {
		t.Errorf("exact model answered by another: %v", err)
	}
}

// A session checked under the same rubric by any model of its tool counts as done; --recheck counts only the
// same model.
func TestCheckedByAnyModelCounts(t *testing.T) {
	items, rb := fixtureItems(t)
	it := items[0]
	other := "claude:claude-sonnet-5-5"
	if it.Tool == "codex" {
		other = "codex:gpt-6.2-sol"
	}
	rec := py.Value{K: py.Obj, O: &py.Object{}}
	rec.O.Set("run_id", py.String(RunID(it.Key, other, rb.Version, rb.Settings)))
	rec.O.Set("error", py.Value{})
	_, todo, _ := EstimateLines(items, DoneIDs([]py.Value{rec}), rb.Version, rb.Settings, 0)
	for _, x := range todo {
		if x.Key == it.Key {
			t.Error("a session checked by another model was selected again")
		}
	}
	_, todo, _ = EstimateLines(items, DoneIDsExact([]py.Value{rec}), rb.Version, rb.Settings, 0)
	found := false
	for _, x := range todo {
		found = found || x.Key == it.Key
	}
	if !found {
		t.Error("--recheck does not select it again")
	}
}

// The account's catalog (`codex debug models`, recorded shape); offline, the bundled list still names the
// default; an unknown price gives the plan-usage wording only.
func TestCodexCatalogAndUnpriced(t *testing.T) {
	fakeCLI(t, "codex-cli")
	t.Setenv("LEEWAY_JUDGE_CMD", "")
	cat, def := CodexCatalog()
	if def != "gpt-6.2-sol" || len(cat) != 3 {
		t.Fatalf("catalog: %v %q", cat, def)
	}
	t.Setenv("LEEWAY_FAKE_CATALOG", "no-astra")
	cat, def = CodexCatalog()
	if c := Candidates("codex", cat, def, nil); c[0].Tier != "codex:gpt-6.2-astra" || c[len(c)-1].Tier != "codex:gpt-6.2-sol" {
		t.Errorf("account list without astra: %+v", c)
	}
	t.Setenv("LEEWAY_FAKE_CATALOG", "offline")
	if cat, def = CodexCatalog(); cat != nil || def != "gpt-6.1-terra" {
		t.Errorf("offline: %v %q", cat, def)
	}
	l := strings.Join(PlanLines(10, 10, 50, []Item{{Key: "a", Tier: "codex:default"}}, 0), "\n")
	if !strings.Contains(l, "no list price for this model") || strings.Contains(l, "$") {
		t.Errorf("unpriced plan line: %s", l)
	}
}
