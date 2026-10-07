package workeragent

import (
	"context"
	"fmt"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/sourcecheckpoint"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

// VerifyContinuationArchive applies the same exact authorization-independent
// identity checks used by restore and private dependency capture. It reads no
// Core state, executes no Git/model code and grants no continuation/replay right.
func VerifyContinuationArchive(ctx context.Context, a workerqueue.Assignment, archive string) (codexexec.SourceCheckpoint, error) {
	var zero codexexec.SourceCheckpoint
	if _, err := workerqueue.Validate(a); err != nil {
		return zero, err
	}
	ref := a.Input.Continuation
	if ref == nil || archive == "" {
		return zero, workerqueue.ErrIdentity
	}
	facts, err := sourcecheckpoint.Verify(ctx, archive, ref.ArchiveDigest, ref.SourceRunID)
	if err != nil {
		return zero, err
	}
	got, err := facts.ContinuationRef()
	if err != nil || got != *ref || facts.TaskDigest != a.Intent.TaskDigest ||
		facts.BaseCommit != strings.ToLower(a.Task.BaseCommit) ||
		"codex/"+facts.Binding.Token.ProfileDigest != a.Input.ToolProfile ||
		facts.Binding.Token.WorkerProfile != a.Input.WorkerProfile {
		return zero, fmt.Errorf("continuation source does not match authorized frozen task/profile")
	}
	return facts, nil
}

// RestoreContinuationSource never fetches an artifact or executes model code.
// The archive locator is from the host's new, exact RunInput approval, not JSON
// received from the model or inherited from a previous Run's authorization.
func RestoreContinuationSource(ctx context.Context, a workerqueue.Assignment, archive, destination string) error {
	facts, err := VerifyContinuationArchive(ctx, a, archive)
	if err != nil {
		return err
	}
	restored, err := sourcecheckpoint.Restore(ctx, archive, facts.ArchiveDigest, facts.Binding.Token.RunID, destination)
	if err != nil {
		return err
	}
	if restored != facts {
		return workerqueue.ErrIdentity
	}
	return nil
}
