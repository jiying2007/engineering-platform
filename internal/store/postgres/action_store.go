package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jiying2007/engineering-platform/internal/action"
)

var _ action.Repository = (*Store)(nil)

func mapActionWriteError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == "23505" {
			return action.ErrOperationExists
		}
	}
	return err
}

func (s *Store) Create(op action.Operation) error {
	input, err := auditInput("action.planned", "ExternalOperation", op.ID, op)
	if err != nil {
		return err
	}
	_, err = s.Mutate(bg(), Mutation{
		Apply: func(ctx context.Context, tx pgx.Tx) error {
			const q = `
INSERT INTO external_operations (
    operation_id,run_id,execution_epoch,recovery_epoch,action,risk_class,capability,
    idempotency_key,request_digest,state,external_ref,observed_state,updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`
			_, err := tx.Exec(
				ctx, q,
				op.ID, op.RunID, op.ExecutionEpoch, op.RecoveryEpoch,
				op.Action, string(op.RiskClass), op.Capability,
				op.IdempotencyKey, op.RequestDigest, string(op.State),
				nullIfEmpty(op.ExternalRef), nullIfEmpty(op.ObservedState), op.UpdatedAt,
			)
			if err != nil {
				return mapActionWriteError(err)
			}
			return nil
		},
		Audit: input,
	})
	return err
}

func (s *Store) Get(id string) (action.Operation, error) {
	return s.readOperation("WHERE operation_id=$1", id)
}

func (s *Store) GetByIdempotencyKey(key string) (action.Operation, error) {
	return s.readOperation("WHERE idempotency_key=$1", key)
}

func (s *Store) readOperation(where string, arg any) (action.Operation, error) {
	const selectBase = `
SELECT operation_id,run_id,execution_epoch,recovery_epoch,action,risk_class,capability,
       idempotency_key,request_digest,state,COALESCE(external_ref,''),
       COALESCE(observed_state,''),updated_at
FROM external_operations `
	var op action.Operation
	var riskClass, state string
	if err := s.pool.QueryRow(bg(), selectBase+where, arg).Scan(
		&op.ID, &op.RunID, &op.ExecutionEpoch, &op.RecoveryEpoch,
		&op.Action, &riskClass, &op.Capability, &op.IdempotencyKey,
		&op.RequestDigest, &state, &op.ExternalRef, &op.ObservedState,
		&op.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return action.Operation{}, action.ErrOperationAbsent
		}
		return action.Operation{}, fmt.Errorf("read external operation: %w", err)
	}
	op.RiskClass = action.RiskClass(riskClass)
	op.State = action.State(state)
	return op, nil
}

func (s *Store) Update(op action.Operation) error {
	allowedPrevious, err := previousStates(op.State)
	if err != nil {
		return err
	}
	input, err := auditInput("action.state_changed", "ExternalOperation", op.ID, op)
	if err != nil {
		return err
	}
	_, err = s.Mutate(bg(), Mutation{
		Apply: func(ctx context.Context, tx pgx.Tx) error {
			const q = `
UPDATE external_operations
SET state=$1,external_ref=$2,observed_state=$3,updated_at=$4
WHERE operation_id=$5
  AND state = ANY($6::text[])`
			tag, err := tx.Exec(
				ctx, q,
				string(op.State), nullIfEmpty(op.ExternalRef), nullIfEmpty(op.ObservedState),
				op.UpdatedAt, op.ID, allowedPrevious,
			)
			if err != nil {
				return mapActionWriteError(err)
			}
			if tag.RowsAffected() != 1 {
				var exists bool
				if scanErr := tx.QueryRow(
					ctx,
					"SELECT EXISTS(SELECT 1 FROM external_operations WHERE operation_id=$1)",
					op.ID,
				).Scan(&exists); scanErr != nil {
					return scanErr
				}
				if !exists {
					return action.ErrOperationAbsent
				}
				return action.ErrInvalidTransition
			}
			return nil
		},
		Audit: input,
	})
	return err
}

func previousStates(target action.State) ([]string, error) {
	switch target {
	case action.Dispatched:
		return []string{string(action.Planned), string(action.SafeToRetry)}, nil
	case action.Confirmed:
		return []string{string(action.Dispatched), string(action.Reconciling)}, nil
	case action.Unknown:
		return []string{string(action.Dispatched)}, nil
	case action.Reconciling:
		return []string{string(action.Unknown)}, nil
	case action.SafeToRetry, action.Manual:
		return []string{string(action.Reconciling)}, nil
	default:
		return nil, fmt.Errorf("%w: cannot persist transition to %s", action.ErrInvalidTransition, target)
	}
}
