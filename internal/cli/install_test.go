package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/install"
)

// skillHome points the agents' folders at a temporary home (never the real one).
func skillHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "Hōme with spaces")
	_ = os.MkdirAll(home, 0o755)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	return home
}

func TestInstallAndUninstallSkills(t *testing.T) {
	home := skillHome(t)
	if _, _, rc := run(t, "install-skills"); rc != 1 {
		t.Error("with no agent folder and no flag, nothing should be installed")
	}
	_ = os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	_ = os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
	// a user's own skill of the same name is backed up, never lost
	own := filepath.Join(home, ".agents", "skills", "agent-leeway", "SKILL.md")
	_ = os.MkdirAll(filepath.Dir(own), 0o755)
	_ = os.WriteFile(own, []byte("my own skill"), 0o644)
	out, errs, rc := run(t, "install-skills", "--dry-run")
	if rc != 0 || !strings.Contains(out, "backed up") {
		t.Fatalf("dry run: %d %s %s", rc, out, errs)
	}
	if b, _ := os.ReadFile(own); string(b) != "my own skill" {
		t.Fatal("dry run changed a file")
	}
	if _, errs, rc = run(t, "install-skills"); rc != 0 {
		t.Fatal(errs)
	}
	for _, p := range []string{filepath.Join(home, ".claude", "skills", "agent-leeway", "SKILL.md"), own} {
		b, err := os.ReadFile(p)
		if err != nil || !strings.Contains(string(b), "agent-leeway leak") || strings.Contains(string(b), "{{") {
			t.Errorf("%s: %v", p, err)
		}
	}
	// each copy judges its own tool's sessions by default
	cb, _ := os.ReadFile(filepath.Join(home, ".claude", "skills", "agent-leeway", "SKILL.md"))
	xb, _ := os.ReadFile(own)
	if !strings.Contains(string(cb), "drift --yes --tool claude") || strings.Contains(string(cb), "--tool codex") ||
		!strings.Contains(string(xb), "drift --yes --tool codex") || strings.Contains(string(xb), "--tool claude") ||
		!strings.Contains(string(cb), "--tool all") {
		t.Error("the installed copies do not pass their own tool")
	}
	baks, _ := filepath.Glob(own + ".bak-*")
	if len(baks) != 1 {
		t.Errorf("backup: %v", baks)
	}
	if out, _, _ = run(t, "install-skills"); !strings.Contains(out, "already up to date") {
		t.Errorf("reinstall: %s", out)
	}
	if _, errs, rc = run(t, "uninstall-skills"); rc != 0 {
		t.Fatal(errs)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "agent-leeway")); !os.IsNotExist(err) {
		t.Error("skill folder left behind")
	}
	if b, _ := os.ReadFile(own); string(b) != "my own skill" {
		t.Error("uninstall must put the user's own skill back")
	}
}

func TestSkillHasNoShellSpecificSyntax(t *testing.T) {
	s := string(install.Skill())
	for _, bad := range []string{"```bash", "```sh", "!`", "$(", "&&", "shell:"} {
		if strings.Contains(s, bad) {
			t.Errorf("SKILL.md contains %q (bash-only constructs fail on Windows without Git Bash)", bad)
		}
	}
	if !strings.HasPrefix(s, "---\nname: agent-leeway\n") {
		t.Error("SKILL.md frontmatter")
	}
}

// The published skill folder (for `npx skills add`) is the rendered template.
func TestRepoSkillIsRendered(t *testing.T) {
	b, err := os.ReadFile(repo("skills", "agent-leeway", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != string(install.SkillFor(install.RepoCopy, "")) {
		t.Error("skills/agent-leeway/SKILL.md is stale: run `go run ./cmd/agent-leeway debug-write-skill`")
	}
}

func TestSelfUpdateCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "agent-leeway/") {
			t.Errorf("user agent %q", r.Header.Get("User-Agent"))
		}
		w.Write([]byte(`{"tag_name": "v9.0.0", "html_url": "https://example.invalid/r"}`))
	}))
	defer srv.Close()
	t.Setenv("LEEWAY_RELEASE_API", srv.URL)
	out, _, rc := run(t, "self-update", "--check")
	if rc != 0 || !strings.Contains(out, "v9.0.0") || !strings.Contains(out, "install line") {
		t.Errorf("rc %d: %s", rc, out)
	}
	if _, _, rc := run(t, "self-update"); rc != 2 {
		t.Error("self-update without --check must not do anything")
	}
	if printable("v1\x1b[31m.0\x07") != "v1[31m.0" {
		t.Error("escape sequences from the server must not reach the terminal")
	}
	// a release build reports the tag without its "v"; a build without ldflags reports 0.1.0-dev
	if !newer("v0.1.0", "0.1.0-dev") || newer("v0.1.0", "0.1.0") || newer("v0.0.9", "0.1.0-dev") ||
		!newer("v0.2.0", "0.2.0-dev") || newer("v0.1.9", "0.2.0-dev") || newer("v0.2.0", "v0.2.0") || !newer("1.2.3.4", "1.2.3") {
		t.Error("version order")
	}
}

// install-skills --auto (run by the installers): exit 3 with one line when no agent is found, not combined with
// --claude/--codex.
func TestInstallSkillsAuto(t *testing.T) {
	home := skillHome(t)
	out, _, rc := run(t, "install-skills", "--auto")
	if rc != 3 || !strings.HasPrefix(out, "Skills: not added. No Claude Code or Codex found") || strings.Count(out, "\n") != 1 {
		t.Errorf("no agent: rc %d %q", rc, out)
	}
	if _, _, rc = run(t, "install-skills", "--auto", "--claude"); rc != 2 {
		t.Errorf("--auto with --claude: rc %d", rc)
	}
	_ = os.MkdirAll(filepath.Join(home, ".codex"), 0o755)
	out, _, rc = run(t, "install-skills", "--auto")
	if rc != 0 || !strings.HasPrefix(out, "Codex: added the skill: ") || strings.Contains(out, "Claude Code") || strings.Contains(out, "Ask your agent") {
		t.Errorf("codex only: rc %d %q", rc, out)
	}
}
