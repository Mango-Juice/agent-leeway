package pipeline

import (
	"math"
	"testing"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/logs"
)

func thread(tool, id string, steps ...logs.Step) *logs.File {
	for i := range steps {
		steps[i].Seq = i
		steps[i].InputTotal = steps[i].Uncached + steps[i].Cached + steps[i].CacheWrite
	}
	return &logs.File{Thread: logs.Thread{Tool: tool, ThreadID: id, SessionID: logs.Some(id), Source: logs.Some("user"), Start: steps[0].TS, End: steps[len(steps)-1].TS,
		Model: steps[0].Model}, Steps: steps}
}

var t0 = time.Date(2026, 1, 10, 9, 0, 0, 0, time.UTC).UnixNano()

func at(min float64) int64 { return t0 + int64(min*float64(time.Minute)) }

func TestEachResponsePricedByItsOwnModel(t *testing.T) {
	tb := FromFiles([]*logs.File{thread("claude", "c",
		logs.Step{TS: at(0), Model: logs.Some("claude-haiku-5-5"), Uncached: 50000},
		logs.Step{TS: at(1), Model: logs.Some("claude-opus-5-5"), Uncached: 50000},
		logs.Step{TS: at(2), Model: logs.Some("claude-3-weird"), Uncached: 10})})
	tb.Build()
	if math.Abs(tb.Steps[0].WCost-0.005) > 1e-12 || math.Abs(tb.Steps[1].WCost-0.2) > 1e-12 {
		t.Errorf("haiku then opus: %v, %v (the thread's first model must not price the switch)", tb.Steps[0].WCost, tb.Steps[1].WCost)
	}
	if tb.Steps[2].Assumed != "claude-3-weird" || tb.Steps[1].Assumed != "" {
		t.Errorf("assumed: %q %q", tb.Steps[2].Assumed, tb.Steps[1].Assumed)
	}
}

func TestCacheLifetimeFromRecordedWrites(t *testing.T) {
	// Claude: a 5-minute write expires after 10 minutes, a 1-hour write does not
	tb := FromFiles([]*logs.File{thread("claude", "c",
		logs.Step{TS: at(0), Model: logs.Some("claude-opus-5-5"), CacheWrite: 10000, CW5m: 10000},
		logs.Step{TS: at(10), Model: logs.Some("claude-opus-5-5"), Cached: 0, CacheWrite: 10000, CW1h: 10000},
		logs.Step{TS: at(20), Model: logs.Some("claude-opus-5-5"), Cached: 0, CacheWrite: 10000, CW1h: 10000}),
		// Codex GPT-5.6+: 30 minutes
		thread("codex", "x",
			logs.Step{TS: at(0), Model: logs.Some("gpt-6-astra"), Uncached: 10000},
			logs.Step{TS: at(20), Model: logs.Some("gpt-6-astra"), Uncached: 10000},
			logs.Step{TS: at(60), Model: logs.Some("gpt-6-astra"), Uncached: 10000})})
	tb.Build()
	exp := map[string][]bool{}
	for _, w := range tb.Waste {
		exp[w.ThreadID] = append(exp[w.ThreadID], w.MissExpiry > 0)
	}
	if got := exp["c"]; len(got) != 3 || !got[1] || got[2] {
		t.Errorf("claude expiries %v: want the 5m write to expire at 10 min and the 1h write not at 10 min", got)
	}
	if got := exp["x"]; len(got) != 3 || got[1] || !got[2] {
		t.Errorf("codex expiries %v: want no expiry at 20 min and one at 40 min", got)
	}
	// the 5-minute write is priced at 1.25x input, the 1-hour write at 2x
	if math.Abs(tb.Steps[0].PWrite-5) > 1e-9 || math.Abs(tb.Steps[1].PWrite-8) > 1e-9 {
		t.Errorf("write rates %v %v", tb.Steps[0].PWrite, tb.Steps[1].PWrite)
	}
}
