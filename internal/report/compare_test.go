package report

import (
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/leak"
)

func recs(n, serious int, tool, tier, ver string) []*Record {
	var out []*Record
	for i := 0; i < n; i++ {
		v := "as_intended"
		if i < serious {
			v = "agent_drift"
		}
		r := &Record{Key: tool + ":" + tier + string(rune('a'+i)), Tool: tool, Tier: tier, RubricVersion: ver, Verdict: v}
		if i == 0 {
			r.Events = []*Event{{Rec: r, Kind: "BROKE_SAID", InstrLoc: "U1"}}
		}
		out = append(out, r)
	}
	return out
}

const month = 30 * 24

// The rules in docs/how-it-works.md: states and thresholds; the minimums; a different judge, model or rubric;
// different tools; coverage; one tool named.
func TestCompareRules(t *testing.T) {
	hab := func(usd float64) map[string]float64 { return map[string]float64{"long_break": usd} }
	if c := CostSide(Agg{Total: 100, Leak: 8.6, Sessions: 10, Habits: hab(8)}, Agg{Total: 100, Leak: 15, Sessions: 10, Habits: hab(14)}); c == nil || c.State != "better" || len(c.Rows) != 1 || c.Rows[0].State != "better" {
		t.Errorf("better: %+v", c)
	}
	if c := CostSide(Agg{Total: 100, Leak: 9, Sessions: 10}, Agg{Total: 100, Leak: 10.5, Sessions: 10}); c.State != "same" {
		t.Errorf("under 2 points is similar: %+v", c)
	}
	if CostSide(Agg{Total: 100, Leak: 9, Sessions: 10}, Agg{Total: 4.9, Leak: 1, Sessions: 10}) != nil ||
		CostSide(Agg{Total: 100, Leak: 9, Sessions: 10}, Agg{Total: 50, Leak: 1, Sessions: 2}) != nil {
		t.Error("previous spend under $5 or fewer than 3 sessions: no comparison")
	}
	// rows under $0.50 per $100 in both periods are left out
	if c := CostSide(Agg{Total: 100, Leak: 9, Sessions: 10, Habits: map[string]float64{"x": 0.4}}, Agg{Total: 100, Leak: 9, Sessions: 10, Habits: map[string]float64{"x": 0.3}}); len(c.Rows) != 0 {
		t.Errorf("tiny rows: %+v", c.Rows)
	}
	now, prev := recs(19, 4, "claude", "claude:opus", "v2"), recs(12, 4, "claude", "claude:opus", "v2")
	c, why := DriftSide(now, prev, map[string]int{"claude": 19}, map[string]int{"claude": 12})
	if c == nil || c.State != "better" || why != "" || c.Rows[0].Key != "BROKE_CHAT" {
		t.Errorf("drift better: %+v %q", c, why)
	}
	if _, why := DriftSide(now, recs(12, 4, "claude", "claude:sonnet", "v2"), nil, nil); why != "cmp_why_judge" {
		t.Errorf("different judge: %q", why)
	}
	if _, why := DriftSide(now, recs(12, 4, "claude", "claude:opus", "v1"), nil, nil); why != "cmp_why_judge" {
		t.Errorf("different rubric: %q", why)
	}
	// the same tier, but the CLI reported another exact model
	other := recs(12, 4, "claude", "claude:opus", "v2")
	for _, r := range other {
		r.Model = "claude-opus-5-1"
	}
	for _, r := range now {
		r.Model = "claude-opus-5-5"
	}
	if _, why := DriftSide(now, other, nil, nil); why != "cmp_why_judge" {
		t.Errorf("different exact model: %q", why)
	}
	for _, r := range now {
		r.Model = ""
	}
	if _, why := DriftSide(now, recs(7, 1, "claude", "claude:opus", "v2"), nil, nil); why != "cmp_why_few" {
		t.Errorf("fewer than 8 in the previous period: %q", why)
	}
	if _, why := DriftSide(recs(7, 1, "claude", "claude:opus", "v2"), prev, nil, nil); why != "cmp_why_few" {
		t.Errorf("fewer than 8 in this period: %q", why)
	}
	if _, why := DriftSide(now, recs(12, 4, "codex", "codex:gpt-6.2-astra", "v2"), nil, nil); why != "cmp_why_tools" {
		t.Errorf("different tools: %q", why)
	}
	if _, why := DriftSide(now, prev, map[string]int{"claude": 19}, map[string]int{"claude": 40}); why != "cmp_why_partial" {
		t.Errorf("under 60%% checked: %q", why)
	}
	both := append(append([]*Record{}, now...), recs(9, 1, "codex", "codex:gpt-6.2-astra", "v2")...)
	if c, _ := DriftSide(both, prev, nil, nil); c == nil || c.Tool != "claude" {
		t.Errorf("only the tool checked in both periods, named: %+v", c)
	}
}

// The noise rule: better or worse only beyond the threshold and beyond what one session (in-depth)
// or the largest single leak (cost) of the smaller period moves.
func TestCompareNoise(t *testing.T) {
	// 1 of 19 against 1 of 5: 15 points, past the 8-point threshold, but one session of five is 20 points
	if st := cmpState(1.0/19, 1.0/5, CmpDriftSame, 1.0/5, shownPct); st != "same" {
		t.Errorf("1/19 vs 1/5: %s", st)
	}
	if _, why := DriftSide(recs(19, 1, "claude", "claude:opus", "v2"), recs(5, 1, "claude", "claude:opus", "v2"), nil, nil); why != "cmp_why_few" {
		t.Errorf("1/19 vs 1/5: five checked sessions are under the minimum of 8: %q", why)
	}
	// 0 of 19 against 1 of 8: exactly one session's worth; 0 of 19 against 2 of 8: more than one
	if c, _ := DriftSide(recs(19, 0, "claude", "claude:opus", "v2"), recs(8, 1, "claude", "claude:opus", "v2"), nil, nil); c == nil || c.State != "same" {
		t.Errorf("0/19 vs 1/8: %+v", c)
	}
	if c, _ := DriftSide(recs(19, 0, "claude", "claude:opus", "v2"), recs(8, 2, "claude", "claude:opus", "v2"), nil, nil); c == nil || c.State != "better" {
		t.Errorf("0/19 vs 2/8: %+v", c)
	}
	// cost: 15% -> 8.6% is past 2 points, but one session leaked $10 of the previous $100
	if c := CostSide(Agg{Total: 100, Leak: 8.6, Sessions: 10}, Agg{Total: 100, Leak: 15, Sessions: 10, MaxLeak: 10}); c.State != "same" {
		t.Errorf("one large leak: %+v", c)
	}
	if c := CostSide(Agg{Total: 100, Leak: 8.6, Sessions: 10, MaxLeak: 2}, Agg{Total: 100, Leak: 15, Sessions: 10, MaxLeak: 3}); c.State != "better" {
		t.Errorf("spread leaks: %+v", c)
	}
}

// A14: the state is computed on the values as shown, so the pill never says "Similar" between two rates two
// points apart, nor "Better" between two equal ones.
func TestCompareStateOnShownValues(t *testing.T) {
	if cmpPct(0.115) != "12%" || cmpPct(0.0951) != "10%" {
		t.Fatalf("fixture: %s %s", cmpPct(0.115), cmpPct(0.0951))
	}
	if st := cmpState(0.0951, 0.115, CmpCostSame, 0, shownPct); st != "better" {
		t.Errorf("12%% -> 10%% (raw 1.99 points) reads %s", st)
	}
	if st := cmpState(0.0951, 0.1049, CmpCostSame, 0, shownPct); st != "same" {
		t.Errorf("10%% -> 10%%: %s", st)
	}
	// raw 0.992 apart, shown $8.84 -> $7.84: a $1.00 change as printed
	if st := cmpState(7.844, 8.836, CmpPer100Same, 0, shownMoney); st != "better" {
		t.Errorf("$8.84 -> $7.84: %s", st)
	}
}

func cmpBoth(st string) *Compare {
	return &Compare{Hours: month, Cost: &CmpSide{State: st, Now: 0.08, Prev: 0.15, Rows: []CmpRow{{"long_break", 7.84, 14.25, st}}},
		Drift: &CmpSide{State: st, Now: 0.21, Prev: 0.33, Rows: []CmpRow{{"BROKE_CHAT", 0.16, 0, st}}}}
}

// Rendering: each state with its icon and word on the card, the full sentence read once by screen readers;
// nothing on the first screen, in the terminal or in Markdown when a side is nil or covers one tool only; the
// reason or the tool in the whole picture instead.
func TestCompareRendering(t *testing.T) {
	R := leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 8, Count: 4})
	for st, icon := range map[string]string{"better": "✓", "worse": "!", "same": "="} {
		s := storyOf(t, R, nil, judged(t, 19, 4)...)
		s.M.Compare = cmpBoth(st)
		h := s.HTML()
		if strings.Count(h, `<p class="chg `+st+`">`) != 2 || !strings.Contains(h, `role="img" aria-label="`+s.T("cmp_"+st, "span", "30 days")+`">`+icon) ||
			!strings.Contains(h, `<span aria-hidden="true">`+s.T("cmp_"+st+"_s")+` </span>`) ||
			!strings.Contains(h, "compared with the 30 days before") || !strings.Contains(h, `class="cmp8"`) {
			t.Errorf("%s not rendered", st)
		}
		if strings.Contains(h, `class="chg `+st+`" title=`) {
			t.Errorf("%s: a title would be read a second time", st)
		}
		term := strings.Join(strings.Fields(Terminal(s.M, "", "")), " ")
		if !strings.Contains(term, s.T("cmp_word_"+st)+" (33% → 21%)") || !strings.Contains(term, "compared with the 30 days before") {
			t.Errorf("%s: terminal short form or its baseline missing", st)
		}
		md := Markdown(s.M)
		if !strings.Contains(md, "| vs previous 30 days |") || !strings.Contains(md, s.T("cmp_word_"+st)+" (15% → 8.0%) |") {
			t.Errorf("%s: Markdown column missing", st)
		}
	}
	// habit and problem-kind rows in the detail, the unit written once
	s := storyOf(t, R, nil, judged(t, 19, 4)...)
	s.M.Compare = cmpBoth("better")
	h := s.HTML()
	// (the made-up records have no problems, so the in-depth block and its rows are not drawn here; the cost
	// block's rows use the same code)
	for _, want := range []string{"$7.84 per $100 spent</b> · was $14.25", "Money wasted by habits"} {
		if !strings.Contains(html.UnescapeString(h), want) {
			t.Errorf("detail row missing: %s", want)
		}
	}
	s.M.Compare = &Compare{Hours: month, CostWhy: "cmp_why_cost_few", DriftWhy: "cmp_why_few"}
	h = s.HTML()
	if strings.Contains(h, `<p class="chg`) || strings.Contains(h, "compared with the 30 days before") || !strings.Contains(h, E(s.T("cmp_why_cost_few", "span", "30 days"))) {
		t.Error("nil sides: nothing on the first screen, the reason in the whole picture")
	}
	if md := Markdown(s.M); strings.Contains(md, "vs previous") {
		t.Error("nil sides: no Markdown column")
	}
	s.M.Compare = &Compare{Hours: month, Cost: &CmpSide{State: "better", Now: 0.04, Prev: 0.1, Tool: "codex"}, CostWhy: "cmp_why_cost_deleted"}
	h = s.HTML()
	term := strings.Join(strings.Fields(Terminal(s.M, "", "")), " ")
	md := Markdown(s.M)
	if strings.Contains(h, `<p class="chg`) || !strings.Contains(h, s.T("cmp_head_tool", "span", "30 days", "tool", "Codex")) || !strings.Contains(h, s.T("cmp_why_cost_deleted")) {
		t.Error("one tool only: no card pill, named in the detail")
	}
	if strings.Contains(term, "10% → 4.0%") || strings.Contains(term, "compared with") || strings.Contains(md, "vs previous") {
		t.Errorf("one tool only: nothing in the terminal or Markdown summary:\n%s", term)
	}
}

// A9: the period's length is said correctly: a day, days, hours; never "1 days".
func TestCompareSpanWording(t *testing.T) {
	R := leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 8, Count: 4})
	for hours, want := range map[int][2]string{24: {"compared with the day before", "직전 24시간과 비교"}, 36: {"compared with the 36 hours before", "직전 36시간과 비교"},
		168: {"compared with the 7 days before", "직전 7일과 비교"}} {
		for i, lang := range []string{"en", "ko"} {
			s := storyOf(t, R, nil, judged(t, 19, 4)...)
			s.M.Lang = lang
			s = NewStory(s.M)
			c := cmpBoth("better")
			c.Hours = hours
			s.M.Compare = c
			h := s.HTML()
			if !strings.Contains(h, want[i]) || strings.Contains(h, " 1 days") || strings.Contains(h, "previous 1 ") {
				t.Errorf("%d hours, %s: want %q", hours, lang, want[i])
			}
		}
	}
}

// Korean pill and detail.
func TestCompareKorean(t *testing.T) {
	R := leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 8, Count: 4})
	s := storyOf(t, R, nil, judged(t, 19, 4)...)
	s.M.Lang = "ko"
	s = NewStory(s.M)
	s.M.Compare = cmpBoth("worse")
	h := html.UnescapeString(s.HTML())
	for _, want := range []string{`aria-label="직전 30일보다 나빠짐">!`, `<span aria-hidden="true">나빠짐 </span><span class="was">· 15% → 8.0%`, "직전 30일과 비교",
		"$100당 $7.84</b> · 전에는 $14.25"} {
		if !strings.Contains(h, want) {
			t.Errorf("Korean: missing %s", want)
		}
	}
	s.M.Compare = &Compare{Hours: month, Cost: &CmpSide{State: "better", Now: 0.04, Prev: 0.1, Tool: "codex"}, CostWhy: "cmp_why_cost_deleted"}
	h = s.HTML()
	if !strings.Contains(h, "직전 30일과 비교(Codex만)") || !strings.Contains(h, "Claude Code가 오래된 로그를 지워서 비용은 Codex만 비교했습니다.") {
		t.Error("Korean one-tool detail")
	}
}

// The first-screen budget, 75 English words and 155 Hangul syllables, holds with the comparison present. The
// comparison itself adds at most 7 words and 12 syllables.
func TestCompareFirstScreenCost(t *testing.T) {
	R := leakReport(100, leak.Habit{Habit: "long_break", Causes: []string{"expiry"}, USD: 8, Count: 4})
	count := func(lang string, c *Compare) int {
		s := storyOf(t, R, nil, judged(t, 19, 4)...)
		s.M.Lang = lang
		s = NewStory(s.M)
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
	for _, lang := range []string{"en", "ko"} {
		add, budget := map[string]int{"en": 7, "ko": 12}[lang], map[string]int{"en": 75, "ko": 155}[lang]
		without, with := count(lang, nil), count(lang, cmpBoth("better"))
		if with-without > add {
			t.Errorf("%s: the comparison adds %d (max %d)", lang, with-without, add)
		}
		if with > budget {
			t.Errorf("%s first screen: %d with the comparison (%d without), budget %d", lang, with, without, budget)
		}
	}
}
