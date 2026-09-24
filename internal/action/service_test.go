package action

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fixedGuard struct {
	err error
}

func (g fixedGuard) CheckRunEpoch(context.Context, string, uint64) error {
	return g.err
}

type fakeProvider struct {
	dispatch    DispatchResult
	dispatchErr error
	reconcile   ReconcileResult
	reconcileErr error
}

func (p fakeProvider) Dispatch(context.Context, Request) (DispatchResult, error) {
	return p.dispatch, p.dispatchErr
}

func (p fakeProvider) Reconcile(context.Context, Operation) (ReconcileResult, error) {
	return p.reconcile, p.reconcileErr
}

func TestActionRequiresCapability(t *testing.T) {
	svc := NewService(
		AllowCapabilities{"ci.dispatch": false},
		fixedGuard{},
		fakeProvider{},
		NewMemoryRepository(),
	)
	_, err := svc.Execute(context.Background(), Request{
		ID: "op-1", RunID: "run-1", ExecutionEpoch: 1,
		Action: "ci.dispatch", Capability: "ci.dispatch", IdempotencyKey: "idem-1",
	})
	if !errors.Is(err, ErrDenied) {
		t.Fatalf("expected denied action, got %v", err)
	}
}

func TestDispatchErrorBecomesUnknownAndMustReconcile(t *testing.T) {
	repo := NewMemoryRepository()
	svc := NewService(
		AllowCapabilities{"ci.dispatch": true},
		fixedGuard{},
		fakeProvider{
			dispatchErr: errors.New("connection reset after request"),
			reconcile: ReconcileResult{
				Outcome:       ReconcileConfirmed,
				ExternalRef:   "ci-run-42",
				ObservedState: "completed",
			},
		},
		repo,
	)
	svc.now = func() time.Time { return time.Unix(1, 0) }

	req := Request{
		ID: "op-1", RunID: "run-1", ExecutionEpoch: 1,
		Action: "ci.dispatch", Capability: "ci.dispatch",
		IdempotencyKey: "idem-1", RequestedBy: "runtime",
	}
	receipt, err := svc.Execute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Result != string(Unknown) {
		t.Fatalf("expected UNKNOWN, got %s", receipt.Result)
	}

	op, err := repo.Get("op-1")
	if err != nil {
		t.Fatal(err)
	}
	if op.State != Unknown {
		t.Fatalf("expected stored UNKNOWN, got %s", op.State)
	}

	reconciled, err := svc.Reconcile(context.Background(), "op-1")
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.Result != string(Confirmed) {
		t.Fatalf("expected CONFIRMED after reconcile, got %s", reconciled.Result)
	}
}

func TestStaleEpochBlocksDispatchBeforeProvider(t *testing.T) {
	svc := NewService(
		AllowCapabilities{"device.flash": true},
		fixedGuard{err: errors.New("stale execution epoch")},
		fakeProvider{dispatch: DispatchResult{Outcome: DispatchConfirmed}},
		NewMemoryRepository(),
	)
	_, err := svc.Execute(context.Background(), Request{
		ID: "op-1", RunID: "run-1", ExecutionEpoch: 1,
		Action: "device.flash", Capability: "device.flash", IdempotencyKey: "idem",
	})
	if err == nil {
		t.Fatal("expected epoch guard failure")
	}
}
