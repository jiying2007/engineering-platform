package action

import (
	"context"
	"strings"
	"testing"
)

func TestInvalidProviderDispatchOutcomePersistsUnknownAndNeverReplays(t *testing.T) {
	repository := NewMemoryRepository()
	calls := 0
	service := NewService(
		AllowCapabilities{"ci.dispatch": true},
		fixedGuard{},
		fakeProvider{
			dispatch: DispatchResult{
				Outcome: "INVALID_PROVIDER_SUCCESS",
				ExternalRef: "untrusted-external-reference",
				ObservedState: "invented-success",
			},
			dispatchCalls: &calls,
		},
		repository,
	)
	request := Request{
		ID: "op-invalid-dispatch", RunID: "run-1", ExecutionEpoch: 1,
		Action: "ci.dispatch", Capability: "ci.dispatch",
		IdempotencyKey: "invalid-dispatch-once",
	}
	receipt, err := service.Execute(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "unsupported provider dispatch outcome") ||
		receipt.OperationID != "" || calls != 1 {
		t.Fatalf("invalid dispatch was acknowledged: receipt=%#v err=%v calls=%d", receipt, err, calls)
	}
	stored, err := repository.Get(request.ID)
	if err != nil || stored.State != Unknown || stored.ExternalRef != "" ||
		stored.ObservedState != "INVALID_PROVIDER_DISPATCH_OUTCOME" {
		t.Fatalf("untrusted dispatch did not persist fail-closed UNKNOWN: state=%#v err=%v", stored, err)
	}
	again, err := service.Execute(context.Background(), request)
	if err != nil || again.Result != string(Unknown) || calls != 1 ||
		again.ExternalRef != "" {
		t.Fatalf("same idempotency key replayed an ambiguous effect: receipt=%#v err=%v calls=%d", again, err, calls)
	}
}

func TestInvalidProviderReconcileOutcomePersistsManual(t *testing.T) {
	repository := NewMemoryRepository()
	if err := repository.Create(Operation{
		ID: "op-invalid-observe", RunID: "run-1", State: Unknown,
		Action: "ci.dispatch", Capability: "ci.dispatch",
		IdempotencyKey: "invalid-observe-once",
	}); err != nil {
		t.Fatal(err)
	}
	service := NewService(
		AllowCapabilities{"ci.dispatch": true},
		fixedGuard{},
		fakeProvider{reconcile: ReconcileResult{
			Outcome: "UNRECOGNIZED_SAFE_TO_RETRY",
			ExternalRef: "untrusted-observation",
			ObservedState: "invented-safe-retry",
		}},
		repository,
	)
	receipt, err := service.Reconcile(context.Background(), "op-invalid-observe")
	if err == nil || !strings.Contains(err.Error(), "unsupported provider reconcile outcome") ||
		receipt.OperationID != "" {
		t.Fatalf("unrecognized reconciliation was acknowledged: receipt=%#v err=%v", receipt, err)
	}
	stored, err := repository.Get("op-invalid-observe")
	if err != nil || stored.State != Manual || stored.ExternalRef != "" ||
		stored.ObservedState != "INVALID_PROVIDER_RECONCILE_OUTCOME" {
		t.Fatalf("unrecognized observation did not persist MANUAL: state=%#v err=%v", stored, err)
	}
	if _, err := service.Reconcile(context.Background(), stored.ID); err == nil {
		t.Fatal("manual reconciliation must not be silently retried as an UNKNOWN effect")
	}
}

func TestInvalidProviderOutcomesCannotClaimUnpersistedSettlement(t *testing.T) {
	t.Run("dispatch", func(t *testing.T) {
		repository := &settlementUpdateRejector{
			MemoryRepository: NewMemoryRepository(), reject: Unknown,
		}
		service := NewService(
			AllowCapabilities{"ci.dispatch": true}, fixedGuard{},
			fakeProvider{dispatch: DispatchResult{Outcome: "BAD"}}, repository,
		)
		receipt, err := service.Execute(context.Background(), Request{
			ID: "op-invalid-unpersisted-dispatch", RunID: "run-1",
			ExecutionEpoch: 1, Action: "ci.dispatch",
			Capability: "ci.dispatch", IdempotencyKey: "invalid-unpersisted-dispatch",
		})
		if err == nil || receipt.OperationID != "" {
			t.Fatalf("unpersisted UNKNOWN was acknowledged: receipt=%#v err=%v", receipt, err)
		}
		stored, err := repository.Get("op-invalid-unpersisted-dispatch")
		if err != nil || stored.State != Dispatched {
			t.Fatalf("expected retained pre-effect reservation: state=%#v err=%v", stored, err)
		}
	})
	t.Run("reconcile", func(t *testing.T) {
		repository := &settlementUpdateRejector{
			MemoryRepository: NewMemoryRepository(), reject: Manual,
		}
		if err := repository.Create(Operation{
			ID: "op-invalid-unpersisted-observe", RunID: "run-1", State: Unknown,
			Action: "ci.dispatch", Capability: "ci.dispatch",
			IdempotencyKey: "invalid-unpersisted-observe",
		}); err != nil {
			t.Fatal(err)
		}
		service := NewService(
			AllowCapabilities{"ci.dispatch": true}, fixedGuard{},
			fakeProvider{reconcile: ReconcileResult{Outcome: "BAD"}}, repository,
		)
		receipt, err := service.Reconcile(context.Background(), "op-invalid-unpersisted-observe")
		if err == nil || receipt.OperationID != "" {
			t.Fatalf("unpersisted MANUAL was acknowledged: receipt=%#v err=%v", receipt, err)
		}
		stored, err := repository.Get("op-invalid-unpersisted-observe")
		if err != nil || stored.State != Reconciling {
			t.Fatalf("expected retained reconciliation reservation: state=%#v err=%v", stored, err)
		}
	})
}
