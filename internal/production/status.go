package production

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

const (
	OperationalStatusVersion = 4

	OperationalObservationRequired = "OBSERVATION_REQUIRED"
	OperationalAuthorityClear      = "CLEAR"
	OperationalScope               = "database-authority-snapshot"
	ServiceReadinessUnobserved     = "NOT_OBSERVED"
	OperationalDegraded            = "DEGRADED"
	OperationalRecoveryRequired    = "RECOVERY_REQUIRED"
)

type WorkerPollObservation struct {
	WorkerProfile   string    `json:"worker_profile"`
	KnownIdentities int64     `json:"known_identities"`
	LatestPollAt    time.Time `json:"latest_poll_at"`
}

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
	PlannedOperations      int64     `json:"planned_operations"`
	DispatchedOperations   int64     `json:"dispatched_operations"`
	UnknownOperations      int64     `json:"unknown_operations"`
	ReconcilingOperations  int64     `json:"reconciling_operations"`
	ManualOperations       int64     `json:"manual_operations"`
	UnknownCodexExecutions int64     `json:"unknown_codex_executions"`
	// Claim attempts update this existing database fact even when no work is
	// available. It is poll history, not a freshness threshold or capacity claim.
	WorkerPolls []WorkerPollObservation `json:"worker_polls"`
	// Database observations, not component heartbeats or capacity claims.
	OldestPendingWorkerAt *time.Time `json:"oldest_pending_worker_at"`
	OldestPendingOutboxAt *time.Time `json:"oldest_pending_outbox_at"`
	LastWorkerAdmissionAt *time.Time `json:"last_worker_admission_at"`
	LastOutboxDispatchAt  *time.Time `json:"last_outbox_dispatch_at"`
}

type OperationalStatus struct {
	Version             int      `json:"version"`
	Scope               string   `json:"scope"`
	AuthorityState      string   `json:"authority_state"`
	AuthorityClear      bool     `json:"authority_clear"`
	ServiceReadiness    string   `json:"service_readiness"`
	ProductionQualified bool     `json:"production_qualified"`
	State               string   `json:"state"`
	Ready               bool     `json:"ready"`
	Reasons             []string `json:"reasons,omitempty"`
	Snapshot            Snapshot `json:"snapshot"`
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
		snapshot.DeadLetterOutbox, snapshot.PlannedOperations, snapshot.DispatchedOperations,
		snapshot.UnknownOperations, snapshot.ReconcilingOperations,
		snapshot.ManualOperations, snapshot.UnknownCodexExecutions,
	}
	for _, count := range counts {
		if count < 0 {
			return OperationalStatus{}, fmt.Errorf("negative operational counter")
		}
	}
	if snapshot.WorkerPolls == nil || len(snapshot.WorkerPolls) > 256 {
		return OperationalStatus{}, fmt.Errorf("worker poll observations required and bounded")
	}
	previousProfile := ""
	for _, observed := range snapshot.WorkerPolls {
		if observed.WorkerProfile == "" || len(observed.WorkerProfile) > 128 ||
			strings.TrimSpace(observed.WorkerProfile) != observed.WorkerProfile ||
			strings.ContainsAny(observed.WorkerProfile, " \t\r\n\x00") ||
			observed.WorkerProfile <= previousProfile || observed.KnownIdentities <= 0 ||
			observed.LatestPollAt.IsZero() || observed.LatestPollAt.After(snapshot.CapturedAt) {
			return OperationalStatus{}, fmt.Errorf("invalid worker poll observation")
		}
		previousProfile = observed.WorkerProfile
	}

	for _, at := range []*time.Time{snapshot.OldestPendingWorkerAt, snapshot.OldestPendingOutboxAt, snapshot.LastWorkerAdmissionAt, snapshot.LastOutboxDispatchAt} {
		if at != nil && (at.IsZero() || at.After(snapshot.CapturedAt)) {
			return OperationalStatus{}, fmt.Errorf("invalid database observation time")
		}
	}
	if (snapshot.PendingWorkerIntents > 0) != (snapshot.OldestPendingWorkerAt != nil) || (snapshot.PendingOutbox > 0) != (snapshot.OldestPendingOutboxAt != nil) {
		return OperationalStatus{}, fmt.Errorf("queue count/oldest observation mismatch")
	}
	status := OperationalStatus{
		Version:          OperationalStatusVersion,
		Scope:            OperationalScope,
		AuthorityState:   OperationalAuthorityClear,
		AuthorityClear:   true,
		ServiceReadiness: ServiceReadinessUnobserved,
		State:            OperationalObservationRequired,
		Ready:            false,
		Snapshot:         snapshot,
	}
	if snapshot.RecoveryMode != "NORMAL" {
		status.State = OperationalRecoveryRequired
		status.AuthorityState = OperationalRecoveryRequired
		status.AuthorityClear = false
		status.Reasons = append(status.Reasons, "recovery_reconciliation_active")
	}
	checks := []struct {
		reason string
		count  int64
	}{
		{"expired_worker_leases", snapshot.ExpiredWorkerLeases},
		{"dead_letter_outbox", snapshot.DeadLetterOutbox},
		// PLANNED and DISPATCHED are live reservations during normal dispatch,
		// but they are also the durable survivors of an interrupted process.
		// Never claim an authority-clear quiescent snapshot while either exists.
		{"planned_external_actions", snapshot.PlannedOperations},
		{"dispatched_external_actions", snapshot.DispatchedOperations},
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
	if len(status.Reasons) > 0 && status.AuthorityState == OperationalAuthorityClear {
		status.State = OperationalDegraded
		status.AuthorityState = OperationalDegraded
		status.AuthorityClear = false
	}
	// Even an idle database does not prove admission/preparation/execution/publisher
	// availability. Leases and historical progress must never stand in for heartbeats.
	status.Reasons = append(status.Reasons, "service_health_and_capacity_not_observed")
	return status, nil
}

// ValidateAt rederives every status field from the bound database facts and rejects
// stale or future observations. These are transport freshness limits, NOT SLOs.
func (s OperationalStatus) ValidateAt(now time.Time) error {
	expected, err := EvaluateSnapshot(s.Snapshot)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(s, expected) {
		return fmt.Errorf("operational status contradicts its snapshot or scope")
	}
	if now.IsZero() || s.Snapshot.CapturedAt.After(now.Add(5*time.Second)) || now.Sub(s.Snapshot.CapturedAt) > 30*time.Second {
		return fmt.Errorf("operational snapshot stale or clock skewed")
	}
	return nil
}
