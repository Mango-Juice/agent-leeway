//go:build !windows

package cli

import (
	"os"

	"golang.org/x/sys/unix"
)

// isTerminal is true for a real terminal (not /dev/null, which is also a character device).
func isTerminal(f *os.File) bool {
	_, err := unix.IoctlGetTermios(int(f.Fd()), ioctlGetTermios)
	return err == nil
}
