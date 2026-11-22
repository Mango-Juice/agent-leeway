package config

import (
	"strings"
	"testing"
	"time"
)

func TestPriceTableRowsHaveSources(t *testing.T) {
	p := Prices()
	if len(p.Models) < 30 || p.Read == "" {
		t.Fatalf("%d rows, read %q", len(p.Models), p.Read)
	}
	for _, m := range p.Models {
		if m.Source == "" || m.Read == "" || m.Input <= 0 || m.Output <= 0 {
			t.Errorf("%s: incomplete row %+v", m.ID, m)
		}
		if m.Provider == "anthropic" && (m.CacheWrite5m <= 0 || m.CacheWrite1h <= 0) {
			t.Errorf("%s: Claude rows need both write rates", m.ID)
		}
		if strings.HasPrefix(m.ID, "gpt-5.6") || strings.HasPrefix(m.ID, "gpt-6") {
			if m.CacheWrite <= 0 || m.Long == nil || m.TTLMinutes != 30 {
				t.Errorf("%s: GPT-5.6+ rows need a write rate, long-context rates and the 30-minute lifetime", m.ID)
			}
		}
	}
	for _, id := range []string{"claude-opus-4-8", "claude-sonnet-4-6", "claude-mythos-5-2", "gpt-6-astra", "gpt-6.2-astra", "gpt-6-luna"} {
		if _, ok := p.Lookup("x", id); !ok {
			t.Errorf("no row for %s", id)
		}
	}
	if r, _ := p.Lookup("claude", "claude-fable-5"); r.CacheRead != 1 {
		t.Errorf("Fable 5 cache read is $1 (0.1x), got %v", r.CacheRead)
	}
	if r, _ := p.Lookup("codex", "gpt-5.6-sol"); r.PromoUntil != "2026-11-30" {
		t.Error("promotional prices carry their end date")
	}
}

func TestLookupMatchesIdsNotPrefixes(t *testing.T) {
	p := Prices()
	for model, want := range map[string]string{
		"claude-haiku-4-5-20251001": "claude-haiku-4-5", "claude-opus-5-5[1m]": "claude-opus-5-5", "claude-opus-5-5": "claude-opus-5-5",
		"claude-opus-4-8": "claude-opus-4-8", "gpt-6.2-astra": "gpt-6.2-astra", "claude-fable-5-1": "claude-fable-5-1",
	} {
		r, ok := p.Lookup("claude", model)
		if !ok || r.ID != want {
			t.Errorf("%s -> %s (%v), want %s", model, r.ID, ok, want)
		}
	}
	for _, model := range []string{"gpt-5-codex", "claude-3-5-sonnet-20241022", "", "gpt-6.2-astra-high"} {
		if _, ok := p.Lookup("codex", model); ok {
			t.Errorf("%q should be unknown (priced by the fallback row and reported)", model)
		}
	}
}

func TestResponsePricing(t *testing.T) {
	p := Prices()
	// Haiku 5.5 above 100K prompt tokens pays the long rates for the whole request
	short := p.Response("claude", "claude-haiku-5-5", 1000, 90000, 0, 0, 0, true)
	long := p.Response("claude", "claude-haiku-5-5", 1000, 100000, 0, 0, 0, true)
	if short.In != 0.10 || long.In != 0.50 || long.Read != 0.05 || long.Out != 2.50 {
		t.Errorf("haiku: short %+v long %+v", short, long)
	}
	// Claude writes: the recorded 5m/1h split decides the rate
	w := p.Response("claude", "claude-opus-5-5", 0, 0, 1000, 500, 500, true)
	if w.Write != (500*5.0+500*8.0)/1000 {
		t.Errorf("mixed write rate %v", w.Write)
	}
	if p.Response("claude", "claude-opus-5-5", 0, 0, 1000, 0, 0, false).Write != 5 || p.Response("claude", "claude-opus-5-5", 0, 0, 1000, 0, 0, true).Write != 8 {
		t.Error("unsplit writes: 5-minute for sub-agents, 1-hour for the main conversation")
	}
	// Codex: long context above 272K, 30-minute lifetime, write rate
	c := p.Response("codex", "gpt-6-astra", 300000, 0, 0, 0, 0, true)
	if c.In != 20 || c.Out != 75 || c.TTL != 30*time.Minute {
		t.Errorf("gpt-6-astra long %+v", c)
	}
	if c := p.Response("codex", "gpt-6.2-astra", 1000, 0, 0, 0, 0, true); c.In != 8 || c.Read != 0.40 || c.Write != 10 || c.Out != 40 {
		t.Errorf("gpt-6.2-astra %+v", c)
	}
	if c := p.Response("codex", "gpt-5", 1000, 0, 0, 0, 0, true); c.TTL != 10*time.Minute || c.Write != 1.25 {
		t.Errorf("gpt-5 (no write charge, short lifetime) %+v", c)
	}
}

func TestJudgePrices(t *testing.T) {
	usd, ok := JudgeUSD("codex:gpt-6.2-astra", Usage{Input: 1e5, CacheRead: 1e5, Output: 1e5})
	if !ok || usd < (0.8+0.04+4)-1e-12 || usd > (0.8+0.04+4)+1e-12 {
		t.Errorf("astra judge %v %v", usd, ok)
	}
	if usd, ok := JudgeUSD("claude:opus", Usage{Input: 1e6}); !ok || usd != 4 {
		t.Errorf("opus judge %v", usd)
	}
	if _, ok := JudgeUSD("claude:claude-opus-5-5", Usage{Input: 1}); !ok {
		t.Error("a full model id is a priced tier")
	}
	if _, ok := JudgeUSD("codex:gpt-9-unknown", Usage{Input: 1}); ok {
		t.Error("an unknown judge model has no price")
	}
	if m := MeasuredUSDPerSession("codex:gpt-6.2-astra"); m < 0.2 || m > 0.22 {
		t.Errorf("restated astra measurement %v", m)
	}
}

// Task item 6: one line when the price table is old or a promotion it lists has ended.
func TestStalePricesNote(t *testing.T) {
	read, _ := time.Parse("2006-01-02", Prices().Read)
	if n := StalePricesNote(read.Add(3 * 24 * time.Hour)); n != "" {
		t.Errorf("fresh table: %q", n)
	}
	if n := StalePricesNote(read.Add(MaxDataAge + 48*time.Hour)); !strings.Contains(n, "days old") || strings.Contains(n, "\n") {
		t.Errorf("old table: %q", n)
	}
	if id, _ := Prices().EndedPromotion(time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC)); id != "gpt-5.6-sol" {
		t.Errorf("ended promotion: %q", id)
	}
	if n := StalePricesNote(time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC)); !strings.Contains(n, "gpt-5.6-sol") {
		t.Errorf("promotion note: %q", n)
	}
}
