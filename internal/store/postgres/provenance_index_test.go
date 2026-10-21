package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/audit"
	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func assertProvenanceIndex(t *testing.T, s *Store) {
	t.Helper()
	var valid bool
	if err := s.pool.QueryRow(context.Background(), `SELECT indisvalid AND indisready FROM pg_catalog.pg_index WHERE indexrelid=pg_catalog.to_regclass('audit_external_operation_provenance_idx')`).Scan(&valid); err != nil || !valid {
		t.Fatal("usable provenance lookup index missing", err)
	}
}

func TestProvenanceIndexUpgradePreservesHistoricalRowsAndIsIdempotent(t *testing.T) {
	s, op, req := actionPlannedRecoveryFixture(t)
	ctx := context.Background()
	if err := s.CreateContext(ctx, *op); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginRecoveryContext(ctx, 0); err != nil {
		t.Fatal(err)
	}
	receipt, err := s.ReconcilePlannedActionAbandoned(ctx, plannedReconciler, req)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := canonical.Digest(receipt)
	var sequence uint64
	var head string
	if err = s.pool.QueryRow(ctx, "SELECT last_sequence,last_digest FROM audit_journal_state WHERE singleton_id=true").Scan(&sequence, &head); err != nil {
		t.Fatal(err)
	}
	// Only this random schema is rolled back to the pre-index v13 shape.
	if _, err = s.pool.Exec(ctx, `DROP INDEX audit_external_operation_provenance_idx; DELETE FROM core_schema_migrations WHERE version=14`, pgx.QueryExecModeSimpleProtocol); err != nil {
		t.Fatal(err)
	}
	if err = s.CheckWorkerSchema(ctx); err == nil {
		t.Fatal("old schema admitted by new binary")
	}
	for n := 0; n < 2; n++ {
		if err = s.ApplyCoreMigration(ctx); err != nil {
			t.Fatal(err)
		}
		if err = s.CheckWorkerSchema(ctx); err != nil {
			t.Fatal(err)
		}
		assertProvenanceIndex(t, s)
		got, err := s.GetPlannedActionAbandonReceipt(ctx, op.ID)
		if err != nil {
			t.Fatal(err)
		}
		after, _ := canonical.Digest(got)
		if before != after {
			t.Fatal("index migration rewrote historical receipt")
		}
		var gotSequence uint64
		var gotHead string
		if err = s.pool.QueryRow(ctx, "SELECT last_sequence,last_digest FROM audit_journal_state WHERE singleton_id=true").Scan(&gotSequence, &gotHead); err != nil || gotSequence != sequence || gotHead != head {
			t.Fatal("index migration changed historical journal", err)
		}
	}
}

// A bounded history-growth regression, not a production SLO benchmark. All
// confirmations and audit markers are valid. The public status deadline is
// exercised unchanged, and an oldest-row negative control prevents skipping
// historical integrity checks as a query-performance shortcut.
func TestOperationalStatusValidHistoryGrowthRetainsOldestProvenance(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	ctx := context.Background()
	runID := setupPostgresActionRun(t, s, "provenance-history")
	assertProvenanceIndex(t, s)
	var sequence uint64
	var previous string
	if err := s.pool.QueryRow(ctx, "SELECT last_sequence,last_digest FROM audit_journal_state WHERE singleton_id=true").Scan(&sequence, &previous); err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, target := range []int{500, 2000, 5000} {
		operationRows := make([][]any, 0, target-total)
		auditRows := make([][]any, 0, (target-total)*5)
		for n := total; n < target; n++ {
			stamp := time.Now().UTC().Truncate(time.Microsecond)
			id := fmt.Sprintf("history-op-%d", n)
			op := action.Operation{ID: id, RunID: runID, ExecutionEpoch: 1, RecoveryEpoch: 0, Action: "ci.dispatch", RiskClass: action.ControlledMutation, Capability: "ci.dispatch", IdempotencyKey: id + "-key", RequestDigest: canonical.BytesDigest([]byte(id)), State: action.Confirmed, UpdatedAt: stamp}
			operationRows = append(operationRows, []any{op.ID, op.RunID, op.ExecutionEpoch, op.RecoveryEpoch, op.Action, string(op.RiskClass), op.Capability, op.IdempotencyKey, op.RequestDigest, string(op.State), nil, nil, op.UpdatedAt})
			payload, err := canonical.Digest(op)
			if err != nil {
				t.Fatal(err)
			}
			for j := 0; j < 5; j++ {
				input := audit.Input{Type: "fixture.noise", AggregateType: "Fixture", AggregateID: id, PayloadDigest: canonical.BytesDigest([]byte(fmt.Sprintf("%s-%d", id, j)))}
				if j == 0 {
					input = audit.Input{Type: "action.state_changed", AggregateType: "ExternalOperation", AggregateID: id, PayloadDigest: payload, CausationID: originalConfirmationCause}
				}
				sequence++
				event, err := audit.Build(sequence, previous, input, stamp.Add(time.Microsecond))
				if err != nil {
					t.Fatal(err)
				}
				previous = event.Digest
				auditRows = append(auditRows, []any{event.Sequence, event.Type, event.AggregateType, event.AggregateID, event.PayloadDigest, nullIfEmpty(event.PreviousDigest), event.Digest, nullIfEmpty(event.CorrelationID), nullIfEmpty(event.CausationID), event.CreatedAt})
			}
		}
		if _, err := s.pool.CopyFrom(ctx, pgx.Identifier{"external_operations"}, []string{"operation_id", "run_id", "execution_epoch", "recovery_epoch", "action", "risk_class", "capability", "idempotency_key", "request_digest", "state", "external_ref", "observed_state", "updated_at"}, pgx.CopyFromRows(operationRows)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.pool.CopyFrom(ctx, pgx.Identifier{"audit_events"}, []string{"sequence", "event_type", "aggregate_type", "aggregate_id", "payload_digest", "previous_digest", "event_digest", "correlation_id", "causation_id", "created_at"}, pgx.CopyFromRows(auditRows)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.pool.Exec(ctx, `UPDATE audit_journal_state SET last_sequence=$1,last_digest=$2 WHERE singleton_id=true`, sequence, previous); err != nil {
			t.Fatal(err)
		}
		if _, err := s.pool.Exec(ctx, `ANALYZE audit_events; ANALYZE external_operations`, pgx.QueryExecModeSimpleProtocol); err != nil {
			t.Fatal(err)
		}
		total = target
		started := time.Now()
		status, err := s.ReadOperationalStatus(ctx)
		t.Logf("valid_history operations=%d audit_events=%d elapsed=%s error=%v", total, sequence, time.Since(started), err)
		if err != nil || !status.AuthorityClear {
			t.Fatalf("valid history did not pass full status validation: %+v %v", status, err)
		}
	}
	rows, err := s.pool.Query(ctx, "EXPLAIN (FORMAT TEXT) "+ordinaryConfirmationSelect)
	if err != nil {
		t.Fatal(err)
	}
	var plan []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	t.Log("ordinary_confirmation_plan\n" + strings.Join(plan, "\n"))
	if _, err = s.BeginRecoveryContext(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateRecoveryProof(ctx, 1, "history-proof"); err != nil {
		t.Fatal("history blocked bounded Recovery proof", err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE audit_events SET payload_digest=$1 WHERE aggregate_type='ExternalOperation' AND aggregate_id='history-op-0' AND causation_id=$2`, canonical.BytesDigest([]byte("oldest-confirmation-corruption")), originalConfirmationCause); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReadOperationalStatus(ctx); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Fatal("optimized status skipped oldest damaged history", err)
	}
	if _, err = s.CompleteRecoveryContext(ctx, 1, true); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Fatal("optimized Recovery skipped oldest damaged history", err)
	}
}
