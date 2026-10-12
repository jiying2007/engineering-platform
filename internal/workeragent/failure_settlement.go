package workeragent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

var ErrFailureReportUnobserved = errors.New("execution failure report not observed; exact Core readback required")

type failureReportError struct{ cause error }

func (e *failureReportError) Error() string        { return ErrFailureReportUnobserved.Error() }
func (e *failureReportError) Unwrap() error        { return e.cause }
func (e *failureReportError) Is(target error) bool { return target == ErrFailureReportUnobserved }

// Record the original execution's failure once, under its own joined, bounded
// lifecycle. This is NOT a model/build retry and does not infer rollback from a
// lost response. A failed or malformed acknowledgement remains visible beside
// the original execution error, including any more precise COMMIT uncertainty.
func settleExecutionFailure(ctx context.Context, c Transport, endpoint string, token any) error {
	if c == nil || (endpoint != "/api/v1/worker/offline/fail" && endpoint != "/api/v1/worker/codex/fail") {
		return &failureReportError{cause: fmt.Errorf("exact failure-settlement endpoint required")}
	}
	reporting, cancel := context.WithTimeout(context.WithoutCancel(ctx), 6*time.Second)
	defer cancel()
	var response struct {
		Recorded bool `json:"recorded"`
	}
	if err := c.Call(reporting, http.MethodPost, endpoint, token, &response); err != nil {
		return &failureReportError{cause: err}
	}
	if !response.Recorded {
		return &failureReportError{cause: fmt.Errorf("failure acknowledgement missing recorded=true")}
	}
	return nil
}
