//go:build !windows

package proc

import (
	"os/exec"
	"syscall"
)

// tree is a process group: the child leads a new group, and killing the group ends its descendants.
type tree struct{ cmd *exec.Cmd }

func newTree(cmd *exec.Cmd) *tree {
	cmd.SysProcAttr = sysAttr()
	return &tree{cmd}
}

func (t *tree) started() {}

func (t *tree) kill() error {
	if t.cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-t.cmd.Process.Pid, syscall.SIGKILL)
}

// close ends what is left of the group only when something is known to be left (a member still held the
// output pipes, or the run was cancelled); after a clean exit the group id may already be free for reuse.
func (t *tree) close(leftover bool) {
	if leftover && t.cmd.Process != nil {
		_ = syscall.Kill(-t.cmd.Process.Pid, syscall.SIGKILL)
	}
}
