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
		WorkerPolls:  []WorkerPollObservation{},
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

func TestEvaluateSnapshotValidatesWorkerPollFactsWithoutInferringReadiness(t *testing.T) {
	s := cleanSnapshot()
	t1 := s.CapturedAt.Add(-2 * time.Second)
	t2 := s.CapturedAt.Add(-time.Second)
	s.WorkerPolls = []WorkerPollObservation{
		{WorkerProfile: "worker/admission-production", KnownIdentities: 2, LatestPollAt: t1},
		{WorkerProfile: "worker/codex-production", KnownIdentities: 1, LatestPollAt: t2},
	}
	status, err := EvaluateSnapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	if status.Ready || status.ProductionQualified || status.ServiceReadiness != ServiceReadinessUnobserved ||
		len(status.Snapshot.WorkerPolls) != 2 {
		t.Fatal("worker poll facts became readiness", status)
	}
	bad := []Snapshot{s, s, s, s}
	bad[0].WorkerPolls = nil
	bad[1].WorkerPolls = []WorkerPollObservation{{WorkerProfile: "worker/x", KnownIdentities: 0, LatestPollAt: t1}}
	bad[2].WorkerPolls = []WorkerPollObservation{
		{WorkerProfile: "worker/z", KnownIdentities: 1, LatestPollAt: t1},
		{WorkerProfile: "worker/a", KnownIdentities: 1, LatestPollAt: t2},
	}
	bad[3].WorkerPolls = []WorkerPollObservation{{WorkerProfile: "worker/x", KnownIdentities: 1, LatestPollAt: s.CapturedAt.Add(time.Second)}}
	for i, candidate := range bad {
		if _, err := EvaluateSnapshot(candidate); err == nil {
			t.Fatal("invalid worker poll facts accepted", i)
		}
	}
}

func TestOperationalStatusV4NeverHidesPreEffectOrInFlightActions(t *testing.T) {
	if OperationalStatusVersion != 4 {
		t.Fatal("PLANNED/DISPATCHED facts require a distinct v4 snapshot")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Snapshot)
		reason string
	}{
		{"planned", func(s *Snapshot) { s.PlannedOperations = 1 }, "planned_external_actions"},
		{"dispatched", func(s *Snapshot) { s.DispatchedOperations = 1 }, "dispatched_external_actions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := cleanSnapshot()
			tc.mutate(&s)
			status, err := EvaluateSnapshot(s)
			if err != nil || status.AuthorityClear || status.Ready ||
				status.State != OperationalDegraded || status.ProductionQualified {
				t.Fatalf("unsettled action hidden from operational status: %#v err=%v", status, err)
			}
			found := false
			for _, reason := range status.Reasons {
				found = found || reason == tc.reason
			}
			if !found {
				t.Fatalf("missing exact unsettled action reason: %#v", status.Reasons)
			}
			if err := status.ValidateAt(s.CapturedAt); err != nil {
				t.Fatal(err)
			}
			// A tampered transport envelope must not conceal a durable action.
			forged := status
			forged.AuthorityClear, forged.Ready = true, true
			if forged.ValidateAt(s.CapturedAt) == nil {
				t.Fatal("forged authority-clear envelope was trusted")
			}
		})
	}
	for _, mutate := range []func(*Snapshot){
		func(s *Snapshot) { s.PlannedOperations = -1 },
		func(s *Snapshot) { s.DispatchedOperations = -1 },
	} {
		s := cleanSnapshot()
		mutate(&s)
		if _, err := EvaluateSnapshot(s); err == nil {
			t.Fatal("negative pre-effect or in-flight Action count accepted")
		}
	}
}
