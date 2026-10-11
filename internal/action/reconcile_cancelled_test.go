package action

import (
	"context"
	"errors"
	"testing"
)

type reconcileCancellationRepository struct {
	*MemoryRepository
	phase   string
	cancel  context.CancelFunc
	gets    int
	updates int
}

func (r *reconcileCancellationRepository) Get(id string) (Operation, error) {
	r.gets++
	value, err := r.MemoryRepository.Get(id)
	if r.phase == "after-read" {
		r.cancel()
	}
	return value, err
}

func (r *reconcileCancellationRepository) Update(op Operation) error {
	r.updates++
	err := r.MemoryRepository.Update(op)
	if err == nil && r.phase == "after-reconciling" && op.State == Reconciling {
		r.cancel()
	}
	return err
}

// Preserve read/post-admission cancellation probes with the required scoped
// repository calls, not the embedded MemoryRepository's default methods.
func (r *reconcileCancellationRepository) GetContext(ctx context.Context, id string) (Operation, error) {
	if err := ctx.Err(); err != nil {
		return Operation{}, err
	}
	return r.Get(id)
}
func (r *reconcileCancellationRepository) UpdateContext(ctx context.Context, op Operation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.Update(op)
}

type reconciliationProbeProvider struct {
	cancel         context.CancelFunc
	result         ReconcileResult
	err            error
	reconcileCalls int
}

func (p *reconciliationProbeProvider) Dispatch(context.Context, Request) (DispatchResult, error) {
	return DispatchResult{}, errors.New("unexpected external dispatch during reconciliation")
}

func (p *reconciliationProbeProvider) Reconcile(context.Context, Operation) (ReconcileResult, error) {
	p.reconcileCalls++
	if p.cancel != nil {
		p.cancel()
	}
	return p.result, p.err
}

func TestCancelledReconciliationCannotStartProviderObservation(t *testing.T) {
	for _, phase := range []string{"before", "after-read", "after-reconciling"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			repo := &reconcileCancellationRepository{
				MemoryRepository: NewMemoryRepository(), phase: phase, cancel: cancel,
			}
			id := "cancel-reconcile-" + phase
			if err := repo.Create(Operation{
				ID: id, RunID: "run-1", State: Unknown,
				IdempotencyKey: "no-replay-" + phase,
			}); err != nil {
				t.Fatal(err)
			}
			provider := &reconciliationProbeProvider{
				result: ReconcileResult{Outcome: ReconcileConfirmed, ExternalRef: "must-not-observe"},
			}
			service := NewService(AllowCapabilities{}, fixedGuard{}, provider, repo)
			if phase == "before" {
				cancel()
			}
			receipt, err := service.Reconcile(ctx, id)
			if !errors.Is(err, context.Canceled) || receipt.OperationID != "" ||
				provider.reconcileCalls != 0 {
				t.Fatalf("cancelled reconciliation started external observation: receipt=%#v err=%v calls=%d",
					receipt, err, provider.reconcileCalls)
			}
			got, err := repo.MemoryRepository.Get(id)
			if err != nil {
				t.Fatal(err)
			}
			want := Unknown
			if phase == "after-reconciling" {
				// The durable RECONCILING state is intentionally unresolved,
				// not falsely CONFIRMED, MANUAL, or safe to retry.
				want = Reconciling
			}
			if got.State != want {
				t.Fatalf("cancelled operation changed authority: got=%s want=%s", got.State, want)
			}
			if phase == "before" && repo.gets != 0 {
				t.Fatalf("pre-cancelled request performed %d database reads", repo.gets)
			}
			if phase == "after-read" && repo.updates != 0 {
				t.Fatalf("cancelled readback performed %d database writes", repo.updates)
			}
			if phase == "after-reconciling" {
				// Repeating the request cannot turn an orphaned reconciliation
				// into a second observation or a success receipt.
				repeated, retryErr := service.Reconcile(context.Background(), id)
				if retryErr == nil || repeated.OperationID != "" ||
					provider.reconcileCalls != 0 {
					t.Fatalf("replayed unresolved observation: receipt=%#v err=%v calls=%d",
						repeated, retryErr, provider.reconcileCalls)
				}
			}
		})
	}
}

func TestReconciliationAfterObservedEffectSettlesDespiteCallerCancellation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result ReconcileResult
		err    error
		want   State
	}{
		{
			name:   "confirmed",
			result: ReconcileResult{Outcome: ReconcileConfirmed, ExternalRef: "observed-remote-pr"},
			want:   Confirmed,
		},
		{
			name: "observation-lost",
			err:  errors.New("readback transport unavailable"),
			want: Manual,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			id := "observed-reconcile-" + tc.name
			repo := &trackingActionContextStore{MemoryRepository: NewMemoryRepository()}
			if err := repo.Create(Operation{
				ID: id, RunID: "run-1", State: Unknown,
				IdempotencyKey: "reconcile-once-" + tc.name,
			}); err != nil {
				t.Fatal(err)
			}
			provider := &reconciliationProbeProvider{
				cancel: cancel, result: tc.result, err: tc.err,
			}
			service := NewService(AllowCapabilities{}, fixedGuard{}, provider, repo)
			receipt, err := service.Reconcile(ctx, id)
			if err != nil || provider.reconcileCalls != 1 ||
				receipt.Result != string(tc.want) {
				t.Fatalf("observed effect lost independent settlement: receipt=%#v err=%v calls=%d",
					receipt, err, provider.reconcileCalls)
			}
			stored, err := repo.Get(id)
			if err != nil || stored.State != tc.want ||
				len(repo.settledStates) != 1 || repo.settledStates[0] != tc.want {
				t.Fatalf("missing bounded durable settlement: stored=%#v err=%v states=%v",
					stored, err, repo.settledStates)
			}
		})
	}
}
