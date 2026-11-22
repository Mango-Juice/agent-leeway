package report

// Regression test for docs/20 L1.

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Report files are private (0600 in a 0700 folder) and a second report in the same minute does not
// overwrite the first.
func TestReportFilesArePrivateAndNotOverwritten(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "reports")
	now := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
	pm, ph, err := Write("# r", "<p>r</p>", dir, now)
	if err != nil {
		t.Fatal(err)
	}
	pm2, _, err := Write("# second", "<p>2</p>", dir, now)
	if err != nil || pm2 == pm {
		t.Fatalf("second report: %s %v", pm2, err)
	}
	if b, _ := os.ReadFile(pm); string(b) != "# r" {
		t.Error("the first report was overwritten")
	}
	if runtime.GOOS == "windows" {
		return
	}
	for _, p := range []string{pm, ph} {
		if fi, _ := os.Stat(p); fi.Mode().Perm()&0o077 != 0 {
			t.Errorf("%s mode %v", p, fi.Mode().Perm())
		}
	}
	if di, _ := os.Stat(dir); di.Mode().Perm()&0o077 != 0 {
		t.Errorf("folder mode %v", di.Mode().Perm())
	}
}
