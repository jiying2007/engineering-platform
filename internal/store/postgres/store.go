package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbschema "github.com/jiying2007/engineering-platform/db"
	"github.com/jiying2007/engineering-platform/internal/canonical"
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
	expectedState := recovery.Manager{
		Epoch: expectedEpoch + 1,
		Mode:  recovery.RecoveryReconciliation,
	}
	input, err := auditInput("recovery.started", "PlatformState", "singleton", expectedState)
	if err != nil {
		return recovery.Manager{}, err
	}
	var result recovery.Manager
	_, err = s.Mutate(context.Background(), Mutation{
		Apply: func(ctx context.Context, tx pgx.Tx) error {
			const query = `
UPDATE platform_state
SET recovery_epoch = recovery_epoch + 1,
    recovery_mode = 'RECOVERY_RECONCILIATION',
    updated_at = now()
WHERE singleton_id = true
  AND recovery_epoch = $1
  AND recovery_mode = 'NORMAL'
RETURNING recovery_epoch,recovery_mode`
			var mode string
			scanErr := tx.QueryRow(ctx, query, expectedEpoch).Scan(&result.Epoch, &mode)
			if errors.Is(scanErr, pgx.ErrNoRows) {
				return corestore.ErrConflict
			}
			if scanErr != nil {
				return fmt.Errorf("begin recovery: %w", scanErr)
			}
			result.Mode = recovery.Mode(mode)
			return nil
		},
		Audit: input,
	})
	if err != nil {
		return recovery.Manager{}, err
	}
	return result, nil
}

func (s *Store) CompleteRecovery(epoch uint64, reconciled bool) (recovery.Manager, error) {
	if !reconciled {
		return recovery.Manager{}, recovery.ErrReconciliationRequired
	}
	if s == nil || s.pool == nil {
		return recovery.Manager{}, fmt.Errorf("PostgreSQL store is not configured")
	}
	ctx := context.Background()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return recovery.Manager{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var currentEpoch uint64
	var mode string
	if err := tx.QueryRow(ctx, `SELECT recovery_epoch,recovery_mode FROM platform_state WHERE singleton_id=true FOR UPDATE`).Scan(&currentEpoch, &mode); err != nil {
		return recovery.Manager{}, err
	}
	if currentEpoch != epoch || mode != string(recovery.RecoveryReconciliation) {
		return recovery.Manager{}, corestore.ErrConflict
	}
	var proofRaw []byte
	if err := tx.QueryRow(ctx, `SELECT proof_json FROM recovery_reconciliation_proofs WHERE recovery_epoch=$1`, epoch).Scan(&proofRaw); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return recovery.Manager{}, recovery.ErrReconciliationRequired
		}
		return recovery.Manager{}, err
	}
	var proof recovery.Proof
	if err := json.Unmarshal(proofRaw, &proof); err != nil || proof.Validate() != nil {
		return recovery.Manager{}, recovery.ErrReconciliationRequired
	}
	facts, err := recoveryFacts(ctx, tx)
	if err != nil {
		return recovery.Manager{}, err
	}
	if !facts.Clear() {
		return recovery.Manager{}, recovery.ErrReconciliationRequired
	}
	factsDigest, err := canonical.Digest(facts)
	if err != nil || factsDigest != proof.FactsDigest {
		return recovery.Manager{}, recovery.ErrReconciliationRequired
	}
	result := recovery.Manager{Epoch: epoch, Mode: recovery.Normal}
	input, err := auditInput("recovery.completed", "PlatformState", "singleton", struct {
		State recovery.Manager `json:"state"`
		Proof recovery.Proof   `json:"proof"`
	}{State: result, Proof: proof})
	if err != nil {
		return recovery.Manager{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE platform_state SET recovery_mode='NORMAL',updated_at=clock_timestamp() WHERE singleton_id=true AND recovery_epoch=$1 AND recovery_mode='RECOVERY_RECONCILIATION'`, epoch); err != nil {
		return recovery.Manager{}, err
	}
	if _, err := appendAudit(ctx, tx, input, time.Now().UTC()); err != nil {
		return recovery.Manager{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return recovery.Manager{}, err
	}
	return result, nil
}
