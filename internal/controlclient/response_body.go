package controlclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Failure identity is retained for cancellation and cleanup without exposing
// remote bytes or credential-bearing transport/decoder diagnostics in Error.
// This is not a server-issued COMMIT or external-effect verdict.
type controlResponseError struct{ cause error }

func (e *controlResponseError) Error() string { return "control response not completely observed" }
func (e *controlResponseError) Unwrap() error { return e.cause }

type controlResponseReader struct {
	ctx    context.Context
	source io.Reader
}

func (r controlResponseReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) > 64<<10 {
		p = p[:64<<10]
	}
	n, err := r.source.Read(p)
	if cancelled := r.ctx.Err(); cancelled != nil {
		return 0, errors.Join(cancelled, err)
	}
	return n, err
}

// Own exactly one body lifetime for success AND error statuses. A truncated or
// cancelled 4xx/5xx is not a complete rejection or precise server classification.
// Cancellation is cooperative; the original HTTP client deadline bounds network
// waits. No detached reader, independent timeout, retry or authority is created.
func readControlResponseBody(ctx context.Context, response *http.Response, limit int64) ([]byte, error) {
	data, readErr := io.ReadAll(io.LimitReader(controlResponseReader{ctx, response.Body}, limit+1))
	closeErr := response.Body.Close()
	if err := errors.Join(ctx.Err(), readErr, closeErr); err != nil {
		return nil, &controlResponseError{cause: err}
	}
	if int64(len(data)) > limit {
		return nil, &controlResponseError{cause: fmt.Errorf("control response exceeds limit")}
	}
	return data, nil
}
