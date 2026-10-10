package action

import (
	"context"
	"errors"
	"testing"
)

type trackingActionContextStore struct {
	*MemoryRepository
	failedSettlement State
	settledStates     []State
}

func (r *trackingActionContextStore) CreateContext(ctx context.Context, op Operation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.MemoryRepository.Create(op)
}

func (r *trackingActionContextStore) GetContext(ctx context.Context, id string) (Operation, error) {
	if err := ctx.Err(); err != nil {
		return Operation{}, err
	}
	return r.MemoryRepository.Get(id)
}

func (r *trackingActionContextStore) GetByIdempotencyKeyContext(ctx context.Context, key string) (Operation, error) {
	if err := ctx.Err(); err != nil {
		return Operation{}, err
	}
	return r.MemoryRepository.GetByIdempotencyKey(key)
}

func (r *trackingActionContextStore) UpdateContext(ctx context.Context, op Operation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch op.State {
	case Confirmed, Unknown, Manual:
		if _, ok := ctx.Deadline(); !ok {
			return errors.New("post-effect Action settlement is missing a deadline")
		}
		r.settledStates = append(r.settledStates, op.State)
		if r.failedSettlement == op.State {
			return errors.New("injected ambiguous post-effect database commit")
		}
	}
	return r.MemoryRepository.Update(op)
}

type cancelAfterEffectProvider struct {
	cancel context.CancelFunc
	result DispatchResult
	err    error
	calls  *int
}

func (p cancelAfterEffectProvider) Dispatch(_ context.Context, _ Request) (DispatchResult, error) {
	(*p.calls)++
	p.cancel()
	return p.result, p.err
}

func (p cancelAfterEffectProvider) Reconcile(context.Context, Operation) (ReconcileResult, error) {
	return ReconcileResult{}, errors.New("unexpected readback in Dispatch test")
}

func TestActionContextRepositorySettlesRemoteEffectAfterCallerCancel(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result DispatchResult
		err    error
		want   State
	}{
		{
			name: "confirmed",
			result: DispatchResult{
				Outcome: DispatchConfirmed, ExternalRef: "remote-effect",
			},
			want: Confirmed,
		},
		{
			name: "provider-response-lost",
			err:  errors.New("response lost after external effect"),
			want: Unknown,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			repo := &trackingActionContextStore{MemoryRepository: NewMemoryRepository()}
			calls := 0
			provider := cancelAfterEffectProvider{
				cancel: cancel, result: tc.result, err: tc.err, calls: &calls,
			}
			service := NewService(AllowCapabilities{"ci.dispatch": true},
				fixedGuard{}, provider, repo)
			req := Request{
				ID: "effect-" + tc.name,
				RunID: "run-1", ExecutionEpoch: 1,
				Action: "ci.dispatch", RiskClass: ControlledMutation,
				Capability: "ci.dispatch",
				IdempotencyKey: "effect-once-" + tc.name,
			}
			receipt, err := service.Execute(ctx, req)
			if err != nil || receipt.Result != string(tc.want) || calls != 1 {
				t.Fatalf("cancel after remote effect lost ledger settlement: receipt=%#v err=%v calls=%d",
					receipt, err, calls)
			}
			stored, err := repo.Get(req.ID)
			if err != nil || stored.State != tc.want ||
				len(repo.settledStates) != 1 || repo.settledStates[0] != tc.want {
				t.Fatalf("remote effect was not settled under independent bounded context: %#v err=%v", stored, err)
			}
		})
	}
}

func TestActionContextRepositoryNeverFabricatesSettlementAfterAmbiguousCommit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repo := &trackingActionContextStore{
		MemoryRepository: NewMemoryRepository(), failedSettlement: Confirmed,
	}
	calls := 0
	service := NewService(AllowCapabilities{"ci.dispatch": true}, fixedGuard{},
		cancelAfterEffectProvider{
			cancel: cancel,
			result: DispatchResult{Outcome: DispatchConfirmed, ExternalRef: "could-have-completed"},
			calls: &calls,
		}, repo)
	req := Request{
		ID: "ambiguous-commit-after-provider", RunID: "run-1",
		ExecutionEpoch: 1, Action: "ci.dispatch", RiskClass: ControlledMutation,
		Capability: "ci.dispatch", IdempotencyKey: "commit-once",
	}
	receipt, err := service.Execute(ctx, req)
	if err == nil || receipt.OperationID != "" || calls != 1 {
		t.Fatalf("ambiguous COMMIT generated successful receipt: %#v err=%v calls=%d", receipt, err, calls)
	}
	stored, err := repo.Get(req.ID)
	if err != nil || stored.State != Dispatched {
		t.Fatalf("lost durable ambiguous pre-settlement state: %#v err=%v", stored, err)
	}
	previous, err := service.Execute(context.Background(), req)
	if err != nil || previous.Result != string(Dispatched) || calls != 1 {
		t.Fatalf("ambiguous external effect was replayed: %#v err=%v calls=%d", previous, err, calls)
	}
}
