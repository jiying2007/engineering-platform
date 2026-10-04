package workeragent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
	"github.com/jiying2007/engineering-platform/internal/sourcecheckpoint"
)

// CheckpointRetainedError preserves an unsuccessful execution and tells the
// operator where exact stopped-source bytes survived. It never means FINISHED.
type CheckpointRetainedError struct {
	Cause      error
	Artifact   sourcecheckpoint.Artifact
	Registered bool
}

func (e *CheckpointRetainedError) Error() string {
	return fmt.Sprintf("execution did not finish; stopped source retained at %s (digest %s; Core registration %t): %v", e.Artifact.Path, e.Artifact.Facts.ArchiveDigest, e.Registered, e.Cause)
}
func (e *CheckpointRetainedError) Unwrap() error { return e.Cause }
func retainStoppedSource(c Transport, p *preparation.Preparer, permit codexexec.Permit, prepared preparation.Result, t codexexec.ControlTranscript, cause error) error {
	if _, err := t.Digest(); err != nil || !t.Close.ProcessScope.Quiescent() {
		return cause
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	source, root, err := p.CheckpointPaths(ctx, c.Subject(), permit.Assignment, prepared, permit.Preparation.FactsDigest)
	if err != nil {
		return errors.Join(cause, fmt.Errorf("stopped source capture denied: %w", err))
	}
	artifact, err := sourcecheckpoint.Capture(ctx, source, root, permit, t)
	if err != nil {
		return errors.Join(cause, fmt.Errorf("stopped source capture failed: %w", err))
	}
	result := &CheckpointRetainedError{Cause: cause, Artifact: artifact}
	recordErr := p.SaveCodex(permit.Assignment, prepared, sandbox.Hash([]byte(permit.Token.ID + ":source-checkpoint"))[7:], artifact)
	var readback codexexec.SourceCheckpoint
	reportErr := c.Call(ctx, http.MethodPost, "/api/v1/worker/codex/source-checkpoint", artifact.Facts, &readback)
	if reportErr == nil && readback != artifact.Facts {
		reportErr = fmt.Errorf("Core checkpoint readback mismatch")
	}
	result.Registered = reportErr == nil
	result.Cause = errors.Join(cause, recordErr, reportErr)
	return result
}
