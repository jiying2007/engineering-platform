package workeragent

import (
	"context"
	"fmt"
	"github.com/jiying2007/engineering-platform/internal/sourcecheckpoint"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
	"strings"
)

// RestoreContinuationSource never fetches an artifact or executes model code.
// The archive locator is from the host's new, exact RunInput approval, not JSON
// received from the model or inherited from a previous Run's authorization.
func RestoreContinuationSource(ctx context.Context, a workerqueue.Assignment, archive, destination string) error {
	if _, err := workerqueue.Validate(a); err != nil {
		return err
	}
	ref := a.Input.Continuation
	if ref == nil || archive == "" {
		return workerqueue.ErrIdentity
	}
	facts, err := sourcecheckpoint.Verify(ctx, archive, ref.ArchiveDigest, ref.SourceRunID)
	if err != nil {
		return err
	}
	got, err := facts.ContinuationRef()
	if err != nil || got != *ref || facts.TaskDigest != a.Intent.TaskDigest || facts.BaseCommit != strings.ToLower(a.Task.BaseCommit) || "codex/"+facts.Binding.Token.ProfileDigest != a.Input.ToolProfile || facts.Binding.Token.WorkerProfile != a.Input.WorkerProfile {
		return fmt.Errorf("continuation source does not match authorized frozen task/profile")
	}
	restored, err := sourcecheckpoint.Restore(ctx, archive, ref.ArchiveDigest, ref.SourceRunID, destination)
	if err != nil {
		return err
	}
	if restored != facts {
		return workerqueue.ErrIdentity
	}
	return nil
}
