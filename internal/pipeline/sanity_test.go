package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompareDetectsDifferences(t *testing.T) {
	legacy(t) // the Python reference's prices (docs/17 section 4)
	tb := buildFixtures(t, nil)
	out := t.TempDir()
	tb.Waste[3].MissCost += 0.001
	tb.Calls[5].Kind = "zzz"
	tb.Steps = tb.Steps[1:]
	if err := tb.Dump(fixtures(t), out); err != nil {
		t.Fatal(err)
	}
	d := CompareDumps(filepath.Join("..", "..", "testdata", "expected", "tables"), out, 1e-9, 100)
	if len(d) < 3 {
		t.Fatalf("comparison missed injected differences: %v", d)
	}
	for _, n := range TableNames() {
		b, _ := os.ReadFile(filepath.Join(out, n+".jsonl"))
		t.Logf("%s: %d bytes", n, len(b))
	}
}
