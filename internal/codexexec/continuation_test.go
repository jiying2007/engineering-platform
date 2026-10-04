package codexexec

import (
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"testing"
	"time"
)

func TestContinuationRequiresUnambiguousRequestedInterruptedTurn(t *testing.T) {
	// Synthetic authority belongs solely to this unit test, not provider evidence.
	b := ControlBinding{Token: Token{ID: stringRepeat('a', 64), RunID: "source", WorkerProfile: "worker/codex", ProfileDigest: canonical.BytesDigest([]byte("profile"))}, ExecutionEpoch: 1, ThreadID: "thread", TurnID: "turn"}
	payload := ControlPayload{Binding: b, Kind: ControlInterrupt}
	d, _ := canonical.Digest(payload)
	now := time.Now().UTC()
	delivery := ControlDelivery{Command: session.SteeringCommand{ID: "cancel", RunID: "source", ExecutionEpoch: 1, Sequence: 1, Actor: "actor", ContentDigest: d, CreatedAt: now}, Payload: payload, State: ControlInterrupted, DispatchedAt: &now, ResolvedAt: &now}
	transcript := ControlTranscript{Version: 2, Close: ControlClose{Binding: b, TurnStatus: "interrupted", ProcessScope: testsupport.ProcessScopeFixture()}, Deliveries: []ControlDelivery{delivery}}
	if !transcript.Continuable() {
		t.Fatal("confirmed interruption denied")
	}
	for _, state := range []string{ControlUnknown, ControlInterruptACK, ControlDispatching, ControlQueued, ControlNotApplied} {
		bad := transcript
		bad.Deliveries = []ControlDelivery{delivery}
		bad.Deliveries[0].State = state
		if bad.Continuable() {
			t.Fatal("unconfirmed outcome admitted", state)
		}
	}
	for _, status := range []string{"completed", "failed", "unknown", ""} {
		bad := transcript
		bad.Close.TurnStatus = status
		if bad.Continuable() {
			t.Fatal("non-interrupted turn admitted", status)
		}
	}
	bad := transcript
	bad.Deliveries = nil
	if bad.Continuable() {
		t.Fatal("metadata cancellation admitted")
	}
	bad = transcript
	bad.Close.ProcessScope.ReapedAt = time.Time{}
	if bad.Continuable() {
		t.Fatal("missing kernel reap admitted")
	}
}
func stringRepeat(c byte, n int) string {
	v := make([]byte, n)
	for i := range v {
		v[i] = c
	}
	return string(v)
}
func TestContinueRequestExactDecision(t *testing.T) {
	q := ContinueRequest{SourceRunID: "source", RunID: "next", AttemptID: "next-attempt", CheckpointDigest: canonical.BytesDigest([]byte("checkpoint")), ExpectedRunVersion: 1, ExpectedWorkVersion: 3, ExecutionEpoch: 1, Reason: "Resume from reviewed stopped source, not model replay"}
	if q.Validate() != nil {
		t.Fatal("valid request rejected")
	}
	for _, mutate := range []func(*ContinueRequest){func(q *ContinueRequest) { q.RunID = q.SourceRunID }, func(q *ContinueRequest) { q.ExpectedRunVersion = 0 }, func(q *ContinueRequest) { q.ExpectedWorkVersion = 0 }, func(q *ContinueRequest) { q.CheckpointDigest = "unknown" }, func(q *ContinueRequest) { q.Reason = " " }, func(q *ContinueRequest) { q.Reason = "a\x00b" }, func(q *ContinueRequest) { q.RecoveryEpoch = 1 << 63 }, func(q *ContinueRequest) { q.AttemptID = "bad\n" }} {
		bad := q
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatal("invalid decision accepted", bad)
		}
	}
}
