package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/store"
)

type ambiguousExecutionAbandonStore struct {
	*reconciliationMemory
}

func (s *ambiguousExecutionAbandonStore) ReconcileExecutionAbandoned(
	_ context.Context, _ string, _ recovery.ExecutionAbandonRequest,
) (recovery.ExecutionAbandonReceipt, error) {
	s.calls++
	return recovery.ExecutionAbandonReceipt{}, fmt.Errorf(
		"private SQL/connection detail: %w", store.ErrCommitOutcomeUnknown,
	)
}

func TestExecutionAbandonLostCommitMustDemandExactReadback(t *testing.T) {
	backend := &ambiguousExecutionAbandonStore{
		reconciliationMemory: &reconciliationMemory{Memory: store.NewMemory()},
	}
	server, pki := securedTestServer(t, backend, nil, AuthenticatedOptions{})
	request := recovery.ExecutionAbandonRequest{
		Version: 1, ExecutionKind: recovery.ExecutionOffline, RunID: "run-recovery",
		ExecutionID: strings.Repeat("a", 64), RecoveryEpoch: 1,
		ObservationDigest: "sha256:" + strings.Repeat("b", 64),
		Disposition:       recovery.AbandonNoReplay,
	}
	body := secureCall(t, pki.Client(t, reconcilerSubject),
		server.URL+"/api/v1/recovery/executions/abandon",
		request, http.StatusServiceUnavailable)
	response := string(body)
	if backend.calls != 1 ||
		!strings.Contains(response, "commit_outcome_unknown") ||
		!strings.Contains(response, "read back") ||
		strings.Contains(response, "private SQL") {
		t.Fatalf("lost Execution Abandon COMMIT was treated as a retryable failure: calls=%d body=%q",
			backend.calls, response)
	}
}
