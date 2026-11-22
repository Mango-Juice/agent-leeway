package logs

// Regression test for docs/20 M5.

import (
	"os"
	"path/filepath"
	"testing"
)

// A parse is stored only under the stamp the file had before reading; a line appended in the meantime makes
// the parse stale, so it is not stored and the next run reads the file again.
func TestCacheNeverStoresAStaleParse(t *testing.T) {
	src, err := filepath.Glob(filepath.Join("..", "..", "testdata", "logs", "claude", "*", "*.jsonl"))
	if err != nil || len(src) == 0 {
		t.Skip("no fixture transcripts")
	}
	b, _ := os.ReadFile(src[0])
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	before := StampOf(p)
	parsed, err := ParseClaude(p)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
	_, _ = f.WriteString("\n{\"partial\": \n")
	f.Close()
	c := NewFileCache(t.TempDir())
	c.Put(p, parsed, before)
	if c.Get(p) != nil {
		t.Fatal("a parse that misses the newest lines was cached")
	}
	// an unchanged file is cached and served
	before = StampOf(p)
	again, _ := ParseClaude(p)
	c.Put(p, again, before)
	if c.Get(p) == nil {
		t.Fatal("an unchanged file was not cached")
	}
}
