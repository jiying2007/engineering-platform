package workeragent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/offline"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
)

// ExecuteOffline is explicitly one-shot. Start ambiguity or local execution
// failure MUST NOT cause automatic re-execution. All commands are offline and
// read-only; revocation is bounded cooperative cancellation, not instantaneous.
func ExecuteOffline(ctx context.Context, c Transport, p *preparation.Preparer, engine *sandbox.Engine, request offline.Start) (receipt offline.Receipt, err error) {
	if c == nil || p == nil || engine == nil || c.Subject() != p.Subject() {
		return receipt, fmt.Errorf("authenticated preparation owner and engine required")
	}
	if err = engine.Ready(ctx, request.Profile); err != nil {
		return receipt, err
	}
	var permit offline.Permit
	if err = c.Call(ctx, http.MethodPost, "/api/v1/worker/offline/start", request, &permit); err != nil {
		return receipt, err
	}
	if err = permit.Check(c.Subject(), request); err != nil {
		return receipt, err
	}
	finished := false
	defer func() {
		if !finished {
			cleanup, stop := context.WithTimeout(context.Background(), 6*time.Second)
			defer stop()
			_ = c.Call(cleanup, http.MethodPost, "/api/v1/worker/offline/fail", permit.Token, nil)
		}
	}()
	running, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	runCtx, stopRun := context.WithCancelCause(running)
	defer stopRun(nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
			}
			renewal, cancel := context.WithTimeout(runCtx, 6*time.Second)
			var response struct {
				LeaseUntil time.Time `json:"lease_until"`
			}
			failure := c.Call(renewal, http.MethodPost, "/api/v1/worker/offline/renew", permit.Token, &response)
			cancel()
			if failure != nil || response.LeaseUntil.IsZero() {
				if failure == nil {
					failure = fmt.Errorf("empty execution lease")
				}
				stopRun(failure)
				return
			}
		}
	}()
	joined := false
	defer func() {
		if !joined {
			stopRun(nil)
			<-done
		}
	}()
	result, err := p.Reopen(runCtx, c.Subject(), permit.Assignment, permit.Preparation.FactsDigest)
	if err != nil {
		return receipt, err
	}
	// This local record survives a crash; it is not a resume token.
	if err = p.SaveOffline(runCtx, permit.Assignment, result, permit.Token.ID, permit); err != nil {
		return receipt, err
	}
	observed, err := engine.Run(runCtx, request.Profile, result.Workspace.WorktreePath, result.BundlePath)
	if err != nil {
		return receipt, err
	}
	if err = p.Recheck(runCtx, permit.Assignment, result); err != nil {
		return receipt, err
	}
	if cause := context.Cause(runCtx); cause != nil {
		return receipt, cause
	}
	// Join the cancellation monitor before the final receipt transaction.
	stopRun(nil)
	<-done
	joined = true
	var renewal struct {
		LeaseUntil time.Time `json:"lease_until"`
	}
	if err = c.Call(ctx, http.MethodPost, "/api/v1/worker/offline/renew", permit.Token, &renewal); err != nil {
		return receipt, err
	}
	report := offline.Report{Token: permit.Token, Result: observed}
	// A separate deterministic local record preserves actual output before network.
	reportID := sandbox.Hash([]byte(permit.Token.ID + ":report"))[7:]
	if err = p.SaveOffline(ctx, permit.Assignment, result, reportID, report); err != nil {
		return receipt, err
	}
	err = c.Call(ctx, http.MethodPost, "/api/v1/worker/offline/report", report, &receipt)
	if err != nil && ctx.Err() == nil {
		var rejected *controlclient.HTTPError
		if !errors.As(err, &rejected) || rejected.Status >= 500 {
			err = c.Call(ctx, http.MethodPost, "/api/v1/worker/offline/report", report, &receipt)
		}
	}
	if err != nil {
		return receipt, err
	}
	if err = receipt.Verify(c.Subject(), permit, observed); err != nil {
		return receipt, err
	}
	finished = true
	return receipt, nil
}
