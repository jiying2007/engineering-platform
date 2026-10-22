package production

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"time"
)

const (
	ObserverVersion     = 1
	ObserverScope       = "database-progress-observer"
	MaxObserverWindows  = 4096
	ObserverWaiting     = "WAITING_FOR_COMPLETE_WINDOW"
	ObserverUnavailable = "OBSERVATION_UNAVAILABLE"
	ObserverAlert       = "DIAGNOSTIC_ALERT"
	ObserverClear       = "NO_DIAGNOSTIC_ALERT"
)

var (
	ErrObserverIdentityChanged = errors.New("observer recovery identity changed; restart observation explicitly")
	observerSession            = regexp.MustCompile(`^[0-9a-f]{32}$`)
	observerDigest             = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// ObserverPolicy is a finite diagnostic schedule, not a production SLO or a
// capacity/availability contract. Each attempt has a complete, separate window.
type ObserverPolicy struct {
	Progress          ProgressPolicy `json:"progress"`
	DurationSeconds   int            `json:"duration_seconds"`
	StaleAfterSeconds int            `json:"stale_after_seconds"`
}

func (p ObserverPolicy) CycleSeconds() int {
	return p.Progress.WindowSeconds + p.Progress.IntervalSeconds
}
func (p ObserverPolicy) WindowLimit() int {
	if p.CycleSeconds() <= 0 {
		return 0
	}
	return p.DurationSeconds / p.CycleSeconds()
}
func (p ObserverPolicy) Validate() error {
	if err := p.Progress.Validate(); err != nil {
		return err
	}
	cycle := p.CycleSeconds()
	if p.DurationSeconds < cycle || p.DurationSeconds > 86400 || p.WindowLimit() > MaxObserverWindows || p.StaleAfterSeconds < cycle || p.StaleAfterSeconds > 86400 {
		return fmt.Errorf("observer requires a finite cycle..24h budget, at most 4096 windows and an explicit cycle..24h stale threshold")
	}
	return nil
}

// ObserverEvent is an immediately emitted diagnostic record. The endpoint
// digest identifies the configured client endpoint; it is not an authenticated
// deployment identity or an Evidence issuer. All authority flags stay false.
type ObserverEvent struct {
	Version               int             `json:"version"`
	Scope                 string          `json:"scope"`
	SessionID             string          `json:"session_id"`
	EndpointDigest        string          `json:"endpoint_digest"`
	Policy                ObserverPolicy  `json:"policy"`
	Sequence              int             `json:"sequence"`
	Kind                  string          `json:"kind"`
	RecordedAt            time.Time       `json:"recorded_at"`
	ExpiresAt             time.Time       `json:"expires_at"`
	State                 string          `json:"state"`
	StateSince            time.Time       `json:"state_since"`
	LastSuccessAt         *time.Time      `json:"last_success_at,omitempty"`
	RecoveryIdentityBound bool            `json:"recovery_identity_bound"`
	RecoveryEpoch         uint64          `json:"recovery_epoch"`
	RecoveryMode          string          `json:"recovery_mode,omitempty"`
	AttemptedWindows      int             `json:"attempted_windows"`
	CompletedWindows      int             `json:"completed_windows"`
	FailedWindows         int             `json:"failed_windows"`
	AlertWindows          int             `json:"alert_windows"`
	HadAlert              bool            `json:"had_alert"`
	Stopped               bool            `json:"stopped"`
	Reason                string          `json:"reason"`
	Window                *ProgressReport `json:"window,omitempty"`
	Ready                 bool            `json:"ready"`
	CapacityObserved      bool            `json:"capacity_observed"`
	ExecutionAuthorized   bool            `json:"execution_authorized"`
	ProductionQualified   bool            `json:"production_qualified"`
}

type Observer struct{ event ObserverEvent }

func NewObserver(p ObserverPolicy, session, endpoint string, at time.Time) (*Observer, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if !observerSession.MatchString(session) || !observerDigest.MatchString(endpoint) || at.IsZero() {
		return nil, fmt.Errorf("bounded observer identity and start time required")
	}
	o := &Observer{event: ObserverEvent{Version: ObserverVersion, Scope: ObserverScope, SessionID: session, EndpointDigest: endpoint, Policy: p, State: ObserverWaiting, StateSince: at.UTC()}}
	return o, nil
}

// BindRecovery pins the first individually validated source snapshot, even if
// its containing window later fails. A changed epoch/mode never auto-recovers.
func (o *Observer) BindRecovery(epoch uint64, mode string) error {
	if o.event.Stopped {
		return fmt.Errorf("observer already stopped")
	}
	if epoch > 1<<62 || (mode != "NORMAL" && mode != "RECOVERY_RECONCILIATION") {
		return fmt.Errorf("invalid observer recovery identity")
	}
	if o.event.RecoveryIdentityBound {
		if o.event.RecoveryEpoch != epoch || o.event.RecoveryMode != mode {
			return ErrObserverIdentityChanged
		}
		return nil
	}
	o.event.RecoveryIdentityBound, o.event.RecoveryEpoch, o.event.RecoveryMode = true, epoch, mode
	return nil
}

func (o *Observer) canEmit(at time.Time) error {
	if o.event.Stopped || at.IsZero() || at.Before(o.event.StateSince) || (!o.event.RecordedAt.IsZero() && at.Before(o.event.RecordedAt)) || o.event.Sequence >= o.event.Policy.WindowLimit()+2 {
		return fmt.Errorf("invalid observer event order or budget")
	}
	return nil
}

func (o *Observer) emit(kind, reason string, at time.Time, window *ProgressReport) (ObserverEvent, error) {
	if err := o.canEmit(at); err != nil {
		return ObserverEvent{}, err
	}
	o.event.Sequence++
	o.event.Kind, o.event.Reason, o.event.RecordedAt = kind, reason, at.UTC()
	o.event.ExpiresAt = at.Add(time.Duration(o.event.Policy.StaleAfterSeconds) * time.Second).UTC()
	o.event.Window = window
	return o.event, nil
}

func (o *Observer) Start(at time.Time) (ObserverEvent, error) {
	if o.event.Sequence != 0 || !at.Equal(o.event.StateSince) {
		return ObserverEvent{}, fmt.Errorf("observer already started or start time changed")
	}
	return o.emit("STARTED", "NO_COMPLETE_WINDOW", at, nil)
}

func (o *Observer) setState(state string, at time.Time) {
	if state != o.event.State {
		o.event.State, o.event.StateSince = state, at.UTC()
	}
}

func (o *Observer) Window(report ProgressReport, at time.Time) (ObserverEvent, error) {
	if err := o.canEmit(at); err != nil {
		return ObserverEvent{}, err
	}
	if o.event.Sequence == 0 || o.event.AttemptedWindows >= o.event.Policy.WindowLimit() || report.Policy != o.event.Policy.Progress {
		return ObserverEvent{}, fmt.Errorf("observer window policy or count mismatch")
	}
	expected, err := EvaluateProgress(report.Policy, report.Observations)
	if err != nil || !reflect.DeepEqual(expected, report) {
		return ObserverEvent{}, fmt.Errorf("observer window contradicts original samples")
	}
	last := report.Observations[len(report.Observations)-1]
	if at.Before(last.ReceivedAt) {
		return ObserverEvent{}, fmt.Errorf("observer event precedes completed window")
	}
	if err := o.BindRecovery(last.Status.Snapshot.RecoveryEpoch, last.Status.Snapshot.RecoveryMode); err != nil {
		return ObserverEvent{}, err
	}
	kind, next := "WINDOW", ObserverClear
	if report.DiagnosticAlert {
		next = ObserverAlert
	}
	if next == ObserverClear && (o.event.State == ObserverUnavailable || o.event.State == ObserverAlert) {
		kind = "RECOVERED"
	}
	o.setState(next, at)
	o.event.AttemptedWindows++
	o.event.CompletedWindows++
	if report.DiagnosticAlert {
		o.event.AlertWindows++
		o.event.HadAlert = true
	}
	finished := last.ReceivedAt.UTC()
	o.event.LastSuccessAt = &finished
	return o.emit(kind, report.State, at, &report)
}

func validObserverFailureReason(reason string) bool {
	switch reason {
	case "SOURCE_UNAVAILABLE", "SOURCE_UNAUTHORIZED", "WINDOW_TIMEOUT", "INVALID_WINDOW", "MISSED_WINDOW", "RECOVERY_IDENTITY_CHANGED", "MONITOR_CANCELLED", "MONITOR_DEADLINE":
		return true
	default:
		return false
	}
}

func validObserverStopReason(reason string) bool {
	switch reason {
	case "SCHEDULE_COMPLETE", "MONITOR_CANCELLED", "MONITOR_DEADLINE", "RECOVERY_IDENTITY_CHANGED":
		return true
	default:
		return false
	}
}

func (o *Observer) Failure(reason string, at time.Time) (ObserverEvent, error) {
	if err := o.canEmit(at); err != nil {
		return ObserverEvent{}, err
	}
	if o.event.Sequence == 0 || o.event.AttemptedWindows >= o.event.Policy.WindowLimit() {
		return ObserverEvent{}, fmt.Errorf("observer failure outside bounded window schedule")
	}
	if !validObserverFailureReason(reason) {
		return ObserverEvent{}, fmt.Errorf("unknown observer failure reason")
	}
	o.setState(ObserverUnavailable, at)
	o.event.AttemptedWindows++
	o.event.FailedWindows++
	o.event.HadAlert = true
	return o.emit("FAILED", reason, at, nil)
}

func (o *Observer) Stop(reason string, at time.Time) (ObserverEvent, error) {
	if o.event.Sequence == 0 {
		return ObserverEvent{}, fmt.Errorf("observer not started")
	}
	if reason == "SCHEDULE_COMPLETE" && o.event.AttemptedWindows != o.event.Policy.WindowLimit() {
		return ObserverEvent{}, fmt.Errorf("observer schedule incomplete")
	}
	if !validObserverStopReason(reason) {
		return ObserverEvent{}, fmt.Errorf("unknown observer stop reason")
	}
	e, err := o.emit("STOPPED", reason, at, nil)
	if err != nil {
		return e, err
	}
	o.event.Stopped = true
	e.Stopped = true
	return e, nil
}

type ObserverFreshness struct {
	ObserverFresh       bool `json:"observer_fresh"`
	SourceFresh         bool `json:"source_fresh"`
	Stopped             bool `json:"stopped"`
	Ready               bool `json:"ready"`
	ProductionQualified bool `json:"production_qualified"`
}

// EvaluateObserverFreshness checks one retained event's shape and expiry for
// an explicitly expected observer session/endpoint. It does not authenticate
// a producer, prove journal completeness, or infer service health/capacity.
func EvaluateObserverFreshness(e ObserverEvent, session, endpoint string, now time.Time) (ObserverFreshness, error) {
	var zero ObserverFreshness
	if err := e.Policy.Validate(); err != nil {
		return zero, err
	}
	if e.Version != ObserverVersion || e.Scope != ObserverScope || !observerSession.MatchString(session) || !observerDigest.MatchString(endpoint) || e.SessionID != session || e.EndpointDigest != endpoint || e.Sequence < 1 || e.Sequence > e.Policy.WindowLimit()+2 || e.RecordedAt.IsZero() || e.StateSince.IsZero() || e.StateSince.After(e.RecordedAt) || now.IsZero() || now.Before(e.RecordedAt) || !e.ExpiresAt.Equal(e.RecordedAt.Add(time.Duration(e.Policy.StaleAfterSeconds)*time.Second)) || e.Ready || e.CapacityObserved || e.ExecutionAuthorized || e.ProductionQualified {
		return zero, fmt.Errorf("invalid observer identity, time or authority claim")
	}
	if e.AttemptedWindows < 0 || e.AttemptedWindows > e.Policy.WindowLimit() || e.CompletedWindows < 0 || e.FailedWindows < 0 || e.AlertWindows < 0 || e.AlertWindows > e.CompletedWindows || e.AttemptedWindows != e.CompletedWindows+e.FailedWindows || e.HadAlert != (e.FailedWindows > 0 || e.AlertWindows > 0) || (e.LastSuccessAt != nil) != (e.CompletedWindows > 0) {
		return zero, fmt.Errorf("invalid observer counters")
	}
	if e.LastSuccessAt != nil && (e.LastSuccessAt.IsZero() || e.LastSuccessAt.After(e.RecordedAt)) {
		return zero, fmt.Errorf("invalid observer source time")
	}
	switch e.State {
	case ObserverWaiting:
		if e.AttemptedWindows != 0 {
			return zero, fmt.Errorf("waiting observer has completed attempts")
		}
	case ObserverUnavailable:
		if e.FailedWindows == 0 {
			return zero, fmt.Errorf("unavailable observer has no failure")
		}
	case ObserverAlert:
		if e.AlertWindows == 0 {
			return zero, fmt.Errorf("alert observer has no alert window")
		}
	case ObserverClear:
		if e.CompletedWindows == 0 {
			return zero, fmt.Errorf("clear observer has no complete window")
		}
	default:
		return zero, fmt.Errorf("invalid observer state")
	}
	wantSequence := e.AttemptedWindows + 1
	if e.Stopped {
		wantSequence++
	}
	if e.Sequence != wantSequence {
		return zero, fmt.Errorf("observer sequence/counter mismatch")
	}
	if e.Kind == "STARTED" && (e.State != ObserverWaiting || e.Reason != "NO_COMPLETE_WINDOW") {
		return zero, fmt.Errorf("invalid observer start event")
	}
	if e.Kind == "FAILED" && (e.State != ObserverUnavailable || !validObserverFailureReason(e.Reason)) {
		return zero, fmt.Errorf("invalid observer failure state or reason")
	}
	if e.Kind == "RECOVERED" && (!e.HadAlert || e.State != ObserverClear) {
		return zero, fmt.Errorf("invalid observer recovery")
	}
	if e.Stopped != (e.Kind == "STOPPED") {
		return zero, fmt.Errorf("invalid observer stop state")
	}
	if e.Stopped && (!validObserverStopReason(e.Reason) || (e.Reason == "SCHEDULE_COMPLETE" && e.AttemptedWindows != e.Policy.WindowLimit())) {
		return zero, fmt.Errorf("invalid observer stop reason or incomplete schedule")
	}
	switch e.Kind {
	case "STARTED", "WINDOW", "RECOVERED", "FAILED", "STOPPED":
	default:
		return zero, fmt.Errorf("invalid observer event kind")
	}
	if e.RecoveryIdentityBound {
		if e.RecoveryEpoch > 1<<62 || (e.RecoveryMode != "NORMAL" && e.RecoveryMode != "RECOVERY_RECONCILIATION") {
			return zero, fmt.Errorf("invalid bound recovery identity")
		}
	} else if e.RecoveryEpoch != 0 || e.RecoveryMode != "" || e.CompletedWindows != 0 {
		return zero, fmt.Errorf("unbound observer identity")
	}
	if e.Window != nil {
		r, err := EvaluateProgress(e.Window.Policy, e.Window.Observations)
		if err != nil || !reflect.DeepEqual(r, *e.Window) || e.Window.Policy != e.Policy.Progress || (e.Kind != "WINDOW" && e.Kind != "RECOVERED") {
			return zero, fmt.Errorf("invalid retained observer window")
		}
		last := r.Observations[len(r.Observations)-1]
		wantState := ObserverClear
		if r.DiagnosticAlert {
			wantState = ObserverAlert
		}
		if !r.DiagnosticAlert && e.AlertWindows >= e.CompletedWindows {
			return zero, fmt.Errorf("clear retained window contradicts alert count")
		}
		if e.State != wantState || e.Reason != r.State || e.LastSuccessAt == nil || !e.LastSuccessAt.Equal(last.ReceivedAt) || e.RecordedAt.Before(last.ReceivedAt) || !e.RecoveryIdentityBound || e.RecoveryEpoch != last.Status.Snapshot.RecoveryEpoch || e.RecoveryMode != last.Status.Snapshot.RecoveryMode {
			return zero, fmt.Errorf("observer summary contradicts retained window")
		}
	} else if e.Kind == "WINDOW" || e.Kind == "RECOVERED" {
		return zero, fmt.Errorf("complete observer window missing")
	}
	fresh := !e.Stopped && now.Before(e.ExpiresAt)
	sourceFresh := fresh && e.State != ObserverUnavailable && e.LastSuccessAt != nil && now.Before(e.LastSuccessAt.Add(time.Duration(e.Policy.StaleAfterSeconds)*time.Second))
	return ObserverFreshness{ObserverFresh: fresh, SourceFresh: sourceFresh, Stopped: e.Stopped}, nil
}
