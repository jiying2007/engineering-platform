package githubpublish

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"time"

	"github.com/jiying2007/engineering-platform/internal/processscope"
)

type gitProofKey struct{}

var ErrPublisherGitQuiescenceUnknown = errors.New("publisher Git process-tree termination unconfirmed; retain private staging and reconcile")

type gitProofLog struct {
	proofs    []processscope.Proof
	uncertain bool
}

// This per-call accumulator is never a durable ledger or an execution queue.
// The synchronous Git calls share it only for cleanup quarantine and evidence.
func publisherGitQuiescent(ctx context.Context) bool {
	state, _ := ctx.Value(gitProofKey{}).(*gitProofLog)
	return state == nil || !state.uncertain
}

// The small stdin gate keeps even a fast Git process alive until the existing
// kernel-scope launcher has recorded its dedicated PID namespace. No untrusted
// text is interpolated into shell syntax and no descriptor is inherited except
// normal stdio. Every Git descendant dies when the namespace init is reaped.
func runPublisherGit(ctx context.Context, git string, args []string, env []string, output, diagnostic *boundedOutput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	argv := append([]string{"-c", `read -r gate && exec "$@"`, "ep-publisher-git", git}, args...)
	cmd := exec.CommandContext(ctx, "/bin/sh", argv...)
	cmd.Env = env
	cmd.Stdout = output
	cmd.Stderr = diagnostic
	cmd.WaitDelay = 2 * time.Second
	gate, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	defer gate.Close()
	scope, err := processscope.Start(cmd)
	if err != nil {
		return err
	}
	_, gateErr := gate.Write([]byte("go\n"))
	closeErr := gate.Close()
	if errors.Is(closeErr, os.ErrClosed) {
		closeErr = nil
	}
	proof, waitErr := scope.Wait(ctx)
	if !proof.Quiescent() {
		stop, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		stopped, stopErr := scope.Stop(stop)
		cancel()
		proof = stopped
		waitErr = errors.Join(waitErr, stopErr)
	}
	if !proof.Quiescent() {
		_ = recordPublicationReap(ctx, proof)
		if state, _ := ctx.Value(gitProofKey{}).(*gitProofLog); state != nil {
			state.uncertain = true
		}
		return errors.Join(ErrPublisherGitQuiescenceUnknown, gateErr, closeErr, waitErr)
	}
	if collector, _ := ctx.Value(gitProofKey{}).(*gitProofLog); collector != nil {
		collector.proofs = append(collector.proofs, proof)
	}
	return errors.Join(gateErr, closeErr, waitErr, recordPublicationReap(ctx, proof))
}
