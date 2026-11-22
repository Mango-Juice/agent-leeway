// Package proc runs a child program with its whole process tree under control: a timeout or cancellation
// ends the child and everything it started (Unix: a process group; Windows: a job object that kills its
// members when closed), and a grandchild that keeps the output pipes open cannot hang the caller.
package proc

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

// Warn receives a message when the process tree cannot be put under control (Windows job object).
var Warn = func(string) {}

// Result of a finished run.
type Result struct {
	Stdout, Stderr []byte
	ExitCode       int
	TimedOut       bool
	Canceled       bool
}

// ErrTimeout is returned when the run exceeded its timeout.
var ErrTimeout = errors.New("timed out")

// waitDelay bounds how long we wait for pipes after the process tree was ended.
const waitDelay = 5 * time.Second

// Run starts name with args in dir, writes stdin, and waits. The tree is ended on ctx cancellation or
// after timeout (0 = none).
func Run(ctx context.Context, name string, args []string, dir string, env []string, stdin []byte, timeout time.Duration) (Result, error) {
	var res Result
	cctx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		cctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(cctx, name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.Stdin = bytes.NewReader(stdin)
	tree := newTree(cmd)
	cmd.Cancel = func() error { return tree.kill() }
	cmd.WaitDelay = waitDelay
	if err := cmd.Start(); err != nil {
		return res, err
	}
	tree.started()
	err := cmd.Wait()
	// a member still holding the pipes, or a cancel/timeout: end what is left of the tree
	tree.close(errors.Is(err, exec.ErrWaitDelay) || cctx.Err() != nil) // Windows: closing the job always ends its members
	res.Stdout, res.Stderr = out.Bytes(), errb.Bytes()
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}
	if ctx.Err() != nil {
		res.Canceled = true
		return res, ctx.Err()
	}
	if cctx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
		return res, ErrTimeout
	}
	var ee *exec.ExitError
	if err != nil && !errors.As(err, &ee) && !errors.Is(err, exec.ErrWaitDelay) {
		return res, err
	}
	return res, nil
}
