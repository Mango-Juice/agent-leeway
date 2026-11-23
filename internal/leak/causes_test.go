package leak

import (
	"strings"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// Two `leak --json` files that counted different sets of causes: each cause is compared on its own, the total and
// the habits are not (a 0.1.7 file has no screenshots cause).
func TestCompareAcrossCauseSets(t *testing.T) {
	doc := func(set int, image bool) py.Value {
		causes := `{"cause": "expiry", "usd": 2, "count": 1}`
		leakUSD := 2.0
		if image {
			causes += `, {"cause": "image", "usd": 3, "count": 4}`
			leakUSD = 5
		}
		s := `{"window_start": "2026-10-01T00:00:00+00:00", "window_end": "2026-10-31T00:00:00+00:00", "total_usd": 100, "leak_usd": ` +
			py.PyStr(py.Value{K: py.Float, F: leakUSD}) + `, "causes": [` + causes + `], "habits": []`
		if set > 1 {
			s += `, "cause_set": ` + py.PyStr(py.Value{K: py.Int, I: int64(set)})
		}
		v, err := py.Loads(s + `}`)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	out := Compare(doc(1, false), doc(CauseSet, true))
	if !strings.Contains(out, messages.Compare["warn_causes"]) || strings.Contains(out, ")  total") {
		t.Errorf("different sets:\n%s", out)
	}
	out = Compare(doc(CauseSet, true), doc(CauseSet, true))
	if strings.Contains(out, messages.Compare["warn_causes"]) || !strings.Contains(out, ")  total") {
		t.Errorf("same set:\n%s", out)
	}
}
