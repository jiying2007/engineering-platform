package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/audit"
	"github.com/jiying2007/engineering-platform/internal/canonical"
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
	if err := s.CreateContext(context.Background(), *op); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginRecoveryContext(context.Background(), 0); err != nil {
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
	if err := s.UpdateContext(context.Background(), *op); !errors.Is(err, recovery.ErrStaleEpoch) {
		t.Fatalf("Recovery authorized new external dispatch: %v", err)
	}
	stored, err := s.GetContext(context.Background(), op.ID)
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
	if _, err := s.BeginRecoveryContext(context.Background(), 0); err != nil {
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
	if err := s.CreateContext(context.Background(), *op); err != nil {
		t.Fatal(err)
	}
	if err := op.Transition(action.Dispatched, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateContext(context.Background(), *op); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginRecoveryContext(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRecoveryProof(context.Background(), 1, "urn:engineering-platform:operator:reconciler"); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Fatalf("in-flight dispatch was erased from Recovery: %v", err)
	}
	// Recovery must allow an old dispatched operation to settle; it must not
	// authorize a NEW dispatch during the reconciliation epoch. Use the actual
	// settlement clock: a synthetic now+2s timestamp can be later than its
	// transaction audit and must not qualify as original completion provenance.
	if err := op.Transition(action.Confirmed, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateContext(context.Background(), *op); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRecoveryProof(context.Background(), 1, "urn:engineering-platform:operator:reconciler"); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryProofOriginalConfirmationChronology(t *testing.T) {
	at := time.Date(2026, time.October, 14, 6, 0, 0, 123456000, time.UTC)
	op := action.Operation{
		ID: "original-clock-op", RunID: "original-clock-run", ExecutionEpoch: 1,
		Action: "ci.dispatch", RiskClass: action.ControlledMutation, Capability: "ci",
		IdempotencyKey: "original-clock-key", RequestDigest: "sha256:" + strings.Repeat("5", 64),
		State: action.Confirmed, UpdatedAt: at,
	}
	payload, err := canonical.Digest(op)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		offset time.Duration
		valid  bool
	}{
		{"audit-before-settlement-two-seconds", -2 * time.Second, false},
		{"audit-before-settlement-one-microsecond", -time.Microsecond, false},
		{"audit-at-settlement", 0, true},
		{"audit-after-settlement", time.Microsecond, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event, err := audit.Build(1, "", audit.Input{
				Type: "action.state_changed", AggregateType: "ExternalOperation", AggregateID: op.ID,
				PayloadDigest: payload, CausationID: originalConfirmationCause,
			}, at.Add(tc.offset))
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal([]audit.Event{event})
			if err != nil {
				t.Fatal(err)
			}
			// All identity, payload and event digest fields are valid in every
			// case. Only chronology decides whether provenance is acceptable.
			err = validateOriginalConfirmation(op, raw)
			if tc.valid {
				if err != nil {
					t.Fatal("valid original chronology rejected", err)
				}
			} else if !errors.Is(err, ErrRecoveryFactsUnresolved) {
				t.Fatal("future settlement incorrectly qualified", err)
			}
		})
	}
}

func TestRecoveryProofBlocksLegacySafeToRetryWithoutQuiescence(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	runID := setupPostgresActionRun(t, s, "unsafe-retry")
	now := time.Now().UTC()
	req := action.Request{
		ID: "unsafe-retry-op", RunID: runID, ExecutionEpoch: 1, RecoveryEpoch: 0,
		Action: "ci.dispatch", RiskClass: action.ControlledMutation, Capability: "ci",
		ParametersDigest: "sha256:" + strings.Repeat("a", 64), IdempotencyKey: "unsafe-retry-op",
		RequestedBy: "runtime", RequestedAt: now,
	}
	op := action.NewWithRequestDigest(req, "sha256:"+strings.Repeat("b", 64), now)
	if err := s.CreateContext(context.Background(), *op); err != nil {
		t.Fatal(err)
	}
	for i, state := range []action.State{action.Dispatched, action.Unknown, action.Reconciling} {
		if err := op.Transition(state, now.Add(time.Duration(i+1)*time.Second)); err != nil {
			t.Fatal(err)
		}
		if err := s.UpdateContext(context.Background(), *op); err != nil {
			t.Fatal(err)
		}
	}
	seedHistoricalSafeToRetry(t, s, op.ID)
	if _, err := s.BeginRecoveryContext(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRecoveryProof(context.Background(), 1, "urn:engineering-platform:operator:reconciler"); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Fatalf("unqualified SAFE_TO_RETRY incorrectly cleared effectful Action: %v", err)
	}
	status, err := s.ReadOperationalStatus(context.Background())
	if err != nil || status.AuthorityClear || status.Snapshot.SafeToRetryOperations != 1 {
		t.Fatalf("unsafe retry reservation hidden from status: %+v err=%v", status, err)
	}
}
