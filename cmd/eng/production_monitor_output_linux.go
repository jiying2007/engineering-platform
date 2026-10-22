//go:build linux

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type productionMonitorPipeOutput struct {
	ctx  context.Context
	file *os.File
	raw  syscall.RawConn
}

func (w *productionMonitorPipeOutput) Write(data []byte) (int, error) {
	if w.ctx.Err() == nil {
		return w.file.Write(data)
	}
	// A drained pipe can still retain the cancellation/deadline terminal event.
	// After cancellation, make exactly one nonblocking attempt: no poll, retry,
	// writer goroutine, or new output deadline may extend the invocation.
	var n int
	var writeErr error
	err := w.raw.Control(func(fd uintptr) {
		flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_GETFL, 0)
		if errno != 0 || flags&syscall.O_NONBLOCK == 0 {
			writeErr = fmt.Errorf("observer pipe output is not nonblocking")
			return
		}
		n, writeErr = syscall.Write(int(fd), data)
	})
	if err != nil {
		return 0, err
	}
	if n < 0 {
		n = 0
	}
	return n, writeErr
}

func sameProductionMonitorPipe(a, b *syscall.Stat_t) bool {
	return a.Mode&syscall.S_IFMT == syscall.S_IFIFO && b.Mode&syscall.S_IFMT == syscall.S_IFIFO && a.Dev == b.Dev && a.Ino == b.Ino
}

// Only Linux FIFO output is made deadline-aware. Reopening through procfs
// creates an independent open-file description: dup plus SetNonblock would
// instead change O_NONBLOCK for the caller's original/shared descriptor.
// Regular files, devices, sockets and arbitrary writers retain their existing
// synchronous I/O boundary.
func prepareProductionMonitorOutput(ctx context.Context, writer io.Writer) (io.Writer, func(), error) {
	noop := func() {}
	original, ok := writer.(*os.File)
	if !ok {
		return writer, noop, nil
	}
	if original == nil {
		return nil, noop, fmt.Errorf("observer pipe output unavailable")
	}
	info, err := original.Stat()
	if err != nil {
		return nil, noop, fmt.Errorf("observer pipe output unavailable")
	}
	if info.Mode()&os.ModeNamedPipe == 0 {
		return writer, noop, nil
	}
	originalStat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, noop, fmt.Errorf("observer pipe output unavailable")
	}
	originalRaw, err := original.SyscallConn()
	if err != nil {
		return nil, noop, fmt.Errorf("observer pipe output unavailable")
	}
	var output *os.File
	var openErr error
	err = originalRaw.Control(func(fd uintptr) {
		var before, after syscall.Stat_t
		if syscall.Fstat(int(fd), &before) != nil || !sameProductionMonitorPipe(originalStat, &before) {
			openErr = fmt.Errorf("observer pipe output changed")
			return
		}
		flags, _, errno := syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_GETFL, 0)
		accessMode := flags & syscall.O_ACCMODE
		if errno != 0 || (accessMode != syscall.O_WRONLY && accessMode != syscall.O_RDWR) {
			openErr = fmt.Errorf("observer pipe output is not writable")
			return
		}
		// RawConn.Control holds the original descriptor live across reopen.
		// Never call File.Fd: it can switch a pollable descriptor to blocking.
		path := "/proc/self/fd/" + strconv.FormatUint(uint64(fd), 10)
		output, openErr = os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if openErr != nil {
			return
		}
		opened, statErr := output.Stat()
		if statErr != nil {
			openErr = statErr
			return
		}
		openedStat, valid := opened.Sys().(*syscall.Stat_t)
		afterFlags, _, afterErrno := syscall.Syscall(syscall.SYS_FCNTL, fd, syscall.F_GETFL, 0)
		if !valid || !sameProductionMonitorPipe(&before, openedStat) || syscall.Fstat(int(fd), &after) != nil || !sameProductionMonitorPipe(&before, &after) || afterErrno != 0 || afterFlags != flags {
			openErr = fmt.Errorf("observer pipe output changed during reopen")
		}
	})
	if err != nil || openErr != nil || output == nil {
		if output != nil {
			output.Close()
		}
		return nil, noop, fmt.Errorf("observer pipe output unavailable")
	}
	outputRaw, err := output.SyscallConn()
	deadline, _ := ctx.Deadline()
	if err != nil || output.SetWriteDeadline(deadline) != nil {
		output.Close()
		return nil, noop, fmt.Errorf("observer pipe output deadline unavailable")
	}
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(done)
		// The handle remains owned until cleanup joins this short callback.
		_ = output.SetWriteDeadline(time.Now())
	})
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			if !stop() {
				<-done
			}
			output.Close()
		})
	}
	return &productionMonitorPipeOutput{ctx: ctx, file: output, raw: outputRaw}, cleanup, nil
}
