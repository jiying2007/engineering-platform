package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	corestore "github.com/jiying2007/engineering-platform/internal/store"
)

func TestNoInvocationDatabaseCorruptionBlocksProofAndReadback(t *testing.T) {
	forNoInvocationVersions(t, testNoInvocationDatabaseCorruptionBlocksProofAndReadback)
}
func testNoInvocationDatabaseCorruptionBlocksProofAndReadback(t *testing.T, version int) {
	for _, mode := range []string{"external-ref", "observation", "missing-receipt", "self-consistent-receipt", "missing-operation", "missing-audit", "changed-time", "unknown-field"} {
		t.Run(mode, func(t *testing.T) {
			s, req, e := noInvocationDatabaseFixture(t, action.Manual, version)
			ctx := context.Background()
			saved, err := s.AbandonUninvokedPublication(ctx, plannedReconciler, req, e)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "external-ref":
				_, err = s.pool.Exec(ctx, "UPDATE external_operations SET external_ref='wrong-ref' WHERE operation_id=$1", req.Identity.OperationID)
			case "observation":
				_, err = s.pool.Exec(ctx, "UPDATE external_operations SET observed_state='{}' WHERE operation_id=$1", req.Identity.OperationID)
			case "missing-receipt":
				_, err = s.pool.Exec(ctx, "UPDATE external_operations SET reconciliation_json='{}'::jsonb WHERE operation_id=$1", req.Identity.OperationID)
			case "missing-operation":
				_, err = s.pool.Exec(ctx, "DELETE FROM external_operations WHERE operation_id=$1", req.Identity.OperationID)
			case "missing-audit":
				_, err = s.pool.Exec(ctx, "DELETE FROM audit_events WHERE event_type='recovery.github.publication.not_invoked'")
			case "changed-time":
				_, err = s.pool.Exec(ctx, "UPDATE external_operations SET updated_at=updated_at+interval '1 second' WHERE operation_id=$1", req.Identity.OperationID)
			case "unknown-field":
				_, err = s.pool.Exec(ctx, `UPDATE external_operations SET reconciliation_json=reconciliation_json || '{"unexpected":true}'::jsonb WHERE operation_id=$1`, req.Identity.OperationID)
			case "self-consistent-receipt":
				// Even a valid, self-hashed replacement receipt is not the audit's original
				// payload. This models a mismatched backup, not permission to edit history.
				saved.Reconciler = "urn:engineering-platform:operator:replacement"
				raw, x := json.Marshal(saved)
				if x != nil {
					t.Fatal(x)
				}
				_, err = s.pool.Exec(ctx, "UPDATE external_operations SET reconciliation_json=$2::jsonb WHERE operation_id=$1", req.Identity.OperationID, string(raw))
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.GetUninvokedPublication(ctx, req.Identity.OperationID); err == nil {
				t.Error("corrupt recovery readback accepted")
			}
			if _, err = s.CreateRecoveryProof(ctx, 1, "urn:engineering-platform:operator:proof"); err == nil {
				t.Error("corrupt recovery evidence cleared proof")
			}
			if _, err = s.AbandonUninvokedPublication(ctx, plannedReconciler, req, e); err == nil {
				t.Error("corrupt exact repeat accepted")
			}
		})
	}
}

func TestNoInvocationDatabaseWaitCancellationIsAtomic(t *testing.T) {
	forNoInvocationVersions(t, testNoInvocationDatabaseWaitCancellationIsAtomic)
}
func testNoInvocationDatabaseWaitCancellationIsAtomic(t *testing.T, version int) {
	for _, table := range []string{"platform_state", "external_operations", "audit_journal_state"} {
		t.Run(table, func(t *testing.T) {
			s, req, e := noInvocationDatabaseFixture(t, action.Manual, version)
			ctx := context.Background()
			before := actionIdentityAuditCount(t, s, req.Identity.OperationID)
			held, err := s.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer rollbackOutbox(held)
			// Names are compile-time test cases, not user-supplied SQL.
			if _, err = held.Exec(ctx, "SELECT 1 FROM "+table+" FOR UPDATE"); err != nil {
				t.Fatal(err)
			}
			bounded, cancel := context.WithTimeout(ctx, 80*time.Millisecond)
			defer cancel()
			if _, err = s.AbandonUninvokedPublication(bounded, plannedReconciler, req, e); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("caller deadline lost: %v", err)
			}
			if err = held.Rollback(ctx); err != nil {
				t.Fatal(err)
			}
			got, err := s.GetContext(ctx, req.Identity.OperationID)
			if err != nil || got.State != action.Manual || actionIdentityAuditCount(t, s, got.ID) != before {
				t.Fatal("cancelled recovery left business/audit residue", got, err)
			}
			if _, err = s.GetUninvokedPublication(ctx, got.ID); !errors.Is(err, corestore.ErrNotFound) {
				t.Fatal("cancelled recovery retained a receipt", err)
			}
		})
	}
}

func TestNoInvocationDatabaseAuditFailureRollsBack(t *testing.T) {
	forNoInvocationVersions(t, testNoInvocationDatabaseAuditFailureRollsBack)
}
func testNoInvocationDatabaseAuditFailureRollsBack(t *testing.T, version int) {
	s, req, e := noInvocationDatabaseFixture(t, action.Manual, version)
	ctx := context.Background()
	if _, err := s.pool.Exec(ctx, "DELETE FROM audit_journal_state"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AbandonUninvokedPublication(ctx, plannedReconciler, req, e); err == nil {
		t.Fatal("recovery succeeded without audit head")
	}
	op, err := s.GetContext(ctx, req.Identity.OperationID)
	if err != nil || op.State != action.Manual {
		t.Fatal("business update escaped failed audit", err)
	}
	if _, err = s.GetUninvokedPublication(ctx, op.ID); !errors.Is(err, corestore.ErrNotFound) {
		t.Fatal("receipt escaped failed audit", err)
	}
}

func TestNoInvocationDatabaseConcurrentSettlementIsSingle(t *testing.T) {
	forNoInvocationVersions(t, testNoInvocationDatabaseConcurrentSettlementIsSingle)
}
func testNoInvocationDatabaseConcurrentSettlementIsSingle(t *testing.T, version int) {
	s, req, e := noInvocationDatabaseFixture(t, action.Manual, version)
	ctx := context.Background()
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := s.AbandonUninvokedPublication(ctx, plannedReconciler, req, e)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
			continue
		}
		var pgerr *pgconn.PgError
		if !errors.As(err, &pgerr) || pgerr.Code != "40001" {
			t.Fatal("unexpected concurrent result", err)
		}
	}
	if success < 1 {
		t.Fatal("neither original settlement completed")
	}
	var count int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_type='recovery.github.publication.not_invoked'").Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate or missing audit", count, err)
	}
	got, err := s.GetUninvokedPublication(ctx, req.Identity.OperationID)
	if err != nil || got.Validate() != nil {
		t.Fatal("authoritative readback failed", err)
	}
	d, _ := canonical.Digest(got)
	again, err := s.AbandonUninvokedPublication(ctx, plannedReconciler, req, e)
	next, _ := canonical.Digest(again)
	if err != nil || next != d {
		t.Fatal("exact readback changed settlement", err)
	}
}

func TestNoInvocationDatabaseRealCommitAckLoss(t *testing.T) {
	forNoInvocationVersions(t, testNoInvocationDatabaseRealCommitAckLoss)
}
func testNoInvocationDatabaseRealCommitAckLoss(t *testing.T, version int) {
	original, req, e := noInvocationDatabaseFixture(t, action.Manual, version)
	cfg := original.pool.Config()
	host := cfg.ConnConfig.Host
	ip := net.ParseIP(host)
	if cfg.ConnConfig.TLSConfig != nil || (host != "localhost" && (ip == nil || !ip.IsLoopback())) {
		t.Skip("ACK cut only allowed on loopback plaintext fixture")
	}
	if !strings.HasPrefix(cfg.ConnConfig.RuntimeParams["search_path"], "ep_test_") {
		t.Fatal("refuse nonisolated fault injection")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	outcome := make(chan error, 1)
	go func() {
		defer listener.Close() // Never leave a second pool connection queued on an unserved listener.
		cutPostgresCommitAcknowledgement(listener, net.JoinHostPort(host, strconv.Itoa(int(cfg.ConnConfig.Port))), outcome)
	}()
	cfg.MaxConns = 1
	cfg.MinConns = 0
	cfg.ConnConfig.DialFunc = func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", listener.Addr().String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	uncertain, err := New(pool).AbandonUninvokedPublication(ctx, plannedReconciler, req, e)
	if !errors.Is(err, corestore.ErrCommitOutcomeUnknown) || uncertain.Kind != "" {
		t.Fatal("lost actual COMMIT ACK was not UNKNOWN", err)
	}
	select {
	case err := <-outcome:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("server COMMIT was not observed")
	}
	saved, err := original.GetUninvokedPublication(ctx, req.Identity.OperationID)
	if err != nil || saved.Validate() != nil {
		t.Fatal("committed receipt missing", err)
	}
	repeated, err := original.AbandonUninvokedPublication(ctx, plannedReconciler, req, e)
	if err != nil || repeated.EvidenceDigest != saved.EvidenceDigest || !repeated.RecordedAt.Equal(saved.RecordedAt) {
		t.Fatal("readback replayed or replaced settlement", err)
	}
	var count int
	if err := original.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events WHERE event_type='recovery.github.publication.not_invoked'").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

func TestNoInvocationDatabaseCompletionRechecksEvidence(t *testing.T) {
	forNoInvocationVersions(t, testNoInvocationDatabaseCompletionRechecksEvidence)
}
func testNoInvocationDatabaseCompletionRechecksEvidence(t *testing.T, version int) {
	s, req, e := noInvocationDatabaseFixture(t, action.Manual, version)
	ctx := context.Background()
	if _, err := s.AbandonUninvokedPublication(ctx, plannedReconciler, req, e); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRecoveryProof(ctx, 1, "urn:engineering-platform:operator:proof"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, "UPDATE external_operations SET observed_state='{}' WHERE operation_id=$1", req.Identity.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteRecoveryContext(ctx, 1, true); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Fatal("stale aggregate proof cleared corrupt evidence", err)
	}
	state, err := s.GetRecoveryContext(ctx)
	if err != nil || string(state.Mode) != "RECOVERY_RECONCILIATION" {
		t.Fatal("corrupt proof advanced Recovery", state, err)
	}
}
