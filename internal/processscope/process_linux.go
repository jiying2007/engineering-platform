//go:build linux

package processscope

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"
)

// Scope owns the launch and reap of one namespace init. The creating OS thread
// remains locked through Wait: Pdeathsig is tied to that thread, not a goroutine.
// Descendants may setsid/double-fork but cannot enter an ancestor PID namespace.
// No fallback to plain exec is permitted when namespaces are unavailable.
type Scope struct {
	cmd   *exec.Cmd
	done  chan struct{}
	mu    sync.Mutex
	proof Proof
	err   error
}

func namespace(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Ino == 0 {
		return 0, fmt.Errorf("Linux namespace identity unavailable")
	}
	return st.Ino, nil
}

func Start(cmd *exec.Cmd) (*Scope, error) {
	if cmd == nil || cmd.Process != nil || cmd.SysProcAttr != nil || len(cmd.ExtraFiles) != 0 {
		return nil, fmt.Errorf("fresh command without process overrides or inherited handles required")
	}
	parent, err := namespace("/proc/self/ns/pid")
	if err != nil {
		return nil, err
	}
	uid, gid := os.Geteuid(), os.Getegid()
	// Preserve the host's numeric UID/GID; this is not a privileged launcher.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:                 syscall.CLONE_NEWUSER | syscall.CLONE_NEWPID,
		UidMappings:                []syscall.SysProcIDMap{{ContainerID: uid, HostID: uid, Size: 1}},
		GidMappings:                []syscall.SysProcIDMap{{ContainerID: gid, HostID: gid, Size: 1}},
		GidMappingsEnableSetgroups: false, Pdeathsig: syscall.SIGKILL,
	}
	s := &Scope{cmd: cmd, done: make(chan struct{})}
	started := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if e := cmd.Start(); e != nil {
			started <- fmt.Errorf("required process namespace unavailable: %w", e)
			close(s.done)
			return
		}
		child, e := namespace(fmt.Sprintf("/proc/%d/ns/pid", cmd.Process.Pid))
		s.proof = Proof{Version: 1, Mechanism: Mechanism, HostPID: cmd.Process.Pid, NamespaceID: child, ParentNamespaceID: parent, StartedAt: time.Now().UTC()}
		if e != nil || child == parent {
			_ = cmd.Process.Kill()
			started <- fmt.Errorf("cannot attest dedicated runtime PID namespace")
			_ = cmd.Wait()
			close(s.done)
			return
		}
		started <- nil
		waitErr := cmd.Wait()
		s.mu.Lock()
		s.err = waitErr
		// Wait returning a ProcessState proves reaping, even for forced termination.
		// A protocol completion without this fact cannot authorize filesystem capture.
		if cmd.ProcessState != nil {
			s.proof.InitReaped = true
			s.proof.ReapedAt = time.Now().UTC()
		}
		s.mu.Unlock()
		close(s.done)
	}()
	if err := <-started; err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Scope) Wait(ctx context.Context) (Proof, error) {
	if s == nil {
		return Proof{}, fmt.Errorf("missing runtime scope")
	}
	select {
	case <-s.done:
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.proof.Quiescent() {
			return s.proof, fmt.Errorf("runtime reap unconfirmed")
		}
		return s.proof, s.err
	case <-ctx.Done():
		s.mu.Lock()
		p := s.proof
		s.mu.Unlock()
		return p, ctx.Err()
	}
}

// Stop kills only this scope's init via Go's Process handle, never a numeric
// process group or an arbitrary PID supplied by a request. A bounded timeout
// yields unconfirmed termination; callers must not capture or deliver then.
func (s *Scope) Stop(ctx context.Context) (Proof, error) {
	if s == nil {
		return Proof{}, fmt.Errorf("missing runtime scope")
	}
	err := s.cmd.Process.Kill()
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		return Proof{}, err
	}
	proof, waitErr := s.Wait(ctx)
	if proof.Quiescent() {
		return proof, nil
	}
	return proof, waitErr
}
