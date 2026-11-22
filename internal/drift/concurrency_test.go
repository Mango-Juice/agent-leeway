package drift

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Mango-Juice/agent-leeway/internal/py"
)

func runWith(t *testing.T, mode string, items []Item, rb *Rubric, o Options) (Outcome, []py.Value) {
	t.Helper()
	useFake(t, mode)
	cache := t.TempDir()
	res := JudgeAll(context.Background(), cache, items, rb, o)
	return res, LoadResults(cache)
}

func TestConcurrentRunJudgesEverySessionOnce(t *testing.T) {
	items, rb := fixtureItems(t)
	res, recs := runWith(t, "ok", items, rb, Options{Concurrency: 3})
	if len(recs) != len(items) || res.StoppedReason != "" {
		t.Fatalf("%d records for %d items, stopped %q", len(recs), len(items), res.StoppedReason)
	}
	seen := map[string]int{}
	for _, r := range recs {
		seen[r.Get("key").S]++
	}
	for _, it := range items {
		if seen[it.Key] != 1 {
			t.Errorf("%s judged %d times", it.Key, seen[it.Key])
		}
	}
}

func TestLimitStopsNewCalls(t *testing.T) {
	items, rb := fixtureItems(t)
	res, recs := runWith(t, "limit", items, rb, Options{Concurrency: 3})
	if !strings.Contains(res.StoppedReason, "usage-limit") || len(recs) != 0 || res.Done != 0 {
		t.Fatalf("stopped %q, %d records", res.StoppedReason, len(recs))
	}
}

func TestTwoFailuresInARowStop(t *testing.T) {
	items, rb := fixtureItems(t)
	res, recs := runWith(t, "fail", items, rb, Options{Concurrency: 1})
	if !strings.Contains(res.StoppedReason, "two judge calls failed") || len(recs) != 2 {
		t.Fatalf("stopped %q, %d records", res.StoppedReason, len(recs))
	}
	// with 3 in flight the calls already running finish and are recorded, then the run stops
	res, recs = runWith(t, "fail", items, rb, Options{Concurrency: 3})
	// Failures may finish on non-adjacent sessions first (e.g. 0, 2, 4), each freeing a slot for the next session.
	// With 3 in flight, at most 3 such failures can be recorded before every remaining finish is adjacent to one of
	// them; the stop then lets the (at most 3) calls in flight finish and be recorded: at most 2 x 3 = 6 records.
	// (CI on linux/arm64 hit 6 once; the earlier bound of 5 was wrong, not the rule.)
	if !strings.Contains(res.StoppedReason, "two judge calls failed") || len(recs) < 2 || len(recs) > 2*3 {
		t.Fatalf("concurrent: stopped %q, %d records", res.StoppedReason, len(recs))
	}
}

func TestBudgetCheckedBeforeEachCall(t *testing.T) {
	items, rb := fixtureItems(t)
	for i := range items {
		items[i].EstUSD = 0.05
	}
	// budget 0.12 with 3 in flight: 2 calls fit (0.10 reserved), the third would exceed it and never starts
	res, recs := runWith(t, "ok", items, rb, Options{Concurrency: 3, Budget: 0.12})
	if !strings.Contains(res.StoppedReason, "budget") || len(recs) != 2 {
		t.Fatalf("stopped %q, %d records", res.StoppedReason, len(recs))
	}
}

func TestTimeoutEndsTheProcessTree(t *testing.T) {
	items, rb := fixtureItems(t)
	pidFile := filepath.Join(t.TempDir(), "pid")
	t.Setenv("LEEWAY_FAKE_PID", pidFile)
	t0 := time.Now()
	res, recs := runWith(t, "grandchild", items[:1], rb, Options{Concurrency: 1, Timeout: 2 * time.Second})
	if time.Since(t0) > 20*time.Second {
		t.Fatalf("run took %s: a grandchild holding the pipes hung it", time.Since(t0))
	}
	if len(recs) != 1 || !strings.Contains(recs[0].Get("error").S, "timeout") || res.Done != 0 {
		t.Fatalf("records %d, error %q", len(recs), recs[0].Get("error").S)
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(string(b))
	time.Sleep(300 * time.Millisecond)
	if alive(pid) {
		t.Errorf("grandchild %d still running after the timeout", pid)
	}
}

func TestCancelEndsCallsWithoutRecording(t *testing.T) {
	items, rb := fixtureItems(t)
	useFake(t, "sleep")
	cache := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(500*time.Millisecond, cancel)
	res := JudgeAll(ctx, cache, items, rb, Options{Concurrency: 3})
	if !strings.Contains(res.StoppedReason, "cancelled") || len(LoadResults(cache)) != 0 {
		t.Fatalf("stopped %q, %d records", res.StoppedReason, len(LoadResults(cache)))
	}
}

func TestPromptOnlyOnStdin(t *testing.T) {
	items, rb := fixtureItems(t)
	argv := filepath.Join(t.TempDir(), "argv")
	t.Setenv("LEEWAY_FAKE_ARGV", argv)
	special := "quotes \" ' % ^ & | < > ! and\nnewlines, 한국어 😀"
	items[0].Body += "\n" + special
	_, recs := runWith(t, "echo", items[:1], rb, Options{Concurrency: 1})
	b, _ := os.ReadFile(argv)
	if strings.Contains(string(b), "[fx:") || strings.Contains(string(b), "한국어") {
		t.Errorf("prompt text appeared on the command line: %q", b)
	}
	if len(recs) != 1 {
		t.Fatal("no record")
	}
	_ = special
}

func TestShimArgumentsFailClosed(t *testing.T) {
	if err := checkShimArgs(`C:\x\claude.cmd`, []string{"-p", "a&b"}); err == nil {
		t.Error("a .cmd shim must refuse arguments with cmd.exe metacharacters")
	}
	if err := checkShimArgs(`C:\x\claude.exe`, []string{"-p", "a&b"}); err != nil {
		t.Error("a native executable takes any argument")
	}
	if err := checkShimArgs(`C:\x\claude.cmd`, []string{"-p", `C:\Users\Some One\AppData\Local\Temp\x.md`}); err != nil { // privacy-ok: invented example path
		t.Error("spaces are fine for a shim (Go quotes them)")
	}
}

// fakeCLI puts copies of the test binary named claude and codex first on PATH.
func fakeCLI(t *testing.T, mode string) string {
	t.Helper()
	exe, _ := os.Executable()
	bin := filepath.Join(t.TempDir(), "bin with space")
	_ = os.MkdirAll(bin, 0o755)
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	src, _ := os.ReadFile(exe)
	for _, n := range []string{"claude", "codex"} {
		if err := os.WriteFile(filepath.Join(bin, n+ext), src, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LEEWAY_JUDGE_CMD", "")
	t.Setenv("LEEWAY_FAKE_MODE", mode)
	t.Setenv("LEEWAY_CACHE", filepath.Join(t.TempDir(), "cache"))
	argv := filepath.Join(t.TempDir(), "argv")
	t.Setenv("LEEWAY_FAKE_ARGV", argv)
	return argv
}

func TestRealCLIProtocolsPromptOnStdin(t *testing.T) {
	prompt := "transcript with \" ' % ^ & | < > ! and\nnewlines, 한국어 😀 [fx:x]"
	system := "system prompt, 규칙"
	argv := fakeCLI(t, "claude-cli")
	text, u, _, err := Call(context.Background(), "claude:opus", system, prompt, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if text != "SYSTEM="+strconv.Itoa(py.Len(system))+" PROMPT="+prompt || u.Model != "claude-opus-5-5" || u.Input != 10 {
		t.Errorf("claude: %q %+v", text, u)
	}
	t.Setenv("LEEWAY_FAKE_MODE", "codex-cli")
	text, u, _, err = Call(context.Background(), "codex:gpt-6.2-astra", system, prompt, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(text, "PROMPT="+prompt) || u.Input != 60 || u.CacheRead != 40 || u.Output != 10 {
		t.Errorf("codex: %q %+v", text, u)
	}
	b, _ := os.ReadFile(argv)
	for _, bad := range []string{"한국어", "[fx:", "규칙", "newlines"} {
		if strings.Contains(string(b), bad) {
			t.Errorf("prompt or system text on the command line: %q", b)
		}
	}
}
