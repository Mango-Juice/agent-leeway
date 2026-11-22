package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/logs"
)

// fixtures returns the synthetic log root (testdata/logs).
func fixtures(t *testing.T) string {
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "logs"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// BuildFixtures reads the fixture logs the way the reference run did (make_expected.py).
func buildFixtures(t *testing.T, cache *logs.FileCache) *Tables {
	root := fixtures(t)
	srcs := logs.Discover(logs.Discovery{CodexDirs: []string{filepath.Join(root, "codex")}, ClaudeDir: filepath.Join(root, "claude")})
	files, skipped := logs.Load(srcs, 4, cache)
	if skipped > 0 {
		t.Fatalf("%d fixture files could not be read", skipped)
	}
	tb := FromFiles(files)
	tb.Build()
	return tb
}

// Gate 1 and 2 (tables): every intermediate table equals the Python reference's dump, row by row.
func TestTablesMatchReference(t *testing.T) {
	legacy(t)                                             // the Python reference's prices (docs/17 section 4)
	for _, withCache := range []bool{false, true, true} { // the second cached run reads from the file cache
		var cache *logs.FileCache
		if withCache {
			cache = logs.NewFileCache(filepath.Join(os.TempDir(), "leeway-test-cache-"+t.Name()))
		}
		tb := buildFixtures(t, cache)
		out := t.TempDir()
		if err := tb.Dump(fixtures(t), out); err != nil {
			t.Fatal(err)
		}
		want := filepath.Join("..", "..", "testdata", "expected", "tables")
		for _, d := range CompareDumps(want, out, 1e-9, 60) {
			t.Error(d)
		}
		if t.Failed() {
			return
		}
	}
}
