//go:build windows

package drift

import (
	"fmt"
	"os"
)

// checkPrivate: on Windows the cache folder is under %LocalAppData%, which only the user can write;
// a reparse point (link) is still refused.
func checkPrivate(dir string) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return fmt.Errorf("%s is not a plain folder; refusing to run the judge there", dir)
	}
	return nil
}
