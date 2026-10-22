package postgres

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/production"
)

func TestReadOperationalStatusTracksRecoveryAndUnknownAction(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	ctx := context.Background()

	status, err := s.ReadOperationalStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Ready || !status.AuthorityClear || status.State != "OBSERVATION_REQUIRED" {
		t.Fatalf("fresh store only establishes database authority health: %#v", status)
	}

	if _, err := s.pool.Exec(ctx, `UPDATE platform_state SET recovery_mode='RECOVERY_RECONCILIATION' WHERE singleton_id=true`); err != nil {
		t.Fatal(err)
	}
	status, err = s.ReadOperationalStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Ready || status.State != "RECOVERY_REQUIRED" {
		t.Fatalf("recovery mode did not block readiness: %#v", status)
	}

	if _, err := s.pool.Exec(ctx, `UPDATE platform_state SET recovery_mode='NORMAL' WHERE singleton_id=true`); err != nil {
		t.Fatal(err)
	}
	runID := setupPostgresActionRun(t, s, "operational-status")
	now := time.Now().UTC()
	req := action.Request{
		ID: "status-unknown-operation", RunID: runID, ExecutionEpoch: 1,
		RecoveryEpoch: 0, Action: "github.publish-pr",
		RiskClass: action.ControlledMutation, Capability: "github.publish-pr",
		ParametersDigest: "sha256:" + strings.Repeat("a", 64),
		IdempotencyKey:   "status-unknown-operation",
		RequestedBy:      "runtime", RequestedAt: now,
	}
	op := action.NewWithRequestDigest(req, "sha256:"+strings.Repeat("b", 64), now)
	if err := s.CreateContext(context.Background(), *op); err != nil {
		t.Fatal(err)
	}
	status, err = s.ReadOperationalStatus(ctx)
	if err != nil || status.AuthorityClear || status.Ready || status.Snapshot.PlannedOperations != 1 ||
		status.Snapshot.DispatchedOperations != 0 || status.State != "DEGRADED" {
		t.Fatalf("PLANNED operation concealed from status: %#v err=%v", status, err)
	}
	if err := op.Transition(action.Dispatched, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateContext(context.Background(), *op); err != nil {
		t.Fatal(err)
	}
	status, err = s.ReadOperationalStatus(ctx)
	if err != nil || status.AuthorityClear || status.Ready || status.Snapshot.PlannedOperations != 0 ||
		status.Snapshot.DispatchedOperations != 1 || status.State != "DEGRADED" {
		t.Fatalf("DISPATCHED operation concealed from status: %#v err=%v", status, err)
	}
	if err := op.Transition(action.Unknown, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateContext(context.Background(), *op); err != nil {
		t.Fatal(err)
	}

	status, err = s.ReadOperationalStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Ready || status.State != "DEGRADED" || status.Snapshot.UnknownOperations != 1 {
		t.Fatalf("UNKNOWN action did not degrade readiness: %#v", status)
	}
	if err := op.Transition(action.Reconciling, now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateContext(context.Background(), *op); err != nil {
		t.Fatal(err)
	}
	seedHistoricalSafeToRetry(t, s, op.ID)
	status, err = s.ReadOperationalStatus(ctx)
	if err != nil || status.Ready || status.AuthorityClear ||
		status.Snapshot.SafeToRetryOperations != 1 || status.State != "DEGRADED" {
		t.Fatalf("unproven SAFE_TO_RETRY concealed from operational status: %#v err=%v", status, err)
	}
}

func TestReadOperationalStatusExposesIdleWorkerPollFactsWithoutReadiness(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	ctx := context.Background()
	_, err := s.pool.Exec(ctx, `INSERT INTO workers(worker_id,protocol_version,runtime_providers,status,attributes_json,last_seen_at)
 VALUES
 ('admit-a','test','[]','ONLINE','{"worker_profile":"worker/admission-production"}',clock_timestamp()-interval '3 seconds'),
 ('admit-b','test','[]','ONLINE','{"worker_profile":"worker/admission-production"}',clock_timestamp()-interval '1 second'),
 ('prepare-a','test','[]','ONLINE','{"worker_profile":"worker/codex-production"}',clock_timestamp()-interval '2 seconds')`)
	if err != nil {
		t.Fatal(err)
	}
	status, err := s.ReadOperationalStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	polls := status.Snapshot.WorkerPolls
	if status.Ready || status.ProductionQualified || len(polls) != 2 ||
		polls[0].WorkerProfile != "worker/admission-production" || polls[0].KnownIdentities != 2 ||
		polls[1].WorkerProfile != "worker/codex-production" || polls[1].KnownIdentities != 1 ||
		polls[0].LatestPollAt.After(status.Snapshot.CapturedAt) || polls[1].LatestPollAt.After(status.Snapshot.CapturedAt) {
		t.Fatal("worker poll observation drift", status)
	}
}

func TestReadOperationalStatusReportsExecutionReservationsWithoutMutation(t *testing.T) {
	for _, lane := range []string{"offline", "codex"} {
		for _, unknown := range []bool{false, true} {
			name := lane + "/expired-authorized"
			if unknown {
				name = lane + "/unknown"
			}
			t.Run(name, func(t *testing.T) {
				s := newIsolatedIntegrationStore(t)
				ctx := context.Background()
				var table, runID string
				var fail, retry func() error
				if lane == "offline" {
					start := offlineFixture(t, s)
					permit, err := s.StartOffline(ctx, "offline-worker", start)
					workerOK(t, err)
					table, runID = "worker_offline_executions", start.RunID
					fail = func() error { return s.FailOffline(ctx, "offline-worker", permit.Token) }
					retry = func() error { _, err := s.StartOffline(ctx, "offline-worker", start); return err }
				} else {
					start := codexFixture(t, s)
					permit, err := s.StartCodex(ctx, codexTestWorker, start)
					workerOK(t, err)
					table, runID = "worker_codex_executions", start.RunID
					fail = func() error { return s.FailCodex(ctx, codexTestWorker, permit.Token) }
					retry = func() error { _, err := s.StartCodex(ctx, codexTestWorker, start); return err }
				}
				status, err := s.ReadOperationalStatus(ctx)
				workerOK(t, err)
				if !status.AuthorityClear || status.Ready || status.ProductionQualified {
					t.Fatalf("live execution incorrectly classified: %#v", status)
				}
				wantState, wantReason := "AUTHORIZED", "expired_"+lane+"_executions"
				if unknown {
					workerOK(t, fail())
					wantState, wantReason = "UNKNOWN", "unknown_"+lane+"_executions"
					// UNKNOWN must block even before its reservation lease expires.
					status, err = s.ReadOperationalStatus(ctx)
					workerOK(t, err)
					if status.AuthorityClear || !slices.Contains(status.Reasons, wantReason) {
						t.Fatalf("live UNKNOWN hidden: %#v", status)
					}
				}
				workerSQL(t, s, "UPDATE "+table+" SET lease_until=clock_timestamp()-interval '1 second' WHERE run_id=$1", runID)
				var auditsBefore int64
				workerOK(t, s.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events").Scan(&auditsBefore))
				for n := 0; n < 2; n++ {
					status, err = s.ReadOperationalStatus(ctx)
					workerOK(t, err)
					if status.AuthorityClear || status.Ready || status.ProductionQualified || status.State != production.OperationalDegraded || !slices.Contains(status.Reasons, wantReason) {
						t.Fatalf("unresolved execution concealed: %#v", status)
					}
					counts := map[string]int64{
						"unknown_offline_executions": status.Snapshot.UnknownOfflineExecutions,
						"unknown_codex_executions":   status.Snapshot.UnknownCodexExecutions,
						"expired_offline_executions": status.Snapshot.ExpiredOfflineExecutions,
						"expired_codex_executions":   status.Snapshot.ExpiredCodexExecutions,
					}
					for reason, count := range counts {
						want := int64(0)
						if reason == wantReason {
							want = 1
						}
						if count != want {
							t.Fatalf("execution counted in wrong lane/state: %s=%d, want %d", reason, count, want)
						}
					}
				}
				var storedState string
				var hasReceipt bool
				workerOK(t, s.pool.QueryRow(ctx, "SELECT state,receipt_json IS NOT NULL FROM "+table+" WHERE run_id=$1", runID).Scan(&storedState, &hasReceipt))
				if storedState != wantState || hasReceipt {
					t.Fatalf("diagnostic mutated execution disposition: state=%s receipt=%v", storedState, hasReceipt)
				}
				var auditsAfter int64
				workerOK(t, s.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events").Scan(&auditsAfter))
				if auditsAfter != auditsBefore {
					t.Fatal("read-only status wrote an authority event")
				}
				if retry() == nil {
					t.Fatal("observing expired/unknown execution authorized replay")
				}
			})
		}
	}
}

func TestReadOperationalStatusReportsExpiredOutboxWithoutReclaim(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	ctx := context.Background()
	queueMessage(t, s, "status-outbox-expiry", action.Observe)
	message := claimOne(t, s, "dispatcher")
	status, err := s.ReadOperationalStatus(ctx)
	workerOK(t, err)
	if !status.AuthorityClear || status.Snapshot.LeasedOutbox != 1 || status.Snapshot.ExpiredOutboxLeases != 0 {
		t.Fatalf("live outbox lease incorrectly classified: %#v", status)
	}
	expireLease(t, s, message.ID)
	for n := 0; n < 2; n++ {
		status, err = s.ReadOperationalStatus(ctx)
		workerOK(t, err)
		if status.AuthorityClear || status.Ready || status.ProductionQualified ||
			status.Snapshot.LeasedOutbox != 1 || status.Snapshot.ExpiredOutboxLeases != 1 ||
			!slices.Contains(status.Reasons, "expired_outbox_leases") {
			t.Fatalf("expired leased outbox hidden: %#v", status)
		}
	}
	stored, err := s.GetOutbox(ctx, message.Key)
	workerOK(t, err)
	if stored.State != message.State || stored.AttemptCount != message.AttemptCount || stored.LeaseOwner != message.LeaseOwner || !stored.DispatchedAt.IsZero() {
		t.Fatalf("read-only diagnostic changed outbox generation/disposition: %#v", stored)
	}
}
