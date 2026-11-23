package logs

import (
	"encoding/base64"
	"encoding/binary"
	"strings"

	"github.com/Mango-Juice/agent-leeway/internal/py"
)

// Images: only the width and height are read, from the first bytes of the image (PNG IHDR, GIF screen, WebP
// header) or, for JPEG, from the segment headers up to the frame header, each decoded on its own from the base64
// text. The image itself is never decoded, stored, hashed or sent.

// Image is one image the model received: when, after which response, and its size in pixels.
type Image struct {
	TS     int64
	Step   int // the last response before the image was added (-1: none); the next response reads it first
	W, H   int
	Detail string // Codex: the detail asked for (low, high, original, auto, ""); Claude Code: ""
}

// claudeImage reads one Claude Code image block ({"type": "image", "source": {"type": "base64", "data": ...}}).
func claudeImage(c py.Value, ts int64, step int) (Image, bool) {
	src := c.Get("source")
	if !isStr(c.Get("type"), "image") || !isStr(src.Get("type"), "base64") || src.Get("data").K != py.Str {
		return Image{}, false
	}
	w, h, ok := ImageSize(src.Get("data").S)
	return Image{TS: ts, Step: step, W: w, H: h}, ok
}

// codexImages reads the input_image items of a Codex message content or tool output (data URLs only).
func codexImages(v py.Value, ts int64, step int) []Image {
	if v.K != py.Arr {
		return nil
	}
	var out []Image
	for _, x := range v.A {
		u := x.Get("image_url")
		if !isStr(x.Get("type"), "input_image") || u.K != py.Str || !strings.HasPrefix(u.S, "data:") {
			continue
		}
		_, data, ok := strings.Cut(u.S, ",")
		if !ok {
			continue
		}
		if w, h, ok := ImageSize(data); ok {
			out = append(out, Image{TS: ts, Step: step, W: w, H: h, Detail: py.PyStr(x.Get("detail"))})
		}
	}
	return out
}

// b64At decodes n bytes of the base64 text s starting at byte offset off (standard or URL alphabet, padded or not).
func b64At(s string, off, n int) []byte {
	if off < 0 || n <= 0 {
		return nil
	}
	a, b := off/3*4, ((off+n+2)/3)*4
	if a >= len(s) {
		return nil
	}
	b = min(b, len(s))
	chunk := strings.TrimRight(s[a:b], "=")
	if len(chunk)%4 == 1 {
		chunk = chunk[:len(chunk)-1]
	}
	enc := base64.RawStdEncoding
	if strings.ContainsAny(chunk, "-_") {
		enc = base64.RawURLEncoding
	}
	raw, err := enc.DecodeString(chunk)
	if err != nil {
		return nil
	}
	skip := off - off/3*3
	if skip >= len(raw) {
		return nil
	}
	raw = raw[skip:]
	return raw[:min(n, len(raw))]
}

// maxJPEGScan and maxJPEGSegments bound how far into a JPEG the segment headers are followed (EXIF and ICC blocks
// come first): 4 MB and 64 segments, so a crafted header cannot make the scan slow.
const (
	maxJPEGScan     = 4 << 20
	maxJPEGSegments = 64
)

// ImageSize reads an image's width and height from its base64 data; ok is false when the format is not known or the
// header is cut short.
func ImageSize(data string) (w, h int, ok bool) {
	head := b64At(data, 0, 30)
	switch {
	case len(head) >= 24 && string(head[:8]) == "\x89PNG\r\n\x1a\n" && string(head[12:16]) == "IHDR":
		return int(binary.BigEndian.Uint32(head[16:20])), int(binary.BigEndian.Uint32(head[20:24])), true
	case len(head) >= 10 && (string(head[:6]) == "GIF87a" || string(head[:6]) == "GIF89a"):
		return int(binary.LittleEndian.Uint16(head[6:8])), int(binary.LittleEndian.Uint16(head[8:10])), true
	case len(head) >= 30 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		switch string(head[12:16]) {
		case "VP8X":
			return 1 + (int(head[24]) | int(head[25])<<8 | int(head[26])<<16), 1 + (int(head[27]) | int(head[28])<<8 | int(head[29])<<16), true
		case "VP8 ":
			return int(binary.LittleEndian.Uint16(head[26:28]) & 0x3fff), int(binary.LittleEndian.Uint16(head[28:30]) & 0x3fff), true
		case "VP8L":
			b := binary.LittleEndian.Uint32(head[21:25])
			return int(b&0x3fff) + 1, int((b>>14)&0x3fff) + 1, true
		}
	case len(head) >= 4 && head[0] == 0xFF && head[1] == 0xD8:
		p := 2
		for seg := 0; p < maxJPEGScan && seg < maxJPEGSegments; seg++ {
			m := b64At(data, p, 9)
			if len(m) < 4 || m[0] != 0xFF {
				return 0, 0, false
			}
			marker := m[1]
			if marker == 0xFF { // fill byte
				p++
				continue
			}
			if marker == 0xD8 || marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
				p += 2
				continue
			}
			l := int(binary.BigEndian.Uint16(m[2:4]))
			if (marker >= 0xC0 && marker <= 0xCF) && marker != 0xC4 && marker != 0xC8 && marker != 0xCC {
				if len(m) < 9 {
					return 0, 0, false
				}
				return int(binary.BigEndian.Uint16(m[7:9])), int(binary.BigEndian.Uint16(m[5:7])), true
			}
			if l < 2 || marker == 0xD9 || marker == 0xDA {
				return 0, 0, false
			}
			p += 2 + l
		}
	}
	return 0, 0, false
}
