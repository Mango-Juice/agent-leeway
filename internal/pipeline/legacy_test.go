package pipeline

import (
	"testing"

	"github.com/Mango-Juice/agent-leeway/internal/config"
)

// legacy makes one parity test use the Python reference's pricing (frozen table, thread-first model, fixed
// cache lifetimes, old judge prices). The product default is the price table in internal/assets/prices.json.
func legacy(t *testing.T) {
	config.Legacy = true
	t.Cleanup(func() { config.Legacy = false })
}
