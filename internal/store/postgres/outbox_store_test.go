package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/outbox"
)

func insertOutboxFixture(ctx context.Context, s *Store, message OutboxMessage) error {
	record, err := auditInput("outbox.fixture", "Test", message.Key, map[string]any{"key": message.Key, "topic": message.Topic})
	if err != nil {
		return err
	}
	_, err = s.Mutate(ctx, Mutation{Audit: record, Outbox: []OutboxMessage{message}})
	return err
}
func queueMessage(t *testing.T, s *Store, key string, risk action.RiskClass) {
	t.Helper()
	if err := insertOutboxFixture(context.Background(), s, OutboxMessage{Key: key, Topic: "test.delivery", RiskClass: risk, Payload: map[string]string{"key": key}}); err != nil {
		t.Fatal(err)
	}
}
func claimOne(t *testing.T, s *Store, worker string) outbox.Message {
	t.Helper()
	messages, err := s.Claim(context.Background(), worker, 1, time.Minute)
	if err != nil || len(messages) != 1 {
		t.Fatalf("claim: %+v %v", messages, err)
	}
	return messages[0]
}
func expireLease(t *testing.T, s *Store, id int64) {
	t.Helper()
	if _, err := s.pool.Exec(context.Background(), `UPDATE outbox_events SET lease_until=clock_timestamp()-interval '1 second' WHERE outbox_id=$1`, id); err != nil {
		t.Fatal(err)
	}
}
func succeed(context.Context, outbox.Message) outbox.Resolution {
	return outbox.Resolution{State: outbox.Dispatched}
}

func TestPostgresOutboxRecoveryGateAndLeaseRecovery(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	queueMessage(t, s, "observe", action.Observe)
	queueMessage(t, s, "mutation", action.ControlledMutation)
	recovery, err := s.BeginRecovery(0)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := s.Claim(ctx, "worker-a", 10, time.Minute)
	if err != nil || len(messages) != 1 || messages[0].Key != "observe" {
		t.Fatalf("recovery claim: %+v %v", messages, err)
	}
	if _, err := s.Dispatch(ctx, messages[0].Lease(), action.Observe, succeed); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteRecovery(recovery.Epoch, true); err != nil {
		t.Fatal(err)
	}
	first := claimOne(t, s, "worker-a")
	other, err := s.Claim(ctx, "worker-b", 1, time.Minute)
	if err != nil || len(other) != 0 {
		t.Fatalf("active lease was stolen: %+v %v", other, err)
	}
	expireLease(t, s, first.ID)
	second := claimOne(t, s, "worker-b")
	if second.AttemptCount != first.AttemptCount+1 || second.Key != first.Key {
		t.Fatal("reclaim identity/generation changed incorrectly")
	}
	if err := s.MarkDispatched(ctx, first.Lease()); !errors.Is(err, outbox.ErrLeaseLost) {
		t.Fatalf("stale ACK: %v", err)
	}
	if _, err := s.Dispatch(ctx, second.Lease(), action.ControlledMutation, succeed); err != nil {
		t.Fatal(err)
	}
	stored, err := s.GetOutbox(ctx, "mutation")
	if err != nil || stored.State != outbox.Dispatched || stored.DispatchedAt.IsZero() {
		t.Fatalf("%+v %v", stored, err)
	}
}

func TestPostgresEverySettlementFencesExpiryAndSameWorkerReclaim(t *testing.T) {
	operations := map[string]func(*Store, outbox.Lease) error{
		"ack": func(s *Store, l outbox.Lease) error { return s.MarkDispatched(context.Background(), l) },
		"retry": func(s *Store, l outbox.Lease) error {
			return s.MarkRetry(context.Background(), l, time.Second, "retry")
		},
		"dead-letter": func(s *Store, l outbox.Lease) error { return s.MarkDeadLetter(context.Background(), l, "failed") },
	}
	for name, settle := range operations {
		t.Run(name, func(t *testing.T) {
			s := integrationStore(t)
			queueMessage(t, s, "same-worker", action.Observe)
			first := claimOne(t, s, "stable-worker")
			expireLease(t, s, first.ID)
			if err := settle(s, first.Lease()); !errors.Is(err, outbox.ErrLeaseLost) {
				t.Fatalf("expired lease settled before reclaim: %v", err)
			}
			second := claimOne(t, s, "stable-worker")
			if second.AttemptCount != first.AttemptCount+1 {
				t.Fatal("generation not advanced")
			}
			if err := settle(s, first.Lease()); !errors.Is(err, outbox.ErrLeaseLost) {
				t.Fatalf("old same-worker generation settled: %v", err)
			}
			current, err := s.GetOutbox(context.Background(), "same-worker")
			if err != nil || current.State != outbox.Leased || current.Lease() != second.Lease() {
				t.Fatalf("new lease corrupted: %+v %v", current, err)
			}
			if err := settle(s, second.Lease()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPostgresRecoveryAfterClaimAndOldEpochAfterCompletion(t *testing.T) {
	for _, risk := range []action.RiskClass{action.ControlledMutation, action.HighRisk} {
		t.Run(string(risk), func(t *testing.T) {
			s := integrationStore(t)
			ctx := context.Background()
			queueMessage(t, s, "change", risk)
			old := claimOne(t, s, "worker")
			recovery, err := s.BeginRecovery(0)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			deliver := func(context.Context, outbox.Message) outbox.Resolution {
				calls++
				return outbox.Resolution{State: outbox.Dispatched}
			}
			if _, err := s.Dispatch(ctx, old.Lease(), risk, deliver); !errors.Is(err, outbox.ErrRecoveryBlocked) {
				t.Fatalf("dispatch in recovery: %v", err)
			}
			if _, err := s.CompleteRecovery(recovery.Epoch, true); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Dispatch(ctx, old.Lease(), risk, deliver); !errors.Is(err, outbox.ErrRecoveryBlocked) {
				t.Fatalf("old epoch resurrected: %v", err)
			}
			if calls != 0 {
				t.Fatal("blocked mutation handler ran")
			}
			expireLease(t, s, old.ID)
			fresh := claimOne(t, s, "worker")
			if fresh.LeaseRecoveryEpoch != recovery.Epoch {
				t.Fatal("claim not bound to current recovery epoch")
			}
			if _, err := s.Dispatch(ctx, fresh.Lease(), risk, deliver); err != nil || calls != 1 {
				t.Fatalf("fresh dispatch: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestPostgresDispatchHoldsRecoveryLockUntilSettlement(t *testing.T) {
	s := integrationStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	queueMessage(t, s, "guarded", action.ControlledMutation)
	m := claimOne(t, s, "worker")
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := s.Dispatch(ctx, m.Lease(), action.ControlledMutation, func(callCtx context.Context, _ outbox.Message) outbox.Resolution {
			close(entered)
			select {
			case <-release:
			case <-callCtx.Done():
			}
			return outbox.Resolution{State: outbox.Dispatched}
		})
		done <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// NOWAIT proves the actual UPDATE-conflicting lock is held, without timing
	// assumptions or completing a recovery update outside its audit transaction.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		close(release)
		<-done
		t.Fatal(err)
	}
	_, lockErr := tx.Exec(ctx, `SELECT singleton_id FROM platform_state WHERE singleton_id=true FOR NO KEY UPDATE NOWAIT`)
	_ = tx.Rollback(ctx)
	var pgErr *pgconn.PgError
	if !errors.As(lockErr, &pgErr) || pgErr.Code != "55P03" {
		t.Errorf("recovery could pass an active dispatch: %v", lockErr)
	}
	// A competing claimant must also skip the callback's locked message.
	claimed, err := s.Claim(ctx, "second-worker", 1, time.Minute)
	if err != nil || len(claimed) != 0 {
		t.Errorf("locked dispatch was reclaimed: %+v %v", claimed, err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginRecovery(0); err != nil {
		t.Fatal(err)
	}
	stored, err := s.GetOutbox(ctx, m.Key)
	if err != nil || stored.State != outbox.Dispatched {
		t.Fatalf("receipt did not precede recovery: %+v %v", stored, err)
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE event_type='outbox.settled' AND aggregate_id=$1`, m.Key).Scan(&count); err != nil || count != 1 {
		t.Fatalf("settlement audit missing: %d %v", count, err)
	}
}

func TestPostgresConcurrentDispatchSameLeaseCallsHandlerOnce(t *testing.T) {
	s := integrationStore(t)
	queueMessage(t, s, "duplicate", action.Observe)
	m := claimOne(t, s, "worker")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var calls atomic.Int64
	outcomes := make(chan error, 2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			_, err := s.Dispatch(ctx, m.Lease(), action.Observe, func(context.Context, outbox.Message) outbox.Resolution {
				calls.Add(1)
				return outbox.Resolution{State: outbox.Dispatched}
			})
			outcomes <- err
		}()
	}
	close(start)
	success, stale := 0, 0
	for i := 0; i < 2; i++ {
		err := <-outcomes
		if err == nil {
			success++
		} else if errors.Is(err, outbox.ErrLeaseLost) {
			stale++
		} else {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 || success != 1 || stale != 1 {
		t.Fatalf("calls=%d success=%d stale=%d", calls.Load(), success, stale)
	}
}

func TestPostgresMismatchedRiskAndMissingRecoveryNeverInvokeHandler(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	queueMessage(t, s, "risk", action.Observe)
	m := claimOne(t, s, "worker")
	deliver := func(context.Context, outbox.Message) outbox.Resolution {
		t.Error("unauthorized handler ran")
		return outbox.Resolution{State: outbox.Dispatched}
	}
	if _, err := s.Dispatch(ctx, m.Lease(), action.HighRisk, deliver); !errors.Is(err, outbox.ErrRiskClass) {
		t.Fatalf("risk mismatch: %v", err)
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM platform_state`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Dispatch(ctx, m.Lease(), action.Observe, deliver); !errors.Is(err, outbox.ErrRecoveryBlocked) {
		t.Fatalf("missing recovery: %v", err)
	}
}

func TestPostgresExpiredAndCancelledHandlersCannotACK(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	queueMessage(t, s, "expire", action.Observe)
	m := claimOne(t, s, "worker")
	expireLease(t, s, m.ID)
	if _, err := s.Dispatch(ctx, m.Lease(), action.Observe, func(context.Context, outbox.Message) outbox.Resolution {
		t.Error("expired handler ran")
		return outbox.Resolution{State: outbox.Dispatched}
	}); !errors.Is(err, outbox.ErrLeaseLost) {
		t.Fatal(err)
	}
	m = claimOne(t, s, "worker")
	callCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	_, err := s.Dispatch(callCtx, m.Lease(), action.Observe, func(ctx context.Context, _ outbox.Message) outbox.Resolution {
		<-ctx.Done()
		return outbox.Resolution{State: outbox.Dispatched}
	})
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, outbox.ErrOutcomeUnknown) {
		t.Fatalf("late handler ACK: %v", err)
	}
	stored, err := s.GetOutbox(ctx, m.Key)
	if err != nil || stored.State != outbox.Leased {
		t.Fatalf("late result settled: %+v %v", stored, err)
	}
}

func TestPostgresOutboxDispatcherRetryAndDeadLetter(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	queueMessage(t, s, "retry", action.Observe)
	d := outbox.Dispatcher{Repository: s, Handlers: map[string]outbox.Registration{"test.delivery": {RiskClass: action.Observe, Handler: outbox.HandlerFunc(func(context.Context, outbox.Message) error { return errors.New("downstream unavailable") })}}, RetryDelay: time.Millisecond, MaxAttempts: 2}
	first, err := d.DispatchBatch(ctx, "worker", 1)
	if err != nil || first.Retried != 1 {
		t.Fatalf("first: %+v %v", first, err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE outbox_events SET next_attempt_at=clock_timestamp()-interval '1 second' WHERE outbox_key='retry'`); err != nil {
		t.Fatal(err)
	}
	second, err := d.DispatchBatch(ctx, "worker", 1)
	if err != nil || second.DeadLettered != 1 {
		t.Fatalf("second: %+v %v", second, err)
	}
	stored, err := s.GetOutbox(ctx, "retry")
	if err != nil || stored.State != outbox.DeadLetter || stored.AttemptCount != 2 || stored.LastError == "" {
		t.Fatalf("%+v %v", stored, err)
	}
}

func TestPostgresUnknownOutcomeIsQuarantinedNotRetried(t *testing.T) {
	s := integrationStore(t)
	queueMessage(t, s, "unknown", action.HighRisk)
	d := outbox.Dispatcher{Repository: s, Handlers: map[string]outbox.Registration{"test.delivery": {RiskClass: action.HighRisk, Handler: outbox.HandlerFunc(func(context.Context, outbox.Message) error { return outbox.ErrOutcomeUnknown })}}}
	first, err := d.DispatchBatch(context.Background(), "worker", 1)
	if err != nil || first.DeadLettered != 1 {
		t.Fatalf("%+v %v", first, err)
	}
	second, err := d.DispatchBatch(context.Background(), "worker", 1)
	if err != nil || second.Claimed != 0 {
		t.Fatalf("unknown outcome retried: %+v %v", second, err)
	}
}

func TestPostgresRiskRejectionRollsBackBusinessAuditAndOutbox(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	for i, risk := range []action.RiskClass{"", "UNKNOWN"} {
		_, err := s.Mutate(ctx, insertWorkMutation(fmt.Sprintf("risk-%d", i), []OutboxMessage{{Key: fmt.Sprintf("risk-%d", i), Topic: "new.topic", RiskClass: risk, Payload: map[string]string{}}}))
		if !errors.Is(err, outbox.ErrRiskClass) {
			t.Fatalf("unknown classification accepted: %v", err)
		}
	}
	for _, table := range []string{"work_items", "audit_events", "outbox_events"} {
		var count int
		if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s survived rollback: %d %v", table, count, err)
		}
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO outbox_events(outbox_key,topic,payload_json) VALUES('missing','new.topic','{}')`)
	if err == nil {
		t.Fatal("DB still defaults omitted risk to OBSERVE")
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO outbox_events(outbox_key,topic,risk_class,payload_json) VALUES('downgrade','run.started','OBSERVE','{}')`)
	if err == nil {
		t.Fatal("DB permits Core topic downgrade")
	}
}

func TestPostgresOutboxMigrationPreservesEvidenceAndQuarantinesLegacyLeases(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	legacy, err := os.ReadFile("../../../db/migrations/0001_core.sql")
	if err != nil {
		t.Fatal(err)
	}
	resetIsolatedSchema(t, s)
	if _, err := s.pool.Exec(ctx, string(legacy), pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO outbox_events(outbox_key,topic,state,payload_json,lease_owner,lease_until,attempt_count) VALUES
  ('known','run.started','PENDING','{}',NULL,NULL,0),
  ('active','run.started','LEASED','{}','old-worker',clock_timestamp()+interval '1 hour',7),
  ('unknown','legacy.unknown','PENDING','{}',NULL,NULL,0),
  ('history','run.started','DISPATCHED','{}',NULL,NULL,3)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyCoreMigration(ctx); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]outbox.State{"known": outbox.Pending, "active": outbox.DeadLetter, "unknown": outbox.DeadLetter, "history": outbox.Dispatched} {
		m, err := s.GetOutbox(ctx, key)
		if err != nil || m.State != want {
			t.Fatalf("%s: %+v %v", key, m, err)
		}
		if key == "active" && (m.AttemptCount != 7 || m.LeaseOwner != "") {
			t.Fatal("legacy lease generation lost or owner survived")
		}
		if key == "history" && (m.AttemptCount != 3 || m.RiskClass != action.Observe) {
			t.Fatal("historical evidence rewritten")
		}
		if key == "known" && m.RiskClass != action.ControlledMutation {
			t.Fatal("run.started still considered OBSERVE")
		}
	}
	m := claimOne(t, s, "new-worker")
	if _, err := s.Dispatch(ctx, m.Lease(), action.ControlledMutation, succeed); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyCoreMigration(ctx); err != nil {
		t.Fatal(err)
	}
	m, err = s.GetOutbox(ctx, "known")
	if err != nil || m.State != outbox.Dispatched || m.AttemptCount != 1 {
		t.Fatalf("migration replay reset work: %+v %v", m, err)
	}
}

func TestPostgresOutboxSettlementUsesWallClockAfterLockWait(t *testing.T) {
	s := integrationStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	queueMessage(t, s, "wait-expiry", action.Observe)
	m := claimOne(t, s, "worker")
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollbackOutbox(tx)
	if _, err := tx.Exec(ctx, `UPDATE outbox_events SET lease_until=clock_timestamp()+interval '200 milliseconds' WHERE outbox_id=$1`, m.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.MarkDispatched(ctx, m.Lease()) }()
	// Wait until ACK actually blocks behind our row lock, then expire the lease
	// in DB time. A transaction-start now() would let this stale receipt through.
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, tx.Conn().PgConn().PID()).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if _, err := tx.Exec(ctx, `SELECT pg_sleep(GREATEST(0,EXTRACT(EPOCH FROM lease_until-clock_timestamp()))+0.02) FROM outbox_events WHERE outbox_id=$1`, m.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, outbox.ErrLeaseLost) {
		t.Fatalf("expired while locked but ACK succeeded: %v", err)
	}
}

func TestPostgresOutboxAuditFailureRollsBackSettlement(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	queueMessage(t, s, "audit-failure", action.Observe)
	m := claimOne(t, s, "worker")
	if _, err := s.pool.Exec(ctx, `DELETE FROM audit_journal_state`); err != nil {
		t.Fatal(err)
	}
	_, err := s.Dispatch(ctx, m.Lease(), action.Observe, succeed)
	if !errors.Is(err, outbox.ErrOutcomeUnknown) {
		t.Fatalf("unrecorded outcome reported success: %v", err)
	}
	stored, err := s.GetOutbox(ctx, m.Key)
	if err != nil || stored.State != outbox.Leased {
		t.Fatalf("settlement committed without audit: %+v %v", stored, err)
	}
}
