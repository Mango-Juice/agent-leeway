//go:build !windows

package drift

import (
	"fmt"
	"os"
	"syscall"
)

// checkPrivate refuses a folder that another account owns or can write to (or a symbolic link).
func checkPrivate(dir string) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return fmt.Errorf("%s is not a plain folder; refusing to run the judge there", dir)
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("%s belongs to another account; refusing to run the judge there", dir)
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("%s can be written by other accounts (mode %v); refusing to run the judge there (chmod 700 it)", dir, fi.Mode().Perm())
	}
	return nil
}
