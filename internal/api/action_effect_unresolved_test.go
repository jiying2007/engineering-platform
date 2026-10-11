package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/store"
)

func TestEffectfulActionUnresolvedRequiresOriginalReadbackWithoutSQLLeak(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		wantCode string
	}{
		{
			name:     "provider-effect-with-failed-settlement",
			err:      fmt.Errorf("private SQL password: %w", action.ErrExternalEffectUnresolved),
			wantCode: "external_effect_unresolved",
		},
		{
			name: "post-provider-commit-unknown-has-priority",
			err: fmt.Errorf("private SQL password: %w: %w",
				action.ErrExternalEffectUnresolved, store.ErrCommitOutcomeUnknown),
			wantCode: "commit_outcome_unknown",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeActionError(rec, tc.err)
			body := rec.Body.String()
			if rec.Code != http.StatusServiceUnavailable ||
				!strings.Contains(body, tc.wantCode) ||
				!strings.Contains(body, "read back") ||
				!(strings.Contains(body, "before any retry") ||
					strings.Contains(body, "before considering any retry")) ||
				strings.Contains(body, "private SQL") ||
				strings.Contains(body, "password") {
				t.Fatalf("ambiguous external Action error lost no-replay/readback boundary: status=%d body=%q", rec.Code, body)
			}
			if tc.wantCode == "external_effect_unresolved" &&
				strings.Contains(body, "commit_outcome_unknown") {
				t.Fatalf("generic post-Provider failure fabricated COMMIT ambiguity: %q", body)
			}
		})
	}
	// Generic non-effectful caller validation/policy errors retain their
	// existing status behavior and do not invent an external-effect claim.
	rec := httptest.NewRecorder()
	writeActionError(rec, errors.New("invalid caller request"))
	if rec.Code != http.StatusUnprocessableEntity ||
		strings.Contains(rec.Body.String(), "external_effect_unresolved") {
		t.Fatalf("ordinary Action error changed classification: %d %q", rec.Code, rec.Body.String())
	}
}
