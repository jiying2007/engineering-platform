package githubpublish

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/canonical"
)

// Version 2 STARTED witnesses guarantee that one inode lock is held from before
// STARTED until the original caller exits; invoked.json must be synced before
// entering Remote.Publish. Version 1 is read-only historical positive evidence,
// never evidence of this newer ordering.
type dispatchLease struct {
	*os.Root
	lock *os.File
}

func (l *dispatchLease) Close() error {
	if l == nil {
		return nil
	}
	var err error
	if l.Root != nil {
		err = l.Root.Close()
		l.Root = nil
	}
	if l.lock != nil {
		err = errors.Join(err, l.lock.Close())
		l.lock = nil
	}
	return err
}
func witnessAbsent(root *os.Root, name string) bool {
	_, err := root.Lstat(name)
	return errors.Is(err, os.ErrNotExist)
}

type DispatchInvocation struct {
	Version     int       `json:"version"`
	StartDigest string    `json:"start_digest"`
	ArmedAt     time.Time `json:"armed_at"`
}

func (i DispatchInvocation) Validate(s DispatchStart) error {
	d, err := canonical.Digest(s)
	if err != nil || s.Validate() != nil || s.Version != 2 || i.Version != 1 || i.StartDigest != d ||
		i.ArmedAt.IsZero() || i.ArmedAt.Before(s.StartedAt) {
		return ErrDispatchEvidenceUnavailable
	}
	return nil
}
func (l *dispatchLease) arm(ctx context.Context, start DispatchStart) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if l == nil || l.Root == nil || l.lock == nil || start.Version != 2 || start.Validate() != nil ||
		!witnessAbsent(l.Root, "not-invoked.json") || !witnessAbsent(l.Root, "completed.json") {
		return ErrDispatchEvidenceUnavailable
	}
	incarnation, err := witnessIncarnation(l.Root)
	if err != nil || incarnation != start.Incarnation {
		return ErrDispatchEvidenceUnavailable
	}
	d, _ := canonical.Digest(start)
	i := DispatchInvocation{Version: 1, StartDigest: d, ArmedAt: time.Now().UTC()}
	if err := i.Validate(start); err != nil {
		return err
	}
	if err := writeWitness(l.Root, "invoked.json", i); err != nil {
		return err
	}
	return ctx.Err() // Cancellation AFTER arming stays ambiguous, never grants retry.
}

type DispatchNonInvocation struct {
	Kind        string    `json:"kind"`
	StartDigest string    `json:"start_digest"`
	SealedAt    time.Time `json:"sealed_at"`
}

func (n DispatchNonInvocation) Validate(s DispatchStart) error {
	d, err := canonical.Digest(s)
	if err != nil || s.Validate() != nil || s.Version != 2 || n.Kind != "PUBLISH_NOT_INVOKED_V1" ||
		n.StartDigest != d || n.SealedAt.IsZero() || n.SealedAt.Before(s.StartedAt) {
		return ErrDispatchEvidenceUnavailable
	}
	return nil
}

// This evidence contains NO external observation or success receipt. The
// independent Publisher sealed a known v2 pre-invocation slot under its lock.
// Absent historical slots, v1 starts and any present/partial invocation marker
// can never enter this path, regardless of remote absence or a stopped PID.
type NonInvocationEvidence struct {
	Version int                   `json:"version"`
	Start   DispatchStart         `json:"start"`
	Seal    DispatchNonInvocation `json:"seal"`
}

func (e NonInvocationEvidence) Validate() error {
	if e.Version != 1 {
		return ErrDispatchEvidenceUnavailable
	}
	return e.Seal.Validate(e.Start)
}
func (j *dispatchJournal) sealUninvoked(ctx context.Context, identity DispatchIdentity, plan Plan) (NonInvocationEvidence, error) {
	var zero NonInvocationEvidence
	if identity.Validate() != nil || plan.Validate() != nil {
		return zero, ErrDispatchEvidenceUnavailable
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	root, err := j.open()
	if err != nil {
		return zero, err
	}
	defer root.Close()
	name := dispatchSlot(plan)
	before, err := root.Lstat(name)
	if err != nil || !witnessPrivateDirectory(before) {
		return zero, ErrDispatchEvidenceUnavailable
	}
	slot, err := root.OpenRoot(name)
	if err != nil {
		return zero, err
	}
	defer slot.Close()
	opened, err := slot.Stat(".")
	if err != nil || !witnessPrivateDirectory(opened) || !os.SameFile(before, opened) {
		return zero, ErrDispatchEvidenceUnavailable
	}
	lock, err := lockWitnessSlot(slot)
	if err != nil {
		return zero, err
	}
	defer lock.Close()
	var start DispatchStart
	if readWitness(slot, "started.json", &start) != nil || start.Version != 2 || start.Validate() != nil || start.Identity != identity || start.Plan != plan {
		return zero, ErrDispatchEvidenceUnavailable
	}
	// A copied/restored directory or a reboot does not prove what the original
	// writer did after that snapshot. Never seal it from missing intent alone.
	incarnation, err := witnessIncarnation(slot)
	if err != nil || incarnation != start.Incarnation {
		return zero, ErrDispatchEvidenceUnavailable
	}
	if !witnessAbsent(slot, "invoked.json") || !witnessAbsent(slot, "completed.json") {
		return zero, ErrDispatchEvidenceUnavailable
	}
	var seal DispatchNonInvocation
	if witnessAbsent(slot, "not-invoked.json") {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		digest, _ := canonical.Digest(start)
		seal = DispatchNonInvocation{Kind: "PUBLISH_NOT_INVOKED_V1", StartDigest: digest, SealedAt: time.Now().UTC()}
		if err := seal.Validate(start); err != nil {
			return zero, err
		}
		if err := writeWitness(slot, "not-invoked.json", seal); err != nil {
			return zero, err
		}
	}
	// Exact repeat never rewrites a marker; readback re-establishes durability.
	if err := readWitness(slot, "not-invoked.json", &seal); err != nil {
		return zero, err
	}
	e := NonInvocationEvidence{Version: 1, Start: start, Seal: seal}
	if e.Validate() != nil {
		return zero, ErrDispatchEvidenceUnavailable
	}
	after, err := root.Lstat(name)
	if err != nil || !os.SameFile(before, after) {
		return zero, ErrDispatchEvidenceUnavailable
	}
	if err := syncRoot(root); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return e, nil
}
func (s *RemoteService) handleNoInvocation(w http.ResponseWriter, r *http.Request) {
	var request publishRequest
	if !decodePublisherRequest(w, r, &request) {
		return
	}
	if request.Identity.Validate() != nil || request.Identity.RunID != request.Plan.RunID || s.authorizePlan(request.Plan) != nil {
		writePublisherError(w, http.StatusForbidden, "exact frozen original identity required")
		return
	}
	e, err := s.journal.sealUninvoked(r.Context(), request.Identity, request.Plan)
	if err != nil {
		writePublisherError(w, http.StatusConflict, "original pre-invocation proof unavailable; no replay")
		return
	}
	writePublisherJSON(w, http.StatusOK, e)
}

type NonInvocationObserver interface {
	FenceUninvokedPublication(context.Context, action.Operation) (NonInvocationEvidence, error)
}
type nonInvocationRemote interface {
	FenceDispatch(context.Context, DispatchIdentity, Plan) (NonInvocationEvidence, error)
}

func (p *Provider) FenceUninvokedPublication(ctx context.Context, op action.Operation) (NonInvocationEvidence, error) {
	var zero NonInvocationEvidence
	if p == nil {
		return zero, ErrDispatchEvidenceUnavailable
	}
	identity, err := IdentityForOperation(op)
	if err != nil {
		return zero, err
	}
	remote, ok := p.remote.(nonInvocationRemote)
	if !ok {
		return zero, ErrDispatchEvidenceUnavailable
	}
	plan, _, err := p.derive(ctx, op.RunID, op.ExecutionEpoch, "", false)
	if err != nil {
		return zero, err
	}
	e, err := remote.FenceDispatch(ctx, identity, plan)
	if err != nil {
		return zero, err
	}
	if e.Validate() != nil || e.Start.Identity != identity || e.Start.Plan != plan {
		return zero, ErrDispatchEvidenceUnavailable
	}
	return e, ctx.Err()
}
func (c *remoteClient) FenceDispatch(ctx context.Context, identity DispatchIdentity, plan Plan) (NonInvocationEvidence, error) {
	var e NonInvocationEvidence
	if c == nil || c.client == nil || identity.Validate() != nil || plan.Validate() != nil || identity.RunID != plan.RunID {
		return e, ErrDispatchEvidenceUnavailable
	}
	if err := c.call(ctx, "/v1/dispatch-no-invocation", publishRequest{Plan: plan, Identity: identity}, &e); err != nil {
		return NonInvocationEvidence{}, err
	}
	if e.Validate() != nil || e.Start.Identity != identity || e.Start.Plan != plan {
		return NonInvocationEvidence{}, ErrDispatchEvidenceUnavailable
	}
	return e, nil
}

const NonInvocationRecoveryKind = "RECOVERY_GITHUB_PUBLICATION_NOT_INVOKED_V1"

type NonInvocationRecoveryReceipt struct {
	Kind                string                     `json:"kind"`
	Request             PublicationRecoveryRequest `json:"request"`
	RequestDigest       string                     `json:"request_digest"`
	Reconciler          string                     `json:"reconciler"`
	PreviousState       action.State               `json:"previous_state"`
	PreviousObservation string                     `json:"previous_observation"`
	Evidence            NonInvocationEvidence      `json:"evidence"`
	EvidenceDigest      string                     `json:"evidence_digest"`
	RecordedAt          time.Time                  `json:"recorded_at"`
	ReplayAuthorized    bool                       `json:"replay_authorized"`
	ProductionQualified bool                       `json:"production_qualified"`
}

func (r NonInvocationRecoveryReceipt) Validate() error {
	d, err := r.Request.Digest()
	e, eerr := canonical.Digest(r.Evidence)
	if err != nil || eerr != nil || r.Kind != NonInvocationRecoveryKind || r.RequestDigest != d || r.EvidenceDigest != e ||
		r.Evidence.Validate() != nil || r.Request.Identity != r.Evidence.Start.Identity || !RecoverablePublicationState(r.PreviousState) ||
		r.Reconciler == "" || len(r.Reconciler) > 256 || strings.TrimSpace(r.Reconciler) != r.Reconciler || strings.ContainsAny(r.Reconciler, "\x00\r\n") ||
		len(r.PreviousObservation) > 64<<10 || r.RecordedAt.IsZero() || r.ReplayAuthorized || r.ProductionQualified {
		return ErrDispatchEvidenceUnavailable
	}
	return nil
}
