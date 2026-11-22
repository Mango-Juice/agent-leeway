package install

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/meta"
)

func scriptPath(name string) string {
	p, _ := filepath.Abs(filepath.Join("..", "..", "install", name))
	return p
}

func TestScriptsUseMetaOwner(t *testing.T) {
	for _, n := range []string{"install.sh", "install.ps1"} {
		b, err := os.ReadFile(scriptPath(n))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `"`+meta.Owner+`"`) && !strings.Contains(string(b), `'`+meta.Owner+`'`) {
			t.Errorf("%s: owner differs from internal/meta (%s)", n, meta.Owner)
		}
	}
}

var (
	builtOnce sync.Once
	built     []byte
	buildErr  error
)

// realBinary is this checkout's agent-leeway, built once per test run.
func realBinary(t *testing.T) []byte {
	builtOnce.Do(func() {
		out := filepath.Join(os.TempDir(), fmt.Sprintf("leeway-script-test-%d", os.Getpid()))
		cmd := exec.Command("go", "build", "-o", out, "../../cmd/agent-leeway")
		if b, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("%v %s", err, b)
			return
		}
		built, buildErr = os.ReadFile(out)
		_ = os.Remove(out)
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return built
}

var (
	stubOnce  sync.Once
	stubBuilt []byte
	stubErr   error
)

// stubBinary builds testdata/v010stub, a stand-in for the v0.1.0 release (install-skills without --auto).
func stubBinary(t *testing.T) []byte {
	stubOnce.Do(func() {
		out := filepath.Join(os.TempDir(), fmt.Sprintf("leeway-stub-test-%d", os.Getpid()))
		cmd := exec.Command("go", "build", "-o", out, "./testdata/v010stub")
		if b, err := cmd.CombinedOutput(); err != nil {
			stubErr = fmt.Errorf("%v %s", err, b)
			return
		}
		stubBuilt, stubErr = os.ReadFile(out)
		_ = os.Remove(out)
	})
	if stubErr != nil {
		t.Fatal(stubErr)
	}
	return stubBuilt
}

// fakeRelease writes agent-leeway_<os>_<arch>.tar.gz with a stand-in executable, and checksums.txt.
func fakeRelease(t *testing.T, corrupt bool) string {
	return releaseWith(t, []byte("#!/bin/sh\necho fake agent-leeway\n"), corrupt)
}

// releaseWith is a local release whose executable is body.
func releaseWith(t *testing.T, body []byte, corrupt bool) string {
	dir := t.TempDir()
	arch := runtime.GOARCH
	name := fmt.Sprintf("agent-leeway_%s_%s.tar.gz", runtime.GOOS, arch)
	f, _ := os.Create(filepath.Join(dir, name))
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "agent-leeway", Mode: 0o755, Size: int64(len(body))})
	_, _ = tw.Write(body)
	for _, n := range []string{"LICENSE", "NOTICE.md", "THIRD_PARTY_LICENSES.txt", "README.md"} {
		txt := []byte("text of " + n + "\n")
		_ = tw.WriteHeader(&tar.Header{Name: n, Mode: 0o644, Size: int64(len(txt))})
		_, _ = tw.Write(txt)
	}
	tw.Close()
	gz.Close()
	f.Close()
	b, _ := os.ReadFile(filepath.Join(dir, name))
	sum := sha256.Sum256(b)
	h := hex.EncodeToString(sum[:])
	if corrupt {
		h = strings.Repeat("0", 64)
	}
	_ = os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(h+"  "+name+"\n"), 0o644)
	return dir
}

func TestInstallShFromLocalRelease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is for macOS and Linux")
	}
	for _, corrupt := range []bool{false, true} {
		rel := fakeRelease(t, corrupt)
		dest := filepath.Join(t.TempDir(), "bin dir")
		cmd := exec.Command("sh", scriptPath("install.sh"))
		data := filepath.Join(t.TempDir(), "share")
		cmd.Env = append(os.Environ(), "LEEWAY_RELEASE_BASE=file://"+rel, "LEEWAY_INSTALL_DIR="+dest, "XDG_DATA_HOME="+data, "LEEWAY_NO_SKILLS=1")
		out, err := cmd.CombinedOutput()
		_, statErr := os.Stat(filepath.Join(dest, "agent-leeway"))
		if corrupt {
			if err == nil || statErr == nil || !strings.Contains(string(out), "checksum mismatch") {
				t.Errorf("a bad checksum must fail and install nothing: %v %s", err, out)
			}
			continue
		}
		if err != nil || statErr != nil {
			t.Fatalf("install failed: %v %s", err, out)
		}
		if run, _ := exec.Command(filepath.Join(dest, "agent-leeway")).Output(); !strings.Contains(string(run), "fake agent-leeway") {
			t.Error("installed file does not run")
		}
		for _, n := range []string{"LICENSE", "NOTICE.md", "THIRD_PARTY_LICENSES.txt"} {
			if b, err := os.ReadFile(filepath.Join(data, "agent-leeway", n)); err != nil || string(b) != "text of "+n+"\n" {
				t.Errorf("license file %s not installed: %v", n, err)
			}
		}
	}
}

// The installer finishes by adding the skill (install-skills --auto): written for the agents found, skipped
// with LEEWAY_NO_SKILLS, one line and a working install when no agent is found, safe to run again, and a failing
// skill step never fails the install.
func TestInstallShAddsSkills(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is for macOS and Linux")
	}
	good := releaseWith(t, realBinary(t), false)
	install := func(t *testing.T, rel, home string, extra ...string) (string, error) {
		dest := filepath.Join(home, "bin dir")
		cmd := exec.Command("sh", scriptPath("install.sh"))
		cmd.Env = append(os.Environ(), "LEEWAY_RELEASE_BASE=file://"+rel, "LEEWAY_INSTALL_DIR="+dest,
			"XDG_DATA_HOME="+filepath.Join(home, "share"), "HOME="+home, "CLAUDE_CONFIG_DIR=", "CODEX_HOME=", "LEEWAY_NO_SKILLS=")
		cmd.Env = append(cmd.Env, extra...)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	newHome := func(agents ...string) string {
		h := filepath.Join(t.TempDir(), "home")
		for _, a := range agents {
			_ = os.MkdirAll(filepath.Join(h, a), 0o755)
		}
		return h
	}
	claude := func(h string) string { return filepath.Join(h, ".claude", "skills", "agent-leeway", "SKILL.md") }
	codex := func(h string) string { return filepath.Join(h, ".agents", "skills", "agent-leeway", "SKILL.md") }

	t.Run("both agents, then a re-run", func(t *testing.T) {
		h := newHome(".claude", ".codex")
		own := codex(h)
		_ = os.MkdirAll(filepath.Dir(own), 0o755)
		_ = os.WriteFile(own, []byte("my own skill"), 0o644)
		out, err := install(t, good, h)
		if err != nil {
			t.Fatalf("%v %s", err, out)
		}
		for _, want := range []string{"Claude Code: added the skill: " + claude(h) + " (undo: agent-leeway uninstall-skills --claude)",
			"Codex: your own " + own + " was not written by agent-leeway; backed up to ", "Codex: added the skill: " + own,
			`Next: ask Claude Code or Codex for a "leeway report". In a terminal instead: `} {
			if !strings.Contains(out, want) {
				t.Errorf("missing %q in:\n%s", want, out)
			}
		}
		if b, _ := os.ReadFile(claude(h)); !strings.Contains(string(b), "--tool claude") {
			t.Error("Claude Code skill not written")
		}
		if baks, _ := filepath.Glob(own + ".bak-*"); len(baks) != 1 {
			t.Errorf("the user's own skill must be backed up once: %v", baks)
		}
		out, err = install(t, good, h)
		if err != nil || strings.Count(out, "the skill is already up to date") != 2 {
			t.Errorf("re-run: %v\n%s", err, out)
		}
		if baks, _ := filepath.Glob(own + ".bak-*"); len(baks) != 1 {
			t.Errorf("a re-run must not make another backup: %v", baks)
		}
	})
	t.Run("opt-out", func(t *testing.T) {
		h := newHome(".claude", ".codex")
		out, err := install(t, good, h, "LEEWAY_NO_SKILLS=1")
		if err != nil || !strings.Contains(out, "Skills: not added (LEEWAY_NO_SKILLS is set).") || !strings.Contains(out, "install-skills` once") {
			t.Errorf("%v\n%s", err, out)
		}
		if _, err := os.Stat(claude(h)); err == nil {
			t.Error("opt-out wrote a skill")
		}
	})
	t.Run("no agent", func(t *testing.T) {
		h := newHome()
		out, err := install(t, good, h)
		if err != nil || !strings.Contains(out, "Skills: not added. No Claude Code or Codex found on this computer") || strings.Contains(out, "see the message above") {
			t.Errorf("%v\n%s", err, out)
		}
		if _, err := os.Stat(filepath.Join(h, "bin dir", "agent-leeway")); err != nil {
			t.Error("program not installed")
		}
	})
	t.Run("older release without --auto", func(t *testing.T) {
		old := releaseWith(t, stubBinary(t), false)
		h := newHome(".claude", ".codex")
		out, err := install(t, old, h)
		if err != nil || strings.Contains(out, "flag provided but not defined") || strings.Contains(out, "see the message above") ||
			!strings.Contains(out, "claude: write "+claude(h)) || !strings.Contains(out, `Next: ask Claude Code or Codex for a "leeway report"`) {
			t.Errorf("%v\n%s", err, out)
		}
		if _, err := os.Stat(codex(h)); err != nil {
			t.Error("Codex skill not written by the older release")
		}
		out, err = install(t, old, newHome())
		if err != nil || !strings.Contains(out, "Skills: not added. No Claude Code or Codex found on this computer") ||
			strings.Contains(out, "see the message above") || strings.Contains(out, "Neither Claude Code") {
			t.Errorf("no agent, older release: %v\n%s", err, out)
		}
	})
	t.Run("skill step fails", func(t *testing.T) {
		h := newHome(".claude")
		bad := releaseWith(t, []byte("#!/bin/sh\necho boom >&2\nexit 1\n"), false)
		out, err := install(t, bad, h)
		if err != nil || !strings.Contains(out, "Skills: not added (see the message above). The program is installed") {
			t.Errorf("a failing skill step must not fail the install: %v\n%s", err, out)
		}
	})
}
