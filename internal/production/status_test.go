package production

import (
	"testing"
	"time"
)

func cleanSnapshot() Snapshot {
	return Snapshot{
		Version: OperationalStatusVersion,
		CapturedAt: time.Unix(1700000000, 0).UTC(),
		RecoveryMode: "NORMAL",
	}
}

func TestEvaluateSnapshotReadyWithBenignBacklog(t *testing.T) {
	s := cleanSnapshot()
	s.ActiveRuns = 2
	s.PendingWorkerIntents = 3
	s.PendingOutbox = 4
	status, err := EvaluateSnapshot(s)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Ready || status.State != OperationalReady || len(status.Reasons) != 0 {
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
	if status.Ready || status.State != OperationalDegraded || len(status.Reasons) != 3 {
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
