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
	if !status.Ready || status.State != "READY" {
		t.Fatalf("fresh store should be READY: %#v", status)
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
	if err := op.Transition(action.Dispatched, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(*op); err != nil {
		t.Fatal(err)
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
