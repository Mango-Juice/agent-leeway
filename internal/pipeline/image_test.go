package pipeline

import "testing"

// The vendors' own worked examples (pages read 2026-11-23).
func TestImageTokensVendorExamples(t *testing.T) {
	for _, c := range []struct {
		tool, model, detail string
		w, h, want          int
	}{
		{"claude", "claude-opus-5-5", "", 1000, 1000, 1296},
		{"claude", "claude-sonnet-4-5-20250929", "", 1000, 1000, 1296},
		{"claude", "claude-opus-5-5", "", 1920, 1080, 2691},            // high-resolution tier: not resized
		{"claude", "claude-sonnet-4-5-20250929", "", 1920, 1080, 1560}, // standard tier: downsized to 1456 × 819
		{"claude", "claude-opus-4-7", "", 8000, 8000, 4761},            // within 4,784 tokens
		{"codex", "gpt-6-astra", "high", 1024, 1024, 1229},
		{"codex", "gpt-6-astra", "high", 2048, 2048, 3000}, // reduced to 1600 × 1600, 2,500 patches
		{"codex", "gpt-6-astra", "high", 4096, 512, 2458},
		{"codex", "gpt-6.2-astra", "original", 2048, 2048, 4916},
		{"codex", "gpt-6-astra", "low", 2048, 1024, 154}, // fits 512 × 256: 16 × 8 patches,
	} {
		if got := ImageTokens(c.tool, c.model, c.detail, c.w, c.h); got != c.want {
			t.Errorf("%s %s %s %dx%d: %d, want %d", c.tool, c.model, c.detail, c.w, c.h, got, c.want)
		}
	}
	for _, c := range []struct {
		model, detail string
		w, h, want    int
	}{
		{"gpt-6-astra", "original", 100000, 100000, 0}, // over 30,000 patches: the API rejects it
		{"gpt-6.2-sol", "original", 1280, 720, 1104},   // not in the table: the gpt-6-astra rule (assumed)
		{"gpt-5.4-mini", "auto", 4096, 4096, 3000},     // auto is high for gpt-5.4: 2,500 patches
		{"gpt-5.5", "original", 6000, 6000, 12000},     // 10,000 patches, 6000 px
		{"gpt-5.2", "low", 2048, 2048, 4916},           // low also 2048 px and 6,144 patches
		{"gpt-4o", "high", 1024, 1024, 0},              // tile-based: not counted
		{"gpt-4.1-mini", "high", 1024, 1024, 1659},     // multiplier 1.62
		{"some-other-model", "high", 1024, 1024, 0},    // not covered
	} {
		if got := ImageTokens("codex", c.model, c.detail, c.w, c.h); got != c.want {
			t.Errorf("%s %s %dx%d: %d, want %d", c.model, c.detail, c.w, c.h, got, c.want)
		}
	}
	if ImageTokens("claude", "claude-opus-5-5", "", 9000, 100) != 0 {
		t.Error("over 8000 px: rejected")
	}
	if ImageTokens("claude", "claude-opus-5-5", "", 0, 10) != 0 {
		t.Error("no size: no tokens")
	}
}
