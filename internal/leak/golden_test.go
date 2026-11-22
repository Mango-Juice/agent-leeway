package leak

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/py"
)

var update = flag.Bool("update", false, "rewrite testdata/expected-go (outputs of the Go product's own defaults)")

// The product's default pricing differs from the Python reference on purpose (docs/17 section 4), so its
// outputs on the fixtures are pinned by goldens written by this test (go test ./internal/leak -update).
func TestLeakGoldenCurrentPrices(t *testing.T) {
	tb := fixtureTables(t)
	x := fixtureReports(t, tb, "")
	loc := fxLoc(t)
	files := map[string]string{
		"leak.txt":  Render(x.R, 5, x.P, x.H, x.A, x.PH, x.PA, fxNow, loc) + "\n",
		"leak.json": py.DumpsOpt(JSON(x.R, x.H, x.A, x.P, x.PH, x.PA), py.DumpOpts{Indent: 1}) + "\n",
	}
	dir := filepath.Join("..", "..", "testdata", "expected-go")
	for name, got := range files {
		p := filepath.Join(dir, name)
		if *update {
			_ = os.MkdirAll(dir, 0o755)
			if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%v (run go test ./internal/leak -run Golden -update)", err)
		}
		diffText(t, name, string(want), got)
	}
}
