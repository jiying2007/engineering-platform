package codexexec

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
)

type ContinueRequest struct {
	SourceRunID         string `json:"source_run_id"`
	RunID               string `json:"run_id"`
	AttemptID           string `json:"attempt_id"`
	CheckpointDigest    string `json:"checkpoint_digest"`
	ExpectedRunVersion  uint64 `json:"expected_run_version"`
	ExpectedWorkVersion uint64 `json:"expected_work_version"`
	ExecutionEpoch      uint64 `json:"execution_epoch"`
	RecoveryEpoch       uint64 `json:"recovery_epoch"`
	Reason              string `json:"reason"`
}

func (c ContinueRequest) Validate() error {
	for _, id := range []string{c.SourceRunID, c.RunID, c.AttemptID} {
		if !boundedID(id) {
			return fmt.Errorf("bounded continuation IDs required")
		}
	}
	if c.RunID == c.SourceRunID || !canonical.ValidDigest(c.CheckpointDigest) || c.ExpectedRunVersion == 0 || c.ExpectedWorkVersion == 0 || c.ExecutionEpoch == 0 || c.ExpectedRunVersion > 1<<62 || c.ExpectedWorkVersion > 1<<62 || c.ExecutionEpoch > 1<<62 || c.RecoveryEpoch > 1<<62 || strings.TrimSpace(c.Reason) == "" || len(c.Reason) > 2048 || !utf8.ValidString(c.Reason) || strings.ContainsRune(c.Reason, 0) {
		return fmt.Errorf("exact continuation decision required")
	}
	return nil
}
func (c SourceCheckpoint) ContinuationRef() (core.ContinuationRef, error) {
	d, err := c.Digest()
	return core.ContinuationRef{SourceRunID: c.Binding.Token.RunID, CheckpointDigest: d, ArchiveDigest: c.ArchiveDigest, ArchiveSize: c.ArchiveSize, SnapshotDigest: c.SnapshotDigest}, err
}

// Continuable requires an observed, explicitly requested interruption, not just a
// dead process, UNKNOWN response, failed turn, or a metadata-only cancellation.
func (t ControlTranscript) Continuable() bool {
	if _, err := t.Digest(); err != nil || t.Close.TurnStatus != "interrupted" || !t.Close.ProcessScope.Quiescent() {
		return false
	}
	interrupted := false
	for _, d := range t.Deliveries {
		switch d.State {
		case ControlAccepted:
		case ControlInterrupted:
			interrupted = true
		case ControlNotApplied:
			if d.DispatchedAt != nil {
				return false
			}
		default:
			return false
		}
	}
	return interrupted
}

type ContinuationReceipt struct {
	Version           int                   `json:"version"`
	Actor             string                `json:"actor"`
	Request           ContinueRequest       `json:"request"`
	Run               run.Run               `json:"run"`
	Attempt           run.Attempt           `json:"attempt"`
	Session           session.Session       `json:"session"`
	Input             core.RunInputManifest `json:"run_input"`
	SourceDisposition string                `json:"source_disposition"`
	CreatedAt         time.Time             `json:"created_at"`
}

func (c ContinuationReceipt) Validate() error {
	d, err := c.Input.Digest()
	if c.Version != 1 || !boundedID(c.Actor) || c.Request.Validate() != nil || err != nil || c.Input.Continuation == nil || c.Input.Continuation.SourceRunID != c.Request.SourceRunID || c.Input.Continuation.CheckpointDigest != c.Request.CheckpointDigest || c.Input.RunID != c.Request.RunID || c.Run.ID != c.Request.RunID || c.Run.RunInputManifestDigest != d || c.Run.TaskContractDigest != c.Input.TaskContractDigest || c.Run.State != run.Running || c.Run.CurrentEpoch != 1 || c.Run.Version != 1 || c.Run.ControlOwner != "RUNTIME" || c.Run.CurrentAttemptID != c.Request.AttemptID || c.Attempt.ID != c.Request.AttemptID || c.Attempt.Epoch != 1 || c.Attempt.StartedAt.IsZero() || !c.Attempt.EndedAt.IsZero() || c.Session.RunID != c.Run.ID || c.Session.ExecutionEpoch != 1 || c.Session.Owner != session.Runtime || c.Session.Paused || c.Session.LastSequence != 0 || c.SourceDisposition != Stopped || c.CreatedAt.IsZero() {
		return fmt.Errorf("invalid continuation receipt")
	}
	return nil
}

type ContinuationRepository interface {
	ContinueCodex(context.Context, string, ContinueRequest) (ContinuationReceipt, error)
	GetCodexContinuation(context.Context, string) (ContinuationReceipt, error)
}
