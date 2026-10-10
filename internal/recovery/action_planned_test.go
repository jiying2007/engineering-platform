package recovery

import (
	"strings"
	"testing"
	"time"
)

func plannedActionFixture() ActionPlannedAbandonRequest {
	return ActionPlannedAbandonRequest{
		Version: 1, OperationID: "action-1", RunID: "run-1", ExecutionEpoch: 1,
		OriginalRecoveryEpoch: 0, RecoveryEpoch: 1, IdempotencyKey: "action-once",
		OriginalRequestDigest: "sha256:" + strings.Repeat("a", 64),
		ObservationDigest: "sha256:" + strings.Repeat("b", 64),
		Disposition: AbandonNoReplay,
	}
}

func TestPlannedActionRequestBindsExactNoReplayIdentity(t *testing.T) {
	good := plannedActionFixture()
	d, err := good.Digest()
	if err != nil || !strings.HasPrefix(d, "sha256:") {
		t.Fatalf("valid request rejected: %s %v", d, err)
	}
	for _, mutate := range []func(*ActionPlannedAbandonRequest){
		func(r *ActionPlannedAbandonRequest) { r.Version = 2 },
		func(r *ActionPlannedAbandonRequest) { r.OperationID = "" },
		func(r *ActionPlannedAbandonRequest) { r.RunID = "other\nrun" },
		func(r *ActionPlannedAbandonRequest) { r.ExecutionEpoch = 0 },
		func(r *ActionPlannedAbandonRequest) { r.OriginalRecoveryEpoch = r.RecoveryEpoch },
		func(r *ActionPlannedAbandonRequest) { r.RecoveryEpoch = 0 },
		func(r *ActionPlannedAbandonRequest) { r.OriginalRequestDigest = "sha256:bad" },
		func(r *ActionPlannedAbandonRequest) { r.ObservationDigest = "" },
		func(r *ActionPlannedAbandonRequest) { r.IdempotencyKey = "" },
		func(r *ActionPlannedAbandonRequest) { r.Disposition = "REPLAY" },
	} {
		bad := good
		mutate(&bad)
		if err := bad.Validate(); err == nil {
			t.Fatalf("invalid request passed: %#v", bad)
		}
	}
}

func TestPlannedActionReceiptCannotClaimEffectOrExecution(t *testing.T) {
	req := plannedActionFixture()
	digest, _ := req.Digest()
	good := ActionPlannedAbandonReceipt{
		Kind: ActionPlannedAbandonmentKind,
		Request: req, RequestDigest: digest,
		Reconciler: "urn:engineering-platform:operator:reconciler",
		PreviousState: "PLANNED", CreatedAt: time.Now().UTC(),
	}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ActionPlannedAbandonReceipt){
		func(r *ActionPlannedAbandonReceipt) { r.Kind = "CONFIRMED" },
		func(r *ActionPlannedAbandonReceipt) { r.RequestDigest = "sha256:" + strings.Repeat("c", 64) },
		func(r *ActionPlannedAbandonReceipt) { r.PreviousState = "DISPATCHED" },
		func(r *ActionPlannedAbandonReceipt) { r.Reconciler = "  " },
		func(r *ActionPlannedAbandonReceipt) { r.CreatedAt = time.Time{} },
		func(r *ActionPlannedAbandonReceipt) { r.EffectConfirmed = true },
		func(r *ActionPlannedAbandonReceipt) { r.ExecutionAuthorized = true },
		func(r *ActionPlannedAbandonReceipt) { r.ReplayAuthorized = true },
		func(r *ActionPlannedAbandonReceipt) { r.ProductionQualified = true },
	} {
		bad := good
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatalf("forged no-replay receipt passed: %#v", bad)
		}
	}
}
