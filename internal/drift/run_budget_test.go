package drift

import (
	"strings"
	"testing"
)

func TestDefaultMaxSessionsIs50(t *testing.T) {
	if DefaultMaxSessions != 50 {
		t.Fatalf("default --max-sessions %v, want 50", DefaultMaxSessions)
	}
}

func TestPlanLinesSessionsFirst(t *testing.T) {
	items := []Item{{Key: "newest", EstUSD: 10}, {Key: "b", EstUSD: 10}, {Key: "c", EstUSD: 10}, {Key: "oldest", EstUSD: 0.5}}
	if n := BudgetFit(items, 30); n != 3 {
		t.Errorf("fit %d, want 3 (in order; the cheap oldest one does not jump the queue)", n)
	}
	if n := BudgetFit(items, 0); n != 4 {
		t.Errorf("no budget: fit %d, want 4", n)
	}
	// the default 50 cuts a larger window: sessions first, one line on how to judge more
	l := PlanLines(120, 50, items, 0)
	joined := strings.Join(l, "\n")
	for _, want := range []string{"120 sessions in the window; the 4 most recent will be judged (one judge call per session)",
		"On a subscription this counts against your plan's usage; with an API key it costs about $30.50",
		"Only the 50 most recent of the 120 sessions are taken", "--max-sessions 0 for all 120"} {
		if !strings.Contains(joined, want) {
			t.Errorf("plan lines lack %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "budget") {
		t.Errorf("no budget line without --budget:\n%s", joined)
	}
	// --budget, when given, still cuts the run and says so
	b := strings.Join(PlanLines(4, 50, items, 30), "\n")
	if !strings.Contains(b, "the 3 most recent will be judged") || !strings.Contains(b, "--budget $30.00") || !strings.Contains(b, "after about 3 of the 4") {
		t.Errorf("budget line:\n%s", b)
	}
	// nothing cut: one line only
	if l := PlanLines(4, 50, items, 0); len(l) != 1 {
		t.Errorf("no cut expected: %q", l)
	}
}
