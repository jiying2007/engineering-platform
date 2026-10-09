package action

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// settlementUpdateRejector simulates a database that accepted an external
// operation's pre-effect state but cannot retain the post-effect settlement.
type settlementUpdateRejector struct {
	*MemoryRepository
	reject State
}

func (r *settlementUpdateRejector) Update(op Operation) error {
	if op.State == r.reject {
		return errors.New("injected durable ledger write failure")
	}
	return r.MemoryRepository.Update(op)
}

func TestAmbiguousDispatchRequiresDurableUnknownBeforeReturningReceipt(t *testing.T) {
	repository := &settlementUpdateRejector{MemoryRepository: NewMemoryRepository(), reject: Unknown}
	calls := 0
	service := NewService(
		AllowCapabilities{"ci.dispatch": true},
		fixedGuard{},
		fakeProvider{dispatchErr: errors.New("connection lost after sending effect"), dispatchCalls: &calls},
		repository,
	)
	request := Request{
		ID: "op-failed-unknown", RunID: "run-1", ExecutionEpoch: 1,
		Action: "ci.dispatch", Capability: "ci.dispatch",
		IdempotencyKey: "frozen-external-operation",
	}
	receipt, err := service.Execute(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "cannot persist UNKNOWN") || receipt.OperationID != "" {
		t.Fatalf("invented settled UNKNOWN despite rejected database write: receipt=%#v err=%v", receipt, err)
	}
	durable, err := repository.Get(request.ID)
	if err != nil || durable.State != Dispatched {
		t.Fatalf("external operation ledger state not preserved: state=%#v err=%v", durable, err)
	}
	// Retrying the same logical action must observe its existing reservation,
	// not send a second non-idempotent external request.
	repeated, err := service.Execute(context.Background(), request)
	if err != nil || repeated.Result == string(Confirmed) || calls != 1 {
		t.Fatalf("ambiguous operation was replayed or confirmed: receipt=%#v err=%v calls=%d", repeated, err, calls)
	}
}

func TestFailedReconciliationRequiresDurableManualBeforeReturningReceipt(t *testing.T) {
	repository := &settlementUpdateRejector{MemoryRepository: NewMemoryRepository(), reject: Manual}
	if err := repository.Create(Operation{
		ID: "op-failed-manual", RunID: "run-1", State: Unknown,
		Action: "ci.dispatch", Capability: "ci.dispatch",
	}); err != nil {
		t.Fatal(err)
	}
	service := NewService(
		AllowCapabilities{"ci.dispatch": true},
		fixedGuard{},
		fakeProvider{reconcileErr: errors.New("upstream readback unavailable")},
		repository,
	)
	receipt, err := service.Reconcile(context.Background(), "op-failed-manual")
	if err == nil || !strings.Contains(err.Error(), "cannot persist MANUAL") || receipt.OperationID != "" {
		t.Fatalf("invented settled MANUAL despite rejected database write: receipt=%#v err=%v", receipt, err)
	}
	durable, err := repository.Get("op-failed-manual")
	if err != nil || durable.State != Reconciling {
		t.Fatalf("reconciliation state not preserved for independent recovery: state=%#v err=%v", durable, err)
	}
}
