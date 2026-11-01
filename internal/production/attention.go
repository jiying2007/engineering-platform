package production

import "time"

const (
	// OperationalAttentionSchema is a read-only projection, never an Action authority.
	OperationalAttentionSchema = "engineering-operational-attention/v1"
	OperationalAttentionScope  = "database-authority-snapshot/advisory"
)

// AttentionItem is a bounded, source-derived reason a person may need to act.
// NextAction is a fixed action *category*, not executable instructions or a grant.
type AttentionItem struct {
	Reason     string `json:"reason"`
	Count      int64  `json:"count"`
	NextAction string `json:"next_action"`
}

// OperationalAttention deliberately contains neither a "READY" flag nor a
// production qualification result. An empty list means there was no actionable
// signal in this snapshot, not that the service is healthy or work is complete.
type OperationalAttention struct {
	SchemaVersion    string          `json:"schema_version"`
	SourceScope      string          `json:"source_scope"`
	CapturedAt       time.Time       `json:"captured_at"`
	RequiresAttention bool           `json:"requires_attention"`
	Items            []AttentionItem `json:"items"`
}

// ProjectOperationalAttention uses only the existing, validated status snapshot.
// It does not query, change, reschedule, retry, send or reconcile any operation.
// In-flight PLANNED/DISPATCHED actions and ordinary workload queues are NOT
// elevated to notifications without an age or identity-based incident witness;
// their existing authority/recovery status is never modified.
func ProjectOperationalAttention(status OperationalStatus, now time.Time) (OperationalAttention, error) {
	if err := status.ValidateAt(now); err != nil {
		return OperationalAttention{}, err
	}
	s := status.Snapshot
	out := OperationalAttention{
		SchemaVersion: OperationalAttentionSchema,
		SourceScope:   OperationalAttentionScope,
		CapturedAt:    s.CapturedAt,
		Items:         []AttentionItem{},
	}
	add := func(reason string, count int64, action string) {
		if count > 0 {
			out.Items = append(out.Items, AttentionItem{Reason: reason, Count: count, NextAction: action})
		}
	}
	if s.RecoveryMode != "NORMAL" {
		add("recovery_reconciliation_active", 1, "inspect_recovery_epoch")
	}
	add("unknown_external_actions", s.UnknownOperations, "reconcile_action_outcome")
	add("reconciling_actions", s.ReconcilingOperations, "review_reconciliation_progress")
	add("manual_actions", s.ManualOperations, "review_manual_action")
	add("unqualified_safe_to_retry_actions", s.SafeToRetryOperations, "review_retry_authority")
	add("unknown_codex_executions", s.UnknownCodexExecutions, "investigate_execution_outcome")
	add("unknown_offline_executions", s.UnknownOfflineExecutions, "investigate_execution_outcome")
	add("expired_offline_executions", s.ExpiredOfflineExecutions, "inspect_execution_lease")
	add("expired_codex_executions", s.ExpiredCodexExecutions, "inspect_execution_lease")
	add("expired_worker_leases", s.ExpiredWorkerLeases, "inspect_worker_lease")
	add("expired_outbox_leases", s.ExpiredOutboxLeases, "inspect_outbox_lease")
	add("dead_letter_outbox", s.DeadLetterOutbox, "inspect_dead_letter")
	out.RequiresAttention = len(out.Items) > 0
	return out, nil
}
