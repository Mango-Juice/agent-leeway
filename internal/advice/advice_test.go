package advice

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

var ids = []string{"leak.long_break", "leak.mid_switch", "leak.let_repeat", "leak.no_map", "leak.big_output", "leak.automation",
	"drift.RESERVED", "drift.FALSE_DONE", "drift.ASKED_ONLY", "drift.BROKE_SAID.chat", "drift.BROKE_SAID.file", "drift.MISTAKE", "drift.PROMPT"}

func TestCatalogueComplete(t *testing.T) {
	c := Builtin()
	date := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	for _, id := range ids {
		e := c.Get(id)
		if e == nil {
			t.Fatalf("missing %s", id)
		}
		for _, tool := range []string{"claude", "codex"} {
			a, ok := e.Tools[tool]
			if !ok {
				t.Fatalf("%s: no %s advice", id, tool)
			}
			switch a.Basis {
			case "official", "study", "ours", "heuristic":
			default:
				t.Errorf("%s/%s: basis %q", id, tool, a.Basis)
			}
			if len(a.Sources) == 0 {
				t.Errorf("%s/%s: no source", id, tool)
			}
			for _, s := range a.Sources {
				if !date.MatchString(s.Read) || strings.TrimSpace(s.Claim) == "" || s.URL == "" || s.Title == "" {
					t.Errorf("%s/%s: incomplete source %+v", id, tool, s)
				}
			}
			for _, lang := range []string{"en", "ko"} {
				if a.Action[lang] == "" {
					t.Errorf("%s/%s: no %s action", id, tool, lang)
				}
				if a.Instruction != nil && a.Instruction[lang] == "" {
					t.Errorf("%s/%s: no %s instruction", id, tool, lang)
				}
			}
		}
	}
}

// Each Do line is one sentence a person can act on without knowing setting names; the exact command or setting
// goes in "how", shown after it.
func TestActionsArePlainSentences(t *testing.T) {
	jargon := regexp.MustCompile(`(?i)/[a-z]+\b|\.json|\.rules|approval_policy|prefix_rule|permissions\.|project_doc_max_bytes|\bMCP\b|Shift\+Tab|\bEsc\b`)
	for _, e := range Builtin().Entries {
		for tool, a := range e.Tools {
			for lang, s := range a.Action {
				if m := jargon.FindString(s); m != "" {
					t.Errorf("%s/%s/%s: setting or command %q in the sentence; put it in how", e.ID, tool, lang, m)
				}
				end := strings.TrimRight(s, " ")
				if !strings.HasSuffix(end, ".") {
					t.Errorf("%s/%s/%s: not one full sentence: %q", e.ID, tool, lang, s)
				}
				if strings.Count(strings.TrimSuffix(end, "."), ". ") > 0 || strings.Contains(s, "; ") {
					t.Errorf("%s/%s/%s: more than one sentence: %q", e.ID, tool, lang, s)
				}
				if utf8.RuneCountInString(s) > 200 {
					t.Errorf("%s/%s/%s: too long", e.ID, tool, lang)
				}
			}
		}
	}
}

func TestOverrideAndStale(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(dir)
	if err != nil || c.Path != "" {
		t.Fatalf("built-in expected: %v %q", err, c.Path)
	}
	if _, _, stale := c.Stale(time.Date(2026, 11, 22, 0, 0, 0, 0, time.UTC)); stale {
		t.Error("fresh catalogue reported stale")
	}
	if old, days, stale := c.Stale(time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)); !stale || old != "2026-11-22" || days != 191 {
		t.Errorf("stale: %s %d %v", old, days, stale)
	}
	own := `{"version":"x","entries":[{"id":"drift.MISTAKE","tools":{"claude":{"basis":"ours","action":{"en":"Mine."},"sources":[{"title":"t","url":"u","read":"2020-01-01","claim":"c"}]}}}]}`
	if err := os.WriteFile(filepath.Join(dir, "advice.json"), []byte(own), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err = Load(dir)
	if err != nil || c.Path == "" || Pick(c.Get("drift.MISTAKE").Tools["claude"].Action, "ko") != "Mine." || c.StaleAfterDays != 180 {
		t.Fatalf("override not used: %v %+v", err, c)
	}
	os.WriteFile(filepath.Join(dir, "advice.json"), []byte("{"), 0o600)
	if c, err = Load(dir); err == nil || c.Path != "" {
		t.Fatal("broken file should fall back with an error")
	}
}
