package drift

import (
	"math"
	"strings"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/config"
)

// The Claude CLI reports its own helper calls (e.g. a Haiku model) next to the judge model in modelUsage.
// The record names the judge the user chose, and the helper usage is priced at its own list price.
func TestClaudeHelperModelIsPricedButNotNamedAsJudge(t *testing.T) {
	out := `{"result":"{}","num_turns":1,"usage":{"input_tokens":1000,"output_tokens":200,"cache_read_input_tokens":0,"cache_creation_input_tokens":0},
	"modelUsage":{"claude-haiku-5-5":{"inputTokens":3000,"outputTokens":100,"cacheReadInputTokens":0,"cacheCreationInputTokens":0},
	"claude-opus-5-5":{"inputTokens":1000,"outputTokens":200,"cacheReadInputTokens":0,"cacheCreationInputTokens":0}}}`
	_, u, err := parseClaude(procResultOut(0, out), config.Prices().JudgeModel("claude:opus"))
	if err != nil {
		t.Fatal(err)
	}
	if u.Model != "claude-opus-5-5" {
		t.Errorf("judge model %q, want claude-opus-5-5", u.Model)
	}
	if len(u.Extra) != 1 || u.Extra[0].Model != "claude-haiku-5-5" || u.Extra[0].Input != 3000 || u.Extra[0].Output != 100 {
		t.Fatalf("extra usage %+v", u.Extra)
	}
	withHelper, ok := config.JudgeUSD("claude:opus", u)
	if !ok {
		t.Fatal("unpriced")
	}
	u2 := u
	u2.Extra = nil
	judgeOnly, _ := config.JudgeUSD("claude:opus", u2)
	h, _ := config.Prices().Lookup("claude", "claude-haiku-5-5")
	want := (3000*h.Rates.Input + 100*h.Rates.Output) / 1e6
	if math.Abs((withHelper-judgeOnly)-want) > 1e-12 || want <= 0 {
		t.Errorf("helper priced at %v, want %v (Haiku list price)", withHelper-judgeOnly, want)
	}

	if strings.Contains(u.Model, "haiku") {
		t.Error("the judged-by model must not name the CLI's helper model")
	}
}

// A CLI answer whose modelUsage has only the judge keeps the reference behavior (no extra usage).
func TestClaudeJudgeOnlyUsageUnchanged(t *testing.T) {
	out := `{"result":"{}","num_turns":1,"usage":{"input_tokens":10,"output_tokens":5},"modelUsage":{"claude-opus-5-5":{}}}`
	_, u, err := parseClaude(procResultOut(0, out), "claude-opus-5-5")
	if err != nil || u.Model != "claude-opus-5-5" || u.Extra != nil {
		t.Fatalf("%v %+v", err, u)
	}
}

// The pre-run estimate is an upper estimate for the input: a cold Claude call is priced at the 1-hour cache
// write rate (what the CLI pays on a first call), so a cold run is not under-estimated and a warm re-run costs less.
func TestEstimateIsUpperForInput(t *testing.T) {
	tok := 50000.0
	est, _ := config.JudgeUSD("claude:opus", EstimateUsage("claude:opus", tok))
	cold, _ := config.JudgeUSD("claude:opus", config.Usage{CacheWrite: tok, CacheWrite1h: tok, Output: OutTokensEst})
	warm, _ := config.JudgeUSD("claude:opus", config.Usage{CacheRead: tok, Output: OutTokensEst})
	plain, _ := config.JudgeUSD("claude:opus", config.Usage{Input: tok, Output: OutTokensEst})
	if est < cold || est < plain || warm >= est {
		t.Errorf("claude estimate %.4f; cold %.4f, plain %.4f, warm re-run %.4f", est, cold, plain, warm)
	}
	cest, _ := config.JudgeUSD("codex:gpt-6.2-astra", EstimateUsage("codex:gpt-6.2-astra", tok))
	cwarm, _ := config.JudgeUSD("codex:gpt-6.2-astra", config.Usage{CacheRead: tok, Output: OutTokensEst})
	if cwarm >= cest {
		t.Errorf("codex estimate %.4f not above a cached re-run %.4f", cest, cwarm)
	}
}
