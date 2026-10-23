package production

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

const (
	OperationalStatusVersion = 6

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
	Version                  int       `json:"version"`
	CapturedAt               time.Time `json:"captured_at"`
	RecoveryEpoch            uint64    `json:"recovery_epoch"`
	RecoveryMode             string    `json:"recovery_mode"`
	ActiveRuns               int64     `json:"active_runs"`
	PendingWorkerIntents     int64     `json:"pending_worker_intents"`
	ActiveWorkerLeases       int64     `json:"active_worker_leases"`
	ExpiredWorkerLeases      int64     `json:"expired_worker_leases"`
	PendingOutbox            int64     `json:"pending_outbox"`
	LeasedOutbox             int64     `json:"leased_outbox"`
	ExpiredOutboxLeases      int64     `json:"expired_outbox_leases"`
	DeadLetterOutbox         int64     `json:"dead_letter_outbox"`
	PlannedOperations        int64     `json:"planned_operations"`
	DispatchedOperations     int64     `json:"dispatched_operations"`
	UnknownOperations        int64     `json:"unknown_operations"`
	ReconcilingOperations    int64     `json:"reconciling_operations"`
	ManualOperations         int64     `json:"manual_operations"`
	SafeToRetryOperations    int64     `json:"safe_to_retry_operations"`
	UnknownCodexExecutions   int64     `json:"unknown_codex_executions"`
	UnknownOfflineExecutions int64     `json:"unknown_offline_executions"`
	// Expired execution counts include only AUTHORIZED reservations. An expired
	// lease is a diagnostic blocker, never evidence of process exit or permission
	// to retry. UNKNOWN executions are counted separately, regardless of expiry.
	ExpiredOfflineExecutions int64 `json:"expired_offline_executions"`
	ExpiredCodexExecutions   int64 `json:"expired_codex_executions"`
	// Claim attempts update this existing database fact even when no work is
	// available. It is poll history, not a freshness threshold or capacity claim.
	WorkerPolls []WorkerPollObservation `json:"worker_polls"`
	// Database observations, not component heartbeats or capacity claims.
	OldestPendingWorkerAt *time.Time `json:"oldest_pending_worker_at"`
	OldestPendingOutboxAt *time.Time `json:"oldest_pending_outbox_at"`
	LastWorkerAdmissionAt *time.Time `json:"last_worker_admission_at"`
	LastOutboxDispatchAt  *time.Time `json:"last_outbox_dispatch_at"`
}

// UnmarshalJSON requires every non-null snapshot fact. Otherwise a truncated
// or older response could omit an unresolved counter and decode it as zero.
// Keep this check on the snapshot so retained progress windows use it too.
func (s *Snapshot) UnmarshalJSON(data []byte) error {
	type wireSnapshot Snapshot
	var decoded wireSnapshot
	if err := strictjson.Decode(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for _, key := range []string{
		"version", "captured_at", "recovery_epoch", "recovery_mode", "active_runs",
		"pending_worker_intents", "active_worker_leases", "expired_worker_leases",
		"pending_outbox", "leased_outbox", "expired_outbox_leases", "dead_letter_outbox",
		"planned_operations", "dispatched_operations", "unknown_operations", "reconciling_operations",
		"manual_operations", "safe_to_retry_operations", "unknown_codex_executions",
		"unknown_offline_executions", "expired_offline_executions", "expired_codex_executions", "worker_polls",
	} {
		if len(fields[key]) == 0 || bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
			return fmt.Errorf("operational snapshot fact %s is required", key)
		}
	}
	// These four observations are explicitly nullable, but not optional on wire.
	for _, key := range []string{"oldest_pending_worker_at", "oldest_pending_outbox_at", "last_worker_admission_at", "last_outbox_dispatch_at"} {
		if len(fields[key]) == 0 {
			return fmt.Errorf("operational snapshot observation %s is required", key)
		}
	}
	*s = Snapshot(decoded)
	return nil
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
		snapshot.ExpiredWorkerLeases, snapshot.PendingOutbox, snapshot.LeasedOutbox, snapshot.ExpiredOutboxLeases,
		snapshot.DeadLetterOutbox, snapshot.PlannedOperations, snapshot.DispatchedOperations,
		snapshot.UnknownOperations, snapshot.ReconcilingOperations,
		snapshot.ManualOperations, snapshot.SafeToRetryOperations, snapshot.UnknownCodexExecutions,
		snapshot.UnknownOfflineExecutions, snapshot.ExpiredOfflineExecutions, snapshot.ExpiredCodexExecutions,
	}
	for _, count := range counts {
		if count < 0 {
			return OperationalStatus{}, fmt.Errorf("negative operational counter")
		}
	}
	if snapshot.ExpiredOutboxLeases > snapshot.LeasedOutbox {
		return OperationalStatus{}, fmt.Errorf("expired outbox leases exceed leased outbox count")
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
		{"expired_outbox_leases", snapshot.ExpiredOutboxLeases},
		{"dead_letter_outbox", snapshot.DeadLetterOutbox},
		// PLANNED and DISPATCHED are live reservations during normal dispatch,
		// but they are also the durable survivors of an interrupted process.
		// Never claim an authority-clear quiescent snapshot while either exists.
		{"planned_external_actions", snapshot.PlannedOperations},
		{"dispatched_external_actions", snapshot.DispatchedOperations},
		{"unknown_external_actions", snapshot.UnknownOperations},
		{"reconciling_actions", snapshot.ReconcilingOperations},
		{"manual_actions", snapshot.ManualOperations},
		{"unqualified_safe_to_retry_actions", snapshot.SafeToRetryOperations},
		{"unknown_codex_executions", snapshot.UnknownCodexExecutions},
		{"unknown_offline_executions", snapshot.UnknownOfflineExecutions},
		{"expired_offline_executions", snapshot.ExpiredOfflineExecutions},
		{"expired_codex_executions", snapshot.ExpiredCodexExecutions},
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
