//go:build !windows && !linux

package proc

import "syscall"

// sysAttr: own process group (macOS and the BSDs have no parent-death signal).
func sysAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }
