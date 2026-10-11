package action

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestUnqualifiedRetryVerdictPersistsManualWithoutReplay(t *testing.T) {
	repository := NewMemoryRepository()
	dispatchCalls := 0
	service := NewService(
		AllowCapabilities{"ci.dispatch": true},
		fixedGuard{},
		fakeProvider{
			dispatchErr:   errors.New("provider response lost after possible effect"),
			dispatchCalls: &dispatchCalls,
			reconcile: ReconcileResult{
				Outcome:       ReconcileSafeToRetry,
				ExternalRef:   "unverified-ref",
				ObservedState: "ABSENT",
			},
		},
		repository,
	)
	req := Request{
		ID: "unsafe-retry", RunID: "run-1", ExecutionEpoch: 1, RecoveryEpoch: 0,
		Action: "ci.dispatch", RiskClass: ControlledMutation, Capability: "ci.dispatch",
		IdempotencyKey: "one-operation", RequestedBy: "runtime",
	}
	if receipt, err := service.Execute(context.Background(), req); err != nil || receipt.Result != string(Unknown) {
		t.Fatalf("initial ambiguous effect was not retained UNKNOWN: %+v err=%v", receipt, err)
	}
	if _, err := service.Reconcile(context.Background(), req.ID); err == nil {
		t.Fatal("unqualified SAFE_TO_RETRY outcome returned a success receipt")
	}
	stored, err := repository.Get(req.ID)
	if err != nil || stored.State != Manual || stored.ObservedState != "UNQUALIFIED_RETRY_VERDICT" ||
		stored.ExternalRef != "" || dispatchCalls != 1 {
		t.Fatalf("unsafe retry observation was not durably fenced: %+v err=%v dispatch=%d", stored, err, dispatchCalls)
	}
	if receipt, err := service.Execute(context.Background(), req); err != nil ||
		receipt.Result != string(Manual) || dispatchCalls != 1 {
		t.Fatalf("duplicate idempotency request replayed external effect: %+v err=%v dispatch=%d", receipt, err, dispatchCalls)
	}
}

func TestHistoricalSafeToRetryCannotBeResubmittedAsNewExternalDispatch(t *testing.T) {
	repository := NewMemoryRepository()
	calls := 0
	service := NewService(
		AllowCapabilities{"ci.dispatch": true},
		fixedGuard{},
		fakeProvider{
			dispatch:      DispatchResult{Outcome: DispatchConfirmed},
			dispatchCalls: &calls,
		},
		repository,
	)
	req := Request{
		ID: "historical-retry", RunID: "run-1", ExecutionEpoch: 1, RecoveryEpoch: 0,
		Action: "ci.dispatch", RiskClass: ControlledMutation, Capability: "ci.dispatch",
		IdempotencyKey: "historical-key", RequestedBy: "runtime",
	}
	digest, err := requestDigest(req)
	if err != nil {
		t.Fatal(err)
	}
	original := NewWithRequestDigest(req, digest, time.Now().UTC())
	original.State = SafeToRetry
	if err := repository.Create(*original); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Execute(context.Background(), req); !errors.Is(err, ErrDenied) || calls != 0 {
		t.Fatalf("legacy SAFE_TO_RETRY allowed replay: err=%v calls=%d", err, calls)
	}
	if stored, err := repository.Get(req.ID); err != nil || stored.State != SafeToRetry {
		t.Fatalf("legacy reservation was rewritten: %+v err=%v", stored, err)
	}
}
