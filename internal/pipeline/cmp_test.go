package pipeline

import (
	"os"
	"strings"
	"testing"
)

// TestCompareLocalDumps compares two dump folders given by LEEWAY_CMP_WANT / LEEWAY_CMP_GOT (manual parity
// check on real logs; skipped otherwise).
func TestCompareLocalDumps(t *testing.T) {
	want, got := os.Getenv("LEEWAY_CMP_WANT"), os.Getenv("LEEWAY_CMP_GOT")
	if want == "" || got == "" {
		t.Skip("set LEEWAY_CMP_WANT and LEEWAY_CMP_GOT")
	}
	var skip []string
	if b, err := os.ReadFile(os.Getenv("LEEWAY_CMP_SKIP")); err == nil {
		skip = strings.Fields(string(b))
	}
	for _, d := range CompareDumpsExcept(want, got, 1e-9, 40, skip) {
		t.Error(d)
	}
}
