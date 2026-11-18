//go:build linux

package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Lock is a kernel-owned lock, not a timestamp or a lease. Children inherit the
// descriptor so a dead supervisor cannot silently release an active workspace.
// Recovery also inspects recorded processes: a native runtime may close fds.
type Lock struct {
	file      *os.File
	key       string
	once      sync.Once
	operation sync.Mutex
}

var schedulerLocks = struct {
	sync.Mutex
	byDir map[string]*Lock
}{byDir: make(map[string]*Lock)}

func acquireFileLock(path string) (*Lock, error) {
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("lock must be a regular file: %s", path)
	}
	if err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("execution owner already holds %s: %w", path, err)
	}
	return &Lock{file: f}, nil
}

// AcquireScheduler must be held by foreground and serve callers alike.
func AcquireScheduler(dataDir string) (*Lock, error) {
	root, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	lock, err := acquireFileLock(filepath.Join(root, "scheduler.lock"))
	if err != nil {
		return nil, err
	}
	lock.key = root
	owner, err := readProcStat(os.Getpid())
	if err != nil {
		lock.Close()
		return nil, err
	}
	boot, err := bootID()
	if err != nil {
		lock.Close()
		return nil, err
	}
	if err := writeLockRecord(lock, ProcessIdentity{PID: owner.pid, PGID: owner.pgid, StartTicks: owner.start, BootID: boot}); err != nil {
		lock.Close()
		return nil, err
	}
	schedulerLocks.Lock()
	schedulerLocks.byDir[root] = lock
	schedulerLocks.Unlock()
	return lock, nil
}

func (l *Lock) Close() error {
	if l == nil {
		return nil
	}
	var err error
	l.once.Do(func() {
		if l.key != "" {
			schedulerLocks.Lock()
			if schedulerLocks.byDir[l.key] == l {
				delete(schedulerLocks.byDir, l.key)
			}
			schedulerLocks.Unlock()
		}
		// Do not LOCK_UN: an inherited descriptor must retain the lock until the
		// last holder exits. Closing our descriptor is sufficient.
		err = l.file.Close()
	})
	return err
}

func schedulerFile(dataDir string) *os.File {
	lock := schedulerLock(dataDir)
	if lock == nil {
		return nil
	}
	return lock.file
}

func schedulerLock(dataDir string) *Lock {
	root, err := filepath.Abs(dataDir)
	if err != nil {
		return nil
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil
	}
	schedulerLocks.Lock()
	defer schedulerLocks.Unlock()
	if lock := schedulerLocks.byDir[root]; lock != nil {
		return lock
	}
	return nil
}

func beginSchedulerOperation(dataDir string) (func(), error) {
	lock := schedulerLock(dataDir)
	if lock == nil {
		return nil, fmt.Errorf("operation requires the single scheduler lock")
	}
	if !lock.operation.TryLock() {
		return nil, fmt.Errorf("%w: this scheduler is already executing or recovering a task", ErrConflict)
	}
	return lock.operation.Unlock, nil
}

// The lock file also records identity. The flock is primary; the record lets an
// explicit stop distinguish a live owner from a dead supervisor whose child
// inherited the descriptor. It is never interpreted as a renewable lease.
func writeLockRecord(lock *Lock, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := lock.file.Truncate(0); err != nil {
		return err
	}
	if _, err := lock.file.WriteAt(append(raw, '\n'), 0); err != nil {
		return err
	}
	return lock.file.Sync()
}

func schedulerOwnerAlive(dataDir string) (bool, error) {
	path := filepath.Join(dataDir, "scheduler.lock")
	lock, err := acquireFileLock(path)
	if err == nil {
		lock.Close()
		return false, nil
	}
	if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
		return false, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	var id ProcessIdentity
	if json.Unmarshal(raw, &id) != nil || id.PID <= 1 || id.StartTicks == 0 || id.BootID == "" {
		return false, fmt.Errorf("scheduler owner identity is unreadable; stop request was recorded")
	}
	boot, err := bootID()
	if err != nil || boot != id.BootID {
		return false, err
	}
	process, err := readProcStat(id.PID)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return process.start == id.StartTicks && process.state != 'Z' && process.state != 'X', nil
}

// MaybeRunChild is called before normal CLI dispatch (and by test TestMain).
// This helper cannot start its command until its parent has persisted identity
// and writes the one-byte gate. Parent death before authorization yields EOF.
func MaybeRunChild() bool {
	if len(os.Args) < 2 || os.Args[1] != "__eng_child" {
		return false
	}
	os.Exit(childMain(os.Args[2:]))
	return true
}

func childMain(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "invalid internal execution helper")
		return 125
	}
	count, err := strconv.Atoi(args[0])
	if err != nil || count < 0 || count > 8 {
		return 125
	}
	gate := os.NewFile(3, "execution-gate")
	if gate == nil {
		return 125
	}
	var authorization [1]byte
	n, err := io.ReadFull(gate, authorization[:])
	gate.Close()
	if err != nil || n != 1 || authorization[0] != 1 {
		return 125
	}
	for fd := 4; fd < 4+count; fd++ {
		if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), syscall.F_SETFD, 0); errno != 0 {
			return 125
		}
	}
	if err := syscall.Exec(args[1], args[1:], os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "execute configured command: %v\n", err)
		return 126
	}
	return 0
}

type procStat struct {
	pid, pgid, sid int
	start          uint64
	state          byte
}

func readProcStat(pid int) (procStat, error) {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return procStat{}, err
	}
	end := strings.LastIndexByte(string(raw), ')')
	if end < 0 {
		return procStat{}, fmt.Errorf("invalid process stat")
	}
	fields := strings.Fields(string(raw[end+1:]))
	if len(fields) < 20 || len(fields[0]) != 1 {
		return procStat{}, fmt.Errorf("incomplete process stat")
	}
	pgid, e1 := strconv.Atoi(fields[2])
	sid, e2 := strconv.Atoi(fields[3])
	start, e3 := strconv.ParseUint(fields[19], 10, 64)
	if e1 != nil || e2 != nil || e3 != nil {
		return procStat{}, fmt.Errorf("invalid process identity")
	}
	return procStat{pid: pid, pgid: pgid, sid: sid, start: start, state: fields[0][0]}, nil
}

func bootID() (string, error) {
	b, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	return strings.TrimSpace(string(b)), err
}

func identifyProcess(pid int) (ProcessIdentity, error) {
	p, err := readProcStat(pid)
	if err != nil {
		return ProcessIdentity{}, err
	}
	boot, err := bootID()
	if err != nil {
		return ProcessIdentity{}, err
	}
	if p.pgid != pid || p.sid != pid {
		return ProcessIdentity{}, fmt.Errorf("child has no private session/process group")
	}
	return ProcessIdentity{PID: pid, PGID: p.pgid, StartTicks: p.start, BootID: boot}, nil
}

// ProcessGroupAlive is conservative about PID reuse. An absent leader does not
// mean the group is stopped; children may remain in the original session.
func ProcessGroupAlive(id ProcessIdentity) (bool, error) {
	if id.PID <= 1 || id.PGID != id.PID || id.StartTicks == 0 || id.BootID == "" {
		return false, fmt.Errorf("invalid saved process identity")
	}
	boot, err := bootID()
	if err != nil {
		return false, err
	}
	if boot != id.BootID {
		return false, nil
	}
	leader, err := readProcStat(id.PID)
	if err == nil && leader.start != id.StartTicks {
		// Linux cannot reuse a numeric PID while it still identifies a living
		// process group/session. A different start time proves the saved group
		// ended; it never authorizes signalling the replacement process.
		return false, nil
	}
	if err == nil && (leader.pgid != id.PGID || leader.sid != id.PID) {
		return false, fmt.Errorf("saved process session/group changed; manual process review required")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		pid, e := strconv.Atoi(entry.Name())
		if e != nil {
			continue
		}
		p, e := readProcStat(pid)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			// On hidepid mounts unrelated users are not visible. Permission errors
			// are not proof that our group disappeared.
			if errors.Is(e, os.ErrPermission) {
				return false, e
			}
			continue
		}
		if p.pgid == id.PGID && p.sid == id.PID && p.state != 'Z' && p.state != 'X' {
			return true, nil
		}
	}
	return false, nil
}

func stopProcessGroup(id ProcessIdentity, grace time.Duration) (bool, error) {
	alive, err := ProcessGroupAlive(id)
	if err != nil || !alive {
		return !alive && err == nil, err
	}
	if err = syscall.Kill(-id.PGID, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return false, err
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		alive, err = ProcessGroupAlive(id)
		if err != nil || !alive {
			return !alive && err == nil, err
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err = syscall.Kill(-id.PGID, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return false, err
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		alive, err = ProcessGroupAlive(id)
		if err != nil || !alive {
			return !alive && err == nil, err
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false, fmt.Errorf("local process group did not stop; retain execution slot")
}

type commandOutcome struct {
	ExitCode  int
	Started   bool
	Stopped   bool
	TimedOut  bool
	Cancelled bool
	Identity  *ProcessIdentity
	Err       error
}

// runSupervised runs only the configured argv. It never inserts model flags,
// retries a command, or interprets exit zero as semantic task completion.
func runSupervised(ctx context.Context, argv []string, cwd, input string, env []string, stdoutPath, stderrPath string, inherited []*os.File, persist func(ProcessIdentity) error) commandOutcome {
	out := commandOutcome{ExitCode: -1, Stopped: true}
	if len(argv) == 0 {
		out.Err = fmt.Errorf("empty command")
		return out
	}
	if err := ctx.Err(); err != nil {
		out.Err = err
		out.Cancelled = errors.Is(err, context.Canceled)
		out.TimedOut = errors.Is(err, context.DeadlineExceeded)
		return out
	}
	executable := argv[0]
	var err error
	if strings.ContainsRune(executable, '/') {
		if !filepath.IsAbs(executable) {
			executable = filepath.Join(cwd, executable)
		}
		executable, err = filepath.Abs(executable)
	} else {
		executable, err = exec.LookPath(executable)
	}
	if err != nil {
		out.Err = err
		return out
	}
	self, err := os.Executable()
	if err != nil {
		out.Err = err
		return out
	}
	stdout, err := os.OpenFile(stdoutPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		out.Err = err
		return out
	}
	defer stdout.Close()
	stderr, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		out.Err = err
		return out
	}
	defer stderr.Close()
	reader, writer, err := os.Pipe()
	if err != nil {
		out.Err = err
		return out
	}
	defer reader.Close()
	defer writer.Close()
	files := make([]*os.File, 0, len(inherited)+1)
	files = append(files, reader)
	for _, f := range inherited {
		if f != nil {
			files = append(files, f)
		}
	}
	args := append([]string{"__eng_child", strconv.Itoa(len(files) - 1), executable}, argv[1:]...)
	cmd := exec.Command(self, args...)
	cmd.Dir, cmd.Env, cmd.Stdin, cmd.Stdout, cmd.Stderr = cwd, env, strings.NewReader(input), stdout, stderr
	cmd.ExtraFiles = files
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err = cmd.Start(); err != nil {
		out.Err = err
		return out
	}
	reader.Close()
	id, err := identifyProcess(cmd.Process.Pid)
	if err != nil {
		writer.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		out.Err = err
		return out
	}
	out.Identity = &id
	// Wait is started before any path can return, so owned child zombies are reaped.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if persist == nil {
		err = fmt.Errorf("durable process identity callback required")
	} else {
		err = persist(id)
	}
	if err != nil {
		writer.Close()
		select {
		case <-done:
		case <-time.After(time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
		out.Err = fmt.Errorf("identity was not durably recorded; command not released: %w", err)
		return out
	}
	if err = ctx.Err(); err != nil {
		writer.Close()
		<-done
		out.Err = err
		out.Cancelled = errors.Is(err, context.Canceled)
		out.TimedOut = errors.Is(err, context.DeadlineExceeded)
		return out
	}
	if _, err = writer.Write([]byte{1}); err != nil {
		writer.Close()
		_ = cmd.Process.Kill()
		<-done
		out.Err = fmt.Errorf("execution gate: %w", err)
		return out
	}
	writer.Close()
	out.Started, out.Stopped = true, false
	var waitErr error
	select {
	case waitErr = <-done:
	case <-ctx.Done():
		out.Cancelled = errors.Is(ctx.Err(), context.Canceled)
		out.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		out.Stopped, out.Err = stopProcessGroup(id, 500*time.Millisecond)
		select {
		case waitErr = <-done:
		case <-time.After(2 * time.Second):
			out.Err = errors.Join(out.Err, fmt.Errorf("child exit remains unobserved"))
			return out
		}
		out.Err = errors.Join(out.Err, ctx.Err())
	}
	if cmd.ProcessState != nil {
		out.ExitCode = cmd.ProcessState.ExitCode()
	}
	alive, observeErr := ProcessGroupAlive(id)
	if observeErr != nil {
		out.Err = errors.Join(out.Err, observeErr)
		return out
	}
	if alive {
		stopped, stopErr := stopProcessGroup(id, 500*time.Millisecond)
		out.Stopped = stopped
		out.Err = errors.Join(out.Err, fmt.Errorf("command left descendant processes; task cannot claim clean completion"), stopErr)
	} else {
		out.Stopped = true
	}
	if waitErr != nil && out.Err == nil {
		out.Err = waitErr
	}
	if err := stdout.Sync(); err != nil {
		out.Err = errors.Join(out.Err, err)
	}
	if err := stderr.Sync(); err != nil {
		out.Err = errors.Join(out.Err, err)
	}
	return out
}
