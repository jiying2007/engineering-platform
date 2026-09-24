package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbschema "github.com/jiying2007/engineering-platform/db"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
)

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("database URL is required")
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL config: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return &Store{pool: pool}, nil
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}

func (s *Store) ApplyCoreMigration(ctx context.Context) error {
	if s == nil || s.pool == nil {
		return fmt.Errorf("PostgreSQL store is not configured")
	}
	if _, err := s.pool.Exec(ctx, dbschema.CoreMigration(), pgx.QueryExecModeSimpleProtocol); err != nil {
		return fmt.Errorf("apply core migration: %w", err)
	}
	return nil
}

func (s *Store) GetRecovery() (recovery.Manager, error) {
	if s == nil || s.pool == nil {
		return recovery.Manager{}, fmt.Errorf("PostgreSQL store is not configured")
	}
	const query = "SELECT recovery_epoch,recovery_mode FROM platform_state WHERE singleton_id=true"

	var epoch uint64
	var mode string
	if err := s.pool.QueryRow(context.Background(), query).Scan(&epoch, &mode); err != nil {
		return recovery.Manager{}, fmt.Errorf("read recovery state: %w", err)
	}
	state := recovery.Manager{Epoch: epoch, Mode: recovery.Mode(mode)}
	if state.Mode != recovery.Normal && state.Mode != recovery.RecoveryReconciliation {
		return recovery.Manager{}, fmt.Errorf("read recovery state: unknown mode %q", mode)
	}
	return state, nil
}

func (s *Store) BeginRecovery(expectedEpoch uint64) (recovery.Manager, error) {
	if s == nil || s.pool == nil {
		return recovery.Manager{}, fmt.Errorf("PostgreSQL store is not configured")
	}
	const query = `
UPDATE platform_state
SET recovery_epoch = recovery_epoch + 1,
    recovery_mode = 'RECOVERY_RECONCILIATION',
    updated_at = now()
WHERE singleton_id = true
  AND recovery_epoch = $1
  AND recovery_mode = 'NORMAL'
RETURNING recovery_epoch,recovery_mode`

	var epoch uint64
	var mode string
	err := s.pool.QueryRow(context.Background(), query, expectedEpoch).Scan(&epoch, &mode)
	if errors.Is(err, pgx.ErrNoRows) {
		return recovery.Manager{}, corestore.ErrConflict
	}
	if err != nil {
		return recovery.Manager{}, fmt.Errorf("begin recovery: %w", err)
	}
	return recovery.Manager{Epoch: epoch, Mode: recovery.Mode(mode)}, nil
}

func (s *Store) CompleteRecovery(epoch uint64, reconciled bool) (recovery.Manager, error) {
	if !reconciled {
		return recovery.Manager{}, recovery.ErrReconciliationRequired
	}
	if s == nil || s.pool == nil {
		return recovery.Manager{}, fmt.Errorf("PostgreSQL store is not configured")
	}
	const query = `
UPDATE platform_state
SET recovery_mode = 'NORMAL',
    updated_at = now()
WHERE singleton_id = true
  AND recovery_epoch = $1
  AND recovery_mode = 'RECOVERY_RECONCILIATION'
RETURNING recovery_epoch,recovery_mode`

	var returnedEpoch uint64
	var mode string
	err := s.pool.QueryRow(context.Background(), query, epoch).Scan(&returnedEpoch, &mode)
	if errors.Is(err, pgx.ErrNoRows) {
		return recovery.Manager{}, corestore.ErrConflict
	}
	if err != nil {
		return recovery.Manager{}, fmt.Errorf("complete recovery: %w", err)
	}
	return recovery.Manager{Epoch: returnedEpoch, Mode: recovery.Mode(mode)}, nil
}
