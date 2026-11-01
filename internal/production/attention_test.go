package production

import (
	"testing"
	"time"
)

func TestOperationalAttentionSuppressesNormalProgressAndInFlightReservations(t *testing.T) {
	s := cleanSnapshot()
	s.CapturedAt = time.Now().UTC()
	s.ActiveRuns = 8
	s.PendingWorkerIntents = 3
	s.PendingOutbox = 2
	s.OldestPendingWorkerAt = &s.CapturedAt
	s.OldestPendingOutboxAt = &s.CapturedAt
	s.PlannedOperations = 1
	s.DispatchedOperations = 1
	status, err := EvaluateSnapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	if status.Ready || status.AuthorityClear {
		t.Fatal("existing status authority was incorrectly relaxed", status)
	}
	out, err := ProjectOperationalAttention(status, s.CapturedAt)
	if err != nil {
		t.Fatal(err)
	}
	if out.RequiresAttention || len(out.Items) != 0 || out.SchemaVersion != OperationalAttentionSchema {
		t.Fatal("ordinary work created false human attention", out)
	}
}

func TestOperationalAttentionProjectsOnlyActionableExistingFacts(t *testing.T) {
	s := cleanSnapshot()
	s.CapturedAt = time.Now().UTC()
	s.RecoveryMode = "RECOVERY_RECONCILIATION"
	s.UnknownOperations = 2
	s.ManualOperations = 1
	s.SafeToRetryOperations = 3
	s.UnknownOfflineExecutions = 1
	s.ExpiredWorkerLeases = 1
	s.DeadLetterOutbox = 5
	status, err := EvaluateSnapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ProjectOperationalAttention(status, s.CapturedAt)
	if err != nil {
		t.Fatal(err)
	}
	want := []AttentionItem{
		{"recovery_reconciliation_active", 1, "inspect_recovery_epoch"},
		{"unknown_external_actions", 2, "reconcile_action_outcome"},
		{"manual_actions", 1, "review_manual_action"},
		{"unqualified_safe_to_retry_actions", 3, "review_retry_authority"},
		{"unknown_offline_executions", 1, "investigate_execution_outcome"},
		{"expired_worker_leases", 1, "inspect_worker_lease"},
		{"dead_letter_outbox", 5, "inspect_dead_letter"},
	}
	if !out.RequiresAttention || len(out.Items) != len(want) {
		t.Fatalf("missing actionable facts: %#v", out)
	}
	for i := range want {
		if out.Items[i] != want[i] {
			t.Fatalf("non-deterministic attention at %d: got %#v want %#v", i, out.Items[i], want[i])
		}
	}
	second, err := ProjectOperationalAttention(status, s.CapturedAt.Add(2*time.Second))
	if err != nil || len(second.Items) != len(out.Items) || second.CapturedAt != out.CapturedAt {
		t.Fatalf("same snapshot should be stable: %#v %v", second, err)
	}
}

func TestOperationalAttentionRejectsUntrustedAndStaleStatus(t *testing.T) {
	s := cleanSnapshot()
	status, err := EvaluateSnapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	forged := status
	forged.Ready = true
	if _, err := ProjectOperationalAttention(forged, s.CapturedAt); err == nil {
		t.Fatal("forged READY was accepted as source")
	}
	for _, at := range []time.Time{s.CapturedAt.Add(31 * time.Second), s.CapturedAt.Add(-6 * time.Second), {}} {
		if _, err := ProjectOperationalAttention(status, at); err == nil {
			t.Fatal("stale or invalid observation accepted", at)
		}
	}
}
