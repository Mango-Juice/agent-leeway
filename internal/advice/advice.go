// Package advice reads the advice catalogue: for each finding id and tool, one plain sentence of what to do,
// the exact command or setting, an optional standing rule to copy, and the sources it rests on (with the date
// each was read). The catalogue ships in the binary; advice.json in the settings folder replaces it, as
// prices.json does. The program never goes online to check it; Stale says when it is old.
package advice

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/assets"
)

// Source is one page or study an item rests on.
type Source struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Read  string `json:"read"`  // YYYY-MM-DD
	Claim string `json:"claim"` // the exact sentence relied on
}

// Setting is an example setting to adapt (the action to guard is not known from the record).
type Setting struct {
	Label  map[string]string `json:"label"`
	File   string            `json:"file"`
	Syntax string            `json:"syntax"`
	Text   string            `json:"text"`
}

// ToolAdvice is one tool's advice for a finding.
type ToolAdvice struct {
	Basis       string            `json:"basis"`  // official | study | ours | heuristic
	Action      map[string]string `json:"action"` // one plain sentence per language
	How         []string          `json:"how"`    // exact commands or settings, shown after the sentence
	Instruction map[string]string `json:"instruction,omitempty"`
	Setting     *Setting          `json:"setting,omitempty"`
	Sources     []Source          `json:"sources"`
}

// Against is evidence that limits or argues against the advice.
type Against struct {
	Title string            `json:"title"`
	URL   string            `json:"url"`
	Tool  string            `json:"tool"`           // "" = both tools
	Kind  string            `json:"kind,omitempty"` // "another_way": an alternative to the advice, not evidence against it
	Note  map[string]string `json:"note"`
}

// Entry is the advice for one finding id.
type Entry struct {
	ID      string                `json:"id"`
	Tools   map[string]ToolAdvice `json:"tools"`
	Against []Against             `json:"against"`
}

// Catalogue is the whole file.
type Catalogue struct {
	Version        string  `json:"version"`
	StaleAfterDays int     `json:"stale_after_days"`
	Entries        []Entry `json:"entries"`
	Path           string  `json:"-"` // where it was read from ("" = built in)
	byID           map[string]*Entry
}

// Parse reads a catalogue.
func Parse(b []byte) (*Catalogue, error) {
	var c Catalogue
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.StaleAfterDays <= 0 {
		c.StaleAfterDays = 180
	}
	c.byID = map[string]*Entry{}
	for i := range c.Entries {
		c.byID[c.Entries[i].ID] = &c.Entries[i]
	}
	return &c, nil
}

// Builtin is the catalogue shipped in the binary.
func Builtin() *Catalogue {
	c, err := Parse(assets.Advice)
	if err != nil {
		panic("built-in advice.json: " + err.Error())
	}
	return c
}

// Load returns advice.json from settingsDir when it exists and parses, else the built-in catalogue. The error
// reports a user file that could not be read (the built-in one is used instead).
func Load(settingsDir string) (*Catalogue, error) {
	p := filepath.Join(settingsDir, "advice.json")
	b, err := os.ReadFile(p)
	if err != nil {
		return Builtin(), nil
	}
	c, err := Parse(b)
	if err != nil {
		return Builtin(), err
	}
	c.Path = p
	return c, nil
}

// Get is the entry for an id (nil when missing).
func (c *Catalogue) Get(id string) *Entry {
	if c == nil {
		return nil
	}
	return c.byID[id]
}

// Oldest is the earliest read date of any source.
func (c *Catalogue) Oldest() string {
	old := ""
	for _, e := range c.Entries {
		for _, t := range e.Tools {
			for _, s := range t.Sources {
				if old == "" || s.Read < old {
					old = s.Read
				}
			}
		}
	}
	return old
}

// Stale says whether the oldest source read date is more than StaleAfterDays before today.
func (c *Catalogue) Stale(today time.Time) (oldest string, days int, stale bool) {
	oldest = c.Oldest()
	t, err := time.Parse("2006-01-02", oldest)
	if err != nil {
		return oldest, 0, false
	}
	days = int(today.Sub(t).Hours() / 24)
	return oldest, days, days > c.StaleAfterDays
}

// Pick is the text for lang, falling back to English.
func Pick(m map[string]string, lang string) string {
	if s := m[lang]; s != "" {
		return s
	}
	return m["en"]
}
