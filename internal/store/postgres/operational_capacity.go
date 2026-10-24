package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/production"
)

// ReadOperationalCapacity reads the existing authority without acquiring a
// Worker lease, updating a heartbeat, or writing an audit/Outbox record. The
// caller supplies the already loaded policy's complete current claimer list.
// This reports reservation limits, not current process or provider capacity.
func (s *Store) ReadOperationalCapacity(ctx context.Context, grants []production.WorkerCapacityGrant) (production.CapacityFacts, error) {
	ctx, stop := authorityContext(ctx)
	defer stop()
	var zero production.CapacityFacts
	if s == nil || s.pool == nil {
		return zero, fmt.Errorf("PostgreSQL capacity source is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	profiles, err := production.CapacityProfiles(grants)
	if err != nil {
		return zero, err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return zero, err
	}
	defer rollbackOutbox(tx)
	facts := production.CapacityFacts{Workers: []production.WorkerCapacityFact{}, Queues: []production.QueueCapacityFact{}}
	// One database timestamp selects every active/expired boundary in this
	// repeatable snapshot, including when a later query waits behind DDL.
	facts.Authority, err = readOperationalStatus(ctx, tx)
	if err != nil {
		return zero, fmt.Errorf("read original capacity authority snapshot: %w", err)
	}
	facts.CapturedAt = facts.Authority.Snapshot.CapturedAt
	facts.RecoveryEpoch = facts.Authority.Snapshot.RecoveryEpoch
	facts.RecoveryMode = facts.Authority.Snapshot.RecoveryMode
	var malformedLease bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM worker_inbox WHERE
 (state='LEASED' AND (lease_owner IS NULL OR btrim(lease_owner)='' OR lease_generation<=0 OR
 lease_recovery_epoch IS NULL OR lease_recovery_epoch<0 OR lease_started_at IS NULL OR lease_until IS NULL))
 OR (state<>'LEASED' AND lease_until IS NOT NULL))`).Scan(&malformedLease); err != nil {
		return zero, err
	}
	if malformedLease {
		return zero, fmt.Errorf("inconsistent retained worker lease facts")
	}
	subjects := make([]string, len(grants))
	for i := range grants {
		subjects[i] = grants[i].Subject
	}
	rows, err := tx.Query(ctx, `/* operational_capacity_workers */
SELECT selected.subject,w.worker_id IS NOT NULL,
 COALESCE(w.protocol_version,''),COALESCE(w.status,''),
 COALESCE(w.attributes_json->>'worker_profile',''),w.last_seen_at,
 COALESCE(jsonb_typeof(w.attributes_json)='object' AND jsonb_typeof(w.attributes_json->'worker_profile')='string',false),
 (SELECT count(*) FROM worker_inbox i WHERE i.lease_owner=selected.subject AND i.state='LEASED' AND i.lease_until>$2),
 (SELECT count(*) FROM worker_inbox i WHERE i.lease_owner=selected.subject AND i.state='LEASED' AND i.lease_until<=$2)
FROM unnest($1::text[]) AS selected(subject)
LEFT JOIN workers w ON w.worker_id=selected.subject
ORDER BY selected.subject`, subjects, facts.CapturedAt)
	if err != nil {
		return zero, fmt.Errorf("read capacity worker reservations: %w", err)
	}
	for rows.Next() {
		var fact production.WorkerCapacityFact
		var subject string
		var attributesValid bool
		if err := rows.Scan(&subject, &fact.Recorded, &fact.Protocol, &fact.Status, &fact.LastPollProfile, &fact.LastPollAt,
			&attributesValid, &fact.ActiveLeases, &fact.ExpiredLeases); err != nil {
			rows.Close()
			return zero, err
		}
		index := len(facts.Workers)
		if index >= len(grants) || subject != grants[index].Subject || (fact.Recorded && !attributesValid) {
			rows.Close()
			return zero, fmt.Errorf("capacity worker identity or registration is inconsistent")
		}
		fact.Grant = grants[index]
		facts.Workers = append(facts.Workers, fact)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return zero, err
	}
	rows.Close()
	queues := make(map[string]production.QueueCapacityFact, len(profiles))
	for _, profile := range profiles {
		queues[profile] = production.QueueCapacityFact{Profile: profile}
	}
	rows, err = tx.Query(ctx, `/* operational_capacity_queues */
SELECT worker_profile,
 count(*) FILTER (WHERE state='PENDING'),
 count(*) FILTER (WHERE state='LEASED' AND lease_until>$2),
 count(*) FILTER (WHERE state='LEASED' AND lease_until<=$2),
 min(received_at) FILTER (WHERE state='PENDING')
FROM worker_inbox WHERE worker_profile=ANY($1::text[]) GROUP BY worker_profile ORDER BY worker_profile`, profiles, facts.CapturedAt)
	if err != nil {
		return zero, fmt.Errorf("read capacity queue facts: %w", err)
	}
	for rows.Next() {
		var fact production.QueueCapacityFact
		if err := rows.Scan(&fact.Profile, &fact.Pending, &fact.ActiveLeases, &fact.ExpiredLeases, &fact.OldestPendingAt); err != nil {
			rows.Close()
			return zero, err
		}
		if _, exists := queues[fact.Profile]; !exists {
			rows.Close()
			return zero, fmt.Errorf("unrequested capacity queue")
		}
		queues[fact.Profile] = fact
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return zero, err
	}
	rows.Close()
	for _, profile := range profiles {
		facts.Queues = append(facts.Queues, queues[profile])
	}
	// End the read-only transaction before sampling the separate local pool;
	// the observer's own connection is not deliberately counted as busy.
	if err := tx.Rollback(ctx); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	stats := s.pool.Stat()
	facts.Pool = production.PoolCapacityFact{
		CapturedAt: time.Now().UTC(), Maximum: int64(stats.MaxConns()), Total: int64(stats.TotalConns()),
		Acquired: int64(stats.AcquiredConns()), Idle: int64(stats.IdleConns()), Constructing: int64(stats.ConstructingConns()),
	}
	if err := facts.Validate(grants); err != nil {
		return zero, err
	}
	return facts, nil
}
