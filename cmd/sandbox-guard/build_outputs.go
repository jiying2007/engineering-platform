package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jiying2007/engineering-platform/internal/sandbox"
)

func runBuild(seconds int, encoded string, argv []string) int {
	contract, err := sandbox.DecodeOutputContract(encoded)
	if err != nil || len(argv) == 0 || !filepath.IsAbs(argv[0]) || argv[0] == "/ep-guard" {
		return 125
	}
	if err = os.Mkdir(sandbox.BuildOutputDirectory, 0700); err != nil {
		return 125 // the guard owns a new directory; never reuse producer contents
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=/tmp", "LANG=C", "TMPDIR=/tmp", "EP_OUTPUT_DIR=" + sandbox.BuildOutputDirectory}
	var stdout, stderr bytes.Buffer
	var overflow atomic.Bool
	limit := func(out io.Writer) *limitWriter {
		return &limitWriter{out: out, remaining: sandbox.OutputLimit, stop: func() { overflow.Store(true); cancel() }}
	}
	cmd.Stdout, cmd.Stderr = limit(&stdout), limit(&stderr)
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
	// Detached/background writers must stop before reading tmpfs output. PID1's
	// namespace-scoped kill cannot signal host processes; reap until ECHILD.
	// Failure is not a quiescence attestation. The external Engine still requires
	// container exit AND confirmed removal before accepting any result.
	if reapBuildChildren() != nil {
		return 125
	}
	if overflow.Load() {
		return 122
	}
	exitCode := 0
	if ctx.Err() != nil {
		exitCode = 124
	} else if err != nil {
		exitCode = 125
		if e, ok := err.(*exec.ExitError); ok && e.ExitCode() >= 0 {
			exitCode = e.ExitCode()
		}
	}
	digest, _ := sandbox.OutputContractDigest(contract)
	output := sandbox.BuildOutputs{Contract: contract, ContractDigest: digest, ChildrenReaped: true, State: "NOT_COLLECTED_EXIT_NONZERO"}
	if exitCode == 0 {
		output.Files, err = collectBuildFiles(sandbox.BuildOutputDirectory, contract)
		if err != nil {
			return 125 // no valid envelope and never partial success
		}
		output.State = "COLLECTED"
	}
	raw, err := sandbox.EncodeBuildEnvelope(sandbox.BuildEnvelope{Version: 1, ExitCode: exitCode, Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), Outputs: output})
	if err != nil {
		return 125
	}
	if n, err := os.Stdout.Write(raw); err != nil || n != len(raw) {
		return 125
	}
	return exitCode
}

func reapBuildChildren() error {
	if os.Getpid() != 1 || os.Getuid() == 0 {
		return sandbox.ErrPolicy // never a host process cleanup interface
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(-1, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
			return err
		}
		var status syscall.WaitStatus
		pid, err := syscall.Wait4(-1, &status, syscall.WNOHANG, nil)
		if err == syscall.ECHILD {
			return nil
		}
		if err != nil && err != syscall.EINTR {
			return err
		}
		if pid == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
	return sandbox.ErrUnknown
}

// Outputs remain untrusted data. Only declared regular single-link leaf files
// from the private, quiescent tmpfs directory may enter the result envelope.
func collectBuildFiles(directory string, contract []sandbox.OutputSpec) ([]sandbox.OutputFile, error) {
	if sandbox.ValidateOutputContract(contract) != nil || !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		return nil, sandbox.ErrPolicy
	}
	before, err := os.Lstat(directory)
	if err != nil || !before.IsDir() || before.Mode().Perm()&0077 != 0 {
		return nil, sandbox.ErrPolicy
	}
	st, ok := before.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != os.Getuid() {
		return nil, sandbox.ErrPolicy
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	actual, err := root.Stat(".")
	if err != nil || !os.SameFile(before, actual) {
		return nil, sandbox.ErrPolicy
	}
	inventory := func() error {
		f, err := root.Open(".")
		if err != nil {
			return err
		}
		entries, readErr := f.ReadDir(len(contract) + 1)
		closeErr := f.Close()
		if (readErr != nil && readErr != io.EOF) || closeErr != nil || len(entries) != len(contract) {
			return sandbox.ErrPolicy
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for i, entry := range entries {
			if entry.Name() != contract[i].Name || !entry.Type().IsRegular() {
				return sandbox.ErrPolicy
			}
		}
		return nil
	}
	if err := inventory(); err != nil {
		return nil, err
	}
	var files []sandbox.OutputFile
	for _, spec := range contract {
		info, err := root.Lstat(spec.Name)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Size() < 0 || info.Size() > int64(spec.MaxBytes) {
			return nil, sandbox.ErrPolicy
		}
		native, ok := info.Sys().(*syscall.Stat_t)
		if !ok || native.Nlink != 1 || int(native.Uid) != os.Getuid() {
			return nil, sandbox.ErrPolicy
		}
		f, err := root.OpenFile(spec.Name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return nil, err
		}
		opened, statErr := f.Stat()
		if statErr != nil || !os.SameFile(info, opened) {
			f.Close()
			return nil, sandbox.ErrPolicy
		}
		raw, readErr := io.ReadAll(io.LimitReader(f, int64(spec.MaxBytes)+1))
		after, afterErr := f.Stat()
		closeErr := f.Close()
		current, currentErr := root.Lstat(spec.Name)
		if errors.Join(readErr, afterErr, closeErr, currentErr) != nil || int64(len(raw)) != info.Size() || !os.SameFile(info, after) || !os.SameFile(info, current) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) || !sameOutputStat(native, after.Sys().(*syscall.Stat_t)) || !sameOutputStat(native, current.Sys().(*syscall.Stat_t)) {
			return nil, sandbox.ErrPolicy
		}
		files = append(files, sandbox.OutputFile{Name: spec.Name, Size: len(raw), Digest: sandbox.Hash(raw), Bytes: raw})
	}
	if err := inventory(); err != nil {
		return nil, err
	}
	after, err := os.Lstat(directory)
	if err != nil || !os.SameFile(before, after) {
		return nil, sandbox.ErrPolicy
	}
	return files, nil
}

// A read may update atime; authority-relevant metadata and mtime/ctime must not
// change. Do not confuse that normal read effect with producer mutation.
func sameOutputStat(a, b *syscall.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Nlink == b.Nlink && a.Mode == b.Mode && a.Uid == b.Uid && a.Gid == b.Gid && a.Size == b.Size && a.Mtim == b.Mtim && a.Ctim == b.Ctim
}
