package recovery

import (
	"strings"
	"testing"
	"time"
)

func TestExecutionAbandonRequestAndReceipt(t *testing.T) {
	request := ExecutionAbandonRequest{
		Version: 1, ExecutionKind: ExecutionOffline, RunID: "run-1",
		ExecutionID: strings.Repeat("a", 64), RecoveryEpoch: 2,
		ObservationDigest: "sha256:" + strings.Repeat("b", 64), Disposition: AbandonNoReplay,
	}
	digest, err := request.Digest()
	if err != nil {
		t.Fatal(err)
	}
	receipt := ExecutionAbandonReceipt{
		Kind: ExecutionAbandonmentKind, Request: request, RequestDigest: digest,
		Reconciler:    "urn:engineering-platform:operator:reconciler",
		PreviousState: "UNKNOWN", CreatedAt: time.Now().UTC(),
	}
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ExecutionAbandonRequest){
		func(v *ExecutionAbandonRequest) { v.Version = 0 },
		func(v *ExecutionAbandonRequest) { v.ExecutionKind = "MODEL" },
		func(v *ExecutionAbandonRequest) { v.ExecutionID = "bad" },
		func(v *ExecutionAbandonRequest) { v.RecoveryEpoch = 0 },
		func(v *ExecutionAbandonRequest) { v.ObservationDigest = "bad" },
		func(v *ExecutionAbandonRequest) { v.Disposition = "RETRY" },
	} {
		bad := request
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("invalid reconciliation request accepted", bad)
		}
	}
	badReceipt := receipt
	badReceipt.ReplayAuthorized = true
	if badReceipt.Validate() == nil {
		t.Fatal("replay-authorized abandonment accepted")
	}
}
