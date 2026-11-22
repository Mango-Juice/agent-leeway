package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/assets"
)

// Rates are USD per million tokens. Claude rows have two write rates (5-minute and 1-hour cache), OpenAI rows one.
type Rates struct {
	Input        float64 `json:"input"`
	CacheRead    float64 `json:"cache_read"`
	CacheWrite   float64 `json:"cache_write,omitempty"`
	CacheWrite5m float64 `json:"cache_write_5m,omitempty"`
	CacheWrite1h float64 `json:"cache_write_1h,omitempty"`
	Output       float64 `json:"output"`
}

// LongRates apply to a whole request whose prompt (input, cache reads and writes) is above Above tokens.
type LongRates struct {
	Above int64 `json:"above"`
	Rates
}

// ModelPrice is one row of the price table.
type ModelPrice struct {
	ID       string `json:"id"`
	Provider string `json:"provider"` // anthropic | openai
	Rates
	Long       *LongRates `json:"long,omitempty"`
	TTLMinutes float64    `json:"cache_ttl_minutes,omitempty"` // OpenAI: minimum cache lifetime
	Source     string     `json:"source"`
	Read       string     `json:"read"`
	PromoUntil string     `json:"promo_until,omitempty"`
	Note       string     `json:"note,omitempty"`
}

// Measured is the average cost of judging one session in a validation run.
type Measured struct {
	USD      float64 `json:"usd"`
	Sessions int     `json:"sessions"`
	Note     string  `json:"note"`
}

// PriceTable is internal/assets/prices.json, or prices.json in the settings folder when present.
type PriceTable struct {
	About       string              `json:"about"`
	Read        string              `json:"read"`
	Fallback    map[string]string   `json:"fallback"`     // tool -> model id used for an unknown model
	JudgeModels map[string]string   `json:"judge_models"` // judge tier alias -> model id
	MeasuredUSD map[string]Measured `json:"measured_judge_usd_per_session"`
	Caching     map[string]string   `json:"caching"`
	Models      []ModelPrice        `json:"models"`
	Path        string              `json:"-"` // where it was read from ("" = built in)
}

var (
	priceOnce  sync.Once
	priceTable *PriceTable
	priceErr   error
)

// PricesPath is where a user's own price table goes.
func PricesPath() string { return filepath.Join(ConfigDir(), "prices.json") }

// ParsePrices reads a price table.
func ParsePrices(b []byte) (*PriceTable, error) {
	var t PriceTable
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// Prices is the price table in use (read once).
func Prices() *PriceTable {
	priceOnce.Do(func() {
		if b, err := os.ReadFile(PricesPath()); err == nil {
			if t, err := ParsePrices(b); err == nil {
				t.Path = PricesPath()
				priceTable = t
				return
			} else {
				priceErr = err
			}
		}
		t, err := ParsePrices(assets.Prices)
		if err != nil {
			panic("built-in prices.json: " + err.Error())
		}
		priceTable = t
	})
	return priceTable
}

// PricesError reports a user price table that could not be read (the built-in one is used instead).
func PricesError() error { Prices(); return priceErr }

var suffix = regexp.MustCompile(`^(-\d{8}|\[.*\]|@.*)$`)

// Lookup finds the row for a model name: the id itself, or the id followed by a date ("-20251001"), a
// bracketed suffix ("[1m]") or "@...". known is false when the fallback row of the tool was used.
func (t *PriceTable) Lookup(tool, model string) (*ModelPrice, bool) {
	best := -1
	for i := range t.Models {
		id := t.Models[i].ID
		if model == id || (strings.HasPrefix(model, id) && suffix.MatchString(model[len(id):])) {
			if best < 0 || len(id) > len(t.Models[best].ID) {
				best = i
			}
		}
	}
	if best >= 0 {
		return &t.Models[best], true
	}
	fb := t.Fallback[tool]
	for i := range t.Models {
		if t.Models[i].ID == fb {
			return &t.Models[i], false
		}
	}
	return &ModelPrice{ID: "unknown", Provider: tool}, false
}

// Priced is what one response costs per token kind, and how long its cache lives.
type Priced struct {
	In, Read, Write, Out float64
	TTL                  time.Duration // 0 = decided by the caller (Claude: by the recorded write lifetime)
	Row                  *ModelPrice
	Known                bool
}

// Response prices one response. cw5m/cw1h are Claude's per-lifetime write counts (both 0 when the log has no
// split); defaultLong tells which lifetime to assume for unsplit writes (true = 1 hour).
func (t *PriceTable) Response(tool, model string, uncached, cached, cw, cw5m, cw1h int64, defaultLong bool) Priced {
	row, known := t.Lookup(tool, model)
	r := row.Rates
	if row.Long != nil && uncached+cached+cw > row.Long.Above {
		r = row.Long.Rates
	}
	p := Priced{In: r.Input, Read: r.CacheRead, Out: r.Output, Row: row, Known: known}
	if row.Provider == "anthropic" {
		w5, w1 := r.CacheWrite5m, r.CacheWrite1h
		switch {
		case cw5m+cw1h > 0:
			p.Write = (float64(cw5m)*w5 + float64(cw1h)*w1) / float64(cw5m+cw1h)
		case defaultLong:
			p.Write = w1
		default:
			p.Write = w5
		}
	} else {
		p.Write = r.CacheWrite
		if p.Write == 0 {
			p.Write = r.Input // models that do not bill cache writes charge them as input
		}
		if row.TTLMinutes > 0 {
			p.TTL = time.Duration(row.TTLMinutes * float64(time.Minute))
		}
	}
	return p
}

// JudgeModel maps a judge tier ("claude:opus", "codex:gpt-6.2-astra") to a price row id.
func (t *PriceTable) JudgeModel(tier string) string {
	if m, ok := t.JudgeModels[tier]; ok {
		return m
	}
	_, m, _ := strings.Cut(tier, ":")
	return m
}

// JudgeRow is the price row of a judge tier; ok is false when the tier's model has no row.
func (t *PriceTable) JudgeRow(tier string) (*ModelPrice, bool) {
	prov, _, _ := strings.Cut(tier, ":")
	return t.Lookup(prov, t.JudgeModel(tier))
}

// MaxDataAge is how old the price table may get before outputs say so. Providers changed these pages at
// every model launch in 2026 (roughly every two to three months, with promotions in between), so after 90
// days a table is likely to be missing a model or a price change; a shorter limit would nag between launches.
const MaxDataAge = 90 * 24 * time.Hour

// Staleness says whether data read on date `read` (YYYY-MM-DD) is older than MaxDataAge at now, and its age in
// days. It is shared by the price table and, later, the advice catalogue.
func Staleness(read string, now time.Time) (days int, stale bool) {
	t, err := time.Parse("2006-01-02", read)
	if err != nil {
		return 0, false
	}
	age := now.Sub(t)
	return int(age.Hours() / 24), age > MaxDataAge
}

// EndedPromotion is the first row whose promotional price ended before now ("" when none).
func (t *PriceTable) EndedPromotion(now time.Time) (string, string) {
	for _, m := range t.Models {
		if m.PromoUntil == "" {
			continue
		}
		if end, err := time.Parse("2006-01-02", m.PromoUntil); err == nil && now.After(end.Add(24*time.Hour)) {
			return m.ID, m.PromoUntil
		}
	}
	return "", ""
}

// StalePricesNote is one line for outputs when the price table is old or a promotion it lists has ended
// ("" otherwise).
func StalePricesNote(now time.Time) string {
	t := Prices()
	if days, stale := Staleness(t.Read, now); stale {
		return fmt.Sprintf("Note: the price table is %d days old (read %s); compare `agent-leeway prices` with the providers' pages, or update agent-leeway.", days, t.Read)
	}
	if id, end := t.EndedPromotion(now); id != "" {
		return fmt.Sprintf("Note: the promotional price of %s in the price table ended on %s; check `agent-leeway prices`, or update agent-leeway.", id, end)
	}
	return ""
}
