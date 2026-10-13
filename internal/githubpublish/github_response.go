package githubpublish

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Only a fully read and closed HTTP response can create this status verdict.
// A transport/body/cancellation error carrying the same status is not absence.
// Neither this verdict nor absence grants retry or external-effect rollback.
type githubStatusError struct{ status int }

func (e *githubStatusError) Error() string {
	return fmt.Sprintf("GitHub API returned status %d", e.status)
}

func githubRefAbsent(status int, err error) bool {
	// Do not use errors.As here: a joined status verdict plus a read failure
	// must not regain absence authority through an embedded status error.
	observed, ok := err.(*githubStatusError)
	return status == http.StatusNotFound && ok && observed.status == http.StatusNotFound
}

func finishGitHubResponse(ctx context.Context, response *http.Response, output any) (int, error) {
	status := response.StatusCode
	data, err := readPublisherResponseBody(ctx, response, 2<<20)
	if err != nil {
		return status, err
	}
	if status < 200 || status >= 300 {
		return status, &githubStatusError{status: status}
	}
	if output == nil {
		return status, nil
	}
	return status, decodePublisherResponse(ctx, data, output, json.Unmarshal)
}
