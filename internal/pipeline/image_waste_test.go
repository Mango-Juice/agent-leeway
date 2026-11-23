package pipeline

import (
	"math"
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/logs"
)

// An image is counted for each later response that re-reads it, at the cache-read price, until compaction; its first
// read is not counted, nor is taking it; it is not counted again as oversized output.
func TestImageCarryOver(t *testing.T) {
	f := thread("claude", "c",
		logs.Step{TS: at(0), Model: logs.Some("claude-opus-5-5"), Uncached: 1000},
		logs.Step{TS: at(1), Model: logs.Some("claude-opus-5-5"), Cached: 2300},                         // first read
		logs.Step{TS: at(2), Model: logs.Some("claude-opus-5-5"), Cached: 2400},                         // re-read
		logs.Step{TS: at(3), Model: logs.Some("claude-opus-5-5"), Cached: 2500},                         // re-read
		logs.Step{TS: at(4), Model: logs.Some("claude-opus-5-5"), Uncached: 900, AfterCompaction: true}, // gone
		logs.Step{TS: at(5), Model: logs.Some("claude-opus-5-5"), Cached: 900})
	f.Images = []logs.Image{{TS: at(0.5), Step: 0, W: 1000, H: 1000}}
	f.Calls = []logs.Call{{TS: at(0), Kind: "read", Step: 0, OutChars: 0, InChars: 10}}
	tb := FromFiles([]*logs.File{f})
	tb.Build()
	var img, over float64
	n := 0
	for _, w := range tb.Waste {
		img += w.Image
		over += w.Oversize
		n += w.NImage
		if w.Image > 0 && w.Seq != 1 {
			t.Errorf("counted on response %d, want the first read (1)", w.Seq)
		}
	}
	want := 1296 * tb.Steps[1].PRead * 2 / 1e6 // two re-reads before the compaction
	if n != 1 || math.Abs(img-want) > 1e-12 || over != 0 {
		t.Errorf("image %v (want %v), %d images, oversize %v", img, want, n, over)
	}
	// the parity tests keep the reference's causes
	legacy(t)
	tb = FromFiles([]*logs.File{f})
	tb.Build()
	for _, w := range tb.Waste {
		if w.Image != 0 {
			t.Fatal("legacy mode counts images")
		}
	}
}

// B1: the carried image tokens never exceed a later response's cached input, each response is priced at its own
// cache-read price, and an image stops counting when the input falls by about its size.
func TestImageCarryOverBounded(t *testing.T) {
	steps := []logs.Step{{TS: at(0), Model: logs.Some("claude-opus-5-5"), Uncached: 1000}, {TS: at(1), Model: logs.Some("claude-opus-5-5"), Cached: 20000, Uncached: 10}}
	for i := 0; i < 10; i++ {
		steps = append(steps, logs.Step{TS: at(2 + float64(i)), Model: logs.Some("claude-opus-5-5"), Cached: 20000, Uncached: 10})
	}
	f := thread("claude", "c", steps...)
	for i := 0; i < 40; i++ {
		f.Images = append(f.Images, logs.Image{TS: at(0.5), Step: 0, W: 1920, H: 1080})
	}
	tb := FromFiles([]*logs.File{f})
	tb.Build()
	img, reads := 0.0, 0.0
	for i, w := range tb.Waste {
		img += w.Image
		if i >= 2 {
			reads += float64(tb.Steps[i].Cached) * tb.Steps[i].PRead / 1e6
		}
	}
	// the 40 images cannot be larger than what their first read added (19,010 tokens), nor than what each later
	// response read from the cache (20,000)
	if want := 10 * 19010 * tb.Steps[2].PRead / 1e6; img > reads+1e-12 || math.Abs(img-want) > 1e-4 { // (each image rounds down to whole tokens)
		t.Errorf("40 images of 1920x1080 over ten responses with 20k cached: $%.6f, cache reads $%.6f", img, reads)
	}
	// the input falls by about one image: it left the context
	g := thread("claude", "d",
		logs.Step{TS: at(0), Model: logs.Some("claude-opus-5-5"), Uncached: 1000},
		logs.Step{TS: at(1), Model: logs.Some("claude-opus-5-5"), Cached: 5000},
		logs.Step{TS: at(2), Model: logs.Some("claude-opus-5-5"), Cached: 5000},
		logs.Step{TS: at(3), Model: logs.Some("claude-opus-5-5"), Cached: 3700},
		logs.Step{TS: at(4), Model: logs.Some("claude-opus-5-5"), Cached: 3800})
	g.Images = []logs.Image{{TS: at(0.5), Step: 0, W: 1000, H: 1000}} // 1,296 tokens
	tb = FromFiles([]*logs.File{g})
	tb.Build()
	img = 0
	for _, w := range tb.Waste {
		img += w.Image
	}
	if want := 1296 * tb.Steps[2].PRead / 1e6; math.Abs(img-want) > 1e-12 {
		t.Errorf("counted after it left: %v, want one re-read %v", img, want)
	}
}
