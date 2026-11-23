package pipeline

import (
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Mango-Juice/agent-leeway/internal/assets"
)

// Image tokens by each vendor's published rule, from internal/assets/image_tokens.json (page and date read there).

type imgSize struct {
	MaxDim  float64 `json:"max_dim"`
	Patches int     `json:"patches"`
}

type imgModel struct {
	Prefixes   []string           `json:"prefixes"`
	Multiplier float64            `json:"multiplier"`
	Auto       string             `json:"auto"`
	Detail     map[string]imgSize `json:"detail"`
	TileBased  bool               `json:"tile_based"`
	Assumed    string             `json:"assumed"`
}

type imgRules struct {
	Anthropic struct {
		Patch          int     `json:"patch"`
		RejectAbovePx  int     `json:"reject_above_px"`
		HighResFrom    string  `json:"high_resolution_from"`
		HighResolution imgTier `json:"high_resolution"`
		Standard       imgTier `json:"standard"`
	} `json:"anthropic"`
	OpenAI struct {
		Patch              int        `json:"patch"`
		RejectAbovePatches int        `json:"reject_above_patches"`
		Models             []imgModel `json:"models"`
	} `json:"openai"`
}

type imgTier struct {
	MaxDim    float64 `json:"max_dim"`
	MaxTokens int     `json:"max_tokens"`
}

var rules = func() imgRules {
	var r imgRules
	if err := json.Unmarshal(assets.ImageTokens, &r); err != nil {
		panic("image_tokens.json: " + err.Error())
	}
	sort.SliceStable(r.OpenAI.Models, func(i, j int) bool { return longest(r.OpenAI.Models[i]) > longest(r.OpenAI.Models[j]) })
	return r
}()

func longest(m imgModel) int {
	n := 0
	for _, p := range m.Prefixes {
		n = max(n, len(p))
	}
	return n
}

// fitScale is the largest scale (at most 1) at which the image fits maxEdge (0: no limit) and the count limit.
func fitScale(w, h int, maxEdge float64, count func(w, h int) int, limit int) (int, int) {
	s := 1.0
	if long := float64(max(w, h)); maxEdge > 0 && long > maxEdge {
		s = maxEdge / long
	}
	sw, sh := int(math.Floor(float64(w)*s)), int(math.Floor(float64(h)*s))
	if limit <= 0 || count(sw, sh) <= limit {
		return max(sw, 1), max(sh, 1)
	}
	lo, hi := 0.0, s
	for i := 0; i < 40; i++ {
		m := (lo + hi) / 2
		if count(int(math.Floor(float64(w)*m)), int(math.Floor(float64(h)*m))) <= limit {
			lo = m
		} else {
			hi = m
		}
	}
	return max(int(math.Floor(float64(w)*lo)), 1), max(int(math.Floor(float64(h)*lo)), 1)
}

func patches(size int) func(w, h int) int {
	return func(w, h int) int { return ((w + size - 1) / size) * ((h + size - 1) / size) }
}

var claudeVer = regexp.MustCompile(`claude-(?:[a-z]+-)?(\d+)(?:[-.](\d+))?`)

// claudeVersion is a Claude model's version as major*100+minor (unknown names count as new ones).
func claudeVersion(model string) int {
	m := claudeVer.FindStringSubmatch(model)
	if m == nil {
		return math.MaxInt32
	}
	major, _ := strconv.Atoi(m[1])
	minor := 0
	if m[2] != "" && len(m[2]) <= 2 { // "claude-opus-4-20250514": the date is not a minor version
		minor, _ = strconv.Atoi(m[2])
	}
	return major*100 + minor
}

func versionOf(s string) int {
	a, b, _ := strings.Cut(s, ".")
	x, _ := strconv.Atoi(a)
	y, _ := strconv.Atoi(b)
	return x*100 + y
}

// openaiModel is the rule row for a model (longest prefix), nil when the model is not covered.
func openaiModel(model string) *imgModel {
	for i := range rules.OpenAI.Models {
		for _, p := range rules.OpenAI.Models[i].Prefixes {
			if strings.HasPrefix(model, p) {
				return &rules.OpenAI.Models[i]
			}
		}
	}
	return nil
}

// ImageTokens is what one image costs in input tokens each time a request carries it; 0 when the API would reject
// it or the model's rule is not implemented (tile-based or unknown models).
func ImageTokens(tool, model, detail string, w, h int) int {
	if w <= 0 || h <= 0 {
		return 0
	}
	if tool == "claude" {
		a := rules.Anthropic
		if w > a.RejectAbovePx || h > a.RejectAbovePx {
			return 0
		}
		t := a.Standard
		if claudeVersion(model) >= versionOf(a.HighResFrom) {
			t = a.HighResolution
		}
		sw, sh := fitScale(w, h, t.MaxDim, patches(a.Patch), t.MaxTokens)
		return patches(a.Patch)(sw, sh)
	}
	m := openaiModel(model)
	if m == nil || m.TileBased {
		return 0
	}
	if detail == "" || detail == "auto" {
		detail = m.Auto
	}
	sz, ok := m.Detail[detail]
	if !ok {
		sz = m.Detail[m.Auto]
	}
	o := rules.OpenAI
	sw, sh := fitScale(w, h, sz.MaxDim, patches(o.Patch), sz.Patches)
	n := patches(o.Patch)(sw, sh)
	if n > o.RejectAbovePatches {
		return 0
	}
	return int(math.Ceil(float64(n) * m.Multiplier))
}
