package drift

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// The grouping prompt is a second prompt sent to the providers: pinned like the judged text. Change it only
// with a new GroupVersion.
const groupPromptSHA256 = "4e770a6264160a762b6c645c61b4851af1925a6baa64d218b65928299ecd67bb"

func TestGroupPromptIsPinned(t *testing.T) {
	sum := sha256.Sum256([]byte(GroupVersion + "\x00" + GroupSystem))
	if got := hex.EncodeToString(sum[:]); got != groupPromptSHA256 {
		t.Fatalf("the grouping prompt changed (sha256 %s, pinned %s); give it a new GroupVersion", got, groupPromptSHA256)
	}
}

// judgedFixture judges the fixture sessions with the fake judge and returns the records.
func judgedFixture(t *testing.T) []py.Value {
	t.Helper()
	en, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "judge"))
	return judgeWith(t, en)
}

// groupWith runs the grouping step with the fake CLI answering text (mode "text"), or another fake mode.
func groupWith(t *testing.T, cache, mode, answer string, recs []py.Value, o Options) (GroupOutcome, []string) {
	t.Helper()
	useFake(t, mode)
	dir := t.TempDir()
	ans, req := filepath.Join(dir, "answer.txt"), filepath.Join(dir, "requests.txt")
	_ = os.WriteFile(ans, []byte(answer), 0o600)
	t.Setenv("LEEWAY_FAKE_TEXT", ans)
	t.Setenv("LEEWAY_FAKE_REQ", req)
	out := GroupAll(context.Background(), cache, recs, map[string]string{"claude": "claude:opus", "codex": "codex:gpt-6.2-astra"}, o, 0)
	b, _ := os.ReadFile(req)
	var reqs []string
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if l != "" {
			reqs = append(reqs, l)
		}
	}
	return out, reqs
}

func groupsBy(t *testing.T, cache string) map[string]py.Value {
	t.Helper()
	out := map[string]py.Value{}
	for _, r := range LoadGroups(cache) {
		out[r.Get("tool").S] = r
	}
	return out
}

func TestGroupingGoodAnswer(t *testing.T) {
	recs := judgedFixture(t)
	cache := t.TempDir()
	// e1..e4 are Claude problems from four sessions; e5 shares a session with e4
	ans := `Here: {"themes": [{"label": "표는 그대로 두기", "events": ["e1", "e2", "e3", "e4"], "rule": "표는 지우거나 바꾸지 말 것.",
		  "stated": {"e1": "keep the table as it is", "e2": "표는 그대로 둬", "e3": "deja la tabla", "e4": "그대로 유지할 것"}},
		{"label": "chart", "events": ["e5"], "stated": {"e5": "차트를 추가해 줘"}, "rule": null}]}`
	out, reqs := groupWith(t, cache, "text", ans, recs, Options{})
	if out.Stop != "" || len(reqs) != 2 {
		t.Fatalf("stop %q, %d calls: %v", out.Stop, len(reqs), out.Lines)
	}
	// each provider's call carries only its own sessions, and nothing beyond the judged fields
	for _, line := range reqs {
		req, _ := py.Loads(line)
		r := req.Get("prompt").S
		isClaude := req.Get("tier").S == "claude:opus"
		if req.Get("system").S != GroupSystem {
			t.Error("the grouping call did not use the grouping prompt")
		}
		if isClaude == strings.Contains(r, "실패하는 테스트를 고쳐") || isClaude != strings.Contains(r, "표는 그대로 둬") {
			t.Errorf("a call carries the other provider's events: %s", py.Head(r, 200))
		}
		if strings.Contains(r, "0000-4000-8000") {
			t.Error("session keys were sent; only short ids should be")
		}
	}
	g := groupsBy(t, cache)["claude"]
	th := g.Get("themes").A
	if len(th) != 1 || th[0].Get("label").S != "표는 그대로 두기" || len(th[0].Get("events").A) != 4 || th[0].Get("draft_rule").S == "" ||
		th[0].Get("n_sessions").I != 4 || th[0].Get("tool").S != "claude" || !strings.HasPrefix(th[0].Get("events").A[0].S, "claude:00000000-0000-4000-8000-000000000101#") {
		t.Fatalf("themes: %s", py.Dumps(g.Get("themes")))
	}
	if d := g.Get("dropped").A; len(d) != 1 || d[0].Get("reason").S != "fewer than two sessions" {
		t.Errorf("dropped: %s", py.Dumps(g.Get("dropped")))
	}
	if g.Get("group_version").S != GroupVersion || g.Get("input_hash").S == "" || g.Get("n_events").I != 5 {
		t.Errorf("record: %s", py.Head(py.Dumps(g), 300))
	}
	if _, ok := g.Get("usd").Num(); !ok || out.Spent <= 0 {
		t.Errorf("cost not counted: %v %v", g.Get("usd"), out.Spent)
	}
	// unchanged input: no second call
	_, reqs = groupWith(t, cache, "text", ans, recs, Options{})
	if len(reqs) != 0 {
		t.Errorf("unchanged input was sent again (%d calls)", len(reqs))
	}
}

func TestGroupingChecks(t *testing.T) {
	evs := []GroupEvent{{ID: "e1", Session: "s1", Quotes: []string{"keep it short"}}, {ID: "e2", Session: "s2", Instruction: "Keep it short, please"},
		{ID: "e3", Session: "s2", Quotes: []string{"short again"}}, {ID: "e4", Session: "s3", Quotes: []string{"short answers only"}}}
	long := strings.Repeat("가", MaxThemeLabel+1)
	st := `"stated": {"e1": "keep it short", "e2": "keep it short", "e3": "short again", "e4": "short answers"}`
	themes, dropped, ok := CheckThemes(`{"themes": [
		{"label": "a", "events": ["e1", "e9"], `+st+`},
		{"label": "b", "events": ["e2", "e3"], `+st+`},
		{"label": "c", "events": ["e1", "e2", "e1"], "rule": "Keep it.", `+st+`},
		{"label": "d", "events": ["e2", "e4"], `+st+`},
		{"label": "`+long+`", "events": ["e3", "e4"], `+st+`},
		{"label": "", "events": ["e3", "e4"], `+st+`},
		{"label": "e", "events": ["e3", "e4"], "rule": "`+strings.Repeat("x", MaxThemeRule+1)+`", `+st+`}]}`, evs)
	if !ok {
		t.Fatal("not parsed")
	}
	reasons := map[string]string{}
	for _, d := range dropped {
		reasons[d.Label] += d.Reason
	}
	if !strings.Contains(reasons["a"], "unknown event id") || !strings.Contains(reasons["b"], "fewer than two sessions") ||
		!strings.Contains(reasons["d"], "already in an earlier theme") || !strings.Contains(reasons[long], "label longer") ||
		!strings.Contains(reasons[""], "no label") || !strings.Contains(reasons["e"], "rule longer") {
		t.Errorf("drop reasons: %v", reasons)
	}
	if len(themes) != 2 || themes[0].Label != "c" || strings.Join(themes[0].Events, ",") != "e1,e2" || themes[1].Label != "e" || themes[1].Rule != "" {
		t.Errorf("kept: %+v", themes)
	}
	if _, _, ok := CheckThemes(`{"themes": "none"}`, evs); ok {
		t.Error("a non-list must fail")
	}
}

// g2: a theme stands only on the person's own words. Members whose "stated" words are missing, or are not in
// their own quotes or cited instruction, do not count; a theme left with fewer than two sessions is dropped.
func TestGroupingNeedsStatedWords(t *testing.T) {
	// the failure class of docs/27: agent mistakes about different facts, the person only corrected each fact
	evs := []GroupEvent{
		{ID: "e1", Session: "s1", Cause: "agent_error", Quotes: []string{"The config lives in src/, not lib/."}, Summary: "The agent claimed the config was in lib/ without checking."},
		{ID: "e2", Session: "s2", Cause: "agent_error", Quotes: []string{"That helper was removed last week."}, Summary: "The agent said a helper still existed without checking."},
		{ID: "e3", Session: "s3", Cause: "agent_error", Quotes: []string{"마이그레이션은 어제 내가 이미 돌렸어."}, Summary: "에이전트가 확인하지 않고 마이그레이션을 안 돌렸다고 가정했다."},
	}
	for _, ans := range []string{
		`{"themes": [{"label": "Check before asserting", "events": ["e1", "e2", "e3"], "rule": "Check before you assert anything."}]}`,
		`{"themes": [{"label": "Check before asserting", "events": ["e1", "e2", "e3"], "stated": {"e1": "check before asserting", "e2": "check first", "e3": "확인하고 말해"}}]}`,
	} {
		themes, dropped, ok := CheckThemes(ans, evs)
		if !ok || len(themes) != 0 || len(dropped) != 1 || !strings.Contains(dropped[0].Reason, "not stated in the person's own words") {
			t.Errorf("a mistake-kind theme was kept: %+v %+v", themes, dropped)
		}
	}
	// the person did restate the request (an agent_error theme can still be right): kept, with a member without
	// matching words removed and recorded
	evs2 := []GroupEvent{
		{ID: "e1", Session: "s1", Cause: "agent_error", Quotes: []string{"한국어로 답해 줘."}},
		{ID: "e2", Session: "s2", Cause: "agent_error", Quotes: []string{"Please  answer in KOREAN again"}},
		{ID: "e3", Session: "s3", Cause: "agent_error", Quotes: []string{"the date is wrong"}},
	}
	themes, dropped, _ := CheckThemes(`{"themes": [{"label": "한국어로 답하기", "events": ["e1", "e2", "e3"],
		"stated": {"e1": "한국어로 답해 줘", "e2": "answer in korean", "e3": "answer in Korean"}}]}`, evs2)
	if len(themes) != 1 || strings.Join(themes[0].Events, ",") != "e1,e2" || len(dropped) != 1 || !strings.Contains(dropped[0].Reason, "1 member(s) removed") {
		t.Errorf("restated request: %+v %+v", themes, dropped)
	}
}

// The person's words only: evidence quoted from the agent's turns (A#) is not sent as what the person said.
func TestGroupInputSendsOnlyThePersonsWords(t *testing.T) {
	r, _ := py.Loads(`{"key": "claude:x", "tool": "claude", "tier": "claude:opus", "error": null, "out": {"events": [
		{"loc": "U3", "rule": "R2a", "cause": "agent_error", "summary": "s", "evidence": [{"loc": "A2", "quote": "I checked everything"}, {"loc": "U3", "quote": "you did not check"}]}]}}`)
	evs := GroupInput([]py.Value{r}, "claude")
	if len(evs) != 1 || strings.Join(evs[0].Quotes, "|") != "you did not check" || strings.Contains(GroupPrompt(evs), "I checked everything") {
		t.Errorf("input: %+v", evs)
	}
}

func TestGroupingInvalidIdsAndSingleSession(t *testing.T) {
	recs := judgedFixture(t)
	cache := t.TempDir()
	groupWith(t, cache, "text", `{"themes": [{"label": "x", "events": ["e1", "e77"], "stated": {"e1": "keep the table"}}, {"label": "y", "events": ["e4", "e5"], "stated": {"e4": "그대로 유지할 것", "e5": "차트를 추가해 줘"}}]}`, recs, Options{})
	g := groupsBy(t, cache)["claude"]
	if len(g.Get("themes").A) != 0 || len(g.Get("dropped").A) != 2 || g.Get("error").Truthy() {
		t.Fatalf("record: %s", py.Head(py.Dumps(g), 400))
	}
}

func TestGroupingMalformedAndFailures(t *testing.T) {
	recs := judgedFixture(t)
	cache := t.TempDir()
	out, _ := groupWith(t, cache, "text", "not json at all", recs, Options{})
	if g := groupsBy(t, cache)["claude"]; g.Get("error").S != "PARSE_ERROR" || out.Stop != "" {
		t.Errorf("malformed: %v %q", g.Get("error"), out.Stop)
	}
	if len(LatestGroups(LoadGroups(cache))) != 0 {
		t.Error("a failed grouping must not be used")
	}
	// a failed answer is tried again next time (no cache hit on an error)
	_, reqs := groupWith(t, cache, "text", "still not json", recs, Options{})
	if len(reqs) != 2 {
		t.Errorf("a failed grouping was not retried: %d calls", len(reqs))
	}
	cache2 := t.TempDir()
	groupWith(t, cache2, "fail", "", recs, Options{})
	if g := groupsBy(t, cache2)["claude"]; !g.Get("error").Truthy() {
		t.Errorf("CLI failure not recorded: %s", py.Dumps(g))
	}
	cache3 := t.TempDir()
	out, _ = groupWith(t, cache3, "limit", "", recs, Options{})
	if !strings.Contains(out.Stop, "usage limit") || len(LoadGroups(cache3)) != 0 {
		t.Errorf("limit: stop %q, %d records", out.Stop, len(LoadGroups(cache3)))
	}
}

func TestGroupingBudget(t *testing.T) {
	recs := judgedFixture(t)
	cache := t.TempDir()
	out, reqs := groupWith(t, cache, "text", `{"themes": []}`, recs, Options{Budget: 0.0001})
	if len(reqs) != 0 || !strings.Contains(strings.Join(out.Lines, "\n"), "budget") {
		t.Errorf("budget not respected: %d calls, %v", len(reqs), out.Lines)
	}
}

// The judge input is untouched by the grouping step.
func TestGroupingDoesNotChangeJudgeText(t *testing.T) {
	rb, _ := LoadRubric("")
	if strings.Contains(BuildSystem(rb.Body, rb.Settings), "themes") {
		t.Error("the grouping instruction leaked into the judge's system prompt")
	}
}
