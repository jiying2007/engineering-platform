package githubpublish

import (
	"context"
	"encoding/json"
	"github.com/jiying2007/engineering-platform/internal/action"
	"testing"
)

// A valid current PR is not a retained original completion. This fake supplies
// Observe only, with no witness interface or private completion record.
func TestPublisherPositiveObservationRequiresRecoveryEvidence(t *testing.T) {
	state, config := publisherFixture(t)
	remote := &publisherRemote{observation: ObserveResult{Outcome: ObservedConfirmed}}
	provider, err := New(config, state, remote)
	if err != nil {
		t.Fatal(err)
	}
	op := action.Operation{ID: "uncertain-original", RunID: state.run.ID, ExecutionEpoch: state.run.CurrentEpoch, Action: Action, Capability: Capability, RiskClass: action.ControlledMutation, State: action.Reconciling, IdempotencyKey: "one"}
	result, err := provider.Reconcile(context.Background(), op)
	if err != nil || result.Outcome != action.ReconcileManual {
		t.Fatalf("ordinary positive observation bypassed original completion evidence: %+v %v", result, err)
	}
	var diagnostic PublicationReceipt
	if json.Unmarshal([]byte(result.ObservedState), &diagnostic) != nil || diagnostic.Validate(remote.plan) != nil || diagnostic.PublicationOutcome != "OBSERVED" {
		t.Fatal("exact diagnostic observation lost", result)
	}
	if remote.publishCalls != 0 || remote.observeCalls != 1 {
		t.Fatal("observation replayed or fabricated original Publish", remote.publishCalls, remote.observeCalls)
	}
}
