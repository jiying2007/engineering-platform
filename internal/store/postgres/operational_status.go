package postgres

import (
	"context"
	"encoding/json"
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
 statement_timestamp(),
 p.recovery_epoch,
 p.recovery_mode,
 (SELECT count(*) FROM runs WHERE state IN ('RUNNING','PAUSED')),
 (SELECT count(*) FROM worker_inbox WHERE state='PENDING'),
 (SELECT count(*) FROM worker_inbox WHERE state='LEASED' AND lease_until>statement_timestamp()),
 (SELECT count(*) FROM worker_inbox WHERE state='LEASED' AND lease_until<=statement_timestamp()),
 (SELECT count(*) FROM outbox_events WHERE state='PENDING'),
 (SELECT count(*) FROM outbox_events WHERE state='LEASED'),
 (SELECT count(*) FROM outbox_events WHERE state='DEAD_LETTER'),
 (SELECT count(*) FROM external_operations WHERE state='UNKNOWN'),
 (SELECT count(*) FROM external_operations WHERE state='RECONCILING'),
 (SELECT count(*) FROM external_operations WHERE state='MANUAL'),
 (SELECT count(*) FROM worker_codex_executions WHERE state='UNKNOWN'),
 (SELECT min(received_at) FROM worker_inbox WHERE state='PENDING'),
 (SELECT min(created_at) FROM outbox_events WHERE state='PENDING'),
 (SELECT max(updated_at) FROM worker_inbox WHERE state='INPUT_VALIDATED'),
 (SELECT max(dispatched_at) FROM outbox_events WHERE state='DISPATCHED'),
 COALESCE((
  SELECT jsonb_agg(jsonb_build_object(
   'worker_profile',q.worker_profile,
   'known_identities',q.known_identities,
   'latest_poll_at',q.latest_poll_at
  ) ORDER BY q.worker_profile)
  FROM (
   SELECT attributes_json->>'worker_profile' AS worker_profile,
          count(*) AS known_identities,
          max(last_seen_at) AS latest_poll_at
   FROM workers
   WHERE status='ONLINE' AND jsonb_typeof(attributes_json)='object'
     AND jsonb_typeof(attributes_json->'worker_profile')='string'
   GROUP BY attributes_json->>'worker_profile'
  ) q
 ), '[]'::jsonb)
FROM platform_state p
WHERE p.singleton_id=true`

	var workerPollsRaw []byte
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
		&snapshot.OldestPendingWorkerAt,
		&snapshot.OldestPendingOutboxAt,
		&snapshot.LastWorkerAdmissionAt,
		&snapshot.LastOutboxDispatchAt,
		&workerPollsRaw,
	); err != nil {
		return production.OperationalStatus{}, fmt.Errorf("read production operational facts: %w", err)
	}
	if err := json.Unmarshal(workerPollsRaw, &snapshot.WorkerPolls); err != nil {
		return production.OperationalStatus{}, fmt.Errorf("decode worker poll observations: %w", err)
	}
	status, err := production.EvaluateSnapshot(snapshot)
	if err != nil {
		return production.OperationalStatus{}, err
	}
	return status, nil
}
