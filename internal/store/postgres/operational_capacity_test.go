package postgres

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/production"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

const capacityTestWorker = "urn:engineering-platform:worker:capacity"

func capacityGrants() []production.WorkerCapacityGrant {
	return []production.WorkerCapacityGrant{{Subject: capacityTestWorker, Profiles: []string{"worker/ubuntu"}, Lane: "ADMISSION"}}
}

func capacityLedger(t *testing.T, s *Store) []byte {
	t.Helper()
	var raw []byte
	workerOK(t, s.pool.QueryRow(context.Background(), `SELECT jsonb_build_object(
 'workers',(SELECT jsonb_agg(to_jsonb(w) ORDER BY worker_id) FROM workers w),
 'inbox',(SELECT jsonb_agg(to_jsonb(i) ORDER BY inbox_id) FROM worker_inbox i),
 'outbox',(SELECT jsonb_agg(to_jsonb(o) ORDER BY outbox_id) FROM outbox_events o),
 'audit',(SELECT jsonb_agg(to_jsonb(a) ORDER BY sequence) FROM audit_events a),
 'platform',(SELECT to_jsonb(p) FROM platform_state p WHERE singleton_id=true))`).Scan(&raw))
	return raw
}

func TestReadOperationalCapacityPreservesAuthorityAndExcludesUnauthorizedHistory(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	ctx := context.Background()
	startWorkerFixture(t, s, "capacity-one")
	startWorkerFixture(t, s, "capacity-two")
	relayWorkerFixture(t, s)
	active := claimWorkerFixture(t, s, capacityTestWorker)
	claimWorkerFixture(t, s, "urn:engineering-platform:worker:retired")
	workerSQL(t, s, `UPDATE workers SET last_seen_at=clock_timestamp()-interval '1 day' WHERE worker_id=$1`, capacityTestWorker)
	// A retained external UNKNOWN remains visible beside any pool headroom.
	runID := setupPostgresActionRun(t, s, "capacity-unknown")
	now := time.Now().UTC()
	req := action.Request{ID: "capacity-unknown", RunID: runID, ExecutionEpoch: 1, Action: "github.publish-pr", RiskClass: action.ControlledMutation,
		Capability: "github.publish-pr", ParametersDigest: "sha256:" + strings.Repeat("a", 64), IdempotencyKey: "capacity-unknown", RequestedBy: "runtime", RequestedAt: now}
	op := action.NewWithRequestDigest(req, "sha256:"+strings.Repeat("b", 64), now)
	workerOK(t, s.CreateContext(ctx, *op))
	workerOK(t, op.Transition(action.Dispatched, now))
	workerOK(t, s.UpdateContext(ctx, *op))
	workerOK(t, op.Transition(action.Unknown, now))
	workerOK(t, s.UpdateContext(ctx, *op))
	before := capacityLedger(t, s)
	for n := 0; n < 2; n++ {
		facts, err := s.ReadOperationalCapacity(ctx, capacityGrants())
		workerOK(t, err)
		if len(facts.Workers) != 1 || facts.Workers[0].Grant.Subject != capacityTestWorker || facts.Workers[0].ActiveLeases != 1 || facts.Workers[0].ExpiredLeases != 0 || !facts.Workers[0].Recorded {
			t.Fatal("current worker population or genuine reservation lost", facts)
		}
		if facts.Authority.AuthorityClear || facts.Authority.Snapshot.UnknownOperations != 1 || facts.Authority.Snapshot.ActiveWorkerLeases != 2 || facts.Queues[0].ActiveLeases != 2 || facts.Authority.Ready || facts.Authority.ProductionQualified {
			t.Fatal("retired identity or UNKNOWN hidden from original global authority", facts)
		}
		if facts.Pool.CapturedAt.Before(facts.CapturedAt) || facts.Pool.Maximum != int64(s.pool.Config().MaxConns) || facts.Pool.Acquired != 0 {
			t.Fatal("pool sample was taken inside the observer transaction or invents DB-wide capacity", facts.Pool)
		}
	}
	if !bytes.Equal(before, capacityLedger(t, s)) {
		t.Fatal("capacity read mutated heartbeat, lease, audit, outbox or Recovery")
	}
	// The existing terminal path deliberately retains owner/generation but clears
	// lease_until. It must not be mistaken for malformed live lease metadata.
	report, err := workerqueue.NewReport(*active)
	workerOK(t, err)
	_, err = s.ReportInput(ctx, capacityTestWorker, report)
	workerOK(t, err)
	terminal, err := s.ReadOperationalCapacity(ctx, capacityGrants())
	workerOK(t, err)
	if terminal.Workers[0].ActiveLeases != 0 || terminal.Workers[0].ExpiredLeases != 0 {
		t.Fatal("completed reservation still active", terminal)
	}
	// A currently granted identity with no row is retained explicitly, never
	// replaced by a historical ungranted worker with a matching profile.
	missing := []production.WorkerCapacityGrant{{Subject: "urn:engineering-platform:worker:new", Profiles: []string{"worker/ubuntu"}, Lane: "ADMISSION"}}
	got, err := s.ReadOperationalCapacity(ctx, missing)
	workerOK(t, err)
	if len(got.Workers) != 1 || got.Workers[0].Recorded || got.Workers[0].LastPollAt != nil || got.Workers[0].ActiveLeases != 0 {
		t.Fatal("unregistered identity became historical capacity", got)
	}
}

type capacityTraceKey struct{}
type capacitySnapshotBarrier struct {
	ready, proceed chan struct{}
	once           sync.Once
	mu             sync.Mutex
	queries        []string
}

func (b *capacitySnapshotBarrier) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	b.mu.Lock()
	b.queries = append(b.queries, data.SQL)
	b.mu.Unlock()
	if strings.Contains(data.SQL, "operations.safe_to_retry") {
		return context.WithValue(ctx, capacityTraceKey{}, true)
	}
	return ctx
}

func (b *capacitySnapshotBarrier) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	if ctx.Value(capacityTraceKey{}) != true || data.Err != nil {
		return
	}
	b.once.Do(func() {
		close(b.ready)
		select {
		case <-b.proceed:
		case <-ctx.Done():
		}
	})
}

func TestReadOperationalCapacityUsesOneReadOnlyRepeatableSnapshot(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	startWorkerFixture(t, s, "capacity-repeatable")
	relayWorkerFixture(t, s)
	assignment := claimWorkerFixture(t, s, capacityTestWorker)
	barrier := &capacitySnapshotBarrier{ready: make(chan struct{}), proceed: make(chan struct{})}
	var release sync.Once
	defer release.Do(func() { close(barrier.proceed) })
	config := s.pool.Config()
	config.ConnConfig.Tracer = barrier
	pool, err := pgxpool.NewWithConfig(ctx, config)
	workerOK(t, err)
	defer pool.Close()
	observed := New(pool)
	type result struct {
		facts production.CapacityFacts
		err   error
	}
	done := make(chan result, 1)
	go func() { f, err := observed.ReadOperationalCapacity(ctx, capacityGrants()); done <- result{f, err} }()
	select {
	case <-barrier.ready:
	case <-time.After(3 * time.Second):
		t.Fatal("original authority snapshot was not observed")
	}
	// This commits after v6 authority readback and before capacity's worker and
	// queue projections. READ COMMITTED would combine incompatible generations.
	tx, err := s.pool.Begin(ctx)
	workerOK(t, err)
	defer rollbackOutbox(tx)
	_, err = tx.Exec(ctx, `UPDATE platform_state SET recovery_epoch=7,recovery_mode='RECOVERY_RECONCILIATION' WHERE singleton_id=true`)
	workerOK(t, err)
	_, err = tx.Exec(ctx, `UPDATE workers SET status='OFFLINE',last_seen_at=clock_timestamp() WHERE worker_id=$1`, capacityTestWorker)
	workerOK(t, err)
	_, err = tx.Exec(ctx, `UPDATE worker_inbox SET lease_until=clock_timestamp()-interval '1 second' WHERE inbox_id=$1`, assignment.Token.InboxID)
	workerOK(t, err)
	workerOK(t, tx.Commit(ctx))
	committedAt := time.Now().UTC()
	release.Do(func() { close(barrier.proceed) })
	var first production.CapacityFacts
	select {
	case result := <-done:
		workerOK(t, result.err)
		first = result.facts
	case <-time.After(3 * time.Second):
		t.Fatal("repeatable read did not complete")
	}
	if first.RecoveryEpoch != 0 || first.RecoveryMode != "NORMAL" || first.Authority.Snapshot.RecoveryEpoch != 0 || first.Workers[0].Status != "ONLINE" || first.Workers[0].ActiveLeases != 1 || first.Workers[0].ExpiredLeases != 0 || first.Queues[0].ActiveLeases != 1 || first.Authority.Snapshot.ActiveWorkerLeases != 1 {
		t.Fatal("capacity mixed old authority with later worker/recovery facts", first)
	}
	if first.Pool.CapturedAt.Before(committedAt) || !first.Pool.CapturedAt.After(first.CapturedAt) {
		t.Fatal("independent pool sample claimed the database snapshot time", first)
	}
	second, err := observed.ReadOperationalCapacity(ctx, capacityGrants())
	workerOK(t, err)
	if second.RecoveryEpoch != 7 || second.Authority.AuthorityClear || second.Workers[0].Status != "OFFLINE" || second.Workers[0].ExpiredLeases != 1 || second.Authority.Snapshot.ExpiredWorkerLeases != 1 || second.Queues[0].ExpiredLeases != 1 {
		t.Fatal("later complete generation was not observed", second)
	}
	barrier.mu.Lock()
	defer barrier.mu.Unlock()
	readOnly := false
	for _, query := range barrier.queries {
		lower := strings.ToLower(strings.TrimSpace(query))
		if strings.HasPrefix(lower, "begin") {
			readOnly = strings.Contains(lower, "repeatable read") && strings.Contains(lower, "read only")
		}
		if strings.HasPrefix(lower, "insert") || strings.HasPrefix(lower, "update") || strings.HasPrefix(lower, "delete") || strings.HasPrefix(lower, "commit") {
			t.Fatal("capacity used a mutable transaction operation", query)
		}
	}
	if !readOnly {
		t.Fatal("actual PostgreSQL transaction was not READ ONLY REPEATABLE READ")
	}
}

func TestReadOperationalCapacityLocalPoolHeadroomAndCancelledWait(t *testing.T) {
	original := newIsolatedIntegrationStore(t)
	config := original.pool.Config()
	config.MaxConns, config.MinConns = 2, 0
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	workerOK(t, err)
	defer pool.Close()
	s := New(pool)
	first, err := pool.Acquire(context.Background())
	workerOK(t, err)
	defer first.Release()
	facts, err := s.ReadOperationalCapacity(context.Background(), capacityGrants())
	workerOK(t, err)
	if facts.Pool.Maximum != 2 || facts.Pool.Acquired != 1 || facts.Pool.Idle != 1 || facts.Pool.Total != 2 || facts.Pool.Constructing != 0 {
		t.Fatal("local pool was not sampled after observation release", facts.Pool)
	}
	second, err := pool.Acquire(context.Background())
	workerOK(t, err)
	defer second.Release()
	before := capacityLedger(t, original)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		got, err := s.ReadOperationalCapacity(ctx, capacityGrants())
		if !reflect.DeepEqual(got, production.CapacityFacts{}) {
			t.Error("cancelled read retained capacity")
		}
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("pool wait lost caller deadline", err)
		}
	case <-time.After(3 * time.Second):
		first.Release()
		second.Release()
		t.Fatal("cancelled capacity read waited for the pool")
	}
	if !bytes.Equal(before, capacityLedger(t, original)) {
		t.Fatal("cancelled read mutated authority")
	}
	second.Release()
	first.Release()
	again, err := s.ReadOperationalCapacity(context.Background(), capacityGrants())
	workerOK(t, err)
	if again.Pool.Acquired != 0 || again.Pool.Maximum != 2 {
		t.Fatal("cancelled observer retained a connection", again.Pool)
	}
}

func TestReadOperationalCapacityRejectsMalformedRetainedLeaseFacts(t *testing.T) {
	for _, mode := range []string{"empty-owner", "nonleased-until", "null-until", "duplicate-owner", "nonstring-profile"} {
		t.Run(mode, func(t *testing.T) {
			s := newIsolatedIntegrationStore(t)
			ctx := context.Background()
			startWorkerFixture(t, s, "capacity-malformed-one")
			startWorkerFixture(t, s, "capacity-malformed-two")
			relayWorkerFixture(t, s)
			first := claimWorkerFixture(t, s, capacityTestWorker)
			other := claimWorkerFixture(t, s, "urn:engineering-platform:worker:retired")
			switch mode {
			case "empty-owner":
				workerSQL(t, s, `UPDATE worker_inbox SET lease_owner='' WHERE inbox_id=$1`, first.Token.InboxID)
			case "nonleased-until":
				workerSQL(t, s, `UPDATE worker_inbox SET state='REVOKED' WHERE inbox_id=$1`, first.Token.InboxID)
			case "duplicate-owner":
				workerSQL(t, s, `UPDATE worker_inbox SET lease_owner=$2 WHERE inbox_id=$1`, other.Token.InboxID, capacityTestWorker)
			case "nonstring-profile":
				workerSQL(t, s, `UPDATE workers SET attributes_json='{"worker_profile":42}'::jsonb WHERE worker_id=$1`, capacityTestWorker)
			case "null-until":
				var constraint string
				workerOK(t, s.pool.QueryRow(ctx, `SELECT conname FROM pg_constraint WHERE conrelid='worker_inbox'::regclass AND contype='c' AND pg_get_constraintdef(oid) LIKE '%lease_started_at%'`).Scan(&constraint))
				workerSQL(t, s, "ALTER TABLE worker_inbox DROP CONSTRAINT "+pgx.Identifier{constraint}.Sanitize())
				workerSQL(t, s, `UPDATE worker_inbox SET lease_until=NULL WHERE inbox_id=$1`, first.Token.InboxID)
			}
			before := capacityLedger(t, s)
			facts, err := s.ReadOperationalCapacity(ctx, capacityGrants())
			if err == nil || !reflect.DeepEqual(facts, production.CapacityFacts{}) {
				t.Fatal("corrupt retained lease became headroom", facts, err)
			}
			if !bytes.Equal(before, capacityLedger(t, s)) {
				t.Fatal("capacity integrity rejection repaired authority implicitly")
			}
		})
	}
}
