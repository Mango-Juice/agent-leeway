package cli

// Regression test for docs/20 L5.

import (
	"testing"
	"time"
)

// --since values that would overflow (or are zero) are rejected instead of giving a future window.
func TestSinceOverflowIsRejected(t *testing.T) {
	now := time.Date(2026, 1, 20, 0, 0, 0, 0, time.UTC)
	for _, s := range []string{"200000d", "99999999999999999999d", "0d", "5300w"} {
		if got, err := parseSince(s, now); err == nil {
			t.Errorf("%s accepted: %v", s, got)
		}
	}
	if got, err := parseSince("36500d", now); err != nil || !got.Before(now) {
		t.Errorf("100 years: %v %v", got, err)
	}
}
