package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/production"
)

const maxObserverEventBytes = 8 << 20
const maxObserverOutputBytes = 128 << 20

type observerWindowReader struct {
	reader   operationalReader
	observer *production.Observer
	now      func() time.Time
	readErr  error
}

func (r *observerWindowReader) Raw(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	requested := r.now()
	raw, err := r.reader.Raw(ctx, method, path, body)
	if err != nil {
		r.readErr = err
		return nil, err
	}
	// Reuse the canonical status validator before pinning even an incomplete
	// window's first source identity. No raw error or response is logged here.
	status, validationErr := decodeProductionStatus(raw, r.now().UTC())
	if validationErr == nil && !status.Snapshot.CapturedAt.Before(requested.Add(-5*time.Second)) {
		if err := r.observer.BindRecovery(status.Snapshot.RecoveryEpoch, status.Snapshot.RecoveryMode); err != nil {
			return nil, err
		}
	}
	return raw, nil
}

func observerFailureReason(err, readErr error) string {
	if errors.Is(err, production.ErrObserverIdentityChanged) {
		return "RECOVERY_IDENTITY_CHANGED"
	}
	if errors.Is(err, context.Canceled) {
		return "MONITOR_CANCELLED"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "WINDOW_TIMEOUT"
	}
	if controlclient.IsCompleteHTTPStatus(readErr, http.StatusUnauthorized) || controlclient.IsCompleteHTTPStatus(readErr, http.StatusForbidden) {
		return "SOURCE_UNAUTHORIZED"
	}
	if readErr != nil {
		return "SOURCE_UNAVAILABLE"
	}
	return "INVALID_WINDOW"
}

// collectProductionMonitor repeats explicitly scheduled read-only windows.
// It never retries a failed request inside a window, catches up missed reads,
// changes endpoints, or clears an alert using an incomplete window.
func collectProductionMonitorWithClock(ctx context.Context, reader operationalReader, policy production.ObserverPolicy, session, endpoint string, now func() time.Time, wait func(context.Context, time.Time) error, emit func(production.ObserverEvent) error) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	start := now()
	observer, err := production.NewObserver(policy, session, endpoint, start)
	if err != nil {
		return false, err
	}
	deadline := start.Add(time.Duration(policy.DurationSeconds) * time.Second)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	initial, err := observer.Start(start)
	if err != nil {
		return false, err
	}
	if err := emit(initial); err != nil {
		return false, err
	}
	stop := func(reason string, cause error) (bool, error) {
		event, err := observer.Stop(reason, now())
		if err != nil {
			return false, err
		}
		if err := emit(event); err != nil {
			return event.HadAlert, err
		}
		return event.HadAlert, cause
	}
	cycle := time.Duration(policy.CycleSeconds()) * time.Second
	interval := time.Duration(policy.Progress.IntervalSeconds) * time.Second
	for i := 0; i < policy.WindowLimit(); i++ {
		if err := ctx.Err(); err != nil {
			reason := "MONITOR_CANCELLED"
			if errors.Is(err, context.DeadlineExceeded) {
				reason = "MONITOR_DEADLINE"
			}
			return stop(reason, err)
		}
		target := start.Add(time.Duration(i) * cycle)
		if err := wait(ctx, target); err != nil {
			reason, cause := "MONITOR_CANCELLED", context.Canceled
			if errors.Is(err, context.DeadlineExceeded) || !now().Before(deadline) {
				reason, cause = "MONITOR_DEADLINE", context.DeadlineExceeded
			}
			return stop(reason, cause)
		}
		at := now()
		if !at.Before(deadline) {
			return stop("MONITOR_DEADLINE", context.DeadlineExceeded)
		}
		if at.Before(target.Add(-interval/4)) || at.After(target.Add(interval/4)) {
			event, err := observer.Failure("MISSED_WINDOW", at)
			if err != nil {
				return false, err
			}
			if err := emit(event); err != nil {
				return event.HadAlert, err
			}
			continue
		}
		windowReader := &observerWindowReader{reader: reader, observer: observer, now: now}
		report, windowErr := collectProductionProgressWithClock(ctx, windowReader, policy.Progress, now, wait)
		at = now()
		if windowErr == nil && !at.Before(deadline) {
			windowErr = context.DeadlineExceeded
		}
		if windowErr != nil {
			reason := observerFailureReason(windowErr, windowReader.readErr)
			if errors.Is(ctx.Err(), context.DeadlineExceeded) || !at.Before(deadline) {
				reason = "MONITOR_DEADLINE"
			}
			event, err := observer.Failure(reason, at)
			if err != nil {
				return false, err
			}
			if err := emit(event); err != nil {
				return event.HadAlert, err
			}
			switch reason {
			case "RECOVERY_IDENTITY_CHANGED":
				return stop(reason, production.ErrObserverIdentityChanged)
			case "MONITOR_CANCELLED":
				return stop(reason, context.Canceled)
			case "MONITOR_DEADLINE":
				return stop(reason, context.DeadlineExceeded)
			}
			continue
		}
		event, err := observer.Window(report, at)
		if err != nil {
			return false, err
		}
		if err := emit(event); err != nil {
			return event.HadAlert, err
		}
	}
	return stop("SCHEDULE_COMPLETE", nil)
}

func observerJSONEmitter(writer io.Writer) func(production.ObserverEvent) error {
	remaining := int64(maxObserverOutputBytes)
	return func(event production.ObserverEvent) error {
		raw, err := json.Marshal(event)
		if err != nil || len(raw) >= maxObserverEventBytes || int64(len(raw)+1) > remaining {
			return fmt.Errorf("observer output size limit reached")
		}
		raw = append(raw, '\n')
		n, err := writer.Write(raw)
		if err != nil || n != len(raw) {
			return fmt.Errorf("observer output write failed")
		}
		remaining -= int64(n)
		return nil
	}
}

func runProductionMonitor(ctx context.Context, reader operationalReader, policy production.ObserverPolicy, endpoint string, writer io.Writer) (bool, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return false, fmt.Errorf("observer session identity unavailable")
	}
	digest := sha256.Sum256([]byte(endpoint))
	return collectProductionMonitorWithClock(ctx, reader, policy, hex.EncodeToString(id[:]), "sha256:"+hex.EncodeToString(digest[:]), time.Now, waitObservation, observerJSONEmitter(writer))
}
