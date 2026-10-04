package production

import (
	"testing"
	"time"
)

func cleanSnapshot() Snapshot {
	return Snapshot{
		Version:      OperationalStatusVersion,
		CapturedAt:   time.Unix(1700000000, 0).UTC(),
		RecoveryMode: "NORMAL",
	}
}

func TestEvaluateSnapshotDoesNotInferReadinessFromBacklog(t *testing.T) {
	s := cleanSnapshot()
	s.ActiveRuns = 2
	s.PendingWorkerIntents = 3
	s.PendingOutbox = 4
	s.OldestPendingWorkerAt = &s.CapturedAt
	s.OldestPendingOutboxAt = &s.CapturedAt
	status, err := EvaluateSnapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	if status.Ready || !status.AuthorityClear || status.State != OperationalObservationRequired || len(status.Reasons) != 1 {
		t.Fatalf("unexpected status: %#v", status)
	}
}

func TestEvaluateSnapshotBlocksOnRecoveryOrAmbiguousEffects(t *testing.T) {
	s := cleanSnapshot()
	s.RecoveryMode = "RECOVERY_RECONCILIATION"
	status, err := EvaluateSnapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	if status.Ready || status.State != OperationalRecoveryRequired {
		t.Fatalf("recovery did not block readiness: %#v", status)
	}

	s = cleanSnapshot()
	s.UnknownOperations = 1
	s.DeadLetterOutbox = 2
	s.UnknownCodexExecutions = 1
	status, err = EvaluateSnapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	if status.Ready || status.State != OperationalDegraded || len(status.Reasons) != 4 {
		t.Fatalf("ambiguous effects did not degrade readiness: %#v", status)
	}
}

func TestEvaluateSnapshotRejectsInvalidFacts(t *testing.T) {
	s := cleanSnapshot()
	s.PendingOutbox = -1
	if _, err := EvaluateSnapshot(s); err == nil {
		t.Fatal("negative operational fact accepted")
	}
	s = cleanSnapshot()
	s.RecoveryMode = "UNKNOWN"
	if _, err := EvaluateSnapshot(s); err == nil {
		t.Fatal("unknown recovery mode accepted")
	}
}

func TestOperationalStatusRejectsMisleadingOrStaleSummary(t *testing.T) {
	snapshot := cleanSnapshot()
	snapshot.ActiveRuns, snapshot.PendingWorkerIntents, snapshot.PendingOutbox = 1000, 1000, 1000
	old := snapshot.CapturedAt.Add(-time.Hour)
	snapshot.OldestPendingWorkerAt, snapshot.OldestPendingOutboxAt = &old, &old
	status, err := EvaluateSnapshot(snapshot)
	if err != nil || status.Ready || status.ProductionQualified || status.ServiceReadiness != ServiceReadinessUnobserved {
		t.Fatal(status, err)
	}
	if err := status.ValidateAt(snapshot.CapturedAt); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*OperationalStatus){
		func(s *OperationalStatus) { s.Ready = true },
		func(s *OperationalStatus) { s.State = "READY" },
		func(s *OperationalStatus) { s.AuthorityState = "READY" },
		func(s *OperationalStatus) { s.Scope = "production" },
		func(s *OperationalStatus) { s.ServiceReadiness = "READY" },
		func(s *OperationalStatus) { s.ProductionQualified = true },
		func(s *OperationalStatus) { s.Reasons = nil },
		func(s *OperationalStatus) { s.Version = 1 },
	} {
		bad := status
		change(&bad)
		if bad.ValidateAt(snapshot.CapturedAt) == nil {
			t.Fatal("contradictory status admitted", bad)
		}
	}
	for _, now := range []time.Time{snapshot.CapturedAt.Add(31 * time.Second), snapshot.CapturedAt.Add(-6 * time.Second), {}} {
		if status.ValidateAt(now) == nil {
			t.Fatal("stale or future status admitted")
		}
	}
	snapshot.OldestPendingWorkerAt = nil
	if _, err = EvaluateSnapshot(snapshot); err == nil {
		t.Fatal("missing queue age admitted")
	}
	snapshot.PendingWorkerIntents = 0
	future := snapshot.CapturedAt.Add(time.Second)
	snapshot.LastOutboxDispatchAt = &future
	if _, err = EvaluateSnapshot(snapshot); err == nil {
		t.Fatal("future database progress admitted")
	}
}
