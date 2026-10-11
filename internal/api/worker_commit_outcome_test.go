package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/store"
)

func TestWorkerCommitUnknownMustRequireReadbackWithoutLeakingSQL(t *testing.T) {
	for _, original := range []error{errors.New("lost PostgreSQL connection"), errors.New("caller cancelled during commit")} {
		recorder := httptest.NewRecorder()
		writeWorkerError(recorder, fmt.Errorf("private SQL command: %w: %w", store.ErrCommitOutcomeUnknown, original))
		if recorder.Code != http.StatusServiceUnavailable ||
			!strings.Contains(recorder.Body.String(), "commit_outcome_unknown") ||
			!strings.Contains(recorder.Body.String(), "read back") ||
			strings.Contains(recorder.Body.String(), "private SQL") {
			t.Fatalf("unknown Worker COMMIT misclassified: status=%d body=%q", recorder.Code, recorder.Body.String())
		}
	}
}
