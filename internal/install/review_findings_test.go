package install

// Regression test for docs/20 M4 and docs/19 C11.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A SKILL.md the user edited after install is backed up (not overwritten) by install-skills and left in place
// by uninstall-skills; uninstalling an unedited copy restores the user's own skill it replaced.
func TestEditedSkillIsKeptAndBackupRestored(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills", "agent-leeway")
	tg := []Target{{Agent: "claude", Dir: dir, Root: filepath.Dir(dir)}}
	p := filepath.Join(dir, "SKILL.md")
	if _, err := Install(tg, false); err != nil {
		t.Fatal(err)
	}
	orig, _ := os.ReadFile(p)
	if !ours(orig) {
		t.Fatal("a fresh install must be recognized as ours")
	}
	edited := append(append([]byte{}, orig...), []byte("\n## My own notes\n")...)
	edited = append([]byte("# my tweak\n"), orig...)
	if err := os.WriteFile(p, edited, 0o644); err != nil {
		t.Fatal(err)
	}
	if ours(edited) {
		t.Fatal("an edited copy must not count as ours")
	}
	if _, err := Install(tg, false); err != nil {
		t.Fatal(err)
	}
	baks, _ := filepath.Glob(p + ".bak-*")
	if len(baks) != 1 {
		t.Fatalf("edited copy not backed up: %v", baks)
	}
	// uninstall removes our fresh copy and restores the edited one
	log, err := Uninstall(tg, false)
	if err != nil {
		t.Fatal(err)
	}
	now, _ := os.ReadFile(p)
	if string(now) != string(edited) || !strings.Contains(strings.Join(log, "\n"), "restored") {
		t.Fatalf("backup not restored: %q", log)
	}
	// and an edited copy is never deleted by uninstall
	if _, err := Uninstall(tg, false); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != string(edited) {
		t.Fatal("uninstall removed a file the user changed")
	}
}
