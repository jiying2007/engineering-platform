package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/recovery"
)

func TestRecoveryProofBlocksUnresolvedExternalOperation(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	runID := setupPostgresActionRun(t, s, "recovery-proof")
	now := time.Now().UTC()
	req := action.Request{
		ID: "recovery-op", RunID: runID, ExecutionEpoch: 1, RecoveryEpoch: 0,
		Action: "ci.dispatch", RiskClass: action.ControlledMutation, Capability: "ci",
		ParametersDigest: "sha256:" + strings.Repeat("1", 64), IdempotencyKey: "recovery-op",
		RequestedBy: "runtime", RequestedAt: now,
	}
	op := action.NewWithRequestDigest(req, "sha256:"+strings.Repeat("2", 64), now)
	if err := s.Create(*op); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginRecovery(0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRecoveryProof(context.Background(), 1, "urn:engineering-platform:operator:reconciler"); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Fatalf("PLANNED operation did not block proof: %v", err)
	}
	// Recovery cannot retroactively authorize a pre-effect PLANNED reservation
	// to become a new external mutation. Its outstanding identity must remain
	// blocked until an independently authorized explicit disposition exists.
	if err := op.Transition(action.Dispatched, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(*op); !errors.Is(err, recovery.ErrStaleEpoch) {
		t.Fatalf("Recovery authorized new external dispatch: %v", err)
	}
	stored, err := s.Get(op.ID)
	if err != nil || stored.State != action.Planned {
		t.Fatalf("Recovery changed pre-effect reservation: %#v err=%v", stored, err)
	}
	if _, err := s.CreateRecoveryProof(context.Background(), 1, "urn:engineering-platform:operator:reconciler"); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Fatalf("orphaned PLANNED Action was silently cleared: %v", err)
	}
}

func TestRecoveryProofBlocksLiveMutationOutboxLease(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	ctx := context.Background()
	if _, err := s.BeginRecovery(0); err != nil {
		t.Fatal(err)
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO outbox_events
	 (outbox_key,topic,risk_class,payload_json,state,lease_owner,lease_until,attempt_count,lease_recovery_epoch)
	 VALUES('recovery-lease','run.started','CONTROLLED_MUTATION','{}','LEASED','dispatcher',clock_timestamp()+interval '1 minute',1,0)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRecoveryProof(ctx, 1, "urn:engineering-platform:operator:reconciler"); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Fatalf("live mutation lease did not block proof: %v", err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE outbox_events SET lease_until=clock_timestamp()-interval '1 second' WHERE outbox_key='recovery-lease'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRecoveryProof(ctx, 1, "urn:engineering-platform:operator:reconciler"); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryProofAcceptsSettlementOfPreviouslyDispatchedAction(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	runID := setupPostgresActionRun(t, s, "recovery-inflight")
	now := time.Now().UTC()
	req := action.Request{
		ID: "recovery-inflight-op", RunID: runID, ExecutionEpoch: 1, RecoveryEpoch: 0,
		Action: "ci.dispatch", RiskClass: action.ControlledMutation, Capability: "ci",
		ParametersDigest: "sha256:" + strings.Repeat("3", 64), IdempotencyKey: "recovery-inflight-op",
		RequestedBy: "runtime", RequestedAt: now,
	}
	op := action.NewWithRequestDigest(req, "sha256:"+strings.Repeat("4", 64), now)
	if err := s.Create(*op); err != nil {
		t.Fatal(err)
	}
	if err := op.Transition(action.Dispatched, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(*op); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginRecovery(0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRecoveryProof(context.Background(), 1, "urn:engineering-platform:operator:reconciler"); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Fatalf("in-flight dispatch was erased from Recovery: %v", err)
	}
	// Recovery must allow an old dispatched operation to settle; it must not
	// authorize a NEW dispatch during the reconciliation epoch.
	if err := op.Transition(action.Confirmed, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(*op); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRecoveryProof(context.Background(), 1, "urn:engineering-platform:operator:reconciler"); err != nil {
		t.Fatal(err)
	}
}
