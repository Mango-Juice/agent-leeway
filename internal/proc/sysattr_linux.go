//go:build linux

package proc

import "syscall"

// sysAttr: own process group, and the child is killed if agent-leeway dies (SIGKILL cannot be handled).
func sysAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}
