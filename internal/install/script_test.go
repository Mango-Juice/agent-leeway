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

// fakeRelease writes agent-leeway_<os>_<arch>.tar.gz with a stand-in executable, and checksums.txt.
func fakeRelease(t *testing.T, corrupt bool) string {
	dir := t.TempDir()
	arch := runtime.GOARCH
	name := fmt.Sprintf("agent-leeway_%s_%s.tar.gz", runtime.GOOS, arch)
	f, _ := os.Create(filepath.Join(dir, name))
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	body := []byte("#!/bin/sh\necho fake agent-leeway\n")
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
		cmd.Env = append(os.Environ(), "LEEWAY_RELEASE_BASE=file://"+rel, "LEEWAY_INSTALL_DIR="+dest, "XDG_DATA_HOME="+data)
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
