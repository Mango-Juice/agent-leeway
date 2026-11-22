//go:build !windows

package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// notifyContext is cancelled by Ctrl-C, SIGTERM (what a tool timeout usually sends) or SIGHUP (a closed
// terminal): every judge process tree is then ended and nothing half-done is recorded.
func notifyContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
}
