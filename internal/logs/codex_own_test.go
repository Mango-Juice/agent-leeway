package logs

import (
	"bytes"
	"encoding/gob"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/meta"
)

// A sub-agent's rollout begins with its own session_meta and a copy of the parent's records; its
// subagent_history_start_ordinal is the first ordinal of its own (codex-rs/rollout/src/ordinal.rs). The file's own
// session_meta is found by the thread id in the file name (36 characters; a reverted thread's name adds
// "_<rollout id>"), else by the first session_meta. The inherited records' cache events stay out of the child and
// stay in the parent; the thread key (fileID, unique per file) does not change.
func TestCodexOwnSessionMeta(t *testing.T) {
	dir := t.TempDir()
	parent := "019f0000-0000-7000-8000-00000000aaaa"
	child := "019f0000-0000-7000-8000-00000000bbbb"
	rollout := "019f0000-0000-7000-8000-00000000cccc"
	line := func(ord int, typ, payload string) string {
		return `{"timestamp":"2026-11-20T10:00:0` + string(rune('0'+ord)) + `Z","ordinal":` + string(rune('0'+ord)) + `,"type":"` + typ + `","payload":` + payload + "}\n"
	}
	meta := func(ord int, id, extra string) string {
		return line(ord, "session_meta", `{"id":"`+id+`","cwd":"/w","source":"cli"`+extra+`}`) // privacy-ok: made-up path
	}
	tc := func(ord int, model string) string {
		return line(ord, "turn_context", `{"model":"`+model+`","cwd":"/w"}`)
	} // privacy-ok: made-up path
	dev := func(ord int) string {
		return line(ord, "response_item", `{"type":"message","role":"developer","content":[{"type":"input_text","text":"x"}]}`)
	}
	// the parent: its own records, two model changes and a developer message
	parentBody := meta(0, parent, "") + tc(1, "a") + dev(2) + tc(3, "b")
	// the child: own session_meta, the parent's copied records (ordinals 1 to 3), then its own from ordinal 4
	childBody := meta(0, child, `,"subagent_history_start_ordinal":4,"parent_thread_id":"`+parent+`"`) + tc(1, "a") + dev(2) + tc(3, "b") +
		tc(4, "c") + tc(5, "d")
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	kinds := func(f *File) map[string]int {
		m := map[string]int{}
		for _, e := range f.Events {
			m[e.Kind]++
		}
		return m
	}
	pp := write("rollout-2026-11-20T10-00-00-"+parent+".jsonl", parentBody)
	cp := write("rollout-2026-11-20T10-00-00-"+child+".jsonl", childBody)
	rp := write("rollout-2026-11-20T11-00-00-"+child+"_"+rollout+".jsonl", childBody) // a reverted rollout of the child thread
	op := write("rollout-odd-name.jsonl", childBody)                                  // a name of another shape: the first session_meta is the file's own
	for _, c := range []struct {
		path               string
		models, developers int
	}{
		{pp, 1, 1}, // a->b and the developer message: the parent keeps its records
		{cp, 1, 0}, // only c->d: the copied a, developer message and b are the parent's
		{rp, 1, 0},
		{op, 1, 0},
	} {
		f, err := ParseCodex(c.path)
		if err != nil {
			t.Fatal(err)
		}
		k := kinds(f)
		if k["model_change"] != c.models || k["developer_msg"] != c.developers {
			t.Errorf("%s: events %v, want %d model changes and %d developer messages", filepath.Base(c.path), k, c.models, c.developers)
		}
		if f.Thread.ThreadID != fileID(c.path) {
			t.Errorf("%s: thread key %q, want the per-file key %q", filepath.Base(c.path), f.Thread.ThreadID, fileID(c.path))
		}
	}
	// a name of another shape whose first session_meta has no usable id: the name slice, in both readers
	ep := write("rollout-odd-empty.jsonl", strings.Replace(childBody, `"id":"`+child+`"`, `"id":""`, 1))
	if e, _ := ParseCodex(ep); e != nil {
		l, _ := parseCodexLegacy(ep)
		if k := kinds(e); k["developer_msg"] != 1 || !reflect.DeepEqual(e, l) {
			t.Errorf("empty first id: events %v, or the two readers differ", k)
		}
	}
	a, _ := ParseCodex(cp)
	b, _ := ParseCodex(rp)
	if a.Thread.ThreadID == b.Thread.ThreadID {
		t.Error("a reverted rollout shares its thread key with the original: their responses would be merged")
	}
	if codexThreadID(rp) != child || codexThreadID(rp+".zst") != child || codexThreadID(op) != "" {
		t.Errorf("thread ids from names: %q %q %q", codexThreadID(rp), codexThreadID(rp+".zst"), codexThreadID(op))
	}
}

// After the cache version changes, an entry written by the previous version is ignored (the file is read again).
// Version 16 is the first whose Codex entries leave a sub-agent's inherited records out of its cache events.
func TestCacheOldVersionIgnored(t *testing.T) {
	if cacheVersion < 16 {
		t.Fatalf("cacheVersion %d: entries parsed before the own session_meta fix would be reused", cacheVersion)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "rollout-2026-11-20T10-00-00-019f0000-0000-7000-8000-00000000aaaa.jsonl")
	_ = os.WriteFile(p, []byte(`{"timestamp":"2026-11-20T10:00:00Z","type":"session_meta","payload":{"id":"x"}}`+"\n"), 0o600)
	c := NewFileCache(filepath.Join(dir, "cache"))
	size, mt, _ := stamp(p)
	var buf bytes.Buffer
	_ = gob.NewEncoder(&buf).Encode(entry{Version: cacheVersion - 1, Binary: meta.Version, Path: p, Size: size, MTime: mt, File: File{Thread: Thread{ThreadID: "stale"}}})
	_ = os.WriteFile(c.name(p), buf.Bytes(), 0o600)
	if c.Get(p) != nil {
		t.Fatal("an entry of the previous cache version was used")
	}
	f, _ := ParseCodex(p)
	c.Put(p, f, StampOf(p))
	if g := c.Get(p); g == nil || g.Thread.ThreadID == "stale" {
		t.Errorf("the entry was not rebuilt: %+v", g)
	}
}

// The main pass keeps its rule: the reference's name slice, so in practice the first session_meta sets the thread's
// metadata even when a later one carries the file's own id. Only the cache events go by the file's own id. And a
// fractional start ordinal is rounded the same way in both (up: ordinal 3 is inherited when the start is 3.5).
func TestCodexMainPassRuleAndFractionalStart(t *testing.T) {
	dir := t.TempDir()
	own := "019f0000-0000-7000-8000-00000000dddd"
	other := "019f0000-0000-7000-8000-00000000eeee"
	line := func(ord int, typ, payload string) string {
		return `{"timestamp":"2026-11-20T10:00:0` + string(rune('0'+ord)) + `Z","ordinal":` + string(rune('0'+ord)) + `,"type":"` + typ + `","payload":` + payload + "}\n"
	}
	tc := func(ord int, model string) string {
		return line(ord, "turn_context", `{"model":"`+model+`","cwd":"/w"}`)
	} // privacy-ok: made-up path
	body := line(0, "session_meta", `{"id":"`+other+`","cwd":"/first","cli_version":"1"}`) + // privacy-ok: made-up path
		line(1, "session_meta", `{"id":"`+own+`","cwd":"/own","cli_version":"2","subagent_history_start_ordinal":3.5}`) + // privacy-ok: made-up path
		tc(2, "a") + tc(3, "b") + tc(4, "c") + tc(5, "d")
	p := filepath.Join(dir, "rollout-2026-11-20T10-00-00-"+own+".jsonl")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := ParseCodex(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Thread.Cwd.S != "/first" || f.Thread.SessionID.S != other || f.Thread.Version.S != "1" { // privacy-ok: made-up path
		t.Errorf("main pass took another session_meta: cwd %q session %q version %q", f.Thread.Cwd.S, f.Thread.SessionID.S, f.Thread.Version.S)
	}
	n := 0
	for _, e := range f.Events {
		if e.Kind == "model_change" {
			n++
		}
	}
	if n != 1 { // c->d only: a and b (ordinals 2 and 3) are before the start 3.5
		t.Errorf("model changes %d, want 1 (start 3.5 rounded up)", n)
	}
	l, _ := parseCodexLegacy(p)
	if !reflect.DeepEqual(f, l) {
		t.Error("the two readers differ")
	}
}
