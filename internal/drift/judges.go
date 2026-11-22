package drift

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/assets"
	"github.com/Mango-Juice/agent-leeway/internal/config"
	"github.com/Mango-Juice/agent-leeway/internal/messages"
	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// Judging models. The rubric is the product; the judging model is a replaceable part chosen per tool from
// internal/assets/judges.json: the newest checked version that is available, then a newer unchecked version of
// a preferred family, then the CLI's own default model. --model overrides the choice. A model is never changed
// after the user approved a run without asking again (ErrModel stops the run).

// Family is a preferred model family of one tool.
type Family struct {
	Name    string `json:"name"`
	Alias   string `json:"alias"` // the CLI's alias for the newest version of the family ("" when none)
	Match   string `json:"match"` // exact pattern of the family's model ids (excludes tier variants)
	Checked []struct {
		Model string `json:"model"`
		Date  string `json:"date"`
		Basis string `json:"basis"`
	} `json:"checked"`
	re *regexp.Regexp
}

var (
	judgesOnce sync.Once
	judgeFams  map[string][]*Family
)

// Families are the preferred families of a tool, in order.
func Families(tool string) []*Family {
	judgesOnce.Do(func() {
		var d struct {
			Tools map[string]struct {
				Families []*Family `json:"families"`
			} `json:"tools"`
		}
		_ = json.Unmarshal(assets.Judges, &d)
		judgeFams = map[string][]*Family{}
		for t, v := range d.Tools {
			for _, f := range v.Families {
				f.re = regexp.MustCompile(f.Match)
			}
			judgeFams[t] = v.Families
		}
	})
	return judgeFams[tool]
}

// Checked says whether an exact model id (or a tier) passed the fixed test set.
func Checked(tierOrModel string) bool {
	tool, m := splitTier(tierOrModel)
	for _, t := range []string{"claude", "codex"} {
		if tool != "" && t != tool {
			continue
		}
		for _, f := range Families(t) {
			for _, c := range f.Checked {
				if c.Model == m {
					return true
				}
			}
		}
	}
	return false
}

// splitTier -> tool, exact model id ("claude:opus" is the checked Opus version the price table names).
func splitTier(s string) (string, string) {
	tool, m, ok := strings.Cut(s, ":")
	if !ok {
		return "", s
	}
	if m == "opus" || m == "sonnet" || m == "haiku" {
		m = config.Prices().JudgeModel(s)
	}
	return tool, m
}

// verNums are the numbers in a model id, for "newest" ("gpt-6.2-astra" -> 6,2; "claude-opus-5-5" -> 5,5).
func verNums(m string) []int {
	var out []int
	for _, x := range regexp.MustCompile(`\d+`).FindAllString(m, -1) {
		n, _ := strconv.Atoi(x)
		out = append(out, n)
	}
	return out
}

func newer(a, b string) bool {
	x, y := verNums(a), verNums(b)
	for i := 0; i < len(x) && i < len(y); i++ {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return len(x) > len(y)
}

// Candidate is one judging model to try.
type Candidate struct {
	Tier    string // the tier key results are stored under
	Model   string // what the CLI is given ("" = no --model / -m: the CLI's default)
	Name    string // readable name
	Checked bool
	Default bool // the CLI's own default model
}

// TierOf is the tier key of an exact model: the checked Opus version keeps "claude:opus", so results stored
// before this choice existed stay valid (run ids include the tier).
func TierOf(tool, model string) string {
	if model == "" {
		return tool + ":default"
	}
	if tool == "claude" && model == config.Prices().JudgeModel("claude:opus") {
		return "claude:opus"
	}
	return tool + ":" + model
}

// CLIModel is the model argument for a tier ("" = leave the CLI's default): the exact checked id for
// "claude:opus" (the alias would follow a newer, unchecked version), "opus" for "claude:opus@latest".
func CLIModel(tier string) string {
	tool, m, _ := strings.Cut(tier, ":")
	switch {
	case m == "default":
		return map[string]string{"claude": "default"}[tool]
	case strings.HasSuffix(m, "@latest"):
		return strings.TrimSuffix(m, "@latest")
	case tool == "claude":
		// a priced alias tier ("claude:opus", "claude:sonnet") is passed as its exact id: the alias alone would
		// follow a newer version the user did not choose
		if x := config.Prices().JudgeModel(tier); strings.Contains(x, "-") {
			return x
		}
	}
	return m
}

// Candidates lists the models to try for a tool, best first, leaving out those known not to work with this CLI
// version. catalog is the account's model list when the CLI can show it without a model call (Codex), nil when
// unknown; catalogDefault is the CLI's default model from it ("" when unknown).
func Candidates(tool string, catalog []string, catalogDefault string, unavailable map[string]bool) []Candidate {
	var out []Candidate
	seen := map[string]bool{}
	add := func(c Candidate) {
		if !seen[c.Tier] && !unavailable[c.Tier] {
			seen[c.Tier] = true
			out = append(out, c)
		}
	}
	for _, f := range Families(tool) {
		ch := make([]string, 0, len(f.Checked))
		for _, c := range f.Checked {
			ch = append(ch, c.Model)
		}
		sort.SliceStable(ch, func(a, b int) bool { return newer(ch[a], ch[b]) })
		for _, m := range ch { // 1. checked versions, newest first; a catalog without them is only a hint, so they
			// are tried anyway (the first call tells), and the user is asked before any other model judges
			add(Candidate{Tier: TierOf(tool, m), Model: m, Name: ModelName(m), Checked: true})
		}
		newest := ""
		if len(ch) > 0 {
			newest = ch[0]
		}
		// 2. a newer unchecked version of the family: from the catalog, or through the family alias
		var newerOnes []string
		for _, m := range catalog {
			if f.re.MatchString(m) && !Checked(tool+":"+m) && (newest == "" || newer(m, newest)) {
				newerOnes = append(newerOnes, m)
			}
		}
		sort.SliceStable(newerOnes, func(a, b int) bool { return newer(newerOnes[a], newerOnes[b]) })
		for _, m := range newerOnes {
			add(Candidate{Tier: TierOf(tool, m), Model: m, Name: ModelName(m)})
		}
		if catalog == nil && f.Alias != "" {
			add(Candidate{Tier: tool + ":" + f.Alias + "@latest", Model: f.Alias, Name: ModelName(f.Alias) + messages.Drift["model_newest"]})
		}
	}
	// 3. the CLI's own default model
	if catalogDefault != "" {
		add(Candidate{Tier: TierOf(tool, catalogDefault), Model: catalogDefault, Name: ModelName(catalogDefault), Checked: Checked(tool + ":" + catalogDefault), Default: true})
	} else {
		add(Candidate{Tier: tool + ":default", Model: CLIModel(tool + ":default"), Name: map[string]string{"claude": "Claude Code", "codex": "Codex"}[tool], Default: true})
	}
	return out
}

// ---------------------------------------------------------------- readable names

var (
	namesMu    sync.Mutex
	codexNames = map[string]string{} // slug -> display name, from the Codex catalog
)

// ModelName is a readable name: "Claude Opus 5.5" for claude-opus-5-5 (and the opus tier), the Codex catalog's
// display name, "GPT-6.2 Astra" for gpt ids, otherwise the id with its words capitalised.
func ModelName(tierOrModel string) string {
	_, m := splitTier(tierOrModel)
	if strings.HasSuffix(m, "@latest") {
		return ModelName(strings.TrimSuffix(m, "@latest")) + messages.Drift["model_newest"]
	}
	switch m {
	case "opus", "sonnet", "haiku":
		return "Claude " + strings.ToUpper(m[:1]) + m[1:]
	case "default":
		_, _, _ = strings.Cut(tierOrModel, ":")
	}
	if strings.HasPrefix(m, "gpt-") { // gpt-6.2-astra -> GPT-6.2 Astra
		parts := strings.Split(m, "-")
		out := "GPT"
		if len(parts) > 1 {
			out += "-" + parts[1]
		}
		for _, w := range parts[2:] {
			if w != "" {
				out += " " + strings.ToUpper(w[:1]) + w[1:]
			}
		}
		return out
	}
	namesMu.Lock()
	n := codexNames[m]
	namesMu.Unlock()
	if n != "" {
		return n
	}
	if rest, ok := strings.CutPrefix(m, "claude-"); ok { // claude-opus-5-5 -> Claude Opus 5.5
		parts := strings.Split(rest, "-")
		var words, nums []string
		for _, p := range parts {
			if _, err := strconv.Atoi(p); err == nil {
				nums = append(nums, p)
			} else {
				words = append(words, strings.ToUpper(p[:1])+p[1:])
			}
		}
		return strings.TrimSpace("Claude " + strings.Join(words, " ") + " " + strings.Join(nums, "."))
	}
	return m
}

// ---------------------------------------------------------------- the Codex catalog (best effort)

// CodexCatalog reads the account's model list with `codex debug models` (no model call; the CLI refreshes the
// catalog for the logged-in account, else shows its bundled one). It is a debugging command, not a documented
// stable interface, so any failure returns nil and the first judge call decides instead.
func CodexCatalog() (slugs []string, def string) {
	exe, err := Resolve("codex")
	if err != nil || os.Getenv("LEEWAY_JUDGE_CMD") != "" {
		return nil, ""
	}
	out, err := runText(exe, "debug", "models")
	if err != nil || !strings.Contains(out, "{") {
		// the account list could not be read: the CLI's bundled list (offline) still names its default model,
		// so "the CLI's default" is a known model and is recorded as such; no list of available models
		b, berr := runText(exe, "debug", "models", "--bundled")
		if berr != nil {
			return nil, ""
		}
		_, def := parseCatalog(b)
		return nil, def
	}
	return parseCatalog(out)
}

// parseCatalog reads `codex debug models` output: the slugs and the default model.
func parseCatalog(out string) (slugs []string, def string) {
	i, j := strings.Index(out, "{"), strings.LastIndex(out, "}")
	if i < 0 || j <= i {
		return nil, ""
	}
	d, err := py.Loads(out[i : j+1])
	if err != nil || d.Get("models").K != py.Arr {
		return nil, ""
	}
	type m struct {
		slug      string
		prio      float64
		def, list bool
	}
	var ms []m
	namesMu.Lock()
	for _, x := range d.Get("models").A {
		slug := x.Get("slug").StrOr("")
		if slug == "" {
			continue
		}
		p, _ := x.Get("priority").Num()
		ms = append(ms, m{slug, p, x.Get("is_default").Truthy(), x.Get("visibility").StrOr("list") == "list"})
		if n := x.Get("display_name").StrOr(""); n != "" {
			codexNames[slug] = n
		}
		slugs = append(slugs, slug)
	}
	namesMu.Unlock()
	sort.SliceStable(ms, func(a, b int) bool { return ms[a].prio < ms[b].prio })
	for _, x := range ms { // codex-rs models-manager default_model_from_available: is_default, else the first
		if x.def && x.list {
			return slugs, x.slug
		}
	}
	for _, x := range ms {
		if x.list {
			return slugs, x.slug
		}
	}
	return slugs, ""
}

// ---------------------------------------------------------------- what did not work, per CLI version

// ErrModel is a judge call refused because the model is unknown or not allowed for the account. The run stops
// (nothing is recorded for it) and the user is asked before another model judges.
type ErrModel struct {
	Tier, Msg string
}

func (e *ErrModel) Error() string { return e.Msg }

// IsErrModel reports whether err is an ErrModel, and returns it.
func IsErrModel(err error) (*ErrModel, bool) {
	var m *ErrModel
	ok := errors.As(err, &m)
	return m, ok
}

func modelsFile(cache string) string { return filepath.Join(cache, "drift", "judge-models.json") }

var modelsMu sync.Mutex

// RefusalTTL is how long a stored refusal is used: an account can gain access, so it is tried again later.
const RefusalTTL = 7 * 24 * time.Hour

// Unavailable is the set of tiers this CLI version refused within RefusalTTL (as stored by MarkUnavailable). An
// explicit --model never consults it.
func Unavailable(cache, tool, version string) map[string]bool {
	modelsMu.Lock()
	defer modelsMu.Unlock()
	out := map[string]bool{}
	for t, at := range readRefusals(cache)[tool+" "+version] {
		if when, err := time.Parse(time.RFC3339, at); err == nil && time.Since(when) < RefusalTTL {
			out[t] = true
		}
	}
	return out
}

func readRefusals(cache string) map[string]map[string]string {
	d := map[string]map[string]string{}
	if b, err := os.ReadFile(modelsFile(cache)); err == nil {
		_ = json.Unmarshal(b, &d)
	}
	return d
}

// MarkUnavailable remembers, with the time, that this CLI version refused a tier.
func MarkUnavailable(cache, tool, version, tier string) {
	modelsMu.Lock()
	defer modelsMu.Unlock()
	d := readRefusals(cache)
	k := tool + " " + version
	if d[k] == nil {
		d[k] = map[string]string{}
	}
	d[k][tier] = time.Now().UTC().Format(time.RFC3339)
	_ = os.MkdirAll(filepath.Dir(modelsFile(cache)), 0o700)
	b, _ := json.MarshalIndent(d, "", " ")
	_ = os.WriteFile(modelsFile(cache), b, 0o600)
}

// modelRefused matches only the refusal texts verified with each CLI (2026-11-23):
//   - claude -p: "There's an issue with the selected model (X). It may not exist or you may not have access to
//     it." (is_error, api_error_status 404; stderr marker [claude-code:unrecognized_model]);
//   - codex exec: turn.failed "The 'X' model is not supported when using Codex with a ChatGPT account."
var modelRefused = regexp.MustCompile(`There's an issue with the selected model|\[claude-code:unrecognized_model\]|model is not supported when using Codex`)
