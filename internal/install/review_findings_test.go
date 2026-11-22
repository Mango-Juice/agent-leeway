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

// The skills CLI (npx skills add -g) puts one copy in ~/.agents/skills and links the Claude Code folder to it.
// install-skills gives Claude Code a folder of its own (the link only is removed), so each agent keeps its own
// copy; a link to a skill agent-leeway did not write is left alone.
func TestLinkedSkillFolder(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, ".agents", "skills", "agent-leeway")
	_ = os.MkdirAll(shared, 0o755)
	_ = os.WriteFile(filepath.Join(shared, "SKILL.md"), SkillFor(RepoCopy, ""), 0o644)
	link := filepath.Join(root, ".claude", "skills", "agent-leeway")
	_ = os.MkdirAll(filepath.Dir(link), 0o755)
	if err := os.Symlink(shared, link); err != nil {
		t.Skip("no symlinks here:", err)
	}
	tg := []Target{{Agent: "claude", Dir: link, Root: filepath.Join(root, ".claude")}, {Agent: "codex", Dir: shared, Root: root}}
	log, err := Install(tg, false)
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("Claude Code folder is still a link: %v %q", err, log)
	}
	cb, _ := os.ReadFile(filepath.Join(link, "SKILL.md"))
	xb, _ := os.ReadFile(filepath.Join(shared, "SKILL.md"))
	if string(cb) != string(SkillForAgent("claude")) || string(xb) != string(SkillForAgent("codex")) {
		t.Errorf("each agent should have its own copy: %q", log)
	}
	// a link to someone else's skill is not touched
	other := filepath.Join(root, "theirs")
	_ = os.MkdirAll(other, 0o755)
	_ = os.WriteFile(filepath.Join(other, "SKILL.md"), []byte("their skill"), 0o644)
	link2 := filepath.Join(root, "c2", "agent-leeway")
	_ = os.MkdirAll(filepath.Dir(link2), 0o755)
	_ = os.Symlink(other, link2)
	log, err = Install([]Target{{Agent: "claude", Dir: link2, Root: root}}, false)
	if b, _ := os.ReadFile(filepath.Join(other, "SKILL.md")); err != nil || string(b) != "their skill" || !strings.Contains(strings.Join(log, "\n"), "left as is") {
		t.Errorf("foreign linked skill changed: %v %q", err, log)
	}
}
