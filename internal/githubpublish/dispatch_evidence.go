package githubpublish

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/canonical"
)

var ErrDispatchEvidenceUnavailable = errors.New("original publication completion evidence unavailable; no replay authorized")

// DispatchIdentity is the original Core reservation, not an operator assertion
// about process termination. The publication action/risk/capability are fixed.
type DispatchIdentity struct {
	OperationID    string `json:"operation_id"`
	RunID          string `json:"run_id"`
	ExecutionEpoch uint64 `json:"execution_epoch"`
	RecoveryEpoch  uint64 `json:"original_recovery_epoch"`
	IdempotencyKey string `json:"idempotency_key"`
	RequestDigest  string `json:"request_digest"`
}

func (d DispatchIdentity) Validate() error {
	for _, s := range []string{d.OperationID, d.RunID, d.IdempotencyKey} {
		if s == "" || len(s) > 256 || strings.TrimSpace(s) != s || strings.ContainsAny(s, "\x00\r\n") {
			return ErrDispatchEvidenceUnavailable
		}
	}
	if d.ExecutionEpoch == 0 || d.ExecutionEpoch > 1<<62 || d.RecoveryEpoch > 1<<62 || !canonical.ValidDigest(d.RequestDigest) {
		return ErrDispatchEvidenceUnavailable
	}
	return nil
}
func IdentityForOperation(o action.Operation) (DispatchIdentity, error) {
	d := DispatchIdentity{o.ID, o.RunID, o.ExecutionEpoch, o.RecoveryEpoch, o.IdempotencyKey, o.RequestDigest}
	if o.Action != Action || o.Capability != Capability || o.RiskClass != action.ControlledMutation {
		return DispatchIdentity{}, ErrDispatchEvidenceUnavailable
	}
	return d, d.Validate()
}

type DispatchStart struct {
	Version   int              `json:"version"`
	Identity  DispatchIdentity `json:"identity"`
	Plan      Plan             `json:"plan"`
	StartedAt time.Time        `json:"started_at"`
}

func (s DispatchStart) Validate() error {
	if s.Version != 1 || s.Identity.Validate() != nil || s.Plan.Validate() != nil || s.Identity.RunID != s.Plan.RunID || s.StartedAt.IsZero() {
		return ErrDispatchEvidenceUnavailable
	}
	return nil
}

type DispatchCompletion struct {
	Version     int                `json:"version"`
	StartDigest string             `json:"start_digest"`
	Receipt     PublicationReceipt `json:"receipt"`
	ReturnedAt  time.Time          `json:"returned_at"`
}

func (c DispatchCompletion) Validate(s DispatchStart) error {
	digest, err := canonical.Digest(s)
	switch c.Receipt.PublicationOutcome {
	case "CREATED", "UPDATED", "EXISTING":
	default:
		return ErrDispatchEvidenceUnavailable
	}
	if err != nil || s.Validate() != nil || c.Version != 1 || c.StartDigest != digest || c.Receipt.Validate(s.Plan) != nil || c.ReturnedAt.IsZero() || c.ReturnedAt.Before(s.StartedAt) {
		return ErrDispatchEvidenceUnavailable
	}
	return nil
}

// PublicationEvidence is obtained by Core directly from the authenticated
// independent Publisher. Hashes bind retained bytes; they do not authenticate
// arbitrary user-uploaded JSON. Only a successful original invocation can
// produce Completion. STARTED-only, failed or cancelled invocations are not
// accepted as quiescent, even when a later query happens to find the remote PR.
type PublicationEvidence struct {
	Version     int                `json:"version"`
	Start       DispatchStart      `json:"start"`
	Completion  DispatchCompletion `json:"completion"`
	Observation ObserveResult      `json:"observation"`
	ObservedAt  time.Time          `json:"observed_at"`
}

func (e PublicationEvidence) Validate() error {
	if e.Version != 1 || e.Completion.Validate(e.Start) != nil || e.Observation.Outcome != ObservedConfirmed || e.Observation.Receipt.PublicationOutcome != "OBSERVED" || e.Observation.Receipt.Validate(e.Start.Plan) != nil || e.ObservedAt.IsZero() || e.ObservedAt.Before(e.Completion.ReturnedAt) {
		return ErrDispatchEvidenceUnavailable
	}
	a, b := e.Completion.Receipt, e.Observation.Receipt
	// A newly created/replacement PR with the same branch is not the original PR.
	if a.PullRequestNumber != b.PullRequestNumber || a.PullRequestURL != b.PullRequestURL {
		return ErrDispatchEvidenceUnavailable
	}
	return nil
}
func (e PublicationEvidence) Digest() (string, error) {
	if err := e.Validate(); err != nil {
		return "", err
	}
	return canonical.Digest(e)
}

type PublicationRecoveryRequest struct {
	Version       int              `json:"version"`
	Identity      DispatchIdentity `json:"identity"`
	RecoveryEpoch uint64           `json:"recovery_epoch"`
}

func (r PublicationRecoveryRequest) Validate() error {
	if r.Version != 1 || r.Identity.Validate() != nil || r.RecoveryEpoch <= r.Identity.RecoveryEpoch || r.RecoveryEpoch > 1<<62 {
		return ErrDispatchEvidenceUnavailable
	}
	return nil
}
func (r PublicationRecoveryRequest) Digest() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return canonical.Digest(r)
}
func RecoverablePublicationState(s action.State) bool {
	switch s {
	case action.Dispatched, action.Unknown, action.Reconciling, action.Manual, action.SafeToRetry:
		return true
	}
	return false
}

type PublicationRecoveryReceipt struct {
	Kind                string                     `json:"kind"`
	Request             PublicationRecoveryRequest `json:"request"`
	RequestDigest       string                     `json:"request_digest"`
	Reconciler          string                     `json:"reconciler"`
	PreviousState       action.State               `json:"previous_state"`
	Evidence            PublicationEvidence        `json:"evidence"`
	EvidenceDigest      string                     `json:"evidence_digest"`
	RecordedAt          time.Time                  `json:"recorded_at"`
	ReplayAuthorized    bool                       `json:"replay_authorized"`
	ProductionQualified bool                       `json:"production_qualified"`
}

func (r PublicationRecoveryReceipt) Validate() error {
	request, err := r.Request.Digest()
	evidence, eerr := r.Evidence.Digest()
	if err != nil || eerr != nil || request != r.RequestDigest || evidence != r.EvidenceDigest || r.Kind != "RECOVERY_GITHUB_PUBLICATION_CONFIRMED_V1" || r.Request.Identity != r.Evidence.Start.Identity || !RecoverablePublicationState(r.PreviousState) || r.Reconciler == "" || len(r.Reconciler) > 256 || strings.TrimSpace(r.Reconciler) != r.Reconciler || strings.ContainsAny(r.Reconciler, "\x00\r\n") || r.RecordedAt.IsZero() || r.ReplayAuthorized || r.ProductionQualified {
		return ErrDispatchEvidenceUnavailable
	}
	return nil
}

type PublicationRecoveryObserver interface {
	ObserveOriginalPublication(context.Context, action.Operation) (PublicationEvidence, error)
}

// dispatchEvidenceRemote intentionally has no publishing method. A direct
// in-process legacy pilot Remote cannot stand in for independent mTLS readback.
type dispatchEvidenceRemote interface {
	ObserveDispatch(context.Context, DispatchIdentity, Plan) (PublicationEvidence, error)
}

func (p *Provider) ObserveOriginalPublication(ctx context.Context, op action.Operation) (PublicationEvidence, error) {
	var zero PublicationEvidence
	if p == nil {
		return zero, ErrDispatchEvidenceUnavailable
	}
	identity, err := IdentityForOperation(op)
	if err != nil {
		return zero, err
	}
	remote, ok := p.remote.(dispatchEvidenceRemote)
	if !ok {
		return zero, ErrDispatchEvidenceUnavailable
	}
	plan, _, err := p.derive(ctx, op.RunID, op.ExecutionEpoch, "", false)
	if err != nil {
		return zero, err
	}
	e, err := remote.ObserveDispatch(ctx, identity, plan)
	if err != nil {
		return zero, err
	}
	if e.Validate() != nil || e.Start.Identity != identity || e.Start.Plan != plan {
		return zero, ErrDispatchEvidenceUnavailable
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return e, nil
}
func (c *remoteClient) ObserveDispatch(ctx context.Context, identity DispatchIdentity, plan Plan) (PublicationEvidence, error) {
	var e PublicationEvidence
	if c == nil || c.client == nil || identity.Validate() != nil || plan.Validate() != nil || identity.RunID != plan.RunID {
		return e, ErrDispatchEvidenceUnavailable
	}
	if err := c.call(ctx, "/v1/dispatch-evidence", publishRequest{Plan: plan, Identity: identity}, &e); err != nil {
		return PublicationEvidence{}, fmt.Errorf("original publisher evidence: %w", err)
	}
	if e.Validate() != nil || e.Start.Identity != identity || e.Start.Plan != plan {
		return PublicationEvidence{}, ErrDispatchEvidenceUnavailable
	}
	return e, nil
}
