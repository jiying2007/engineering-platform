package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/action"
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
	if err := s.Create(*op); err != nil {
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
	if err := s.Update(*op); err != nil {
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
	if err := s.Update(*op); err != nil {
		t.Fatal(err)
	}

	status, err = s.ReadOperationalStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.Ready || status.State != "DEGRADED" || status.Snapshot.UnknownOperations != 1 {
		t.Fatalf("UNKNOWN action did not degrade readiness: %#v", status)
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
