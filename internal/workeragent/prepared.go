package workeragent

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

// PrepareOnce runs trusted Git and hashes approved bytes; it never starts the
// requested Runtime, build scripts or model. A dedicated grant admits this lane.
func PrepareOnce(ctx context.Context, c Transport, profile string, p *preparation.Preparer) (*preparation.Receipt, error) {
	if c == nil || p == nil || c.Subject() != p.Subject() || !workerqueue.ValidProfile(profile) {
		return nil, workerqueue.ErrIdentity
	}
	var claimed struct {
		Assignment *workerqueue.Assignment `json:"assignment"`
	}
	if err := c.Call(ctx, http.MethodPost, "/api/v1/worker/prepare-claim", struct {
		Profile string `json:"worker_profile"`
	}{profile}, &claimed); err != nil {
		return nil, err
	}
	if claimed.Assignment == nil {
		return nil, nil
	}
	a := *claimed.Assignment
	if a.Token.Profile != profile {
		return nil, workerqueue.ErrIdentity
	}
	input, err := workerqueue.NewReport(a)
	if err != nil {
		return nil, err
	}
	opctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	renew := func(callCtx context.Context) error {
		var response struct {
			Until time.Time `json:"lease_until"`
		}
		if err := c.Call(callCtx, http.MethodPost, "/api/v1/worker/renew", a.Token, &response); err != nil {
			return err
		}
		if response.Until.IsZero() {
			return workerqueue.ErrLease
		}
		return nil
	}
	if err := renew(opctx); err != nil {
		return nil, err
	}
	// Cancellation interrupts Git/streaming reads. Join the renewal loop before
	// reporting, so it cannot race a terminal receipt and falsely fail the cycle.
	done := make(chan error, 1)
	stopRenew := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopRenew:
				done <- nil
				return
			case <-opctx.Done():
				done <- opctx.Err()
				return
			case <-ticker.C:
				if err := renew(opctx); err != nil {
					cancel()
					done <- err
					return
				}
			}
		}
	}()
	result, prepareErr := p.Prepare(opctx, c.Subject(), a)
	close(stopRenew)
	leaseErr := <-done
	if prepareErr != nil || leaseErr != nil {
		if prepareErr == nil {
			_ = p.Cleanup(context.Background(), result)
		}
		return nil, errors.Join(prepareErr, leaseErr)
	}
	if err := renew(opctx); err != nil {
		_ = p.Cleanup(context.Background(), result)
		return nil, err
	}
	if err := p.Recheck(opctx, a, result); err != nil {
		_ = p.Cleanup(context.Background(), result)
		return nil, err
	}
	report := preparation.Report{Input: input, Facts: result.Facts}
	var receipt preparation.Receipt
	err = c.Call(opctx, http.MethodPost, "/api/v1/worker/prepared", report, &receipt)
	if err != nil && opctx.Err() == nil {
		var rejection *controlclient.HTTPError
		if !errors.As(err, &rejection) || rejection.Status >= 500 {
			err = c.Call(opctx, http.MethodPost, "/api/v1/worker/prepared", report, &receipt)
		}
	}
	// Keep prepared.json and the owned slot after report ambiguity, so an operator
	// can reconcile exact remote receipt history. Never repeat local side effects.
	if err != nil {
		return nil, err
	}
	if err := preparation.Verify(a, receipt, result.Facts, c.Subject()); err != nil {
		return nil, err
	}
	return &receipt, nil
}
