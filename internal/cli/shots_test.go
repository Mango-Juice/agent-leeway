package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Made-up logs with images: a Claude Code session that reads screenshots (some carried for many responses), a
// sub-agent thread of it with its own screenshot, and a Codex session (gpt-5.4, not the astra rule) with an image
// pasted after a tool call. The report shows a non-zero screenshot card, the same in HTML, Markdown and the terminal.
func shotLogs(t *testing.T, root string) {
	t.Helper()
	pngB64 := func(w, h int) string {
		var b bytes.Buffer
		if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
			t.Fatal(err)
		}
		return base64.StdEncoding.EncodeToString(b.Bytes())
	}
	shot := pngB64(1920, 1080)
	t0 := time.Date(2026, 2, 20, 9, 0, 0, 0, time.UTC)
	ts := func(i int) string {
		return t0.Add(time.Duration(i) * 20 * time.Second).Format("2006-01-02T15:04:05.000Z")
	}
	write := func(p string, recs []map[string]any) {
		var b strings.Builder
		for _, r := range recs {
			x, _ := json.Marshal(r)
			b.Write(x)
			b.WriteByte('\n')
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	claude := func(sid string, sub bool, nImages, steps int) []map[string]any {
		cwd := "/home/dev/trail-planner" // privacy-ok: made-up path
		base := func(typ string, i int) map[string]any {
			return map[string]any{"type": typ, "sessionId": sid, "cwd": cwd, "version": "2.1.40", "entrypoint": "cli", "timestamp": ts(i), "uuid": fmt.Sprintf("%s-%d", sid, i)}
		}
		u := base("user", 0)
		u["message"] = map[string]any{"role": "user", "content": "fix the map page layout"}
		u["origin"] = map[string]any{"kind": "human"}
		u["promptSource"] = "typed"
		recs := []map[string]any{u}
		ctx := 30000
		for i := 1; i <= steps; i++ {
			a := base("assistant", 2*i)
			content := []any{map[string]any{"type": "text", "text": "ok"}}
			if i <= nImages {
				content = []any{map[string]any{"type": "tool_use", "id": fmt.Sprintf("%s-t%d", sid, i), "name": "Read", "input": map[string]any{"file_path": fmt.Sprintf("/home/dev/shot-%d.png", i)}}} // privacy-ok: made-up path
			}
			a["message"] = map[string]any{"id": fmt.Sprintf("%s-m%d", sid, i), "model": "claude-opus-5-5", "role": "assistant", "content": content,
				"usage": map[string]any{"input_tokens": 10, "cache_read_input_tokens": ctx, "cache_creation_input_tokens": 3000, "output_tokens": 200}}
			recs = append(recs, a)
			ctx += 3000
			if i <= nImages {
				r := base("user", 2*i+1)
				r["message"] = map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": fmt.Sprintf("%s-t%d", sid, i),
					"content": []any{map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": shot}}}}}}
				recs = append(recs, r)
			}
		}
		_ = sub
		return recs
	}
	sid := "sess-shots-0001"
	write(filepath.Join(root, "claude", "-home-dev-trail-planner", sid+".jsonl"), claude(sid, false, 6, 60))
	write(filepath.Join(root, "claude", "-home-dev-trail-planner", sid, "subagents", "agent-a1.jsonl"), claude(sid, true, 2, 20))
	// Codex: a tool call, then an image pasted before the call's response usage arrives
	cx := []map[string]any{
		{"timestamp": ts(0), "type": "session_meta", "payload": map[string]any{"id": "thread-shots", "cwd": "/home/dev/invoice-sync", "originator": "codex-tui", "source": "cli", "cli_version": "0.171.0"}}, // privacy-ok: made-up path
		{"timestamp": ts(0), "type": "event_msg", "payload": map[string]any{"type": "task_started", "turn_attribution": map[string]any{"turn_trigger": "composer"}}},
		{"timestamp": ts(0), "type": "turn_context", "payload": map[string]any{"model": "gpt-5.4", "cwd": "/home/dev/invoice-sync"}}, // privacy-ok: made-up path
		{"timestamp": ts(1), "type": "response_item", "payload": map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "check the export page"}}}},
	}
	total := 0
	tok := func(i, in, cached int) map[string]any {
		total++
		return map[string]any{"timestamp": ts(i), "type": "event_msg", "payload": map[string]any{"type": "token_count", "info": map[string]any{
			"last_token_usage":  map[string]any{"input_tokens": in, "cached_input_tokens": cached, "output_tokens": 100},
			"total_token_usage": map[string]any{"input_tokens": total}}}}
	}
	cx = append(cx,
		map[string]any{"timestamp": ts(2), "type": "response_item", "payload": map[string]any{"type": "function_call", "name": "exec_command", "arguments": `{"cmd":"ls"}`, "call_id": "c1"}},
		map[string]any{"timestamp": ts(3), "type": "response_item", "payload": map[string]any{"type": "function_call_output", "call_id": "c1", "output": "Process exited with code 0"}},
		map[string]any{"timestamp": ts(4), "type": "response_item", "payload": map[string]any{"type": "message", "role": "user", "content": []any{
			map[string]any{"type": "input_image", "image_url": "data:image/png;base64," + shot, "detail": "high"}}}},
		tok(5, 20000, 0))
	in := 23000
	for i := 6; i < 40; i++ {
		cx = append(cx, tok(i, in, in-500))
		in += 400
	}
	write(filepath.Join(root, "codex", "2026", "02", "20", "rollout-2026-02-20T09-00-00-thread-shots.jsonl"), cx)
}

func TestScreenshotReportGolden(t *testing.T) {
	root := t.TempDir()
	shotLogs(t, root)
	for k, v := range map[string]string{"LEEWAY_NOW": "2026-03-01T00:00:00Z", "LEEWAY_CLAUDE_DIR": filepath.Join(root, "claude"), "LEEWAY_CODEX_DIR": filepath.Join(root, "codex"),
		"LEEWAY_CONFIG": filepath.Join(root, "config"), "LEEWAY_CACHE": filepath.Join(root, "cache"), "CLAUDE_CONFIG_DIR": filepath.Join(root, "claude-home"),
		"CODEX_HOME": filepath.Join(root, "codex-home")} {
		t.Setenv(k, v)
	}
	old := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = old })
	for _, lang := range []string{"en", "ko"} {
		dir := filepath.Join(root, "reports-"+lang)
		out, errs, rc := run(t, "report", "--lang", lang, "--out", dir, "--no-archived")
		if rc != 0 {
			t.Fatalf("report: %d %s", rc, errs)
		}
		files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
		if len(files) != 1 {
			t.Fatalf("files %v", files)
		}
		md, _ := os.ReadFile(files[0])
		h, _ := os.ReadFile(strings.TrimSuffix(files[0], ".md") + ".html")
		term := strings.ReplaceAll(out, dir+string(filepath.Separator), "<reports>/")
		golden(t, "report-shots."+lang+".md", string(md))
		golden(t, "report-shots."+lang+".html", string(h))
		golden(t, "report-shots-terminal."+lang+".txt", term)
		label := map[string]string{"en": "Leaving screenshots in the conversation", "ko": "스크린샷을 대화에 남겨 두기"}[lang]
		about := map[string]string{"en": "about $", "ko": "약 $"}[lang]
		for name, s := range map[string]string{"md": string(md), "html": string(h)} {
			if !strings.Contains(s, label) || !strings.Contains(s, about) {
				t.Errorf("%s %s: no screenshot card with an estimated amount", lang, name)
			}
		}
		if !strings.Contains(term, about) {
			t.Errorf("%s terminal: no estimated amount\n%s", lang, term)
		}
	}
}
