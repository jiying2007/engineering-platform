package production

import (
	"fmt"
	"time"
)

const (
	OperationalStatusVersion = 1

	OperationalReady            = "READY"
	OperationalDegraded         = "DEGRADED"
	OperationalRecoveryRequired = "RECOVERY_REQUIRED"
)

type Snapshot struct {
	Version                int       `json:"version"`
	CapturedAt             time.Time `json:"captured_at"`
	RecoveryEpoch          uint64    `json:"recovery_epoch"`
	RecoveryMode           string    `json:"recovery_mode"`
	ActiveRuns             int64     `json:"active_runs"`
	PendingWorkerIntents   int64     `json:"pending_worker_intents"`
	ActiveWorkerLeases     int64     `json:"active_worker_leases"`
	ExpiredWorkerLeases    int64     `json:"expired_worker_leases"`
	PendingOutbox          int64     `json:"pending_outbox"`
	LeasedOutbox           int64     `json:"leased_outbox"`
	DeadLetterOutbox       int64     `json:"dead_letter_outbox"`
	UnknownOperations      int64     `json:"unknown_operations"`
	ReconcilingOperations  int64     `json:"reconciling_operations"`
	ManualOperations       int64     `json:"manual_operations"`
	UnknownCodexExecutions int64     `json:"unknown_codex_executions"`
}

type OperationalStatus struct {
	Version  int      `json:"version"`
	State    string   `json:"state"`
	Ready    bool     `json:"ready"`
	Reasons  []string `json:"reasons,omitempty"`
	Snapshot Snapshot `json:"snapshot"`
}

func EvaluateSnapshot(snapshot Snapshot) (OperationalStatus, error) {
	if snapshot.Version != OperationalStatusVersion || snapshot.CapturedAt.IsZero() ||
		snapshot.RecoveryEpoch > 1<<62 ||
		(snapshot.RecoveryMode != "NORMAL" && snapshot.RecoveryMode != "RECOVERY_RECONCILIATION") {
		return OperationalStatus{}, fmt.Errorf("invalid production operational snapshot")
	}
	counts := []int64{
		snapshot.ActiveRuns, snapshot.PendingWorkerIntents, snapshot.ActiveWorkerLeases,
		snapshot.ExpiredWorkerLeases, snapshot.PendingOutbox, snapshot.LeasedOutbox,
		snapshot.DeadLetterOutbox, snapshot.UnknownOperations, snapshot.ReconcilingOperations,
		snapshot.ManualOperations, snapshot.UnknownCodexExecutions,
	}
	for _, count := range counts {
		if count < 0 {
			return OperationalStatus{}, fmt.Errorf("negative operational counter")
		}
	}

	status := OperationalStatus{
		Version:  OperationalStatusVersion,
		State:    OperationalReady,
		Ready:    true,
		Snapshot: snapshot,
	}
	if snapshot.RecoveryMode != "NORMAL" {
		status.State = OperationalRecoveryRequired
		status.Ready = false
		status.Reasons = append(status.Reasons, "recovery_reconciliation_active")
	}
	checks := []struct {
		reason string
		count  int64
	}{
		{"expired_worker_leases", snapshot.ExpiredWorkerLeases},
		{"dead_letter_outbox", snapshot.DeadLetterOutbox},
		{"unknown_external_actions", snapshot.UnknownOperations},
		{"reconciling_actions", snapshot.ReconcilingOperations},
		{"manual_actions", snapshot.ManualOperations},
		{"unknown_codex_executions", snapshot.UnknownCodexExecutions},
	}
	for _, check := range checks {
		if check.count > 0 {
			status.Reasons = append(status.Reasons, check.reason)
		}
	}
	if len(status.Reasons) > 0 && status.State == OperationalReady {
		status.State = OperationalDegraded
		status.Ready = false
	}
	return status, nil
}
