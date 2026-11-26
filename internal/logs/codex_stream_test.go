package logs

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// sameParse says the streaming reader and the reader of v0.1.13 give the same File (or the same error).
func sameParse(t *testing.T, path string) {
	t.Helper()
	a, errA := ParseCodex(path)
	b, errB := parseCodexLegacy(path)
	if (errA == nil) != (errB == nil) || (errA != nil && errA.Error() != errB.Error()) {
		t.Errorf("%s: errors differ: %v / %v", filepath.Base(path), errA, errB)
		return
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("%s: the streaming reader differs from v0.1.13", filepath.Base(path))
	}
}

// Every Codex rollout in the test fixtures parses to the same File as before.
func TestCodexStreamParityFixtures(t *testing.T) {
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..", "testdata")
	n := 0
	_ = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() && strings.HasPrefix(fi.Name(), "rollout-") && (strings.HasSuffix(p, ".jsonl") || strings.HasSuffix(p, ".jsonl.zst")) {
			n++
			sameParse(t, p)
		}
		return nil
	})
	if n < 50 {
		t.Fatalf("only %d fixtures found", n)
	}
}

func zst(t *testing.T, plain []byte) []byte {
	t.Helper()
	w, _ := zstd.NewWriter(nil)
	return w.EncodeAll(plain, nil)
}

// Odd files read the same as before: broken and invalid lines, blank lines, a value that is not an object, an
// empty file, a cut last line, an oversized line, an unknown record type, a broken .zst, a .zst cut short.
func TestCodexStreamParityOdd(t *testing.T) {
	dir := t.TempDir()
	good := `{"timestamp":"2026-11-20T10:00:00Z","type":"session_meta","payload":{"id":"thread-a","cwd":"/w","source":"cli"}}` + "\n" + // privacy-ok: made-up path
		`{"timestamp":"2026-11-20T10:00:01Z","type":"turn_context","payload":{"model":"gpt-6.2-astra","cwd":"/w"}}` + "\n" +
		`{"timestamp":"2026-11-20T10:00:02Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"fix it"}]}}` + "\n" +
		`{"timestamp":"2026-11-20T10:00:03Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10},"last_token_usage":{"input_tokens":10,"cached_input_tokens":4,"output_tokens":3}}}}` + "\n"
	big := `{"timestamp":"2026-11-20T10:00:04Z","type":"response_item","payload":{"type":"function_call_output","call_id":"c","output":"` + strings.Repeat("x", 6<<20) + `"}}` + "\n"
	cases := map[string][]byte{
		"empty":     nil,
		"good":      []byte(good),
		"broken":    []byte(good + "{not json\n\n   \n[1,2]\n\"text\"\n" + `{"type":"brand_new_kind","payload":{}}` + "\n"),
		"cut":       []byte(good + `{"timestamp":"2026-11-20T10:00:05Z","type":"event_msg","payload":{"type":"tok`),
		"crlf":      []byte(strings.ReplaceAll(good, "\n", "\r\n")),
		"long-line": []byte(good + big + good),
	}
	for name, body := range cases {
		p := filepath.Join(dir, "rollout-2026-11-20T10-00-00-"+name+".jsonl")
		_ = os.WriteFile(p, body, 0o600)
		sameParse(t, p)
		z := p + ".zst"
		_ = os.WriteFile(z, zst(t, body), 0o600)
		sameParse(t, z)
	}
	old := MaxLine
	MaxLine = 1 << 20 // an oversized line is skipped and counted as broken, in both
	sameParse(t, filepath.Join(dir, "rollout-2026-11-20T10-00-00-long-line.jsonl"))
	MaxLine = old
	bad := filepath.Join(dir, "rollout-2026-11-20T10-00-00-badzst.jsonl.zst")
	_ = os.WriteFile(bad, []byte("not zstd at all"), 0o600)
	sameParse(t, bad)
	full := zst(t, []byte(good+good))
	short := filepath.Join(dir, "rollout-2026-11-20T10-00-00-shortzst.jsonl.zst")
	_ = os.WriteFile(short, full[:len(full)-7], 0o600)
	sameParse(t, short)
}

// A session being written grows between the two passes: the second pass stops where the first did, so the result
// is the file as the first pass saw it (a cut last line included). Both passes read the same open file, so a file
// moved, deleted or replaced by name in between reads as it was. A file rewritten in place with other content is
// read again; rewritten every time, it is an error, never a mix of two contents.
func TestCodexStreamFileChanges(t *testing.T) {
	dir := t.TempDir()
	line := func(ts, text string) string {
		return `{"timestamp":"` + ts + `","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + text + `"}]}}` + "\n"
	}
	head := `{"timestamp":"2026-11-20T10:00:00Z","type":"session_meta","payload":{"id":"thread-g","cwd":"/w","source":"cli"}}` + "\n" + line("2026-11-20T10:00:01Z", "one") // privacy-ok: made-up path
	cut := `{"timestamp":"2026-11-20T10:00:02Z","type":"response_item","payload":{"type":"mess`
	for _, ext := range []string{".jsonl", ".jsonl.zst"} {
		p := filepath.Join(dir, "rollout-2026-11-20T10-00-00-thread-g"+ext)
		enc := func(s string) []byte {
			if ext == ".jsonl" {
				return []byte(s)
			}
			return zst(t, []byte(s))
		}
		byName := func(s string) { // a new file under the same name
			tmp := p + ".tmp"
			_ = os.WriteFile(tmp, enc(s), 0o600)
			_ = os.Rename(tmp, p)
		}
		inPlace := func(s string) { _ = os.WriteFile(p, enc(s), 0o600) } // truncated and rewritten, same file
		run := func(name string, start string, between func(), want *File) {
			t.Helper()
			byName(start)
			if want == nil {
				want, _ = parseCodexLegacy(p)
			}
			betweenPasses = func(string) { between() }
			got, err := ParseCodex(p)
			betweenPasses = nil
			if err != nil || got == nil {
				t.Errorf("%s %s: %v", ext, name, err)
				return
			}
			got.Thread.Bytes, got.Thread.File = want.Thread.Bytes, want.Thread.File // the size on disk when parsing ends, as before
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s %s: %d turns, want %d", ext, name, len(got.Turns), len(want.Turns))
			}
		}
		if ext == ".jsonl" { // grows: appended to, as Codex writes a session
			run("grown", head+cut, func() {
				f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
				f.WriteString(`sage","role":"user","content":[{"type":"input_text","text":"two"}]}}` + "\n" + line("2026-11-20T10:00:03Z", "three"))
				f.Close()
			}, nil)
		}
		run("replaced by name", head, func() { byName(strings.Replace(head, `"one"`, `"uno"`, 1) + line("2026-11-20T10:00:04Z", "other")) }, nil)
		run("moved away", head, func() { _ = os.Rename(p, p+".moved") }, nil)
		run("deleted", head, func() { _ = os.Remove(p) }, nil)
		// rewritten in place once, longer or shorter: read again, the result is the new content
		for _, next := range []string{strings.Replace(head, `"one"`, `"uno"`, 1) + line("2026-11-20T10:00:04Z", "other"), head[:len(head)/2] + "\n"} {
			byName(next)
			want, _ := parseCodexLegacy(p)
			calls := 0
			run("rewritten in place", head, func() {
				if calls++; calls == 1 {
					inPlace(next)
				}
			}, want)
		}
		// rewritten in place at every read: an error
		byName(head)
		n := 0
		betweenPasses = func(string) { n++; inPlace(strings.Replace(head, `"one"`, `"`+strings.Repeat("z", n)+`"`, 1)) }
		_, err := ParseCodex(p)
		betweenPasses = nil
		if !errors.Is(err, errChanged) {
			t.Errorf("%s rewritten always: %v", ext, err)
		}
	}
}

// Review of the streaming reader (f2182bb): cases that a broken second pass or first-pass count would change.
//   - a complete last line without a newline at the end is a record;
//   - a file whose own session_meta starts its history at an ordinal: the cache events before it are the parent's
//     (inherited) and left out, which only the first pass can know;
//   - token_count and token_usage_record in one file, and token_usage_record alone (UsageOnlyNew).
func TestCodexStreamParityReview(t *testing.T) {
	dir := t.TempDir()
	tc := func(ord int, model string) string {
		return `{"timestamp":"2026-11-20T10:00:0` + string(rune('0'+ord)) + `Z","ordinal":` + string(rune('0'+ord)) + `,"type":"turn_context","payload":{"model":"` + model + `","cwd":"/w"}}` + "\n" // privacy-ok: made-up path
	}
	usage := `{"timestamp":"2026-11-20T10:00:08Z","type":"token_usage_record","payload":{"input_tokens":5}}` + "\n"
	count := `{"timestamp":"2026-11-20T10:00:09Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":10},"last_token_usage":{"input_tokens":10,"cached_input_tokens":4,"output_tokens":3}}}}` + "\n"
	for name, body := range map[string]func(id string) string{
		"no-final-newline": func(id string) string {
			return `{"timestamp":"2026-11-20T10:00:00Z","type":"session_meta","payload":{"id":"` + id + `","cwd":"/w","source":"cli"}}` + "\n" + tc(1, "a") + strings.TrimSuffix(count, "\n") // privacy-ok: made-up path
		},
		"own-history": func(id string) string {
			return tc(1, "a") + tc(2, "b") + `{"timestamp":"2026-11-20T10:00:03Z","ordinal":3,"type":"session_meta","payload":{"id":"` + id + `","cwd":"/w","source":{"subagent":"x"},"subagent_history_start_ordinal":4}}` + "\n" + // privacy-ok: made-up path
				tc(4, "c") + tc(5, "d") + tc(6, "e")
		},
		"both-usage-kinds":  func(id string) string { return tc(1, "a") + usage + count },
		"usage-record-only": func(id string) string { return tc(1, "a") + usage },
	} {
		p := filepath.Join(dir, "rollout-2026-11-20T10-00-00-"+name+"-0000-4000-8000-000000000001.jsonl")
		body := body(fileID(p))
		_ = os.WriteFile(p, []byte(body), 0o600)
		sameParse(t, p)
		_ = os.WriteFile(p+".zst", zst(t, []byte(body)), 0o600)
		sameParse(t, p+".zst")
		f, err := ParseCodex(p)
		if err != nil {
			t.Fatal(err)
		}
		switch name { // the case is what it says, so the comparison above is not vacuous
		case "no-final-newline":
			if len(f.Steps) != 1 {
				t.Errorf("%s: %d steps", name, len(f.Steps))
			}
		case "own-history":
			if len(f.Events) != 2 { // model b->c is left out (before the history start), c->d and d->e are kept
				t.Errorf("%s: events %+v", name, f.Events)
			}
		case "both-usage-kinds":
			if f.Stats.UsageOnlyNew != 0 {
				t.Errorf("%s: UsageOnlyNew %d", name, f.Stats.UsageOnlyNew)
			}
		case "usage-record-only":
			if f.Stats.UsageOnlyNew != 1 {
				t.Errorf("%s: UsageOnlyNew %d", name, f.Stats.UsageOnlyNew)
			}
		}
	}
}
