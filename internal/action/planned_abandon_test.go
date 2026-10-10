package action

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestIdempotentAbandonedActionCannotReplayOrClaimConfirmed(t *testing.T) {
	repo := NewMemoryRepository()
	req := Request{
		ID: "pre-dispatch-reservation", RunID: "run-1",
		ExecutionEpoch: 1, RecoveryEpoch: 0,
		Action: "ci.dispatch", RiskClass: ControlledMutation,
		Capability: "ci.dispatch", IdempotencyKey: "frozen-idempotency",
		RequestedBy: "runtime",
	}
	digest, err := requestDigest(req)
	if err != nil {
		t.Fatal(err)
	}
	op := NewWithRequestDigest(req, digest, time.Now().UTC())
	op.State = AbandonedReconciled // fixture simulates Recovery-only durable readback
	if err := repo.Create(*op); err != nil {
		t.Fatal(err)
	}
	calls := 0
	svc := NewService(AllowCapabilities{"ci.dispatch": true}, fixedGuard{},
		fakeProvider{dispatch: DispatchResult{Outcome: DispatchConfirmed}, dispatchCalls: &calls}, repo)
	receipt, err := svc.Execute(context.Background(), req)
	if !errors.Is(err, ErrDenied) || receipt.OperationID != "" || calls != 0 {
		t.Fatalf("abandoned operation replayed or confirmed: receipt=%#v err=%v calls=%d", receipt, err, calls)
	}
	second := req
	second.ID = "another-action-request"
	if _, err := svc.Execute(context.Background(), second); !errors.Is(err, ErrIdempotencyConflict) || calls != 0 {
		t.Fatalf("abandoned idempotency reservation was reused: %v calls=%d", err, calls)
	}
	if err := op.Transition(Dispatched, time.Now().UTC()); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("abandoned terminal state permitted dispatch: %v", err)
	}
}
