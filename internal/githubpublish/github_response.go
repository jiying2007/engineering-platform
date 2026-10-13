package githubpublish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
)

// Only a fully read and closed HTTP response can create this status verdict.
// A transport/body/cancellation error carrying the same status is not absence.
// Neither this verdict nor absence grants retry or external-effect rollback.
type githubStatusError struct{ status int }

func (e *githubStatusError) Error() string {
	return fmt.Sprintf("GitHub API returned status %d", e.status)
}

// Keep the original local cause available to errors.Is without reflecting
// remote response bodies, credential-bearing errors or decode details in text.
type githubResponseError struct{ cause error }

func (e *githubResponseError) Error() string { return "GitHub API response not completely observed" }
func (e *githubResponseError) Unwrap() error { return e.cause }

func githubRefAbsent(status int, err error) bool {
	// Do not use errors.As here: a joined status verdict plus a read failure
	// must not regain absence authority through an embedded status error.
	observed, ok := err.(*githubStatusError)
	return status == http.StatusNotFound && ok && observed.status == http.StatusNotFound
}

func finishGitHubResponse(ctx context.Context, response *http.Response, output any) (int, error) {
	status := response.StatusCode
	data, readErr := io.ReadAll(io.LimitReader(bundleContextReader{ctx, response.Body}, (2<<20)+1))
	closeErr := response.Body.Close()
	if err := errors.Join(ctx.Err(), readErr, closeErr); err != nil {
		return status, &githubResponseError{cause: err}
	}
	if len(data) > 2<<20 {
		return status, &githubResponseError{}
	}
	if status < 200 || status >= 300 {
		return status, &githubStatusError{status: status}
	}
	if output == nil {
		return status, nil
	}
	value := reflect.ValueOf(output)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return status, &githubResponseError{}
	}
	// Decode in isolation. A partial decode, failed close or observed caller
	// cancellation cannot mutate a previously populated observation/receipt.
	fresh := reflect.New(value.Elem().Type())
	if len(data) == 0 || json.Unmarshal(data, fresh.Interface()) != nil {
		return status, &githubResponseError{}
	}
	if err := ctx.Err(); err != nil {
		return status, &githubResponseError{cause: err}
	}
	value.Elem().Set(fresh.Elem())
	return status, nil
}
