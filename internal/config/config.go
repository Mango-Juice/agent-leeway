// Package config holds prices, thresholds, judge defaults and the locations of logs, cache and settings.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Price is USD per 1M tokens: input, cache read, cache write, output (the legacy four-number form).
type Price struct{ In, Read, Write, Out float64 }

// Legacy selects the reference implementation's pricing (research/oracle): its own price table, every
// response priced by its thread's first model, a fixed cache lifetime per tool, and the old judge prices.
// Only the parity tests set it; the product uses the price table in internal/assets/prices.json.
var Legacy bool

// legacyPrices is the reference's table, frozen for the parity tests.
var legacyPrices = []struct {
	Model string
	P     Price
}{
	{"claude-fable-5-2", Price{8.0, 0.20, 16.0, 40.0}},
	{"claude-fable-5-1", Price{10.0, 0.25, 20.0, 50.0}},
	{"claude-fable-5", Price{10.0, 0.25, 20.0, 50.0}},
	{"claude-opus-5-5", Price{4.0, 0.20, 8.0, 20.0}},
	{"claude-opus-5", Price{5.0, 0.50, 10.0, 25.0}},
	{"claude-sonnet-5-5", Price{2.0, 0.10, 4.0, 10.0}},
	{"claude-sonnet-5", Price{2.0, 0.20, 4.0, 10.0}},
	{"claude-haiku-5-5", Price{0.10, 0.01, 0.20, 0.50}},
	{"claude-haiku-4-5", Price{1.0, 0.10, 2.0, 5.0}},
}

// LegacyCodexDefault and LegacyClaudeDefault are the reference's prices for models it did not list.
var (
	LegacyCodexDefault  = Price{1.25, 0.125, 0.0, 10.0}
	LegacyClaudeDefault = Price{4.0, 0.20, 8.0, 20.0}
)

var legacyByLen = func() []int {
	idx := make([]int, len(legacyPrices))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return len(legacyPrices[idx[a]].Model) > len(legacyPrices[idx[b]].Model) })
	return idx
}()

// LegacyPriceOf is the reference's price_of(): longest matching prefix, else a tool default.
func LegacyPriceOf(tool, model string) Price {
	for _, i := range legacyByLen {
		if strings.HasPrefix(model, legacyPrices[i].Model) {
			return legacyPrices[i].P
		}
	}
	if tool == "codex" {
		return LegacyCodexDefault
	}
	return LegacyClaudeDefault
}

const (
	CommitRE          = `\[([^\]\s]+)(?: \(root-commit\))? ([0-9a-f]{7,40})\]`
	ExcessOutputChars = 40000 // tool output above this (~10k tokens) is "oversized"
	CharsPerToken     = 4
)

// LegacyCacheTTL is the reference's fixed cache lifetime per tool (parity tests only).
var LegacyCacheTTL = map[string]time.Duration{"codex": 10 * time.Minute, "claude": 60 * time.Minute}

// ---------------------------------------------------------------- judges

// JudgeDefault is the judging model per provider ("<provider>:<model alias for that provider's CLI>").
// A session is judged only by its own provider. Fable-class models are never a default.
var JudgeDefault = map[string]string{"claude": "claude:opus", "codex": "codex:gpt-6.2-astra"}

// JudgeLower are lower tiers that invent citations more often (a warning is shown).
var JudgeLower = map[string]bool{"claude:haiku": true, "claude:sonnet": true, "codex:gpt-6-luna": true}

// legacyJudgePrice and legacyMeasured are the reference's judge prices (parity tests only).
var legacyJudgePrice = map[string]Price{
	"claude:haiku": {0.10, 0.01, 0.20, 0.50}, "claude:sonnet": {2.0, 0.10, 4.0, 10.0}, "claude:opus": {4.0, 0.20, 8.0, 20.0},
	"codex:gpt-6-luna": {0.25, 0.025, 0.0, 2.0}, "codex:gpt-6.2-sol": {1.25, 0.125, 0.0, 10.0}, "codex:gpt-6.2-astra": {1.25, 0.125, 0.0, 10.0},
}
var legacyMeasured = map[string]float64{"claude:opus": 0.24, "codex:gpt-6.2-astra": 0.038}

// CodexEffort is the reasoning effort passed to codex exec per model.
var CodexEffort = map[string]string{"gpt-6.2-astra": "medium", "gpt-6.2-sol": "medium", "gpt-6-luna": "low"}

// MeasuredUSDPerSession is the measured average cost of judging one session (0 when not measured).
func MeasuredUSDPerSession(tier string) float64 {
	if Legacy {
		return legacyMeasured[tier]
	}
	return Prices().MeasuredUSD[tier].USD
}

// Usage is token usage of one judge call. CacheWrite5m/1h split Claude's writes when the CLI reports them.
type Usage struct {
	Input, CacheRead, CacheWrite, Output float64
	CacheWrite5m, CacheWrite1h           float64
	Model                                string
	// Fallback: the CLI answered with another model than the one asked for (Model names it); priced as that model.
	Fallback bool
	// Extra is usage the judge CLI reported for other models in the same call (the Claude CLI makes its own
	// small helper calls, e.g. with a Haiku model). It is priced at each model's own list price.
	Extra []ModelTokens
}

// ModelTokens is one model's token counts inside a judge call.
type ModelTokens struct {
	Model                                string
	Input, CacheRead, CacheWrite, Output float64
}

// ExtraUSD prices the extra models of a judge call (cache writes at the 5-minute rate when no split is known).
func ExtraUSD(u Usage) float64 {
	if Legacy {
		return 0
	}
	t, sum := Prices(), 0.0
	for _, e := range u.Extra {
		pr := t.Response("claude", e.Model, int64(e.Input), int64(e.CacheRead), int64(e.CacheWrite), 0, 0, false)
		sum += (e.Input*pr.In + e.CacheRead*pr.Read + e.CacheWrite*pr.Write + e.Output*pr.Out) / 1e6
	}
	return sum
}

// JudgeUSD prices a judge call at list price; ok is false when the tier's model has no price row.
func JudgeUSD(tier string, u Usage) (float64, bool) {
	if Legacy {
		p, ok := legacyJudgePrice[tier]
		if !ok {
			return 0, false
		}
		return (u.Input*p.In + u.CacheRead*p.Read + u.CacheWrite*p.Write + u.Output*p.Out) / 1e6, true
	}
	t := Prices()
	row, ok := t.JudgeRow(tier)
	if u.Fallback { // priced as the model that answered; no price row, no dollar amount
		prov, _, _ := strings.Cut(tier, ":")
		row, ok = t.Lookup(prov, u.Model)
	}
	if !ok {
		return 0, false
	}
	pr := t.Response(row.Provider, row.ID, int64(u.Input), int64(u.CacheRead), int64(u.CacheWrite), int64(u.CacheWrite5m), int64(u.CacheWrite1h), true)
	return (u.Input*pr.In+u.CacheRead*pr.Read+u.CacheWrite*pr.Write+u.Output*pr.Out)/1e6 + ExtraUSD(u), true
}

// ---------------------------------------------------------------- locations

// Name is the command and folder name.
const Name = "agent-leeway"

// Home is the user's home folder (%USERPROFILE% on Windows).
func Home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return h
}

func env(k string) string { return os.Getenv(k) }

// ClaudeConfigDir is $CLAUDE_CONFIG_DIR or <home>/.claude.
func ClaudeConfigDir() string {
	if v := env("CLAUDE_CONFIG_DIR"); v != "" {
		return v
	}
	return filepath.Join(Home(), ".claude")
}

// CodexHome is $CODEX_HOME or <home>/.codex.
func CodexHome() string {
	if v := env("CODEX_HOME"); v != "" {
		return v
	}
	return filepath.Join(Home(), ".codex")
}

// ClaudeDir is where Claude Code transcripts are read from.
func ClaudeDir() string {
	if v := env("LEEWAY_CLAUDE_DIR"); v != "" {
		return v
	}
	return filepath.Join(ClaudeConfigDir(), "projects")
}

// CodexDir is where Codex rollouts are read from (sessions).
func CodexDir() string {
	if v := env("LEEWAY_CODEX_DIR"); v != "" {
		return v
	}
	return filepath.Join(CodexHome(), "sessions")
}

// CodexArchivedDir is Codex's archived_sessions folder (read after sessions; files already seen are skipped).
func CodexArchivedDir() string { return filepath.Join(CodexHome(), "archived_sessions") }

// CacheDir is the local cache (never sent anywhere).
func CacheDir() string {
	if v := env("LEEWAY_CACHE"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		if d, err := os.UserCacheDir(); err == nil { // %LocalAppData%
			return filepath.Join(d, Name)
		}
	}
	base := env("XDG_CACHE_HOME")
	if base == "" {
		base = filepath.Join(Home(), ".cache")
	}
	return filepath.Join(base, Name)
}

// ReportDir is where report files are written.
func ReportDir() string {
	if v := env("LEEWAY_REPORTS"); v != "" {
		return v
	}
	return filepath.Join(CacheDir(), "reports")
}

// ConfigDir holds settings.json.
func ConfigDir() string {
	if v := env("LEEWAY_CONFIG"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		if d, err := os.UserConfigDir(); err == nil { // %AppData%
			return filepath.Join(d, Name)
		}
	}
	base := env("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(Home(), ".config")
	}
	return filepath.Join(base, Name)
}

// Settings is settings.json, e.g. {"judge": {"claude": "claude:opus"}, "rubric": "/path/rubric.md",
// "rubric_settings": {"S1": "4"}, "claude_dir": "...", "codex_dir": "..."}.
type Settings struct {
	Judge          map[string]string `json:"judge"`
	Rubric         string            `json:"rubric"`
	RubricSettings map[string]any    `json:"rubric_settings"`
	ClaudeDir      string            `json:"claude_dir"`
	CodexDir       string            `json:"codex_dir"`
	Origin         *OriginRules      `json:"origin"`
	Language       string            `json:"language"` // report language: "en" or "ko"
	Grouping       map[string]bool   `json:"grouping"` // grouping step per tool, e.g. {"claude": true}; missing tools keep the default
}

// OriginRules tell automation from a person's own conversations (settings.json "origin"; missing lists keep
// the defaults). A session is automation when a user prompt contains a worker marker, or the launcher the log
// records is listed as automation; it is a person's conversation when the launcher is listed as interactive
// (or a prompt was typed in the terminal UI); anything else is counted as uncertain.
type OriginRules struct {
	WorkerMarkers          []string `json:"worker_markers"`          // phrases of an orchestrator's worker preamble
	AutoEntrypoints        []string `json:"auto_entrypoints"`        // Claude Code entrypoint values
	InteractiveEntrypoints []string `json:"interactive_entrypoints"` //
	AutoOriginators        []string `json:"auto_originators"`        // Codex originator values
	InteractiveOriginators []string `json:"interactive_originators"` //
	AutoSources            []string `json:"auto_sources"`            // Codex session source values
}

// DefaultOrigin: today's markers. Claude Code's transcript format is internal and its entrypoint values are not
// documented; these are the values seen in logs (cli, claude-desktop, claude-vscode for the official apps,
// sdk-cli for "claude -p"). Codex's originator values are not documented either (codex_exec for "codex exec",
// codex_cli_rs / codex-tui for the terminal UI, Codex Desktop and codex_vscode for the apps).
var DefaultOrigin = OriginRules{
	WorkerMarkers:          []string{"You are a dispatched worker", "You are working inside Orca"},
	AutoEntrypoints:        []string{"sdk-cli"},
	InteractiveEntrypoints: []string{"cli", "claude-desktop", "claude-vscode"},
	AutoOriginators:        []string{"codex_exec", "orca_desktop"},
	InteractiveOriginators: []string{"codex_cli_rs", "codex-tui", "Codex Desktop", "codex_vscode", "codex_work_desktop"},
	AutoSources:            []string{"exec", "mcp"},
}

// Origin returns the origin rules: settings.json "origin" over the defaults, list by list.
func Origin() OriginRules {
	r := DefaultOrigin
	if o := UserSettings().Origin; o != nil {
		for _, x := range []struct {
			dst *[]string
			src []string
		}{{&r.WorkerMarkers, o.WorkerMarkers}, {&r.AutoEntrypoints, o.AutoEntrypoints},
			{&r.InteractiveEntrypoints, o.InteractiveEntrypoints}, {&r.AutoOriginators, o.AutoOriginators},
			{&r.InteractiveOriginators, o.InteractiveOriginators}, {&r.AutoSources, o.AutoSources}} {
			if x.src != nil {
				*x.dst = x.src
			}
		}
	}
	return r
}

// UserSettings reads settings.json (empty when missing or unreadable).
func UserSettings() Settings {
	var s Settings
	b, err := os.ReadFile(filepath.Join(ConfigDir(), "settings.json"))
	if err != nil {
		return s
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber() // rubric_settings numbers keep their spelling ("4" stays 4, as Python's str() writes it)
	_ = dec.Decode(&s)
	return s
}

// Now is the current time; LEEWAY_NOW=<RFC 3339 time> fixes it (reproducible test runs).
func Now() time.Time {
	if v := env("LEEWAY_NOW"); v != "" {
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC()
}

// Languages are the report languages.
var Languages = []string{"en", "ko"}

// langOf maps "ko", "ko_KR.UTF-8", "en-US" and the like to a report language ("" when not supported).
func langOf(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if i := strings.IndexAny(v, "_-.@"); i >= 0 {
		v = v[:i]
	}
	for _, l := range Languages {
		if v == l {
			return l
		}
	}
	return ""
}

// Language resolves the report language: the --lang flag, then settings.json "language", then LC_ALL, then
// LANG, then English. A value that is set but not supported falls through to the next source.
func Language(flag string) string {
	for _, v := range []string{flag, UserSettings().Language, env("LC_ALL"), env("LANG")} {
		if l := langOf(v); l != "" {
			return l
		}
	}
	return "en"
}

// ValidLanguage says whether a --lang value names a supported language.
func ValidLanguage(v string) bool { return v == "" || langOf(v) != "" }

// DefaultGrouping is whether the grouping step runs for each tool when settings.json and --grouping say nothing.
// Both on: prompt g2 passed the pre-registered re-check for both providers (docs/27 §12-13), after g1 had kept
// Claude Code off (docs/27 §8).
var DefaultGrouping = map[string]bool{"claude": true, "codex": true}

// Grouping is the grouping step's state per tool: the defaults, then settings.json "grouping", then flag
// ("claude=on,codex=off"; "all=off" for every tool). An unknown tool or value is an error.
func Grouping(flag string) (map[string]bool, error) {
	out := map[string]bool{}
	for k, v := range DefaultGrouping {
		out[k] = v
	}
	for k, v := range UserSettings().Grouping {
		if _, ok := out[k]; ok {
			out[k] = v
		}
	}
	for _, part := range strings.Split(flag, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		var on bool
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "on", "true", "1":
			on = true
		case "off", "false", "0":
		default:
			return nil, fmt.Errorf("--grouping: use tool=on or tool=off (tools: claude, codex, all), not %q", part)
		}
		switch k = strings.TrimSpace(k); k {
		case "all":
			for t := range out {
				out[t] = on
			}
		case "claude", "codex":
			out[k] = on
		default:
			return nil, fmt.Errorf("--grouping: unknown tool %q", k)
		}
	}
	return out, nil
}
