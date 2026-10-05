package production

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func progressSamples(t *testing.T, mutate func(int, *Snapshot)) (ProgressPolicy, []ProgressObservation) {
	t.Helper()
	p := ProgressPolicy{WindowSeconds: 20, IntervalSeconds: 10, MaxPendingAgeSeconds: 60}
	start := time.Unix(1700000000, 0).UTC()
	old := start.Add(-2 * time.Minute)
	var observations []ProgressObservation
	for i := 0; i < 3; i++ {
		requested := start.Add(time.Duration(i) * 10 * time.Second)
		s := Snapshot{Version: OperationalStatusVersion, CapturedAt: requested.Add(time.Millisecond), RecoveryMode: "NORMAL", PendingWorkerIntents: 1000, OldestPendingWorkerAt: &old}
		if mutate != nil {
			mutate(i, &s)
		}
		status, err := EvaluateSnapshot(s)
		if err != nil {
			t.Fatal(err)
		}
		observations = append(observations, ProgressObservation{RequestedAt: requested, ReceivedAt: requested.Add(2 * time.Millisecond), Status: status})
	}
	return p, observations
}

func TestProgressWindowDistinguishesQueueFactsFromReadiness(t *testing.T) {
	cases := []struct {
		name                  string
		mutate                func(int, *Snapshot)
		worker, outbox, state string
		alert, advanced       bool
	}{
		{"unconsumed", nil, "AGED_BACKLOG_NO_PROGRESS_MARKER", "EMPTY_AT_ALL_SAMPLES", "QUEUE_PROGRESS_ALERT", true, false},
		{"count-decrease-not-completion", func(i int, s *Snapshot) { s.PendingWorkerIntents -= int64(i) * 100 }, "AGED_BACKLOG_NO_PROGRESS_MARKER", "EMPTY_AT_ALL_SAMPLES", "QUEUE_PROGRESS_ALERT", true, false},
		{"progress-with-oldest-starvation", func(i int, s *Snapshot) { at := s.CapturedAt; s.LastWorkerAdmissionAt = &at }, "AGED_BACKLOG_WITH_PROGRESS_MARKER", "EMPTY_AT_ALL_SAMPLES", "QUEUE_PROGRESS_ALERT", true, true},
		{"idle-is-not-ready", func(_ int, s *Snapshot) { s.PendingWorkerIntents = 0; s.OldestPendingWorkerAt = nil }, "EMPTY_AT_ALL_SAMPLES", "EMPTY_AT_ALL_SAMPLES", "NO_DIAGNOSTIC_ALERT", false, false},
		{"drained-is-not-throughput", func(i int, s *Snapshot) {
			if i == 2 {
				s.PendingWorkerIntents = 0
				s.OldestPendingWorkerAt = nil
			}
		}, "EMPTY_AT_FINAL_SAMPLE", "EMPTY_AT_ALL_SAMPLES", "NO_DIAGNOSTIC_ALERT", false, false},
		{"queue-appeared", func(i int, s *Snapshot) {
			if i == 0 {
				s.PendingWorkerIntents = 0
				s.OldestPendingWorkerAt = nil
			}
		}, "BACKLOG_CHANGED_DURING_WINDOW", "EMPTY_AT_ALL_SAMPLES", "NO_DIAGNOSTIC_ALERT", false, false},
		{"oldest-replaced", func(i int, s *Snapshot) {
			at := s.OldestPendingWorkerAt.Add(time.Duration(i) * time.Second)
			s.OldestPendingWorkerAt = &at
		}, "BACKLOG_CHANGED_DURING_WINDOW", "EMPTY_AT_ALL_SAMPLES", "NO_DIAGNOSTIC_ALERT", false, false},
		{"young-backlog", func(_ int, s *Snapshot) { at := time.Unix(1700000000, 0).UTC(); s.OldestPendingWorkerAt = &at }, "BACKLOG_BELOW_AGE_THRESHOLD", "EMPTY_AT_ALL_SAMPLES", "NO_DIAGNOSTIC_ALERT", false, false},
		{"outbox-backlog", func(_ int, s *Snapshot) {
			s.PendingWorkerIntents = 0
			s.PendingOutbox = 10
			s.OldestPendingOutboxAt = s.OldestPendingWorkerAt
			s.OldestPendingWorkerAt = nil
		}, "EMPTY_AT_ALL_SAMPLES", "AGED_BACKLOG_NO_PROGRESS_MARKER", "QUEUE_PROGRESS_ALERT", true, false},
		{"past-authority-hazard", func(i int, s *Snapshot) {
			s.PendingWorkerIntents = 0
			s.OldestPendingWorkerAt = nil
			if i == 1 {
				s.UnknownOperations = 1
			}
		}, "EMPTY_AT_ALL_SAMPLES", "EMPTY_AT_ALL_SAMPLES", "AUTHORITY_BLOCKED_IN_WINDOW", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, o := progressSamples(t, c.mutate)
			r, err := EvaluateProgress(p, o)
			if err != nil {
				t.Fatal(err)
			}
			if !r.Complete || r.WorkerQueue.State != c.worker || r.OutboxQueue.State != c.outbox || r.State != c.state || r.DiagnosticAlert != c.alert || r.WorkerQueue.ProgressMarkerAdvanced != c.advanced {
				t.Fatalf("unexpected interpretation: %#v", r)
			}
			if r.Ready || r.CapacityObserved || r.ExecutionAuthorized || r.ProductionQualified || r.ServiceReadiness != ServiceReadinessUnobserved {
				t.Fatal("invented readiness or authority")
			}
			if c.name == "unconsumed" && (r.WorkerQueue.PendingAtEnd != 1000 || r.WorkerQueue.OldestAgeAtEndMillis != 140001 || r.WorkerQueue.PeakPendingAtSamples != 1000) {
				t.Fatal("lost quantitative facts")
			}
			if c.name == "past-authority-hazard" && r.AuthorityClearAtAllSamples {
				t.Fatal("final clean sample erased prior blocker")
			}
			raw, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			for _, claim := range []string{`"ready":true`, `"execution_authorized":true`, `"production_qualified":true`, `"capacity_observed":true`} {
				if strings.Contains(string(raw), claim) {
					t.Fatal("claim leaked", claim)
				}
			}
		})
	}
}

func TestProgressWindowRejectsIncompleteOrIncomparableObservations(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ProgressPolicy, *[]ProgressObservation)
	}{
		{"missing-sample", func(_ *ProgressPolicy, o *[]ProgressObservation) { *o = (*o)[:2] }},
		{"extra-sample", func(_ *ProgressPolicy, o *[]ProgressObservation) { *o = append(*o, (*o)[2]) }},
		{"forged-ready", func(_ *ProgressPolicy, o *[]ProgressObservation) { (*o)[1].Status.Ready = true }},
		{"negative-count", func(_ *ProgressPolicy, o *[]ProgressObservation) { (*o)[1].Status.Snapshot.PendingWorkerIntents = -1 }},
		{"missing-oldest", func(_ *ProgressPolicy, o *[]ProgressObservation) { (*o)[1].Status.Snapshot.OldestPendingWorkerAt = nil }},
		{"request-zero", func(_ *ProgressPolicy, o *[]ProgressObservation) { (*o)[0].RequestedAt = time.Time{} }},
		{"received-zero", func(_ *ProgressPolicy, o *[]ProgressObservation) { (*o)[0].ReceivedAt = time.Time{} }},
		{"received-before-request", func(_ *ProgressPolicy, o *[]ProgressObservation) {
			(*o)[1].ReceivedAt = (*o)[1].RequestedAt.Add(-time.Second)
		}},
		{"over-budget-read", func(_ *ProgressPolicy, o *[]ProgressObservation) {
			(*o)[1].ReceivedAt = (*o)[1].RequestedAt.Add(11 * time.Second)
		}},
		{"stale-but-v2-fresh", func(_ *ProgressPolicy, o *[]ProgressObservation) {
			(*o)[0].Status.Snapshot.CapturedAt = (*o)[0].RequestedAt.Add(-6 * time.Second)
		}},
		{"future-snapshot", func(_ *ProgressPolicy, o *[]ProgressObservation) {
			(*o)[1].Status.Snapshot.CapturedAt = (*o)[1].ReceivedAt.Add(6 * time.Second)
		}},
		{"duplicate-snapshot", func(_ *ProgressPolicy, o *[]ProgressObservation) {
			(*o)[1].Status.Snapshot.CapturedAt = (*o)[0].Status.Snapshot.CapturedAt
		}},
		{"backwards-snapshot", func(_ *ProgressPolicy, o *[]ProgressObservation) {
			(*o)[1].Status.Snapshot.CapturedAt = (*o)[0].Status.Snapshot.CapturedAt.Add(-time.Second)
		}},
		{"recovery-epoch", func(_ *ProgressPolicy, o *[]ProgressObservation) { (*o)[1].Status.Snapshot.RecoveryEpoch++ }},
		{"recovery-mode", func(_ *ProgressPolicy, o *[]ProgressObservation) {
			s := (*o)[1].Status.Snapshot
			s.RecoveryMode = "RECOVERY_RECONCILIATION"
			(*o)[1].Status, _ = EvaluateSnapshot(s)
		}},
		{"missed-slot", func(_ *ProgressPolicy, o *[]ProgressObservation) {
			(*o)[1].RequestedAt = (*o)[1].RequestedAt.Add(-3 * time.Second)
		}},
		{"compressed-window", func(_ *ProgressPolicy, o *[]ProgressObservation) {
			for i := 1; i < len(*o); i++ {
				offset := time.Duration(i) * 2 * time.Second
				(*o)[i].RequestedAt = (*o)[i].RequestedAt.Add(-offset)
				(*o)[i].ReceivedAt = (*o)[i].ReceivedAt.Add(-offset)
				(*o)[i].Status.Snapshot.CapturedAt = (*o)[i].Status.Snapshot.CapturedAt.Add(-offset)
			}
		}},
		{"regressed-worker-marker", func(_ *ProgressPolicy, o *[]ProgressObservation) {
			at := (*o)[0].RequestedAt.Add(-time.Minute)
			(*o)[0].Status.Snapshot.LastWorkerAdmissionAt = &at
		}},
		{"regressed-outbox-marker", func(_ *ProgressPolicy, o *[]ProgressObservation) {
			at := (*o)[0].RequestedAt.Add(-time.Minute)
			earlier := at.Add(-time.Second)
			(*o)[0].Status.Snapshot.LastOutboxDispatchAt = &at
			(*o)[1].Status.Snapshot.LastOutboxDispatchAt = &earlier
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, o := progressSamples(t, nil)
			c.mutate(&p, &o)
			r, err := EvaluateProgress(p, o)
			if err == nil || r.Complete || len(r.Observations) != 0 {
				t.Fatalf("accepted invalid window: %#v %v", r, err)
			}
		})
	}
}

func TestProgressPolicyRejectsUnboundedOrAmbiguousWindows(t *testing.T) {
	for _, p := range []ProgressPolicy{{}, {2, 0, 1}, {1, 1, 1}, {301, 5, 60}, {300, 1, 60}, {120, 61, 60}, {11, 5, 60}, {2, 1, 0}, {2, 1, 86401}, {-2, 1, 60}} {
		if p.Validate() == nil {
			t.Fatal("accepted", p)
		}
	}
	for _, p := range []ProgressPolicy{{2, 1, 1}, {60, 1, 86400}, {300, 5, 60}, {120, 60, 60}} {
		if err := p.Validate(); err != nil {
			t.Fatal(p, err)
		}
	}
}
