package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/gateway"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
)

const plannedReconciler = "urn:engineering-platform:operator:reconciler"

func actionPlannedRecoveryFixture(t *testing.T) (*Store, *action.Operation, recovery.ActionPlannedAbandonRequest) {
	t.Helper()
	s := newIsolatedIntegrationStore(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	runID := setupPostgresActionRun(t, s, suffix)
	req := action.Request{
		ID: "action-planned-" + suffix, RunID: runID, ExecutionEpoch: 1,
		RecoveryEpoch: 0, Action: "ci.dispatch",
		RiskClass: action.ControlledMutation, Capability: "ci.dispatch",
		IdempotencyKey: "action-planned-once-" + suffix, RequestedBy: "runtime",
	}
	original := canonical.BytesDigest([]byte("frozen-request:" + suffix))
	op := action.NewWithRequestDigest(req, original, time.Now().UTC())
	abandon := recovery.ActionPlannedAbandonRequest{
		Version: 1, OperationID: op.ID, RunID: op.RunID,
		ExecutionEpoch: op.ExecutionEpoch,
		OriginalRecoveryEpoch: op.RecoveryEpoch, RecoveryEpoch: 1,
		IdempotencyKey: op.IdempotencyKey,
		OriginalRequestDigest: op.RequestDigest,
		ObservationDigest: canonical.BytesDigest([]byte("independently-retained-operator-observation:" + suffix)),
		Disposition: recovery.AbandonNoReplay,
	}
	return s, op, abandon
}

func TestPlannedActionRecoveryNoEffectNoReplayAndIndependentProof(t *testing.T) {
	s, op, req := actionPlannedRecoveryFixture(t)
	ctx := context.Background()
	if err := s.Create(*op); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReconcilePlannedActionAbandoned(ctx, plannedReconciler, req); !errors.Is(err, recovery.ErrStaleEpoch) {
		t.Fatalf("normal-mode PLANNED reservation was abandoned outside Recovery: %v", err)
	}
	if _, err := s.BeginRecovery(0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRecoveryProof(ctx, 1, plannedReconciler); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Fatalf("unresolved PLANNED reservation did not block proof: %v", err)
	}
	receipt, err := s.ReconcilePlannedActionAbandoned(ctx, plannedReconciler, req)
	if err != nil || receipt.Validate() != nil || receipt.PreviousState != "PLANNED" ||
		receipt.ReplayAuthorized || receipt.ExecutionAuthorized || receipt.EffectConfirmed {
		t.Fatalf("invalid planned-only disposition: %#v err=%v", receipt, err)
	}
	stored, err := s.Get(op.ID)
	if err != nil || stored.State != action.AbandonedReconciled ||
		stored.RequestDigest != op.RequestDigest || stored.IdempotencyKey != op.IdempotencyKey ||
		stored.ExternalRef != "" || stored.ObservedState != "" {
		t.Fatalf("lost exact original non-replayable reservation: %#v err=%v", stored, err)
	}
	readback, err := s.GetPlannedActionAbandonReceipt(ctx, op.ID)
	if err != nil || readback.RequestDigest != receipt.RequestDigest ||
		!readback.CreatedAt.Equal(receipt.CreatedAt) || readback.Reconciler != plannedReconciler {
		t.Fatalf("independent stored receipt readback failed: %#v err=%v", readback, err)
	}
	repeat, err := s.ReconcilePlannedActionAbandoned(ctx, plannedReconciler, req)
	if err != nil || repeat.RequestDigest != receipt.RequestDigest || !repeat.CreatedAt.Equal(receipt.CreatedAt) {
		t.Fatalf("exact duplicate did not read back immutable receipt: %#v err=%v", repeat, err)
	}
	bad := req
	bad.ObservationDigest = canonical.BytesDigest([]byte("conflicting-readback"))
	if _, err := s.ReconcilePlannedActionAbandoned(ctx, plannedReconciler, bad); !errors.Is(err, corestore.ErrConflict) {
		t.Fatalf("conflicting operator observation accepted: %v", err)
	}
	if _, err := s.ReconcilePlannedActionAbandoned(ctx, "urn:engineering-platform:operator:second", req); !errors.Is(err, corestore.ErrConflict) {
		t.Fatalf("other reconciler replayed original disposition: %v", err)
	}
	// Do not allow a generic state transition or an Action Service call to
	// re-enter this terminal operation's external dispatch.
	provider := &postgresActionProvider{dispatch: action.DispatchResult{Outcome: action.DispatchConfirmed}}
	authority := gateway.NewAuthority(s)
	service := action.NewService(authority, authority, provider, s)
	request := action.Request{
		ID: op.ID, RunID: op.RunID, ExecutionEpoch: op.ExecutionEpoch,
		RecoveryEpoch: op.RecoveryEpoch, Action: op.Action,
		RiskClass: op.RiskClass, Capability: op.Capability,
		IdempotencyKey: op.IdempotencyKey,
	}
	// Digest of this request differs from our fixture's manually frozen
	// digest, so this path must first reject identity reuse as a conflict.
	if _, err := service.Execute(ctx, request); !errors.Is(err, action.ErrIdempotencyConflict) || provider.dispatchCalls != 0 {
		t.Fatalf("different request reused old operation: %v calls=%d", err, provider.dispatchCalls)
	}
	if err := s.Update(action.Operation{ID: op.ID, State: action.Dispatched}); err == nil {
		t.Fatal("ordinary repository update re-entered abandoned Action")
	}
	proof, err := s.CreateRecoveryProof(ctx, 1, plannedReconciler)
	if err != nil || !proof.Facts.Clear() {
		t.Fatalf("planned no-replay disposition failed to clear exact Recovery proof: %#v err=%v", proof, err)
	}
	if err := s.AuthorizeCompletion(ctx, plannedReconciler, 1); err == nil {
		t.Fatal("same Recovery reconciler authorized completion")
	}
	if err := s.AuthorizeCompletion(ctx, "urn:engineering-platform:operator:completer", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteRecovery(1, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPlannedActionAbandonReceipt(ctx, op.ID); err != nil {
		t.Fatal("readback lost after Recovery completion", err)
	}
	if _, err := s.ReconcilePlannedActionAbandoned(ctx, plannedReconciler, req); !errors.Is(err, recovery.ErrReconciliationRequired) {
		t.Fatalf("repeated Recovery mutation admitted after completion: %v", err)
	}
	var auditCount int
	if err := s.pool.QueryRow(ctx,
		"SELECT count(*) FROM audit_events WHERE event_type='recovery.action.planned.abandoned' AND aggregate_id=$1",
		op.ID,
	).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("no exact-once audit for abandoned reservation: count=%d err=%v", auditCount, err)
	}
}

func TestPlannedRecoveryRefusesPotentiallyEffectfulActionStates(t *testing.T) {
	for _, state := range []action.State{action.Dispatched, action.Unknown, action.Reconciling, action.Manual, action.Confirmed} {
		t.Run(string(state), func(t *testing.T) {
			s, op, req := actionPlannedRecoveryFixture(t)
			ctx := context.Background()
			if err := s.Create(*op); err != nil {
				t.Fatal(err)
			}
			if err := op.Transition(action.Dispatched, time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			if err := s.Update(*op); err != nil {
				t.Fatal(err)
			}
			if state != action.Dispatched {
				var sequence []action.State
				if state == action.Confirmed {
					sequence = []action.State{action.Confirmed}
				} else {
					sequence = []action.State{action.Unknown}
					if state == action.Reconciling || state == action.Manual {
						sequence = append(sequence, action.Reconciling)
						if state == action.Manual {
							sequence = append(sequence, action.Manual)
						}
					}
				}
				for _, next := range sequence {
					if err := op.Transition(next, time.Now().UTC()); err != nil {
						t.Fatal(err)
					}
					if err := s.Update(*op); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := s.BeginRecovery(0); err != nil {
				t.Fatal(err)
			}
			if _, err := s.ReconcilePlannedActionAbandoned(ctx, plannedReconciler, req); !errors.Is(err, corestore.ErrConflict) {
				t.Fatalf("potentially effectful state %s was abandoned as PLANNED: %v", state, err)
			}
			original, err := s.Get(op.ID)
			if err != nil || original.State != state {
				t.Fatalf("Action state silently changed: %#v err=%v", original, err)
			}
			if _, err := s.CreateRecoveryProof(ctx, 1, plannedReconciler); state != action.Confirmed && !errors.Is(err, ErrRecoveryFactsUnresolved) {
				t.Fatalf("unresolved %s incorrectly passed proof: %v", state, err)
			}
		})
	}
}

func TestPlannedActionRecoveryCannotPersistWithoutAudit(t *testing.T) {
	s, op, req := actionPlannedRecoveryFixture(t)
	ctx := context.Background()
	if err := s.Create(*op); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginRecovery(0); err != nil {
		t.Fatal(err)
	}
	// Fault injection is confined to this test's own isolated PostgreSQL schema.
	if _, err := s.pool.Exec(ctx, "DELETE FROM audit_journal_state"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReconcilePlannedActionAbandoned(ctx, plannedReconciler, req); err == nil {
		t.Fatal("lost audit head still allowed terminal Action settlement")
	}
	stored, err := s.Get(op.ID)
	if err != nil || stored.State != action.Planned {
		t.Fatalf("Action settlement survived failed atomic audit: %#v %v", stored, err)
	}
	if _, err := s.GetPlannedActionAbandonReceipt(ctx, op.ID); !errors.Is(err, corestore.ErrNotFound) {
		t.Fatalf("invented receipt after rollback: %v", err)
	}
}

func TestPlannedActionRecoveryRequestMustMatchFrozenReservation(t *testing.T) {
	s, op, req := actionPlannedRecoveryFixture(t)
	if err := s.Create(*op); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginRecovery(0); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*recovery.ActionPlannedAbandonRequest){
		func(r *recovery.ActionPlannedAbandonRequest) { r.OperationID = "missing" },
		func(r *recovery.ActionPlannedAbandonRequest) { r.RunID = "wrong-run" },
		func(r *recovery.ActionPlannedAbandonRequest) { r.ExecutionEpoch++ },
		func(r *recovery.ActionPlannedAbandonRequest) { r.IdempotencyKey = "different" },
		func(r *recovery.ActionPlannedAbandonRequest) { r.OriginalRequestDigest = canonical.BytesDigest([]byte("different")) },
	} {
		bad := req
		mutate(&bad)
		if _, err := s.ReconcilePlannedActionAbandoned(context.Background(), plannedReconciler, bad); err == nil {
			t.Fatalf("changed frozen Action identity accepted: %#v", bad)
		}
	}
	original, err := s.Get(op.ID)
	if err != nil || original.State != action.Planned {
		t.Fatalf("changed request mutated old reservation: %#v err=%v", original, err)
	}
}

func TestPlannedActionRecoveryHonorsCancelledCaller(t *testing.T) {
	s, op, req := actionPlannedRecoveryFixture(t)
	if err := s.Create(*op); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginRecovery(0); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.ReconcilePlannedActionAbandoned(ctx, plannedReconciler, req); err == nil {
		t.Fatal("cancelled Action reconciliation returned a success receipt")
	}
	stored, err := s.Get(op.ID)
	if err != nil || stored.State != action.Planned {
		t.Fatalf("cancelled recovery changed Action: %#v %v", stored, err)
	}
}
