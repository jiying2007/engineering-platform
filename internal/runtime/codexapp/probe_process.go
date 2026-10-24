package codexapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/processscope"
)

var (
	ErrProbeOutputLimit     = errors.New("Codex probe output exceeds limit")
	ErrProbeStopUnconfirmed = errors.New("Codex probe process-scope termination unconfirmed")
)

type probeBudget struct {
	timeout     time.Duration
	stdoutBytes int
	stderrBytes int // zero combines stderr with stdout under one total limit
}

const probeStopTimeout = 5 * time.Second

func scopeStartError(cmd *exec.Cmd, err, unconfirmed error) error {
	// Every caller owns a fresh command. A non-nil process after Start fails
	// means launch happened, but neither its scope nor its reap is confirmed.
	// The launcher retains the sole Wait; do not infer proof from this handle.
	if err != nil && cmd.Process != nil {
		return errors.Join(err, unconfirmed)
	}
	return err
}

func cleanupProbeDirectory(path string, probeErr error) {
	if !errors.Is(probeErr, ErrProbeStopUnconfirmed) {
		_ = os.RemoveAll(path)
	}
}

// A short-lived --version command can exit before Start reads its namespace
// identity from /proc. The fixed shell waits for one parent-owned stdin byte
// before exec; positional arguments never become shell source. After this
// handshake the native executable is still the same namespace init. It receives
// EOF on stdin and an explicit credential-free environment.
const probeStartGate = `IFS= read -r probe_ready || exit 125; exec "$@"`

// Both descriptors belong to the caller. In particular, StdinPipe would let
// Cmd.Wait close the writer while a fast command races the startup handshake.
// Passing the read end as *os.File avoids that extra os/exec ownership.
func probeStdinPipe(cmd *exec.Cmd) (childInput, input *os.File, err error) {
	childInput, input, err = os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stdin = childInput
	return childInput, input, nil
}

// probeOutput is safe to inspect even when bounded cleanup cannot establish a
// reap. Overflow cancels the owning scope immediately while keeping memory and
// all returned diagnostics bounded; no caller accepts a truncated success.
type probeOutput struct {
	mu      sync.Mutex
	data    []byte
	limit   int
	cancel  context.CancelCauseFunc
	exceeds bool
}

func (b *probeOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	remaining := b.limit - len(b.data)
	if len(p) > remaining {
		b.data = append(b.data, p[:remaining]...)
		b.exceeds = true
		b.cancel(ErrProbeOutputLimit)
	} else {
		b.data = append(b.data, p...)
	}
	return n, nil
}

func (b *probeOutput) snapshot() ([]byte, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.data...), b.exceeds
}

func settleProbeExit(ctx context.Context, stopScope func(context.Context) (processscope.Proof, error), proof processscope.Proof, prior error) (processscope.Proof, error) {
	if proof.Quiescent() {
		return proof, prior
	}
	stopping, cancel := context.WithTimeout(context.WithoutCancel(ctx), probeStopTimeout)
	defer cancel()
	proof, err := stopScope(stopping)
	if err != nil || !proof.Quiescent() {
		return proof, errors.Join(prior, ErrProbeStopUnconfirmed, err)
	}
	return proof, prior
}

func runCodexProbe(ctx context.Context, executable, expectedDigest, home string, budget probeBudget, args ...string) (output, diagnostics []byte, runErr error) {
	if !canonical.ValidDigest(expectedDigest) || budget.timeout <= 0 || budget.stdoutBytes <= 0 || budget.stderrBytes < 0 {
		return nil, nil, fmt.Errorf("pinned Codex executable and bounded probe budget required")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	executable, err := trustedExecutable(executable, expectedDigest)
	if err != nil {
		return nil, nil, err
	}
	for _, dir := range []string{home, filepath.Join(home, ".codex"), filepath.Join(home, ".config"), filepath.Join(home, ".cache")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, nil, err
		}
	}
	deadline, end := context.WithTimeout(ctx, budget.timeout)
	defer end()
	probing, cancel := context.WithCancelCause(deadline)
	defer cancel(nil)
	gateArgs := append([]string{"-c", probeStartGate, "codex-probe", executable}, args...)
	cmd := exec.CommandContext(probing, "/bin/sh", gateArgs...)
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"LANG=C.UTF-8",
		"TZ=UTC",
		"HOME=" + home,
		"CODEX_HOME=" + filepath.Join(home, ".codex"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
	}
	cmd.WaitDelay = time.Second
	stdout := &probeOutput{limit: budget.stdoutBytes, cancel: cancel}
	stderr := stdout
	if budget.stderrBytes != 0 {
		stderr = &probeOutput{limit: budget.stderrBytes, cancel: cancel}
	}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	childInput, input, err := probeStdinPipe(cmd)
	if err != nil {
		return nil, nil, err
	}
	defer childInput.Close()
	defer input.Close()
	scope, err := processscope.Start(cmd)
	if err != nil {
		return nil, nil, scopeStartError(cmd, err, ErrProbeStopUnconfirmed)
	}
	var proof processscope.Proof
	defer func() {
		proof, runErr = settleProbeExit(ctx, scope.Stop, proof, runErr)
		var overflow, stderrOverflow bool
		output, overflow = stdout.snapshot()
		if stderr != stdout {
			diagnostics, stderrOverflow = stderr.snapshot()
		}
		if overflow || stderrOverflow {
			runErr = errors.Join(runErr, ErrProbeOutputLimit)
		}
		if cause := context.Cause(probing); cause != nil && !errors.Is(runErr, cause) {
			runErr = errors.Join(runErr, cause)
		}
	}()
	// Start has duplicated this descriptor into the child. Close only the
	// parent's copy; Wait has no ownership of either caller-held descriptor.
	if err := childInput.Close(); err != nil {
		return nil, nil, fmt.Errorf("close Codex probe parent stdin reader: %w", err)
	}
	if _, err := input.Write([]byte("\n")); err != nil {
		return nil, nil, fmt.Errorf("release Codex probe startup gate: %w", err)
	}
	if err := input.Close(); err != nil {
		return nil, nil, fmt.Errorf("close Codex probe startup gate: %w", err)
	}
	proof, runErr = scope.Wait(probing)
	return nil, nil, runErr
}
