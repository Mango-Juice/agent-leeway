//go:build windows

package cli

import (
	"os"

	"golang.org/x/sys/windows"
)

// isTerminal is true for a console handle.
func isTerminal(f *os.File) bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(f.Fd()), &mode) == nil
}
