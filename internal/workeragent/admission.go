// Package workeragent implements explicit input-admission processing, not an
// unsandboxed Runtime fallback. It never invokes a shell, Git, Codex or tools.
package workeragent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

type Transport interface {
	Call(context.Context, string, string, any, any) error
	Subject() string
}

// Once performs claim -> independent identity validation -> live lease renewal
// -> deterministic report -> retained receipt verification. Only the identical
// report may be retried after a transport ambiguity; claim is never auto-retried.
func Once(ctx context.Context, c Transport, profile string) (*workerqueue.Receipt, error) {
	if c == nil || c.Subject() == "" || !workerqueue.ValidProfile(profile) {
		return nil, fmt.Errorf("authenticated client and worker profile required")
	}
	var claimed struct {
		Assignment *workerqueue.Assignment `json:"assignment"`
	}
	if err := c.Call(ctx, http.MethodPost, "/api/v1/worker/claim", struct {
		Profile string `json:"worker_profile"`
	}{profile}, &claimed); err != nil {
		return nil, err
	}
	if claimed.Assignment == nil {
		return nil, nil
	}
	a := *claimed.Assignment
	if a.Input.WorkerProfile != profile {
		return nil, workerqueue.ErrIdentity
	}
	report, err := workerqueue.NewReport(a)
	if err != nil {
		return nil, err
	}
	var renewed struct {
		LeaseUntil time.Time `json:"lease_until"`
	}
	if err := c.Call(ctx, http.MethodPost, "/api/v1/worker/renew", a.Token, &renewed); err != nil {
		return nil, err
	}
	if renewed.LeaseUntil.IsZero() {
		return nil, workerqueue.ErrLease
	}
	var receipt workerqueue.Receipt
	err = c.Call(ctx, http.MethodPost, "/api/v1/worker/report", report, &receipt)
	if err != nil && ctx.Err() == nil {
		var httpErr *controlclient.HTTPError
		// Never retry an explicit authorization/lease/client rejection. A retry here
		// cannot re-execute work: the report endpoint only records identity admission.
		if !errors.As(err, &httpErr) || httpErr.Status >= 500 {
			err = c.Call(ctx, http.MethodPost, "/api/v1/worker/report", report, &receipt)
		}
	}
	if err != nil {
		return nil, err
	}
	if receipt.Worker != c.Subject() {
		return nil, workerqueue.ErrIdentity
	}
	if err := workerqueue.VerifyReceipt(a, receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}
