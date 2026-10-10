package action

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

var (
	ErrDenied              = errors.New("action denied")
	ErrOperationExists     = errors.New("operation already exists")
	ErrOperationAbsent     = errors.New("operation not found")
	ErrIdempotencyConflict = errors.New("idempotency key reused for different request")
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

type AuthorityGuard interface {
	CheckRunEpoch(context.Context, string, uint64) error
	CheckRecoveryEpoch(context.Context, uint64, RiskClass) error
}

type Provider interface {
	Dispatch(context.Context, Request) (DispatchResult, error)
	Reconcile(context.Context, Operation) (ReconcileResult, error)
}

type Repository interface {
	Create(Operation) error
	Get(string) (Operation, error)
	GetByIdempotencyKey(string) (Operation, error)
	Update(Operation) error
}

// ContextRepository is a storage capability, not another Action authority.
// Production PostgreSQL implements it so cancellation can interrupt a blocked
// transaction; in-memory test doubles may still implement Repository directly.
// All mutations keep the same single authoritative ledger and audit journal.
type ContextRepository interface {
	CreateContext(context.Context, Operation) error
	GetContext(context.Context, string) (Operation, error)
	GetByIdempotencyKeyContext(context.Context, string) (Operation, error)
	UpdateContext(context.Context, Operation) error
}

type Service struct {
	authorizer Authorizer
	guard      AuthorityGuard
	provider   Provider
	repository Repository
	now        func() time.Time
}

func NewService(authorizer Authorizer, guard AuthorityGuard, provider Provider, repository Repository) *Service {
	return &Service{
		authorizer: authorizer,
		guard:      guard,
		provider:   provider,
		repository: repository,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// Prefer caller-bound database operations when supported. Plain Repository
// remains the deliberately bounded test-double interface. Production Core
// injects the PostgreSQL Store, whose contextual methods use the same
// transaction/operation identifiers and cannot bypass authorization.
func (s *Service) createOperation(ctx context.Context, op Operation) error {
	if store, ok := s.repository.(ContextRepository); ok {
		return store.CreateContext(ctx, op)
	}
	return s.repository.Create(op)
}
func (s *Service) getByKey(ctx context.Context, key string) (Operation, error) {
	if store, ok := s.repository.(ContextRepository); ok {
		return store.GetByIdempotencyKeyContext(ctx, key)
	}
	return s.repository.GetByIdempotencyKey(key)
}
func (s *Service) getOperation(ctx context.Context, id string) (Operation, error) {
	if store, ok := s.repository.(ContextRepository); ok {
		return store.GetContext(ctx, id)
	}
	return s.repository.Get(id)
}
func (s *Service) updateOperation(ctx context.Context, op Operation) error {
	if store, ok := s.repository.(ContextRepository); ok {
		return store.UpdateContext(ctx, op)
	}
	return s.repository.Update(op)
}

// Once a remote effect or a provider observation may have happened, the
// database settlement must not be abandoned just because the HTTP client
// disconnected. Attempt one independently bounded ledger write, never a
// second model/publication call. Any failed or ambiguous COMMIT returns an
// error and requires exact persisted readback before retry.
func (s *Service) settleExternalEffect(ctx context.Context, op Operation) error {
	settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return s.updateOperation(settleCtx, op)
}

type requestIdentity struct {
	ID               string    `json:"action_request_id"`
	RunID            string    `json:"run_id"`
	ExecutionEpoch   uint64    `json:"execution_epoch"`
	RecoveryEpoch    uint64    `json:"recovery_epoch"`
	Action           string    `json:"action"`
	RiskClass        RiskClass `json:"risk_class"`
	Capability       string    `json:"capability"`
	ParametersDigest string    `json:"parameters_digest"`
	RequestedBy      string    `json:"requested_by"`
}

func requestDigest(req Request) (string, error) {
	return canonical.Digest(requestIdentity{
		ID:               req.ID,
		RunID:            req.RunID,
		ExecutionEpoch:   req.ExecutionEpoch,
		RecoveryEpoch:    req.RecoveryEpoch,
		Action:           req.Action,
		RiskClass:        req.RiskClass,
		Capability:       req.Capability,
		ParametersDigest: req.ParametersDigest,
		RequestedBy:      req.RequestedBy,
	})
}

func (s *Service) Execute(ctx context.Context, req Request) (Receipt, error) {
	if s.authorizer == nil || s.guard == nil || s.provider == nil || s.repository == nil {
		return Receipt{}, fmt.Errorf("action service is not fully configured")
	}
	if req.IdempotencyKey == "" {
		return Receipt{}, fmt.Errorf("idempotency key is required")
	}
	// A request cancelled before reservation must never reach a model, CI,
	// Git/PR or device provider. A database-side timeout alone cannot infer
	// the caller's intent after a lock wait.
	if err := ctx.Err(); err != nil {
		return Receipt{}, err
	}
	digest, err := requestDigest(req)
	if err != nil {
		return Receipt{}, err
	}
	if existing, getErr := s.getByKey(ctx, req.IdempotencyKey); getErr == nil {
		if existing.RequestDigest != digest {
			return Receipt{}, ErrIdempotencyConflict
		}
		if existing.State == AbandonedReconciled {
			return Receipt{}, fmt.Errorf("%w: abandoned external reservation has no replay authority", ErrDenied)
		}
		return receiptFromOperation(req.ID, existing, s.now()), nil
	} else if !errors.Is(getErr, ErrOperationAbsent) {
		return Receipt{}, getErr
	}
	// The idempotency lookup is a database call and can outlive cancellation
	// until its bounded SQL deadline. Do not turn an absent lookup into new work.
	if err := ctx.Err(); err != nil {
		return Receipt{}, err
	}

	if err := s.authorizer.Authorize(ctx, req); err != nil {
		return Receipt{}, fmt.Errorf("%w: %v", ErrDenied, err)
	}
	if err := ctx.Err(); err != nil {
		return Receipt{}, err
	}
	if err := s.guard.CheckRunEpoch(ctx, req.RunID, req.ExecutionEpoch); err != nil {
		return Receipt{}, err
	}
	if err := ctx.Err(); err != nil {
		return Receipt{}, err
	}
	if err := s.guard.CheckRecoveryEpoch(ctx, req.RecoveryEpoch, req.RiskClass); err != nil {
		return Receipt{}, err
	}
	// Authorization and guard implementations can block. Recheck the caller
	// after each logical pre-dispatch phase; the database transaction below
	// independently fences current Core authority.
	if err := ctx.Err(); err != nil {
		return Receipt{}, err
	}

	now := s.now()
	op := NewWithRequestDigest(req, digest, now)
	if err := s.createOperation(ctx, *op); err != nil {
		if errors.Is(err, ErrOperationExists) {
			existing, getErr := s.getByKey(ctx, req.IdempotencyKey)
			if getErr == nil && existing.RequestDigest == digest {
				if existing.State == AbandonedReconciled {
					return Receipt{}, fmt.Errorf("%w: abandoned external reservation has no replay authority", ErrDenied)
				}
				return receiptFromOperation(req.ID, existing, s.now()), nil
			}
		}
		return Receipt{}, err
	}
	// Create may have committed PLANNED before its caller was cancelled.
	// Stop without dispatching: the reserved idempotency key remains durable
	// and the exact PLANNED row can be handled by Recovery's no-replay path.
	if err := ctx.Err(); err != nil {
		return Receipt{}, err
	}
	if err := op.Transition(Dispatched, now); err != nil {
		return Receipt{}, err
	}
	if err := s.updateOperation(ctx, *op); err != nil {
		return Receipt{}, err
	}
	// No external provider call may start after cancellation is observed.
	// A committed DISPATCHED row remains ambiguous (and never replayable)
	// even if the caller was cancelled immediately before the remote call.
	if err := ctx.Err(); err != nil {
		return Receipt{}, err
	}

	result, err := s.provider.Dispatch(ctx, req)
	if err != nil {
		if transitionErr := op.Transition(Unknown, s.now()); transitionErr != nil {
			return Receipt{}, transitionErr
		}
		if updateErr := s.settleExternalEffect(ctx, *op); updateErr != nil {
			// The external effect is ambiguous; an in-memory UNKNOWN
			// receipt must not pretend the authoritative ledger settled.
			return Receipt{}, fmt.Errorf("cannot persist UNKNOWN external operation: %w", updateErr)
		}
		return receiptFromOperation(req.ID, *op, s.now()), nil
	}

	invalidDispatchOutcome := false
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
		// The Provider may have completed the external effect even when its
		// response is malformed. Persist UNKNOWN before reporting an error;
		// never leave DISPATCHED as the last known ledger fact or accept
		// an untrusted external reference from this response.
		invalidDispatchOutcome = true
		if err := op.Transition(Unknown, s.now()); err != nil {
			return Receipt{}, err
		}
		result.ExternalRef = ""
		result.ObservedState = "INVALID_PROVIDER_DISPATCH_OUTCOME"
	}
	op.ExternalRef = result.ExternalRef
	op.ObservedState = result.ObservedState
	if err := s.settleExternalEffect(ctx, *op); err != nil {
		return Receipt{}, err
	}
	if invalidDispatchOutcome {
		return Receipt{}, fmt.Errorf("unsupported provider dispatch outcome; durable reconciliation required")
	}
	return receiptFromOperation(req.ID, *op, s.now()), nil
}

func (s *Service) Get(operationID string) (Operation, error) {
	if s == nil || s.repository == nil {
		return Operation{}, fmt.Errorf("action service repository is not configured")
	}
	return s.repository.Get(operationID)
}

func (s *Service) Reconcile(ctx context.Context, operationID string) (Receipt, error) {
	op, err := s.getOperation(ctx, operationID)
	if err != nil {
		return Receipt{}, err
	}
	if op.State != Unknown {
		return Receipt{}, fmt.Errorf("operation %s is %s, not UNKNOWN", op.ID, op.State)
	}
	if err := op.Transition(Reconciling, s.now()); err != nil {
		return Receipt{}, err
	}
	if err := s.updateOperation(ctx, op); err != nil {
		return Receipt{}, err
	}

	result, err := s.provider.Reconcile(ctx, op)
	if err != nil {
		op.State = Manual
		op.UpdatedAt = s.now()
		if updateErr := s.settleExternalEffect(ctx, op); updateErr != nil {
			// A reconciliation failure followed by a ledger write failure
			// must not be reported as durable MANUAL settlement.
			return Receipt{}, fmt.Errorf("cannot persist MANUAL external operation: %w", updateErr)
		}
		return receiptFromOperation("", op, s.now()), nil
	}

	invalidReconcileOutcome := false
	switch result.Outcome {
	case ReconcileConfirmed:
		err = op.Transition(Confirmed, s.now())
	case ReconcileSafeToRetry:
		err = op.Transition(SafeToRetry, s.now())
	case ReconcileManual:
		err = op.Transition(Manual, s.now())
	default:
		// An unknown observation cannot settle an ambiguous external effect.
		// Move to MANUAL rather than leaving a terminal-looking RECONCILING
		// row which the normal UNKNOWN entrypoint cannot safely resume.
		invalidReconcileOutcome = true
		err = op.Transition(Manual, s.now())
		result.ExternalRef = ""
		result.ObservedState = "INVALID_PROVIDER_RECONCILE_OUTCOME"
	}
	if err != nil {
		return Receipt{}, err
	}
	op.ExternalRef = result.ExternalRef
	op.ObservedState = result.ObservedState
	if err := s.settleExternalEffect(ctx, op); err != nil {
		return Receipt{}, err
	}
	if invalidReconcileOutcome {
		return Receipt{}, fmt.Errorf("unsupported provider reconcile outcome; manual reconciliation required")
	}
	return receiptFromOperation("", op, s.now()), nil
}

func receiptFromOperation(requestID string, op Operation, now time.Time) Receipt {
	if requestID == "" {
		requestID = op.ID
	}
	return Receipt{
		ID:            "receipt:" + op.ID + ":" + string(op.State),
		RequestID:     requestID,
		OperationID:   op.ID,
		Result:        string(op.State),
		ExternalRef:   op.ExternalRef,
		ObservedState: op.ObservedState,
		CreatedAt:     now,
	}
}

type MemoryRepository struct {
	mu          sync.RWMutex
	items       map[string]Operation
	idempotency map[string]string
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		items:       make(map[string]Operation),
		idempotency: make(map[string]string),
	}
}

func (r *MemoryRepository) Create(op Operation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[op.ID]; ok {
		return ErrOperationExists
	}
	if _, ok := r.idempotency[op.IdempotencyKey]; ok {
		return ErrOperationExists
	}
	r.items[op.ID] = op
	r.idempotency[op.IdempotencyKey] = op.ID
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

func (r *MemoryRepository) GetByIdempotencyKey(key string) (Operation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.idempotency[key]
	if !ok {
		return Operation{}, ErrOperationAbsent
	}
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
