package logs

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color/palette"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
	"time"
)

// Sizes come from the header bytes alone; a JPEG's frame header is found past a large metadata block.
func TestImageSize(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1366, 768))
	enc := func(f func(*bytes.Buffer)) string {
		var b bytes.Buffer
		f(&b)
		return base64.StdEncoding.EncodeToString(b.Bytes())
	}
	pngS := enc(func(b *bytes.Buffer) { _ = png.Encode(b, img) })
	gifS := enc(func(b *bytes.Buffer) {
		_ = gif.Encode(b, image.NewPaletted(image.Rect(0, 0, 300, 200), palette.Plan9), nil)
	})
	jpg := func(meta int) string {
		var b bytes.Buffer
		_ = jpeg.Encode(&b, img, nil)
		raw := b.Bytes()
		if meta > 0 { // an APP1 block before the frame header, as EXIF adds
			seg := make([]byte, 4+meta)
			seg[0], seg[1] = 0xFF, 0xE1
			binary.BigEndian.PutUint16(seg[2:4], uint16(meta+2))
			raw = append(append(append([]byte{}, raw[:2]...), seg...), raw[2:]...)
		}
		return base64.StdEncoding.EncodeToString(raw)
	}
	webpX := base64.StdEncoding.EncodeToString(append([]byte("RIFF\x00\x00\x00\x00WEBPVP8X\x0a\x00\x00\x00\x00\x00\x00\x00"), 0x55, 0x05, 0x00, 0xff, 0x02, 0x00))
	for name, c := range map[string]struct {
		data string
		w, h int
	}{"png": {pngS, 1366, 768}, "gif": {gifS, 300, 200}, "jpeg": {jpg(0), 1366, 768}, "jpeg+exif": {jpg(60000), 1366, 768},
		"webp": {webpX, 1366, 768}, "url-safe png": {base64.URLEncoding.EncodeToString(mustB64(pngS)), 1366, 768}} {
		w, h, ok := ImageSize(c.data)
		if !ok || w != c.w || h != c.h {
			t.Errorf("%s: %dx%d %v", name, w, h, ok)
		}
	}
	for _, bad := range []string{"", "bm90IGFuIGltYWdl", pngS[:20], "////"} {
		if _, _, ok := ImageSize(bad); ok {
			t.Errorf("%q read as an image", bad)
		}
	}
}

func mustB64(s string) []byte {
	b, _ := base64.StdEncoding.DecodeString(s)
	return b
}

func pngB64(t *testing.T, w, h int) string {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

// Images in tool results are recorded by size and the response that issued the call; they add no characters to the
// tool output (so oversized output never counts them). Nothing of the image is kept but its size.
func TestImagesInResults(t *testing.T) {
	dir := t.TempDir()
	data := pngB64(t, 1280, 800)
	p := dir + "/c.jsonl"
	writeFile(t, p, jl(t,
		ccLine("assistant", map[string]any{"message": map[string]any{"id": "m1", "model": "claude-opus-5-5", "usage": map[string]any{"input_tokens": 10},
			"content": []any{map[string]any{"type": "tool_use", "id": "t1", "name": "Read", "input": map[string]any{"file_path": "/w/shot.png"}}}}}), // privacy-ok: made-up path
		ccLine("user", map[string]any{"message": map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "t1",
			"content": []any{map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": data}}}}}}}),
		ccLine("assistant", map[string]any{"message": map[string]any{"id": "m2", "model": "claude-opus-5-5", "usage": map[string]any{"input_tokens": 10},
			"content": []any{map[string]any{"type": "text", "text": "ok"}}}}),
	))
	f, err := ParseClaude(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Images) != 1 || f.Images[0].W != 1280 || f.Images[0].H != 800 || f.Images[0].Step != 0 || f.Calls[0].OutChars != 0 {
		t.Fatalf("claude: %+v, out %d", f.Images, f.Calls[0].OutChars)
	}
	// Codex: the issuing response's usage comes after the output; the image belongs to that response's call
	q := dir + "/rollout-x.jsonl"
	codexFile(t, q,
		cxLine("response_item", map[string]any{"type": "function_call", "name": "view_image", "arguments": "{}", "call_id": "c1"}),
		cxLine("response_item", map[string]any{"type": "function_call_output", "call_id": "c1", "output": []any{
			map[string]any{"type": "input_text", "text": "image"}, map[string]any{"type": "input_image", "image_url": "data:image/png;base64," + data, "detail": "original"}}}),
		cxLine("event_msg", map[string]any{"type": "token_count", "info": map[string]any{"last_token_usage": map[string]any{"input_tokens": 100}, "total_token_usage": map[string]any{"input_tokens": 1}}}),
		cxLine("event_msg", map[string]any{"type": "token_count", "info": map[string]any{"last_token_usage": map[string]any{"input_tokens": 1300}, "total_token_usage": map[string]any{"input_tokens": 2}}}),
	)
	g, err := ParseCodex(q)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Images) != 1 || g.Images[0].Step != 0 || g.Images[0].Detail != "original" || g.Calls[0].OutChars > len("image\n") {
		t.Fatalf("codex: %+v, out %d", g.Images, g.Calls[0].OutChars)
	}
}

// B9: a crafted JPEG with endless empty segments is given up on quickly.
func TestJPEGSegmentCap(t *testing.T) {
	raw := []byte{0xFF, 0xD8}
	for i := 0; i < 200000; i++ {
		raw = append(raw, 0xFF, 0xE2, 0x00, 0x02)
	}
	raw = append(raw, 0xFF, 0xC0, 0x00, 0x11, 0x08, 0x02, 0x00, 0x03, 0x00)
	start := time.Now()
	if _, _, ok := ImageSize(base64.StdEncoding.EncodeToString(raw)); ok {
		t.Error("read past the segment cap")
	}
	if d := time.Since(start); d > 500*time.Millisecond { // generous: a slow CI runner took 25 ms; the cap itself is checked above
		t.Errorf("took %v", d)
	}
}

// B4: a Codex image pasted after a tool call, before the call's response usage is logged, is first read by the
// response after that one.
func TestCodexUserImageAfterCall(t *testing.T) {
	dir := t.TempDir()
	data := pngB64(t, 800, 600)
	q := dir + "/rollout-y.jsonl"
	codexFile(t, q,
		cxLine("event_msg", map[string]any{"type": "token_count", "info": map[string]any{"last_token_usage": map[string]any{"input_tokens": 50}, "total_token_usage": map[string]any{"input_tokens": 1}}}),
		cxLine("response_item", map[string]any{"type": "function_call", "name": "exec_command", "arguments": "{}", "call_id": "c1"}),
		cxLine("response_item", map[string]any{"type": "function_call_output", "call_id": "c1", "output": "ok"}),
		cxLine("response_item", map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_image", "image_url": "data:image/png;base64," + data}}}),
		cxLine("event_msg", map[string]any{"type": "token_count", "info": map[string]any{"last_token_usage": map[string]any{"input_tokens": 100}, "total_token_usage": map[string]any{"input_tokens": 2}}}),
		cxLine("event_msg", map[string]any{"type": "token_count", "info": map[string]any{"last_token_usage": map[string]any{"input_tokens": 700}, "total_token_usage": map[string]any{"input_tokens": 3}}}),
	)
	g, err := ParseCodex(q)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Images) != 1 || g.Images[0].Step != 1 {
		t.Fatalf("%+v (want the image after response 1, the call's response)", g.Images)
	}
}
