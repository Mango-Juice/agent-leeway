package install

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/meta"
)

// The release location used to be a placeholder. Built from two halves so this file does not contain it.
var oldPlaceholder = "OWNER-" + "PLACEHOLDER"

func repoRoot(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// The module path, the GoReleaser config and the release workflow all name the same repository as internal/meta.
func TestReleaseLocationAgrees(t *testing.T) {
	root := repoRoot(t)
	want := "github.com/" + meta.Owner + "/" + meta.Repo
	gomod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(gomod), "module "+want+"\n") {
		t.Errorf("go.mod module path is not %s", want)
	}
	if b, err := os.ReadFile(filepath.Join(root, ".goreleaser.yaml")); err == nil {
		s := string(b)
		if !strings.Contains(s, "-X "+want+"/internal/meta.Version=") {
			t.Errorf(".goreleaser.yaml: ldflags do not set %s/internal/meta.Version", want)
		}
		if n := strings.Count(s, "owner: "); n == 0 || n != strings.Count(s, "owner: "+meta.Owner+"\n") {
			t.Errorf(".goreleaser.yaml: every owner must be %s", meta.Owner)
		}
	}
}

// No file that ships may still carry the old placeholder (the export check scans for it too).
func TestNoReleasePlaceholderLeft(t *testing.T) {
	root := repoRoot(t)
	roots := []string{"cmd", "internal", "install", "skills", "testdata", ".github", ".goreleaser.yaml", "go.mod"}
	if _, err := os.Stat(filepath.Join(root, "public")); err == nil {
		roots = append(roots, "public") // research repository: the public docs live here
	} else {
		roots = append(roots, "docs", "README.md", "README.ko.md", "SECURITY.md", "CONTRIBUTING.md", "CHANGELOG.md")
	}
	for _, r := range roots {
		_ = filepath.WalkDir(filepath.Join(root, r), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			b, err := os.ReadFile(p)
			if err == nil && strings.Contains(string(b), oldPlaceholder) {
				rel, _ := filepath.Rel(root, p)
				t.Errorf("%s still contains the release placeholder", rel)
			}
			return nil
		})
	}
}
