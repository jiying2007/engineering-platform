package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jiying2007/engineering-platform/internal/production"
)

type operationalReader interface {
	Raw(context.Context, string, string, []byte) ([]byte, error)
}

// collectProductionProgress makes bounded sequential GETs through one existing
// direct-mTLS client. A failed/late read aborts; no retries, overlap, catch-up
// bursts, endpoint switching, work writes or state-changing repair are allowed.
func collectProductionProgress(ctx context.Context, reader operationalReader, policy production.ProgressPolicy) (production.ProgressReport, error) {
	return collectProductionProgressWithClock(ctx, reader, policy, time.Now, waitObservation)
}

func waitObservation(ctx context.Context, until time.Time) error {
	timer := time.NewTimer(time.Until(until))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

func collectProductionProgressWithClock(ctx context.Context, reader operationalReader, policy production.ProgressPolicy, now func() time.Time, wait func(context.Context, time.Time) error) (production.ProgressReport, error) {
	var zero production.ProgressReport
	if err := policy.Validate(); err != nil {
		return zero, err
	}
	interval := time.Duration(policy.IntervalSeconds) * time.Second
	start := now()
	ctx, cancel := context.WithDeadline(ctx, start.Add(time.Duration(policy.WindowSeconds)*time.Second+interval))
	defer cancel()
	count := 1 + policy.WindowSeconds/policy.IntervalSeconds
	samples := make([]production.ProgressObservation, 0, count)
	for i := 0; i < count; i++ {
		target := start.Add(time.Duration(i) * interval)
		if err := wait(ctx, target); err != nil {
			return zero, err
		}
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		requested := now()
		if requested.Sub(target) > interval/4 || requested.Before(target.Add(-interval/4)) {
			return zero, fmt.Errorf("missed observation slot; no catch-up requests sent")
		}
		requestCtx, stop := context.WithDeadline(ctx, target.Add(interval))
		data, err := reader.Raw(requestCtx, http.MethodGet, "/api/v1/operations/status", nil)
		received := now()
		requestErr := requestCtx.Err()
		stop()
		if err != nil {
			return zero, fmt.Errorf("observation %d incomplete: %w", i+1, err)
		}
		if requestErr != nil {
			return zero, requestErr
		}
		status, err := decodeProductionStatus(data, received.UTC())
		if err != nil {
			return zero, err
		}
		samples = append(samples, production.ProgressObservation{RequestedAt: requested.UTC(), ReceivedAt: received.UTC(), Status: status})
		if err := production.ValidateProgressPrefix(policy, samples); err != nil {
			return zero, err
		}
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	return production.EvaluateProgress(policy, samples)
}
