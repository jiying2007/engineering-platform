package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jiying2007/engineering-platform/internal/production"
)

func (s *Store) ReadOperationalStatus(ctx context.Context) (production.OperationalStatus, error) {
	if s == nil || s.pool == nil {
		return production.OperationalStatus{}, fmt.Errorf("PostgreSQL store is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	snapshot := production.Snapshot{Version: production.OperationalStatusVersion}
	const query = `SELECT
 clock_timestamp(),
 p.recovery_epoch,
 p.recovery_mode,
 (SELECT count(*) FROM runs WHERE state IN ('RUNNING','PAUSED')),
 (SELECT count(*) FROM worker_inbox WHERE state='PENDING'),
 (SELECT count(*) FROM worker_inbox WHERE state='LEASED' AND lease_until>clock_timestamp()),
 (SELECT count(*) FROM worker_inbox WHERE state='LEASED' AND lease_until<=clock_timestamp()),
 (SELECT count(*) FROM outbox_events WHERE state='PENDING'),
 (SELECT count(*) FROM outbox_events WHERE state='LEASED'),
 (SELECT count(*) FROM outbox_events WHERE state='DEAD_LETTER'),
 (SELECT count(*) FROM external_operations WHERE state='UNKNOWN'),
 (SELECT count(*) FROM external_operations WHERE state='RECONCILING'),
 (SELECT count(*) FROM external_operations WHERE state='MANUAL'),
 (SELECT count(*) FROM worker_codex_executions WHERE state='UNKNOWN')
FROM platform_state p
WHERE p.singleton_id=true`

	if err := s.pool.QueryRow(ctx, query).Scan(
		&snapshot.CapturedAt,
		&snapshot.RecoveryEpoch,
		&snapshot.RecoveryMode,
		&snapshot.ActiveRuns,
		&snapshot.PendingWorkerIntents,
		&snapshot.ActiveWorkerLeases,
		&snapshot.ExpiredWorkerLeases,
		&snapshot.PendingOutbox,
		&snapshot.LeasedOutbox,
		&snapshot.DeadLetterOutbox,
		&snapshot.UnknownOperations,
		&snapshot.ReconcilingOperations,
		&snapshot.ManualOperations,
		&snapshot.UnknownCodexExecutions,
	); err != nil {
		return production.OperationalStatus{}, fmt.Errorf("read production operational facts: %w", err)
	}
	status, err := production.EvaluateSnapshot(snapshot)
	if err != nil {
		return production.OperationalStatus{}, err
	}
	return status, nil
}
