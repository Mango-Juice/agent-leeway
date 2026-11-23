package report

import (
	"html"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/leak"
)

const month = 30 * 24

var sep1 = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func check(n, serious int, at time.Time, keys ...string) CheckAgg {
	return CheckAgg{Tool: "claude", CheckedAt: at, WindowStart: at.AddDate(0, 0, -30), WindowHours: month, Checked: n, Serious: serious,
		Kinds: map[string]int{"MISTAKE": serious}, Tier: "claude:opus", Model: "claude-opus-5-5", Family: "claude:claude-opus", Rubric: "v2.1-x+t2",
		Rule: 50, Scope: "all", Keys: keys}
}

// The cost side: at least $5 and 10 sessions in each period; a word only past 2 points and past the largest single
// leak of the smaller period; otherwise the neutral line with both rates.
func TestCostRules(t *testing.T) {
	hab := func(usd float64) map[string]float64 { return map[string]float64{"long_break": usd} }
	if c := CostSide(Agg{Total: 100, Leak: 8.6, Sessions: 10, Habits: hab(8)}, Agg{Total: 100, Leak: 15, Sessions: 10, Habits: hab(14)}); c == nil || c.State != "better" || !c.Pill || len(c.Rows) != 1 {
		t.Errorf("better: %+v", c)
	}
	if c := CostSide(Agg{Total: 100, Leak: 9, Sessions: 10}, Agg{Total: 100, Leak: 10.5, Sessions: 10}); c.State != "neutral" || !c.Pill {
		t.Errorf("under 2 points: the neutral line: %+v", c)
	}
	if c := CostSide(Agg{Total: 100, Leak: 8.6, Sessions: 10}, Agg{Total: 100, Leak: 15, Sessions: 10, MaxLeak: 10}); c.State != "neutral" {
		t.Errorf("one large leak: %+v", c)
	}
	if CostSide(Agg{Total: 100, Leak: 9, Sessions: 10}, Agg{Total: 4.9, Leak: 1, Sessions: 10}) != nil ||
		CostSide(Agg{Total: 100, Leak: 9, Sessions: 10}, Agg{Total: 50, Leak: 1, Sessions: 9}) != nil ||
		CostSide(Agg{Total: 100, Leak: 9, Sessions: 9}, Agg{Total: 50, Leak: 1, Sessions: 20}) != nil {
		t.Error("previous spend under $5 or fewer than 10 sessions on either side: no comparison")
	}
	if st := cmpState(0.0951, 0.115, CmpCostSame, 0, shownPct); st != "better" {
		t.Errorf("12%% -> 10%% (raw 1.99 points) reads %s", st)
	}
	if st := cmpState(0.0951, 0.1049, CmpCostSame, 0, shownPct); st != "neutral" {
		t.Errorf("10%% -> 10%%: %s", st)
	}
}

// (b) The word only for a change bigger than normal swings: at n = 50 on each side, 33% -> 21% is not, 40% -> 18% is.
func TestSignificantBoundary(t *testing.T) {
	if Significant(17, 50, 10, 50) { // 34% -> 20%
		t.Error("34% -> 20% at 50/50 is within normal swings")
	}
	if !Significant(9, 50, 20, 50) { // 40% -> 18%
		t.Error("40% -> 18% at 50/50 is a change")
	}
	if Significant(0, 15, 4, 15) { // 0% -> 27% at 15/15: past two standard errors, but Fisher p = 0.10
		t.Errorf("0 of 15 vs 4 of 15 must be neutral (Fisher p %.3f)", FisherP(0, 15, 4, 15))
	}
	if p := FisherP(0, 15, 4, 15); p < 0.09 || p > 0.11 {
		t.Errorf("Fisher p %.4f, want about 0.100", p)
	}
	if p := FisherP(20, 50, 9, 50); p > 0.05 {
		t.Errorf("40%% vs 18%% at 50/50: Fisher p %.4f", p)
	}
	if Significant(0, 20, 0, 20) || Significant(5, 20, 5, 20) || Significant(1, 0, 1, 20) {
		t.Error("no change, or an empty side")
	}
	base, cur := check(50, 20, sep1), check(50, 9, sep1.AddDate(0, 0, 30))
	if c := DriftCompare(&cur, &base); c == nil || c.State != "better" || !c.Pill || c.Now != 0.18 || c.Prev != 0.4 {
		t.Errorf("40%% -> 18%%: %+v", c)
	}
	base.Serious = 17
	cur.Serious = 10
	if c := DriftCompare(&cur, &base); c == nil || c.State != "neutral" || !c.Pill {
		t.Errorf("34%% -> 20%%: the neutral line: %+v", c)
	}
}

// (c) The baseline: the most recent earlier check with the same rule, scope, window length, model family and rubric,
// at most 60 days old, sharing no session; otherwise the reason found on the last check.
func TestPickCheck(t *testing.T) {
	now := sep1.AddDate(0, 0, 30)
	cur := check(20, 4, now, "k3", "k4")
	if _, why, _ := PickCheck(&cur, nil, now); why != "cmp_first" {
		t.Errorf("first visit: %q", why)
	}
	ok := check(20, 8, sep1, "k1", "k2")
	if b, why, _ := PickCheck(&cur, []CheckAgg{ok}, now); b == nil || why != "" {
		t.Errorf("same everything: %v %q", b, why)
	}
	for name, c := range map[string]struct {
		mod  func(*CheckAgg)
		want string
	}{
		"family":  {func(r *CheckAgg) { r.Family = "claude:claude-sonnet" }, "cmp_why_judge"},
		"rubric":  {func(r *CheckAgg) { r.Rubric = "v2.2-y+t2" }, "cmp_why_judge"},
		"61 days": {func(r *CheckAgg) { r.CheckedAt = now.AddDate(0, 0, -61) }, "cmp_why_old"},
		"overlap": {func(r *CheckAgg) { r.Keys = []string{"k1", "k4"} }, "cmp_why_overlap"},
		"scope":   {func(r *CheckAgg) { r.Scope = "project:abc" }, "cmp_why_scope_all"},
		"window":  {func(r *CheckAgg) { r.WindowHours = 336 }, "cmp_why_window"},
		"rule":    {func(r *CheckAgg) { r.Rule = 0 }, "cmp_why_rule"},
		"tool":    {func(r *CheckAgg) { r.Tool = "codex" }, "cmp_first"},
		"folders": {func(r *CheckAgg) { r.Source = "other" }, "cmp_first"},
	} {
		r := ok
		c.mod(&r)
		if b, why, _ := PickCheck(&cur, []CheckAgg{r}, now); b != nil || why != c.want {
			t.Errorf("%s: %v %q, want %q", name, b, why, c.want)
		}
	}
	// the most recent qualifying one wins; one sharing sessions is skipped
	older := check(20, 2, sep1.AddDate(0, 0, -10), "k0")
	same := check(20, 2, now.AddDate(0, 0, -1), "k3")
	if b, _, _ := PickCheck(&cur, []CheckAgg{older, ok, same}, now); b == nil || !b.CheckedAt.Equal(sep1) {
		t.Errorf("most recent qualifying: %+v", b)
	}
	if ModelFamily("claude:opus", "claude-opus-5-5") != ModelFamily("claude:opus", "claude-opus-5-6") ||
		ModelFamily("codex:gpt-6.2-astra", "gpt-6.2-astra") != ModelFamily("codex:gpt-6.3-astra", "gpt-6.3-astra") ||
		ModelFamily("claude:opus", "claude-opus-5-5") == ModelFamily("claude:claude-sonnet-5-5", "claude-sonnet-5-5") {
		t.Error("model family")
	}
}

// (c) Card line and detail by size and model: 14 on one side gives no card line (it needs 15, the detail 10), 15 does;
// a newer exact version of the same model gives the detail only, with its note.
func TestDriftCompareSizes(t *testing.T) {
	base := check(14, 9, sep1)
	cur := check(40, 4, sep1.AddDate(0, 0, 30))
	if c := DriftCompare(&cur, &base); c == nil || c.Pill {
		t.Errorf("14 on one side: detail only: %+v", c)
	}
	base = check(15, 9, sep1)
	if c := DriftCompare(&cur, &base); c == nil || !c.Pill {
		t.Errorf("15 on each side: a card line: %+v", c)
	}
	base.Checked, base.Serious = 9, 4
	if DriftCompare(&cur, &base) != nil {
		t.Error("9 on one side: nothing")
	}
	base = check(30, 12, sep1)
	cur.Model = "claude-opus-5-6"
	if c := DriftCompare(&cur, &base); c == nil || c.Pill || !c.Updated {
		t.Errorf("newer version: %+v", c)
	}
	cur.Model, cur.InWindow, base.InWindow = "claude-opus-5-5", 150, 140
	if c := DriftCompare(&cur, &base); c == nil || c.InWindow != 150 || c.Rows[0].Key != "SERIOUS" || c.Rows[0].Prev != 12 {
		t.Errorf("slice and counts: %+v", c)
	}
}

func cmpBoth(st string) *Compare {
	return &Compare{Hours: month,
		Cost: &CmpSide{State: st, Now: 0.08, Prev: 0.15, NNow: 19, NPrv: 15, Pill: true, Base: BasePrior, Rows: []CmpRow{{Key: "long_break", Now: 7.84, Prev: 14.25, State: st}}},
		Drift: &CmpSide{State: st, Now: 0.21, Prev: 0.33, NNow: 24, NPrv: 21, Pill: true, Base: BaseCheck, BaseAt: sep1, BaseFrom: sep1.AddDate(0, 0, -30), BaseTo: sep1,
			Counts: true, Rows: []CmpRow{{"SERIOUS", 5, 7, "", 24, 21}, {"MISTAKE", 2, 6, "", 24, 21}}}}
}

func storyLang(t *testing.T, lang string) *Story {
	R := leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 8, Count: 4})
	recs := judged(t, 15, 0)
	for i := 0; i < 4; i++ { // four with a problem, so the in-depth block of the whole picture is drawn
		recs = append(recs, withVerdict(rec(t, "p"+itoa(i), "claude", "p", ev{Loc: "U1", Rule: "R2a", Cause: "agent_error", Size: "major", Quote: "x"}), "agent_drift"))
	}
	s := storyOf(t, R, nil, recs...)
	s.M.Lang = lang
	return NewStory(s.M)
}

// Rendering: a word line has its icon and the full sentence as its accessible name; a neutral line has neither, only
// both rates and the baseline; each card says its own baseline and the date line says none; the detail heading has
// the baseline's dates and each side's number; the terminal and Markdown repeat the card lines.
func TestCompareRendering(t *testing.T) {
	old := time.Local
	time.Local = time.UTC
	defer func() { time.Local = old }()
	s := storyLang(t, "en")
	s.M.Compare = cmpBoth("better")
	h := html.UnescapeString(s.HTML())
	for _, want := range []string{
		`<p class="chg better"><span class="ci" role="img" aria-label="Better than the 1 Sep check">✓</span>`,
		`· 33% → 21% · vs 1 Sep check`, `· 15% → 8.0% · vs prior 30 days`, `aria-label="Better than the previous 30 days"`,
		"Compared with the 1 Sep check (2 Aug – 1 Sep): 21 sessions then, 24 now", "Serious misses</span><span class=\"cv\"><b>7 of 21 → 5 of 24</b>",
		"Changes can reflect different work; small ones can be chance.", "Compared with the previous 30 days",
	} {
		if !strings.Contains(h, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(h, "compared with the 30 days before") || strings.Contains(h, `title=`) && strings.Contains(h, `class="chg`) && strings.Contains(h, `chg better" title`) {
		t.Error("the date line no longer carries a comparison note")
	}
	term := strings.Join(strings.Fields(Terminal(s.M, "", "")), " ")
	md := Markdown(s.M)
	for _, want := range []string{"better, 33% → 21% vs 1 Sep check", "better, 15% → 8.0% vs prior 30 days"} {
		if !strings.Contains(term, want) || !strings.Contains(md, want) {
			t.Errorf("terminal/Markdown missing %q", want)
		}
	}
	// neutral
	s.M.Compare = cmpBoth("neutral")
	h = html.UnescapeString(s.HTML())
	if !strings.Contains(h, `<p class="chg neutral"><span class="was">33% → 21% · vs 1 Sep check</span></p>`) || strings.Contains(h, "Similar") {
		t.Errorf("neutral line")
	}
	if term := strings.Join(strings.Fields(Terminal(s.M, "", "")), " "); !strings.Contains(term, "(33% → 21% vs 1 Sep check)") && !strings.Contains(term, "33% → 21% vs 1 Sep check") {
		t.Errorf("terminal neutral: %s", term)
	}
	// a dated report, the selection line, the model note
	c := cmpBoth("better")
	c.Cost.Base, c.Cost.BaseAt, c.Cost.BaseFrom, c.Cost.BaseTo = BaseReport, sep1, sep1.AddDate(0, 0, -30), sep1
	c.Drift.InWindow, c.Drift.Updated, c.Drift.Pill = 150, true, false
	s.M.Compare = c
	h = html.UnescapeString(s.HTML())
	for _, want := range []string{"vs 1 Sep report", "Compared with the 1 Sep report (2 Aug – 1 Sep)", "Each check: the 24 most recent of about 150.", "The judging model was updated."} {
		if !strings.Contains(h, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(h, "vs 1 Sep check") {
		t.Error("model updated: no card line")
	}
	// nothing to compare: one line each
	s.M.Compare = &Compare{Hours: month, CostWhy: "cmp_first", DriftWhy: "cmp_why_scope_one", DriftArgs: map[string]any{"project": "app"}}
	h = html.UnescapeString(s.HTML())
	if strings.Contains(h, `<p class="chg`) || !strings.Contains(h, "Next report compares with this one.") ||
		!strings.Contains(h, "The last check covered all projects; this one covers app only.") {
		t.Error("reasons")
	}
	if md := Markdown(s.M); strings.Contains(md, "Compared with |") {
		t.Error("no card line: no Markdown column")
	}
}

// Korean: the word line, the neutral line, the dated baseline, the detail.
func TestCompareKorean(t *testing.T) {
	old := time.Local
	time.Local = time.UTC
	defer func() { time.Local = old }()
	s := storyLang(t, "ko")
	c := cmpBoth("better")
	c.Drift.State = "neutral"
	c.Cost.Base, c.Cost.BaseAt, c.Cost.BaseFrom, c.Cost.BaseTo = BaseReport, sep1, sep1.AddDate(0, 0, -30), sep1
	s.M.Compare = c
	h := html.UnescapeString(s.HTML())
	for _, want := range []string{`aria-label="9월 1일 리포트보다 나아짐">✓`, `<span aria-hidden="true">나아짐 </span><span class="was">· 15% → 8.0% · 9월 1일 리포트 대비`,
		`<p class="chg neutral"><span class="was">33% → 21% · 9월 1일 점검 대비</span></p>`, "9월 1일 점검(8월 2일 – 9월 1일)과 비교: 그때 세션 21개, 지금 24개", "21개 중 7개 → 24개 중 5개",
		"9월 1일 리포트(8월 2일 – 9월 1일)와 비교", "작은 차이는 우연일 수 있습니다."} {
		if !strings.Contains(h, want) {
			t.Errorf("missing %q", want)
		}
	}
}

// The period's length in the prior label: a day, days, hours; never "1 days".
func TestCompareSpanWording(t *testing.T) {
	for hours, want := range map[int][2]string{24: {"vs prior day", "직전 24시간 대비"}, 36: {"vs prior 36 hours", "직전 36시간 대비"}, 168: {"vs prior 7 days", "직전 7일 대비"}} {
		for i, lang := range []string{"en", "ko"} {
			s := storyLang(t, lang)
			c := cmpBoth("better")
			c.Hours = hours
			s.M.Compare = c
			if h := s.HTML(); !strings.Contains(h, want[i]) || strings.Contains(h, " 1 days") {
				t.Errorf("%d hours, %s: want %q", hours, lang, want[i])
			}
		}
	}
}

// The first-screen budget, 75 English words and 155 Hangul syllables, holds with a word line and a dated label on
// both cards (the longest form). The counts are logged.
func TestCompareFirstScreenCost(t *testing.T) {
	count := func(lang string, c *Compare) int {
		s := storyLang(t, lang)
		s.M.Compare = c
		h := s.HTML()
		i := strings.Index(h, `<section class="sec verdict"`)
		j := strings.Index(h[i:], "</section>")
		txt := html.UnescapeString(regexp.MustCompile(`<[^>]+>`).ReplaceAllString(h[i:i+j], " "))
		if lang == "en" {
			return len(regexp.MustCompile(`[A-Za-z][A-Za-z'’.-]*`).FindAllString(txt, -1))
		}
		return len(regexp.MustCompile(`[가-힣]`).FindAllString(txt, -1))
	}
	longest := cmpBoth("better")
	longest.Cost.Base, longest.Cost.BaseAt = BaseReport, sep1
	for _, lang := range []string{"en", "ko"} {
		budget := map[string]int{"en": 75, "ko": 155}[lang]
		without, with := count(lang, nil), count(lang, longest)
		t.Logf("%s first screen: %d without the comparison, %d with two dated word lines", lang, without, with)
		if with > budget {
			t.Errorf("%s first screen: %d with the comparison (%d without), budget %d", lang, with, without, budget)
		}
	}
}

// Scope reasons: projects first, then automations ("all" against "all+auto" is not "one project").
func TestScopeWhy(t *testing.T) {
	for then, cases := range map[string]map[string]string{
		"all":            {"all+auto": "cmp_why_auto_now", "project:a": "cmp_why_scope_one", "all": ""},
		"all+auto":       {"all": "cmp_why_auto_then", "project:a+auto": "cmp_why_scope_one"},
		"project:a":      {"all": "cmp_why_scope_all", "project:b": "cmp_why_scope_other", "project:a+auto": "cmp_why_auto_now"},
		"project:a+auto": {"project:a": "cmp_why_auto_then"},
	} {
		for now, want := range cases {
			if got := ScopeWhy(then, now); got != want {
				t.Errorf("%s -> %s: %q, want %q", then, now, got, want)
			}
		}
	}
	base, cur := check(20, 8, sep1), check(20, 4, sep1.AddDate(0, 0, 30))
	base.Scope = "all+auto"
	if _, why, _ := PickCheck(&cur, []CheckAgg{base}, sep1.AddDate(0, 0, 30)); why != "cmp_why_auto_then" {
		t.Errorf("automations: %q", why)
	}
}

// The model note: a newer exact version says the judging model was updated; an older or unknown one says only that
// another version was used; neither has a card line.
func TestModelVersionNote(t *testing.T) {
	base, cur := check(30, 12, sep1), check(30, 4, sep1.AddDate(0, 0, 30))
	for model, want := range map[string]string{"claude-opus-5-6": "updated", "claude-opus-5-4": "other", "": "other"} {
		cur.Model = model
		c := DriftCompare(&cur, &base)
		if c == nil || c.Pill || (want == "updated") != c.Updated || (want == "other") != c.OtherModel {
			t.Errorf("%q: %+v", model, c)
		}
	}
	if !newerModel("gpt-6.3-astra", "gpt-6.2-astra") || newerModel("gpt-6.2-astra", "gpt-6.3-astra") || newerModel("x", "y-1") {
		t.Error("version order")
	}
}

// Rows: counts out of each side's sessions, sorted by the change in rate (4 of 21 -> 12 of 63 is no change); a
// neutral cost row has no icon.
func TestDetailRows(t *testing.T) {
	base, cur := check(21, 4, sep1), check(63, 12, sep1.AddDate(0, 0, 30))
	base.Kinds = map[string]int{"MISTAKE": 4, "ASKED_ONLY": 1}
	cur.Kinds = map[string]int{"MISTAKE": 12, "ASKED_ONLY": 20}
	c := DriftCompare(&cur, &base)
	if c.Rows[1].Key != "ASKED_ONLY" {
		t.Errorf("sorted by rate change: %+v", c.Rows)
	}
	s := storyLang(t, "en")
	s.M.Compare = &Compare{Hours: month, Drift: c, Cost: &CmpSide{State: "neutral", Now: 0.1, Prev: 0.105, NNow: 20, NPrv: 20, Pill: true, Base: BasePrior,
		Rows: []CmpRow{{Key: "long_break", Now: 5, Prev: 5.2, State: "neutral"}}}}
	h := html.UnescapeString(s.HTML())
	if !strings.Contains(h, "<b>4 of 21 → 12 of 63</b>") {
		t.Error("count rows")
	}
	if strings.Contains(h, `<li class="neutral"><span class="ci" aria-hidden="true">=</span>`) {
		t.Error("a neutral cost row has no icon")
	}
}

// Two tools, one compared: the note names the other and why.
func TestOtherToolNote(t *testing.T) {
	s := storyLang(t, "en")
	c := cmpBoth("better")
	c.Drift.Tool, c.Drift.Pill, c.Drift.Other, c.Drift.OtherWhy = "claude", false, "codex", "cmp_why_other_day"
	s.M.Compare = c
	if h := html.UnescapeString(s.HTML()); !strings.Contains(h, "Codex is not compared: its last check was made on another day.") {
		t.Error("other tool note")
	}
}
