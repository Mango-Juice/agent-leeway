//go:build windows

package cli

import (
	"context"
	"os"
	"os/signal"
)

// notifyContext is cancelled by Ctrl-C or Ctrl-Break. A closed console or a killed parent ends the judges
// through the kill-on-close job object.
func notifyContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}
