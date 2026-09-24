package action

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrDenied          = errors.New("action denied")
	ErrOperationExists = errors.New("operation already exists")
	ErrOperationAbsent = errors.New("operation not found")
)

type DispatchOutcome string

const (
	DispatchConfirmed DispatchOutcome = "CONFIRMED"
	DispatchUnknown   DispatchOutcome = "UNKNOWN"
)

type ReconcileOutcome string

const (
	ReconcileConfirmed   ReconcileOutcome = "CONFIRMED"
	ReconcileSafeToRetry ReconcileOutcome = "SAFE_TO_RETRY"
	ReconcileManual      ReconcileOutcome = "MANUAL"
)

type DispatchResult struct {
	Outcome       DispatchOutcome
	ExternalRef   string
	ObservedState string
}

type ReconcileResult struct {
	Outcome       ReconcileOutcome
	ExternalRef   string
	ObservedState string
}

type Authorizer interface {
	Authorize(context.Context, Request) error
}

type EpochGuard interface {
	CheckRunEpoch(context.Context, string, uint64) error
}

type Provider interface {
	Dispatch(context.Context, Request) (DispatchResult, error)
	Reconcile(context.Context, Operation) (ReconcileResult, error)
}

type Repository interface {
	Create(Operation) error
	Get(string) (Operation, error)
	Update(Operation) error
}

type Service struct {
	authorizer Authorizer
	guard      EpochGuard
	provider   Provider
	repository Repository
	now        func() time.Time
}

func NewService(authorizer Authorizer, guard EpochGuard, provider Provider, repository Repository) *Service {
	return &Service{
		authorizer: authorizer,
		guard:      guard,
		provider:   provider,
		repository: repository,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Execute(ctx context.Context, req Request) (Receipt, error) {
	if s.authorizer == nil || s.guard == nil || s.provider == nil || s.repository == nil {
		return Receipt{}, fmt.Errorf("action service is not fully configured")
	}
	if err := s.authorizer.Authorize(ctx, req); err != nil {
		return Receipt{}, fmt.Errorf("%w: %v", ErrDenied, err)
	}
	if err := s.guard.CheckRunEpoch(ctx, req.RunID, req.ExecutionEpoch); err != nil {
		return Receipt{}, err
	}

	now := s.now()
	op := New(req.ID, req.RunID, req.Action, req.IdempotencyKey, now)
	if err := s.repository.Create(*op); err != nil {
		return Receipt{}, err
	}
	if err := op.Transition(Dispatched, now); err != nil {
		return Receipt{}, err
	}
	if err := s.repository.Update(*op); err != nil {
		return Receipt{}, err
	}

	result, err := s.provider.Dispatch(ctx, req)
	if err != nil {
		// Once Dispatch has been called, a transport/provider error is ambiguous.
		if transitionErr := op.Transition(Unknown, s.now()); transitionErr != nil {
			return Receipt{}, transitionErr
		}
		_ = s.repository.Update(*op)
		return Receipt{
			ID:          "receipt:" + req.ID,
			RequestID:   req.ID,
			OperationID: op.ID,
			Result:      string(Unknown),
			CreatedAt:   s.now(),
		}, nil
	}

	switch result.Outcome {
	case DispatchConfirmed:
		if err := op.Transition(Confirmed, s.now()); err != nil {
			return Receipt{}, err
		}
	case DispatchUnknown:
		if err := op.Transition(Unknown, s.now()); err != nil {
			return Receipt{}, err
		}
	default:
		return Receipt{}, fmt.Errorf("unsupported dispatch outcome %q", result.Outcome)
	}
	op.ExternalRef = result.ExternalRef
	if err := s.repository.Update(*op); err != nil {
		return Receipt{}, err
	}
	return Receipt{
		ID:            "receipt:" + req.ID,
		RequestID:     req.ID,
		OperationID:   op.ID,
		Result:        string(op.State),
		ExternalRef:   result.ExternalRef,
		ObservedState: result.ObservedState,
		CreatedAt:     s.now(),
	}, nil
}

func (s *Service) Reconcile(ctx context.Context, operationID string) (Receipt, error) {
	op, err := s.repository.Get(operationID)
	if err != nil {
		return Receipt{}, err
	}
	if op.State != Unknown {
		return Receipt{}, fmt.Errorf("operation %s is %s, not UNKNOWN", op.ID, op.State)
	}
	if err := op.Transition(Reconciling, s.now()); err != nil {
		return Receipt{}, err
	}
	if err := s.repository.Update(op); err != nil {
		return Receipt{}, err
	}

	result, err := s.provider.Reconcile(ctx, op)
	if err != nil {
		op.State = Manual
		op.UpdatedAt = s.now()
		_ = s.repository.Update(op)
		return Receipt{
			ID:          "receipt:reconcile:" + op.ID,
			OperationID: op.ID,
			Result:      string(Manual),
			CreatedAt:   s.now(),
		}, nil
	}

	switch result.Outcome {
	case ReconcileConfirmed:
		err = op.Transition(Confirmed, s.now())
	case ReconcileSafeToRetry:
		err = op.Transition(SafeToRetry, s.now())
	case ReconcileManual:
		err = op.Transition(Manual, s.now())
	default:
		return Receipt{}, fmt.Errorf("unsupported reconcile outcome %q", result.Outcome)
	}
	if err != nil {
		return Receipt{}, err
	}
	op.ExternalRef = result.ExternalRef
	if err := s.repository.Update(op); err != nil {
		return Receipt{}, err
	}
	return Receipt{
		ID:            "receipt:reconcile:" + op.ID,
		OperationID:   op.ID,
		Result:        string(op.State),
		ExternalRef:   result.ExternalRef,
		ObservedState: result.ObservedState,
		CreatedAt:     s.now(),
	}, nil
}

type MemoryRepository struct {
	mu    sync.RWMutex
	items map[string]Operation
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{items: make(map[string]Operation)}
}

func (r *MemoryRepository) Create(op Operation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[op.ID]; ok {
		return ErrOperationExists
	}
	r.items[op.ID] = op
	return nil
}

func (r *MemoryRepository) Get(id string) (Operation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	op, ok := r.items[id]
	if !ok {
		return Operation{}, ErrOperationAbsent
	}
	return op, nil
}

func (r *MemoryRepository) Update(op Operation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[op.ID]; !ok {
		return ErrOperationAbsent
	}
	r.items[op.ID] = op
	return nil
}

type AllowCapabilities map[string]bool

func (a AllowCapabilities) Authorize(_ context.Context, req Request) error {
	if !a[req.Capability] {
		return fmt.Errorf("capability %q is not allowed", req.Capability)
	}
	return nil
}
