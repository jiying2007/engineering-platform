package githubpublish

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
)

// Preserve local failure identity without reflecting credential-bearing
// transport errors, remote bodies or decoder diagnostics in public text.
type publisherResponseError struct{ cause error }

func (e *publisherResponseError) Error() string { return "publisher response not completely observed" }
func (e *publisherResponseError) Unwrap() error { return e.cause }

// Both GitHub REST and the independent Publisher mTLS client require the same
// complete, bounded, caller-owned body lifetime. Neither a failed close nor an
// observed cancellation is a completed observation. This performs no retry.
func readPublisherResponseBody(ctx context.Context, response *http.Response, limit int64) ([]byte, error) {
	data, readErr := io.ReadAll(io.LimitReader(bundleContextReader{ctx, response.Body}, limit+1))
	closeErr := response.Body.Close()
	if err := errors.Join(ctx.Err(), readErr, closeErr); err != nil {
		return nil, &publisherResponseError{cause: err}
	}
	if int64(len(data)) > limit {
		return nil, &publisherResponseError{}
	}
	return data, nil
}

// Decode a fresh value and commit it only after all checks. Callers supply their
// original decoder: GitHub's foreign schema and the strict internal mTLS schema
// must not be conflated. Failed decoding cannot partially replace a receipt.
func decodePublisherResponse(ctx context.Context, data []byte, output any, decode func([]byte, any) error) error {
	if err := ctx.Err(); err != nil {
		return &publisherResponseError{cause: err}
	}
	value := reflect.ValueOf(output)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return &publisherResponseError{}
	}
	fresh := reflect.New(value.Elem().Type())
	if len(data) == 0 || decode(data, fresh.Interface()) != nil {
		return &publisherResponseError{}
	}
	if err := ctx.Err(); err != nil {
		return &publisherResponseError{cause: err}
	}
	value.Elem().Set(fresh.Elem())
	return nil
}
