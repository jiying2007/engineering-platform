// sandbox-guard is the container's PID 1, not a host execution interface. Build
// statically and pin its bytes in the operator's Profile. Its independent timer
// survives Worker death; exiting PID 1 tears down the container PID namespace.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
)

func main() { os.Exit(run(os.Args[1:])) }
func run(args []string) int {
	if os.Getpid() != 1 || os.Getuid() == 0 || len(args) < 2 {
		fmt.Fprintln(os.Stderr, "guard requires non-root container PID 1")
		return 125
	}
	seconds, err := strconv.Atoi(args[0])
	if err != nil || seconds < 1 || seconds > 45 {
		return 125
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[1], args[2:]...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=/tmp", "LANG=C", "TMPDIR=/tmp"}
	var overflow atomic.Bool
	limit := func(out io.Writer) *limitWriter {
		return &limitWriter{out: out, remaining: 128 << 10, stop: func() { overflow.Store(true); cancel() }}
	}
	cmd.Stdout, cmd.Stderr = limit(os.Stdout), limit(os.Stderr)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = time.Second
	err = cmd.Run()
	if overflow.Load() {
		return 122
	}
	if ctx.Err() != nil {
		return 124
	}
	if err == nil {
		return 0
	}
	if e, ok := err.(*exec.ExitError); ok && e.ExitCode() >= 0 {
		return e.ExitCode()
	}
	return 125
}

// Bound pipe writes before they reach the Engine log driver, so a rotating log
// cannot be mistaken for a complete stdout/stderr transcript.
type limitWriter struct {
	out       io.Writer
	remaining int
	stop      func()
}

func (w *limitWriter) Write(p []byte) (int, error) {
	if len(p) > w.remaining {
		w.stop()
		return 0, fmt.Errorf("container output budget exceeded")
	}
	n, err := w.out.Write(p)
	w.remaining -= n
	return n, err
}
