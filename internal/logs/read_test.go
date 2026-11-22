package logs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/py"

	"github.com/klauspost/compress/zstd"
)

func writeFile(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadLinesLongCRLFPartialInvalid(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("가", 3_000_000) // 9 MB line: a fixed-size scanner would fail here
	body := `{"a":1}` + "\r\n" + `{"t":"` + big + `"}` + "\n" + "[1]\n" + "not json\n" + "\n" + `{"b":"x` + "\xff" + `y"}` + "\n" + `{"partial":`
	p := filepath.Join(dir, "x.jsonl")
	writeFile(t, p, []byte(body))
	var st Stats
	var got []py.Value
	if err := ReadLines(p, &st, func(v py.Value) { got = append(got, v) }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 { // {"a"}, {"t"}, [1], {"b"}
		t.Fatalf("got %d values", len(got))
	}
	if py.Len(got[1].Get("t").S) != 3_000_000 {
		t.Errorf("long line: %d code points", py.Len(got[1].Get("t").S))
	}
	if got[3].Get("b").S != "x�y" {
		t.Errorf("invalid UTF-8 should become U+FFFD: %q", got[3].Get("b").S)
	}
	if st.Broken != 2 || st.Blank != 1 {
		t.Errorf("stats %+v", st)
	}
}

func TestZstdAndTwins(t *testing.T) {
	dir := t.TempDir()
	plain := []byte(`{"timestamp":"2026-01-01T00:00:00Z","type":"session_meta","payload":{"id":"x"}}` + "\n")
	enc, _ := zstd.NewWriter(nil)
	z := enc.EncodeAll(plain, nil)
	writeFile(t, filepath.Join(dir, "s", "2026", "rollout-a.jsonl.zst"), z)
	writeFile(t, filepath.Join(dir, "s", "2026", "rollout-b.jsonl"), plain)
	writeFile(t, filepath.Join(dir, "s", "2026", "rollout-b.jsonl.zst"), z)
	writeFile(t, filepath.Join(dir, "s", ".hidden", "rollout-c.jsonl"), plain)
	writeFile(t, filepath.Join(dir, "arch", "rollout-a.jsonl"), plain) // already seen in sessions
	writeFile(t, filepath.Join(dir, "arch", "rollout-d.jsonl"), plain)
	srcs := Discover(Discovery{CodexDirs: []string{filepath.Join(dir, "s"), filepath.Join(dir, "arch")}})
	var names []string
	for _, s := range srcs {
		names = append(names, filepath.Base(s.Path))
	}
	if strings.Join(names, ",") != "rollout-a.jsonl.zst,rollout-b.jsonl,rollout-d.jsonl" {
		t.Fatalf("discovered %v", names)
	}
	var st Stats
	n := 0
	if err := ReadLines(srcs[0].Path, &st, func(py.Value) { n++ }); err != nil || n != 1 {
		t.Fatalf("zstd read: %v %d", err, n)
	}
}

func TestCodePointLengthsNotBytes(t *testing.T) {
	dir := t.TempDir()
	out := strings.Repeat("é", 40001) // 40,001 characters (80,002 bytes): oversized by characters
	line := `{"type":"assistant","sessionId":"s","timestamp":"2026-01-01T00:00:00Z","message":{"id":"m","model":"claude-opus-5-5","content":[{"type":"tool_use","id":"t","name":"Bash","input":{"command":"cat a"}}],"usage":{"input_tokens":1}}}` + "\n" +
		`{"type":"user","sessionId":"s","timestamp":"2026-01-01T00:00:01Z","message":{"content":[{"type":"tool_result","tool_use_id":"t","content":"` + out + `"}]}}` + "\n"
	p := filepath.Join(dir, "p", "s.jsonl")
	writeFile(t, p, []byte(line))
	f, err := ParseClaude(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Calls[0].OutChars != 40001 || f.Calls[0].Kind != "read" {
		t.Errorf("call %+v", f.Calls[0])
	}
}

func TestParseTime(t *testing.T) {
	cases := map[string]int64{
		"2026-01-10T09:00:00Z":           1768035600e9,
		"2026-01-10T09:00:00.5Z":         1768035600e9 + 5e8,
		"2026-01-10T18:00:00+09:00":      1768035600e9,
		"2026-01-10T09:00:00":            1768035600e9,
		"2026-01-10T09:00:00.123456789Z": 1768035600e9 + 123456789,
		"garbage":                        NaT,
		"2026-02-30T00:00:00Z":           NaT,
	}
	for s, want := range cases {
		if got := ParseTime(s); got != want {
			t.Errorf("ParseTime(%q) = %d, want %d", s, got, want)
		}
	}
}

// Long paths (over 260 characters), non-ASCII names and spaces are read on every OS.
func TestLongNonASCIIPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "사용자 Ünïcode name", ".claude", "projects")
	deep := root
	for i := 0; i < 12; i++ {
		deep = filepath.Join(deep, "a folder with spaces and 한글 "+strings.Repeat("x", 12))
	}
	p := filepath.Join(deep, "s.jsonl")
	writeFile(t, p, []byte(`{"type":"user","sessionId":"s","timestamp":"2026-01-01T00:00:00Z","message":{"content":"hi there"},"origin":{"kind":"human"}}`+"\r\n"))
	if len(p) < 300 {
		t.Fatalf("path is only %d characters", len(p))
	}
	srcs := Discover(Discovery{ClaudeDir: root})
	if len(srcs) != 1 {
		t.Fatalf("found %d files", len(srcs))
	}
	f, err := ParseClaude(srcs[0].Path)
	if err != nil || len(f.Turns) != 1 || f.Turns[0].Text != "hi there" {
		t.Fatalf("%v %+v", err, f)
	}
}

func TestPerResponseModelAndWriteSplit(t *testing.T) {
	dir := t.TempDir()
	cl := `{"type":"assistant","sessionId":"s","timestamp":"2026-01-01T00:00:00Z","message":{"id":"a","model":"claude-haiku-5-5","content":[],"usage":{"input_tokens":1,"cache_creation_input_tokens":30,"cache_creation":{"ephemeral_5m_input_tokens":10,"ephemeral_1h_input_tokens":20}}}}` + "\n" +
		`{"type":"assistant","sessionId":"s","timestamp":"2026-01-01T00:01:00Z","message":{"id":"b","model":"claude-opus-5-5","content":[],"usage":{"input_tokens":1}}}` + "\n"
	p := filepath.Join(dir, "p", "s.jsonl")
	writeFile(t, p, []byte(cl))
	f, err := ParseClaude(p)
	if err != nil || len(f.Steps) != 2 {
		t.Fatal(err)
	}
	if f.Steps[0].Model.S != "claude-haiku-5-5" || f.Steps[1].Model.S != "claude-opus-5-5" || f.Steps[0].CW5m != 10 || f.Steps[0].CW1h != 20 {
		t.Errorf("claude steps %+v", f.Steps)
	}
	cx := `{"timestamp":"2026-01-01T00:00:00Z","type":"session_meta","payload":{"id":"x","session_id":"x"}}` + "\n" +
		`{"timestamp":"2026-01-01T00:00:01Z","type":"turn_context","payload":{"model":"gpt-6.2-sol"}}` + "\n" +
		`{"timestamp":"2026-01-01T00:00:02Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":5},"total_token_usage":{"input_tokens":5}}}}` + "\n" +
		`{"timestamp":"2026-01-01T00:00:03Z","type":"turn_context","payload":{"model":"gpt-6-astra"}}` + "\n" +
		`{"timestamp":"2026-01-01T00:00:04Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":6},"total_token_usage":{"input_tokens":11}}}}` + "\n"
	p = filepath.Join(dir, "rollout-2026-01-01T00-00-00-00000000-0000-4000-8000-000000000001.jsonl")
	writeFile(t, p, []byte(cx))
	g, err := ParseCodex(p)
	if err != nil || len(g.Steps) != 2 || g.Steps[0].Model.S != "gpt-6.2-sol" || g.Steps[1].Model.S != "gpt-6-astra" || g.Thread.Model.S != "gpt-6.2-sol" {
		t.Errorf("codex steps %+v", g.Steps)
	}
}

func TestOversizedLineAndHugeNumbersAreContained(t *testing.T) {
	old := MaxLine
	MaxLine = 1 << 20
	defer func() { MaxLine = old }()
	p := filepath.Join(t.TempDir(), "x.jsonl")
	writeFile(t, p, []byte(`{"a":1}`+"\n"+`{"t":"`+strings.Repeat("x", 3<<20)+`"}`+"\n"+`{"b":2}`+"\n"))
	var st Stats
	n := 0
	if err := ReadLines(p, &st, func(py.Value) { n++ }); err != nil || n != 2 || st.Broken != 1 {
		t.Fatalf("n %d broken %d err %v", n, st.Broken, err)
	}
	if v, _ := py.Loads(`1e30`); intOf(v) != maxTokens {
		t.Errorf("1e30 -> %d", intOf(v))
	}
	if v, _ := py.Loads(`-1e30`); intOf(v) > 0 {
		t.Error("negative clamp")
	}
}
