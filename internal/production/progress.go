package production

import (
	"fmt"
	"time"
)

// ProgressPolicy is an operator-selected diagnostic window, not a calibrated
// service SLO. It grants no work/recovery authority and does not infer capacity.
type ProgressPolicy struct {
	WindowSeconds        int `json:"window_seconds"`
	IntervalSeconds      int `json:"interval_seconds"`
	MaxPendingAgeSeconds int `json:"max_pending_age_seconds"`
}

func (p ProgressPolicy) Validate() error {
	if p.WindowSeconds < 2 || p.WindowSeconds > 300 || p.IntervalSeconds < 1 || p.IntervalSeconds > 60 || p.WindowSeconds%p.IntervalSeconds != 0 || p.WindowSeconds/p.IntervalSeconds < 2 || p.WindowSeconds/p.IntervalSeconds > 60 || p.MaxPendingAgeSeconds < 1 || p.MaxPendingAgeSeconds > 86400 {
		return fmt.Errorf("bounded observation requires 3..61 samples, 1..60s interval, 2..300s window and explicit 1s..24h pending-age threshold")
	}
	return nil
}

type ProgressObservation struct {
	RequestedAt time.Time         `json:"requested_at"`
	ReceivedAt  time.Time         `json:"received_at"`
	Status      OperationalStatus `json:"status"`
}

type QueueProgress struct {
	State                  string `json:"state"`
	PendingAtStart         int64  `json:"pending_at_start"`
	PendingAtEnd           int64  `json:"pending_at_end"`
	PeakPendingAtSamples   int64  `json:"peak_pending_at_samples"`
	OldestAgeAtEndMillis   int64  `json:"oldest_age_at_end_millis"`
	SameOldestAtAllSamples bool   `json:"same_oldest_at_all_samples"`
	ProgressMarkerAdvanced bool   `json:"progress_marker_advanced"`
	Alert                  bool   `json:"alert"`
}

type ProgressReport struct {
	Version                    int                   `json:"version"`
	Scope                      string                `json:"scope"`
	Policy                     ProgressPolicy        `json:"policy"`
	Complete                   bool                  `json:"complete"`
	State                      string                `json:"state"`
	Observations               []ProgressObservation `json:"observations"`
	WorkerQueue                QueueProgress         `json:"worker_queue"`
	OutboxQueue                QueueProgress         `json:"outbox_queue"`
	AuthorityClearAtAllSamples bool                  `json:"authority_clear_at_all_samples"`
	DiagnosticAlert            bool                  `json:"diagnostic_alert"`
	ServiceReadiness           string                `json:"service_readiness"`
	CapacityObserved           bool                  `json:"capacity_observed"`
	Ready                      bool                  `json:"ready"`
	ExecutionAuthorized        bool                  `json:"execution_authorized"`
	ProductionQualified        bool                  `json:"production_qualified"`
}

// EvaluateProgress validates the complete sampled window before interpreting
// queue facts. Replayed/stale/contradictory data or recovery-boundary changes
// reject the window; missing samples must never be filled or treated as healthy.
func EvaluateProgress(p ProgressPolicy, samples []ProgressObservation) (ProgressReport, error) {
	var zero ProgressReport
	if err := ValidateProgressPrefix(p, samples); err != nil {
		return zero, err
	}
	if len(samples) != 1+p.WindowSeconds/p.IntervalSeconds {
		return zero, fmt.Errorf("incomplete observation window")
	}
	authorityClear := true
	for _, sample := range samples {
		authorityClear = authorityClear && sample.Status.AuthorityClear
	}
	worker := evaluateQueueProgress(p, samples, true)
	outbox := evaluateQueueProgress(p, samples, false)
	state := "NO_DIAGNOSTIC_ALERT"
	alert := worker.Alert || outbox.Alert || !authorityClear
	if worker.Alert || outbox.Alert {
		state = "QUEUE_PROGRESS_ALERT"
	}
	if !authorityClear {
		state = "AUTHORITY_BLOCKED_IN_WINDOW"
	}
	return ProgressReport{Version: 1, Scope: "database-queue-progress-window", Policy: p, Complete: true, State: state, Observations: samples, WorkerQueue: worker, OutboxQueue: outbox, AuthorityClearAtAllSamples: authorityClear, DiagnosticAlert: alert, ServiceReadiness: ServiceReadinessUnobserved}, nil
}

// ValidateProgressPrefix checks each response before a collector sends another
// GET. A valid prefix is NOT a complete window or a health/authority decision.
func ValidateProgressPrefix(p ProgressPolicy, samples []ProgressObservation) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if len(samples) < 1 || len(samples) > 1+p.WindowSeconds/p.IntervalSeconds {
		return fmt.Errorf("invalid observation prefix length")
	}
	interval := time.Duration(p.IntervalSeconds) * time.Second
	first := samples[0]
	for i, o := range samples {
		if o.RequestedAt.IsZero() || o.ReceivedAt.IsZero() || o.ReceivedAt.Before(o.RequestedAt) || o.ReceivedAt.Sub(o.RequestedAt) > interval {
			return fmt.Errorf("invalid or over-budget observation request")
		}
		if err := o.Status.ValidateAt(o.ReceivedAt); err != nil {
			return err
		}
		s := o.Status.Snapshot
		// Existing freshness alone permits a recent cached sample. Also bind the
		// server snapshot to this request window with the existing 5s skew bound.
		if s.CapturedAt.Before(o.RequestedAt.Add(-5 * time.Second)) {
			return fmt.Errorf("snapshot predates request window")
		}
		if s.RecoveryEpoch != first.Status.Snapshot.RecoveryEpoch || s.RecoveryMode != first.Status.Snapshot.RecoveryMode {
			return fmt.Errorf("recovery boundary changed; start a new observation")
		}
		if i > 0 {
			previous := samples[i-1]
			if !s.CapturedAt.After(previous.Status.Snapshot.CapturedAt) {
				return fmt.Errorf("replayed or backwards database snapshot")
			}
			// Validate each absolute target, not only adjacent gaps: individually
			// small timing errors must not accumulate into a shorter fake window.
			target := first.RequestedAt.Add(time.Duration(i) * interval)
			late := o.RequestedAt.Sub(target)
			if late < -interval/4 || late > interval/4 || !o.ReceivedAt.After(previous.ReceivedAt) {
				return fmt.Errorf("observation cadence incomplete")
			}
			for _, pair := range [][2]*time.Time{{previous.Status.Snapshot.LastWorkerAdmissionAt, s.LastWorkerAdmissionAt}, {previous.Status.Snapshot.LastOutboxDispatchAt, s.LastOutboxDispatchAt}} {
				if pair[0] != nil && (pair[1] == nil || pair[1].Before(*pair[0])) {
					return fmt.Errorf("database progress marker regressed; observation incomparable")
				}
			}
		}
	}
	return nil
}

func evaluateQueueProgress(p ProgressPolicy, samples []ProgressObservation, worker bool) QueueProgress {
	selectQueue := func(s Snapshot) (int64, *time.Time, *time.Time) {
		if worker {
			return s.PendingWorkerIntents, s.OldestPendingWorkerAt, s.LastWorkerAdmissionAt
		}
		return s.PendingOutbox, s.OldestPendingOutboxAt, s.LastOutboxDispatchAt
	}
	firstCount, firstOldest, firstProgress := selectQueue(samples[0].Status.Snapshot)
	last := samples[len(samples)-1].Status.Snapshot
	lastCount, lastOldest, lastProgress := selectQueue(last)
	r := QueueProgress{PendingAtStart: firstCount, PendingAtEnd: lastCount, SameOldestAtAllSamples: firstOldest != nil}
	allEmpty := true
	for _, sample := range samples {
		count, oldest, _ := selectQueue(sample.Status.Snapshot)
		allEmpty = allEmpty && count == 0
		if count > r.PeakPendingAtSamples {
			r.PeakPendingAtSamples = count
		}
		if oldest == nil || firstOldest == nil || !oldest.Equal(*firstOldest) {
			r.SameOldestAtAllSamples = false
		}
	}
	if lastOldest != nil {
		r.OldestAgeAtEndMillis = last.CapturedAt.Sub(*lastOldest).Milliseconds()
	}
	// Advancing MAX(receipt timestamp) is a marker, not a throughput counter,
	// full task completion, a live heartbeat, or proof that the oldest progressed.
	r.ProgressMarkerAdvanced = lastProgress != nil && (firstProgress == nil || lastProgress.After(*firstProgress))
	switch {
	case allEmpty:
		r.State = "EMPTY_AT_ALL_SAMPLES"
	case lastCount == 0:
		r.State = "EMPTY_AT_FINAL_SAMPLE"
	case !r.SameOldestAtAllSamples:
		r.State = "BACKLOG_CHANGED_DURING_WINDOW"
	case r.OldestAgeAtEndMillis < int64(p.MaxPendingAgeSeconds)*1000:
		r.State = "BACKLOG_BELOW_AGE_THRESHOLD"
	case r.ProgressMarkerAdvanced:
		r.State = "AGED_BACKLOG_WITH_PROGRESS_MARKER"
		r.Alert = true
	default:
		r.State = "AGED_BACKLOG_NO_PROGRESS_MARKER"
		r.Alert = true
	}
	return r
}
