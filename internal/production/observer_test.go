package production

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

const testObserverSession = "0123456789abcdef0123456789abcdef"

var testObserverEndpoint = "sha256:" + strings.Repeat("a", 64)

func observerTestPolicy() ObserverPolicy {
	return ObserverPolicy{Progress: ProgressPolicy{WindowSeconds: 2, IntervalSeconds: 1, MaxPendingAgeSeconds: 60}, DurationSeconds: 12, StaleAfterSeconds: 4}
}

func observerTestWindow(t *testing.T, p ProgressPolicy, start time.Time, epoch uint64, alert bool) ProgressReport {
	t.Helper()
	var samples []ProgressObservation
	old := start.Add(-time.Hour)
	for i := 0; i <= p.WindowSeconds/p.IntervalSeconds; i++ {
		at := start.Add(time.Duration(i*p.IntervalSeconds) * time.Second)
		s := Snapshot{Version: OperationalStatusVersion, CapturedAt: at, RecoveryMode: "NORMAL", RecoveryEpoch: epoch, WorkerPolls: []WorkerPollObservation{}}
		if alert {
			s.PendingWorkerIntents = 1
			s.OldestPendingWorkerAt = &old
		}
		status, err := EvaluateSnapshot(s)
		if err != nil {
			t.Fatal(err)
		}
		samples = append(samples, ProgressObservation{RequestedAt: at, ReceivedAt: at, Status: status})
	}
	r, err := EvaluateProgress(p, samples)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestObserverFailureRecoveryAndFreshness(t *testing.T) {
	p := observerTestPolicy()
	start := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	o, err := NewObserver(p, testObserverSession, testObserverEndpoint, start)
	if err != nil {
		t.Fatal(err)
	}
	e, err := o.Start(start)
	if err != nil {
		t.Fatal(err)
	}
	f, err := EvaluateObserverFreshness(e, testObserverSession, testObserverEndpoint, start)
	if err != nil || !f.ObserverFresh || f.SourceFresh || f.Ready || f.ProductionQualified {
		t.Fatal(f, err)
	}
	if err := o.BindRecovery(2, "NORMAL"); err != nil {
		t.Fatal(err)
	}
	e, err = o.Failure("SOURCE_UNAVAILABLE", start.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if e.State != ObserverUnavailable || !e.HadAlert || e.FailedWindows != 1 || e.LastSuccessAt != nil {
		t.Fatal(e)
	}
	r := observerTestWindow(t, p.Progress, start.Add(3*time.Second), 2, false)
	e, err = o.Window(r, start.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if e.Kind != "RECOVERED" || e.State != ObserverClear || !e.HadAlert || e.FailedWindows != 1 || e.CompletedWindows != 1 {
		t.Fatal(e)
	}
	f, err = EvaluateObserverFreshness(e, testObserverSession, testObserverEndpoint, start.Add(8*time.Second))
	if err != nil || !f.ObserverFresh || !f.SourceFresh {
		t.Fatal(f, err)
	}
	f, err = EvaluateObserverFreshness(e, testObserverSession, testObserverEndpoint, start.Add(9*time.Second))
	if err != nil || f.ObserverFresh || f.SourceFresh {
		t.Fatal(f, err)
	}
	r = observerTestWindow(t, p.Progress, start.Add(6*time.Second), 2, true)
	e, err = o.Window(r, start.Add(8*time.Second))
	if err != nil || e.State != ObserverAlert || e.AlertWindows != 1 {
		t.Fatal(e, err)
	}
	r = observerTestWindow(t, p.Progress, start.Add(9*time.Second), 2, false)
	e, err = o.Window(r, start.Add(11*time.Second))
	if err != nil || e.Kind != "RECOVERED" || e.AlertWindows != 1 || e.FailedWindows != 1 {
		t.Fatal(e, err)
	}
	e, err = o.Stop("SCHEDULE_COMPLETE", start.Add(11*time.Second))
	if err != nil || !e.Stopped {
		t.Fatal(e, err)
	}
	f, err = EvaluateObserverFreshness(e, testObserverSession, testObserverEndpoint, start.Add(11*time.Second))
	if err != nil || f.ObserverFresh || f.SourceFresh || !f.Stopped {
		t.Fatal(f, err)
	}
	if _, err = o.Failure("SOURCE_UNAVAILABLE", start.Add(12*time.Second)); err == nil {
		t.Fatal("stopped observer accepted more work")
	}
}

func TestObserverRejectsIdentityDriftAndForgedCompleteWindows(t *testing.T) {
	p := observerTestPolicy()
	start := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	o, _ := NewObserver(p, testObserverSession, testObserverEndpoint, start)
	_, _ = o.Start(start)
	if err := o.BindRecovery(1, "NORMAL"); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct {
		epoch uint64
		mode  string
	}{{2, "NORMAL"}, {1, "RECOVERY_RECONCILIATION"}} {
		if !errors.Is(o.BindRecovery(change.epoch, change.mode), ErrObserverIdentityChanged) {
			t.Fatal("recovery identity changed silently")
		}
	}
	r := observerTestWindow(t, p.Progress, start, 2, false)
	if _, err := o.Window(r, start.Add(2*time.Second)); !errors.Is(err, ErrObserverIdentityChanged) {
		t.Fatal(err)
	}
	r = observerTestWindow(t, p.Progress, start, 1, false)
	r.Ready = true
	if _, err := o.Window(r, start.Add(2*time.Second)); err == nil {
		t.Fatal("forged Ready accepted")
	}
	r = observerTestWindow(t, p.Progress, start, 1, false)
	r.Observations = r.Observations[:2]
	if _, err := o.Window(r, start.Add(2*time.Second)); err == nil {
		t.Fatal("partial window accepted")
	}
	before := o.event
	if _, err := o.Failure("SOURCE_UNAVAILABLE", start.Add(-time.Second)); err == nil || !reflect.DeepEqual(before, o.event) {
		t.Fatal("backwards event mutated observer")
	}
	if _, err := o.Stop("SCHEDULE_COMPLETE", start); err == nil {
		t.Fatal("incomplete schedule reported complete")
	}
}

func TestObserverPoliciesAreFiniteAndExplicit(t *testing.T) {
	valid := observerTestPolicy()
	for _, mutate := range []func(*ObserverPolicy){
		func(p *ObserverPolicy) { p.DurationSeconds = 0 }, func(p *ObserverPolicy) { p.DurationSeconds = 2 }, func(p *ObserverPolicy) { p.DurationSeconds = 86401 },
		func(p *ObserverPolicy) { p.DurationSeconds = 86400 }, func(p *ObserverPolicy) { p.StaleAfterSeconds = 0 }, func(p *ObserverPolicy) { p.StaleAfterSeconds = 2 },
		func(p *ObserverPolicy) { p.StaleAfterSeconds = 86401 }, func(p *ObserverPolicy) { p.Progress.IntervalSeconds = 0 },
	} {
		p := valid
		mutate(&p)
		if p.Validate() == nil {
			t.Fatal("accepted invalid observer policy", p)
		}
	}
	if _, err := NewObserver(valid, "identity", testObserverEndpoint, time.Now()); err == nil {
		t.Fatal("unbounded session identity")
	}
	if _, err := NewObserver(valid, testObserverSession, "https://private.example", time.Now()); err == nil {
		t.Fatal("raw endpoint accepted")
	}
}

func TestObserverRetainedEventRejectsContradictionAndWrongObject(t *testing.T) {
	p := observerTestPolicy()
	start := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	o, _ := NewObserver(p, testObserverSession, testObserverEndpoint, start)
	_, _ = o.Start(start)
	r := observerTestWindow(t, p.Progress, start, 1, false)
	e, err := o.Window(r, start.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ObserverEvent){
		func(e *ObserverEvent) { e.Ready = true }, func(e *ObserverEvent) { e.ProductionQualified = true }, func(e *ObserverEvent) { e.ExpiresAt = e.ExpiresAt.Add(time.Second) },
		func(e *ObserverEvent) { e.Sequence++ }, func(e *ObserverEvent) { e.HadAlert = true }, func(e *ObserverEvent) { e.RecoveryEpoch++ },
		func(e *ObserverEvent) { e.AlertWindows = 1; e.HadAlert = true },
		func(e *ObserverEvent) { e.State = ObserverUnavailable }, func(e *ObserverEvent) { e.Kind = "RECOVERED" }, func(e *ObserverEvent) { e.Window = nil },
		func(e *ObserverEvent) { e.LastSuccessAt = nil }, func(e *ObserverEvent) { e.CompletedWindows++ }, func(e *ObserverEvent) { e.Stopped = true },
	} {
		candidate := e
		mutate(&candidate)
		if _, err := EvaluateObserverFreshness(candidate, testObserverSession, testObserverEndpoint, e.RecordedAt); err == nil {
			t.Fatal("accepted contradictory retained event", candidate)
		}
	}
	if _, err := EvaluateObserverFreshness(e, strings.Repeat("b", 32), testObserverEndpoint, e.RecordedAt); err == nil {
		t.Fatal("wrong session accepted")
	}
	if _, err := EvaluateObserverFreshness(e, testObserverSession, "sha256:"+strings.Repeat("b", 64), e.RecordedAt); err == nil {
		t.Fatal("wrong endpoint accepted")
	}
	if _, err := EvaluateObserverFreshness(e, testObserverSession, testObserverEndpoint, start); err == nil {
		t.Fatal("future event accepted")
	}
}

func TestObserverRetainedFailureAndStopReasonsAreBounded(t *testing.T) {
	p := observerTestPolicy()
	start := time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)
	o, _ := NewObserver(p, testObserverSession, testObserverEndpoint, start)
	_, _ = o.Start(start)
	failed, err := o.Failure("SOURCE_UNAUTHORIZED", start)
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := o.Stop("MONITOR_CANCELLED", start)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []ObserverEvent{failed, stopped} {
		if _, err := EvaluateObserverFreshness(e, testObserverSession, testObserverEndpoint, start); err != nil {
			t.Fatal("valid diagnostic reason rejected", err)
		}
		e.Reason = "untrusted upstream error text"
		if _, err := EvaluateObserverFreshness(e, testObserverSession, testObserverEndpoint, start); err == nil {
			t.Fatal("unbounded retained reason accepted")
		}
	}
	stopped.Reason = "SCHEDULE_COMPLETE"
	if _, err := EvaluateObserverFreshness(stopped, testObserverSession, testObserverEndpoint, start); err == nil {
		t.Fatal("incomplete retained schedule reported complete")
	}
}
