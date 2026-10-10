package action

import (
	"context"
	"errors"
	"testing"
)

type cancelAtActionRepository struct {
	*MemoryRepository
	phase  string
	cancel context.CancelFunc
}

func (r *cancelAtActionRepository) GetByIdempotencyKey(key string) (Operation, error) {
	result, err := r.MemoryRepository.GetByIdempotencyKey(key)
	if r.phase == "lookup" {
		r.cancel()
	}
	return result, err
}

func (r *cancelAtActionRepository) Create(op Operation) error {
	err := r.MemoryRepository.Create(op)
	if err == nil && r.phase == "created" {
		r.cancel()
	}
	return err
}

func (r *cancelAtActionRepository) Update(op Operation) error {
	err := r.MemoryRepository.Update(op)
	if err == nil && r.phase == "dispatched" && op.State == Dispatched {
		r.cancel()
	}
	return err
}

type cancelAtAuthorization struct {
	cancel context.CancelFunc
}

func (a cancelAtAuthorization) Authorize(context.Context, Request) error {
	a.cancel()
	return nil
}

type cancelAtRunGuard struct {
	cancel context.CancelFunc
}

func (g cancelAtRunGuard) CheckRunEpoch(context.Context, string, uint64) error {
	g.cancel()
	return nil
}
func (g cancelAtRunGuard) CheckRecoveryEpoch(context.Context, uint64, RiskClass) error {
	return nil
}

func TestActionCancelledBeforeExternalDispatchPreservesDurableNoReplayBoundary(t *testing.T) {
	for _, phase := range []string{
		"before", "lookup", "authorization", "run-guard", "created", "dispatched",
	} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			repo := &cancelAtActionRepository{
				MemoryRepository: NewMemoryRepository(), phase: phase, cancel: cancel,
			}
			if phase == "before" {
				cancel()
			}
			var authorizer Authorizer = AllowCapabilities{"ci.dispatch": true}
			var guard AuthorityGuard = fixedGuard{}
			if phase == "authorization" {
				authorizer = cancelAtAuthorization{cancel: cancel}
			}
			if phase == "run-guard" {
				guard = cancelAtRunGuard{cancel: cancel}
			}
			dispatchCalls := 0
			svc := NewService(authorizer, guard, fakeProvider{
				dispatch:      DispatchResult{Outcome: DispatchConfirmed, ExternalRef: "fake-remote-effect"},
				dispatchCalls: &dispatchCalls,
			}, repo)
			req := Request{
				ID:               "action-cancelled-" + phase,
				RunID:            "run-1",
				ExecutionEpoch:   1,
				RecoveryEpoch:    0,
				Action:           "ci.dispatch",
				RiskClass:        ControlledMutation,
				Capability:       "ci.dispatch",
				ParametersDigest: "sha256:frozen-input",
				IdempotencyKey:   "once-" + phase,
				RequestedBy:      "runtime",
			}
			receipt, err := svc.Execute(ctx, req)
			if !errors.Is(err, context.Canceled) || dispatchCalls != 0 ||
				receipt.OperationID != "" || receipt.Result != "" {
				t.Fatalf("cancellation admitted a provider effect: receipt=%#v err=%v calls=%d", receipt, err, dispatchCalls)
			}
			stored, readErr := repo.Get(req.ID)
			switch phase {
			case "created":
				if readErr != nil || stored.State != Planned {
					t.Fatalf("post-reservation cancellation did not retain a non-replayable PLANNED row: %#v err=%v", stored, readErr)
				}
			case "dispatched":
				if readErr != nil || stored.State != Dispatched {
					t.Fatalf("post-dispatch cancellation did not retain its ambiguous DISPATCHED row: %#v err=%v", stored, readErr)
				}
			default:
				if !errors.Is(readErr, ErrOperationAbsent) {
					t.Fatalf("cancelled request unexpectedly reserved an Action: %#v err=%v", stored, readErr)
				}
			}
			if phase == "created" || phase == "dispatched" {
				// An independent reissued request may only read the old
				// reservation; it may never skip PLANNED/DISPATCHED and
				// submit another external effect.
				snapshot, err := svc.Execute(context.Background(), req)
				if err != nil || snapshot.Result != string(stored.State) ||
					dispatchCalls != 0 {
					t.Fatalf("idempotent retry after cancellation dispatched or fabricated completion: %#v err=%v calls=%d", snapshot, err, dispatchCalls)
				}
			}
		})
	}
}

func TestActionCancellationDoesNotRedefineConfirmedRemoteEffects(t *testing.T) {
	repo := NewMemoryRepository()
	calls := 0
	svc := NewService(AllowCapabilities{"ci.dispatch": true}, fixedGuard{},
		fakeProvider{
			dispatch:      DispatchResult{Outcome: DispatchConfirmed, ExternalRef: "actual-receipt"},
			dispatchCalls: &calls,
		}, repo)
	req := Request{
		ID: "existing-success", RunID: "run-1", ExecutionEpoch: 1,
		Action: "ci.dispatch", RiskClass: ControlledMutation,
		Capability: "ci.dispatch", IdempotencyKey: "existing-success",
	}
	receipt, err := svc.Execute(context.Background(), req)
	if err != nil || receipt.Result != string(Confirmed) || calls != 1 {
		t.Fatalf("valid confirmed action regressed: receipt=%#v err=%v calls=%d", receipt, err, calls)
	}
}
