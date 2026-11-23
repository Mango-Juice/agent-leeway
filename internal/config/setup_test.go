package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Only the one top-level key is read; a key of the same name inside a table, a comment or a bad value gives 0.
func TestCodexDocMaxBytes(t *testing.T) {
	d := t.TempDir()
	t.Setenv("CODEX_HOME", d)
	p := filepath.Join(d, "config.toml")
	for body, want := range map[string]int{
		"model = \"x\"\nproject_doc_max_bytes = 65_536 # bigger\n[mcp_servers.a]\nenv = { TOKEN = \"secret\" }\n": 65536,
		"[profiles.a]\nproject_doc_max_bytes = 9000\n":                                                            0,
		"project_doc_max_bytes = \"lots\"\n":                                                                      0,
		"# project_doc_max_bytes = 100\n":                                                                         0,
	} {
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := CodexDocMaxBytes(); got != want {
			t.Errorf("%q: %d", body, got)
		}
	}
	os.Remove(p)
	if CodexDocMaxBytes() != 0 {
		t.Error("no file")
	}
}

// "name-only" from the user, project and local settings; a later file overrides an earlier one.
func TestSkillNameOnly(t *testing.T) {
	d := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(d, "user"))
	proj := filepath.Join(d, "proj")
	w := func(p, s string) {
		_ = os.MkdirAll(filepath.Dir(p), 0o700)
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	w(filepath.Join(d, "user", "settings.json"), `{"skillOverrides": {"a": "name-only", "b": "name-only"}, "env": {"K": "v"}}`)
	w(filepath.Join(proj, ".claude", "settings.json"), `{"skillOverrides": {"c": "name-only"}}`)
	w(filepath.Join(proj, ".claude", "settings.local.json"), `{"skillOverrides": {"b": "on"}}`)
	got := SkillNameOnly(proj)
	if !got["a"] || got["b"] || !got["c"] || len(got) != 2 {
		t.Errorf("%v", got)
	}
}
