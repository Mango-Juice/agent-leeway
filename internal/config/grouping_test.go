package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGroupingDefaultsSettingsAndFlag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LEEWAY_CONFIG", dir)
	g, err := Grouping("")
	if err != nil || !g["claude"] || !g["codex"] { // both on since the g2 re-check (docs/27 §13)
		t.Fatalf("defaults: %v %v", g, err)
	}
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"grouping": {"claude": false}}`), 0o600)
	if g, _ = Grouping(""); g["claude"] || !g["codex"] {
		t.Errorf("settings: %v", g)
	}
	if g, _ = Grouping("codex=off, claude=off"); g["claude"] || g["codex"] {
		t.Errorf("flag: %v", g)
	}
	if g, _ = Grouping("all=on"); !g["claude"] || !g["codex"] {
		t.Errorf("all: %v", g)
	}
	for _, bad := range []string{"claude", "gemini=on", "claude=maybe"} {
		if _, err := Grouping(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
