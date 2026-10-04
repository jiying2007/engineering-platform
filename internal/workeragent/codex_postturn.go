package workeragent

import (
	"errors"
	"fmt"

	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
)

// PostTurnFailureError is an operator-facing observation, not authoritative Run
// state. A report reply can be lost after Core committed FINISHED. Inspect that
// exact execution; never infer from this error that the model may be replayed.
type PostTurnFailureError struct {
	Phase        string
	JournalSaved bool
	Cause        error
}

func (e *PostTurnFailureError) Error() string {
	return fmt.Sprintf("post-turn phase %s failed (local journal saved %t; inspect exact Core execution, do not replay): %v", e.Phase, e.JournalSaved, e.Cause)
}
func (e *PostTurnFailureError) Unwrap() error { return e.Cause }

// Phase records use the existing owned reconciliation store, not another Core
// ledger or permission. Fixed names, original identities and no raw errors keep
// them bounded. ENTERED does not claim the phase finished. No public upload.
type postTurnRecord struct {
	Version                   int                         `json:"version"`
	Token                     codexexec.Token             `json:"token"`
	Phase                     string                      `json:"phase"`
	Observation               string                      `json:"observation"`
	TaskDigest                string                      `json:"task_contract_digest"`
	InputDigest               string                      `json:"run_input_manifest_digest"`
	TranscriptDigest          string                      `json:"control_transcript_digest"`
	ResultDigest              string                      `json:"expected_result_digest,omitempty"`
	Checkpoint                *codexexec.SourceCheckpoint `json:"source_checkpoint,omitempty"`
	RegistrationConfirmed     bool                        `json:"registration_confirmed"`
	DeliveryConfirmedByWorker bool                        `json:"delivery_confirmed_by_worker"`
	ExecutionAuthorized       bool                        `json:"execution_authorized"`
}

func phaseRecord(permit codexexec.Permit, t codexexec.ControlTranscript, phase, resultDigest string) (postTurnRecord, error) {
	d, err := t.Digest()
	if err != nil || t.Close.Binding.Token != permit.Token || t.Close.TurnStatus != "completed" || !t.Close.ProcessScope.Quiescent() {
		return postTurnRecord{}, fmt.Errorf("sealed completed/quiescent turn required")
	}
	switch phase {
	case "TURN_RECEIPT", "FINALIZE", "RESULT_PERSIST", "REPORT_RENEW", "RESULT_REPORT", "RECEIPT_VERIFY":
	default:
		return postTurnRecord{}, fmt.Errorf("unknown post-turn phase")
	}
	return postTurnRecord{Version: 1, Token: permit.Token, Phase: phase, Observation: "ENTERED", TaskDigest: permit.Assignment.Intent.TaskDigest, InputDigest: permit.Assignment.Intent.InputDigest, TranscriptDigest: d, ResultDigest: resultDigest}, nil
}
func savePostTurnPhase(p *preparation.Preparer, permit codexexec.Permit, prepared preparation.Result, t codexexec.ControlTranscript, phase, resultDigest string) error {
	record, err := phaseRecord(permit, t, phase, resultDigest)
	if err != nil {
		return err
	}
	return p.SaveCodex(permit.Assignment, prepared, sandbox.Hash([]byte(permit.Token.ID + ":post-turn:" + phase))[7:], record)
}
func retainPostTurnFailure(c Transport, p *preparation.Preparer, permit codexexec.Permit, prepared preparation.Result, t codexexec.ControlTranscript, phase, resultDigest string, finalizeEntered bool, cause error) error {
	result := &PostTurnFailureError{Phase: phase, Cause: cause}
	record, err := phaseRecord(permit, t, phase, resultDigest)
	if err != nil {
		result.Cause = errors.Join(cause, err)
		return result
	}
	result.Cause = retainSource(c, p, permit, prepared, t, cause, finalizeEntered)
	record.Observation = "FAILED_UNCONFIRMED"
	var retained *CheckpointRetainedError
	if errors.As(result.Cause, &retained) {
		record.Checkpoint = &retained.Artifact.Facts
		record.RegistrationConfirmed = retained.Registered
	}
	journalErr := p.SaveCodex(permit.Assignment, prepared, sandbox.Hash([]byte(permit.Token.ID + ":post-turn-failure"))[7:], record)
	result.JournalSaved = journalErr == nil
	result.Cause = errors.Join(result.Cause, journalErr)
	return result
}
