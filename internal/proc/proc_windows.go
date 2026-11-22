//go:build windows

package proc

import (
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// tree is a job object with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE: every process the child starts joins the job,
// and terminating or closing the job ends them all. The child is assigned right after it starts; a
// grandchild started within that instant would escape (documented limitation).
type tree struct {
	cmd *exec.Cmd
	job windows.Handle
}

func newTree(cmd *exec.Cmd) *tree {
	cmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
	t := &tree{cmd: cmd}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		Warn("could not create a job object (" + err.Error() + "): a timeout or cancel ends only the judge process, not what it started")
		return t
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		Warn("could not configure the job object (" + err.Error() + "): a timeout or cancel ends only the judge process")
		return t
	}
	t.job = job
	return t
}

func (t *tree) started() {
	if t.job == 0 || t.cmd.Process == nil {
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(t.cmd.Process.Pid))
	if err != nil {
		Warn("could not open the judge process for its job object (" + err.Error() + ")")
		return
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(t.job, h); err != nil {
		Warn("could not add the judge process to its job object (" + err.Error() + ")")
	}
}

func (t *tree) kill() error {
	if t.job != 0 {
		return windows.TerminateJobObject(t.job, 1)
	}
	if t.cmd.Process != nil {
		return t.cmd.Process.Kill()
	}
	return nil
}

func (t *tree) close(bool) {
	if t.job != 0 {
		windows.CloseHandle(t.job)
		t.job = 0
	}
}
