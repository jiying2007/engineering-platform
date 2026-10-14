package action

import (
	"context"
	"errors"
	"testing"
	"time"
)

// The ordinary result has no original-dispatch completion or quiescence proof.
func TestActionPositiveObservationCannotConfirmMutation(t *testing.T) {
	for _, risk := range []RiskClass{Observe, ControlledMutation, HighRisk, "", "UNKNOWN_RISK"} {
		t.Run(string(risk), func(t *testing.T) {
			repo := NewMemoryRepository()
			req := Request{ID: "positive-without-origin", RunID: "run", ExecutionEpoch: 1, Action: "operation", Capability: "operation", RiskClass: risk, IdempotencyKey: "reserved"}
			digest, err := RequestDigest(req)
			if err != nil {
				t.Fatal(err)
			}
			original := *NewWithRequestDigest(req, digest, time.Now().UTC())
			original.State = Unknown
			if err := repo.CreateContext(context.Background(), original); err != nil {
				t.Fatal(err)
			}
			calls := 0
			provider := fakeProvider{dispatchCalls: &calls, reconcile: ReconcileResult{Outcome: ReconcileConfirmed, ExternalRef: "observed-only", ObservedState: "currently visible"}}
			svc := NewService(AllowCapabilities{"operation": true}, fixedGuard{}, provider, repo)
			receipt, err := svc.Reconcile(context.Background(), original.ID)
			stored, getErr := repo.GetContext(context.Background(), original.ID)
			if getErr != nil || !stored.SameReservation(original) {
				t.Fatal("original reservation changed", getErr)
			}
			if risk == Observe {
				if err != nil || receipt.Result != string(Confirmed) || stored.State != Confirmed {
					t.Fatalf("read-only reconciliation regressed: %+v %v", receipt, err)
				}
			} else {
				if !errors.Is(err, ErrExternalEffectUnresolved) || receipt.OperationID != "" || stored.State != Manual {
					t.Fatalf("positive-only mutation became terminal authority: receipt=%+v state=%s err=%v", receipt, stored.State, err)
				}
				if stored.ExternalRef != "" || stored.ObservedState != "UNQUALIFIED_CONFIRMATION_VERDICT" {
					t.Fatal("unqualified confirmation fields accepted", stored)
				}
				repeat, repeatedErr := svc.Execute(context.Background(), req)
				if repeatedErr != nil || repeat.Result != string(Manual) || calls != 0 {
					t.Fatal("original idempotency reservation replayed", repeat, repeatedErr)
				}
			}
		})
	}
}

func TestActionMemoryUpdatesKeepNormalTransitionBoundary(t *testing.T) {
	states := []State{Planned, Dispatched, Unknown, Reconciling, Confirmed, Manual, SafeToRetry, AbandonedReconciled, "INVALID"}
	ordinary := map[[2]State]bool{{Planned, Dispatched}: true, {Dispatched, Confirmed}: true, {Dispatched, Unknown}: true, {Unknown, Reconciling}: true, {Reconciling, Manual}: true}
	for _, risk := range []RiskClass{Observe, ControlledMutation, HighRisk, ""} {
		for _, from := range states {
			for _, to := range states {
				t.Run(string(risk)+"/"+string(from)+"/"+string(to), func(t *testing.T) {
					repo := NewMemoryRepository()
					original := Operation{ID: "one", RunID: "run", Action: "once", Capability: "once", IdempotencyKey: "key", ExecutionEpoch: 1, RiskClass: risk, State: from, UpdatedAt: time.Unix(1, 0)}
					if err := repo.CreateContext(context.Background(), original); err != nil {
						t.Fatal(err)
					}
					changed := original
					changed.State = to
					changed.UpdatedAt = time.Unix(2, 0)
					allowed := ordinary[[2]State{from, to}] || risk == Observe && from == Reconciling && to == Confirmed
					err := repo.UpdateContext(context.Background(), changed)
					if (err == nil) != allowed {
						t.Fatalf("normal ledger boundary %s -> %s risk=%s: %v", from, to, risk, err)
					}
					got, err := repo.GetByIdempotencyKeyContext(context.Background(), original.IdempotencyKey)
					if err != nil {
						t.Fatal(err)
					}
					if allowed {
						if got != changed {
							t.Fatal("valid update lost")
						}
					} else if got != original {
						t.Fatal("rejected update changed record")
					}
				})
			}
		}
	}
}

func TestActionUnqualifiedPositiveSettlesManualAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo := &trackingActionContextStore{MemoryRepository: NewMemoryRepository()}
	original := Operation{ID: "cancel-positive", RunID: "run", RiskClass: ControlledMutation, State: Unknown, IdempotencyKey: "once"}
	if err := repo.CreateContext(context.Background(), original); err != nil {
		t.Fatal(err)
	}
	provider := &reconciliationProbeProvider{cancel: cancel, result: ReconcileResult{Outcome: ReconcileConfirmed, ExternalRef: "visible-not-quiescent"}}
	svc := NewService(AllowCapabilities{}, fixedGuard{}, provider, repo)
	receipt, err := svc.Reconcile(ctx, original.ID)
	if !errors.Is(err, ErrExternalEffectUnresolved) || receipt.OperationID != "" {
		t.Fatal("cancelled unqualified verdict was accepted", receipt, err)
	}
	stored, err := repo.GetContext(context.Background(), original.ID)
	if err != nil || stored.State != Manual || len(repo.settledStates) != 1 || repo.settledStates[0] != Manual || provider.reconcileCalls != 1 {
		t.Fatal("independent one-attempt MANUAL settlement was lost", stored, err, repo.settledStates)
	}
}
