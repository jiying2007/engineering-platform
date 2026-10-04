package codexexec

import (
	"encoding/json"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/session"
)

func controlFixture() ControlDelivery {
	created := time.Unix(100, 0).UTC()
	dispatched := created.Add(time.Second)
	resolved := created.Add(2 * time.Second)
	b := ControlBinding{Token: Token{ID: strings.Repeat("a", 64), RunID: "run", WorkerProfile: "worker/codex", ProfileDigest: canonical.BytesDigest([]byte("profile"))}, ExecutionEpoch: 1, ThreadID: "thread", TurnID: "turn"}
	payload := ControlPayload{Binding: b, Kind: ControlSteer, Text: "保持任务范围。"}
	hash, _ := canonical.Digest(payload)
	return ControlDelivery{Command: session.SteeringCommand{ID: "control", RunID: "run", ExecutionEpoch: 1, Sequence: 1, Actor: "urn:engineering-platform:engineer:alice", ContentDigest: hash, CreatedAt: created}, Payload: payload, State: ControlAccepted, DispatchedAt: &dispatched, ResolvedAt: &resolved}
}
func TestControlExactInputAndOutcomeValidation(t *testing.T) {
	d := controlFixture()
	if err := d.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*ControlDelivery){
		func(v *ControlDelivery) { v.Payload.Text = "mutated" },
		func(v *ControlDelivery) { v.Payload.Binding.TurnID = "different" },
		func(v *ControlDelivery) { v.Command.Actor = "\x00actor" },
		func(v *ControlDelivery) { v.Command.Sequence = 0 },
		func(v *ControlDelivery) { v.Command.ExecutionEpoch++ },
		func(v *ControlDelivery) { v.State = ControlInterruptACK },
		func(v *ControlDelivery) { v.State = ControlQueued },
		func(v *ControlDelivery) { v.DispatchedAt = nil },
		func(v *ControlDelivery) { v.ResolvedAt = nil },
		func(v *ControlDelivery) { before := v.Command.CreatedAt.Add(-time.Second); v.ResolvedAt = &before },
	} {
		bad := d
		change(&bad)
		if bad.Validate() == nil {
			t.Fatalf("invalid delivery accepted %+v", bad)
		}
	}
	for _, text := range []string{"", " ", strings.Repeat("x", 8193), "\xff", "before\x00after"} {
		bad := d
		bad.Payload.Text = text
		bad.Command.ContentDigest, _ = canonical.Digest(bad.Payload)
		if bad.Validate() == nil {
			t.Fatal("invalid input text accepted")
		}
	}
}
func TestTranscriptCannotTurnACKOrMissingControlsIntoQualification(t *testing.T) {
	d := controlFixture()
	c := ControlClose{Binding: d.Payload.Binding, TurnStatus: "completed", ProcessScope: testsupport.ProcessScopeFixture()}
	tr := ControlTranscript{Version: 2, Close: c, Deliveries: []ControlDelivery{d}}
	if !tr.AllowsDelivery() {
		t.Fatal("valid transcript rejected")
	}
	hash, err := tr.Digest()
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*ControlTranscript){
		func(v *ControlTranscript) { v.Close.ProcessScope.InitReaped = false },
		func(v *ControlTranscript) { v.Close.TurnStatus = "interrupted" },
		func(v *ControlTranscript) { v.Deliveries[0].State = ControlUnknown },
		func(v *ControlTranscript) { v.Deliveries[0].State = ControlNotApplied },
		func(v *ControlTranscript) {
			v.Deliveries[0].State = ControlDispatching
			v.Deliveries[0].ResolvedAt = nil
		},
	} {
		bad := tr
		bad.Deliveries = append([]ControlDelivery{}, tr.Deliveries...)
		change(&bad)
		if bad.AllowsDelivery() {
			t.Fatal("unsafe transcript allowed delivery")
		}
	}
	duplicate := tr
	duplicate.Deliveries = append([]ControlDelivery{d}, d)
	duplicate.Deliveries[1].Command.Sequence = 2
	if _, err = duplicate.Digest(); err == nil {
		t.Fatal("duplicate command in transcript")
	}
	receipt := Receipt{Token: c.Binding.Token, Result: Result{ControlTranscriptDigest: hash}}
	receipt.Result.Codex.ProcessScope = c.ProcessScope
	receipt.Result.Codex.ThreadID = "thread"
	receipt.Result.Codex.TurnID = "turn"
	status := Status{State: Finished, Token: c.Binding.Token, Receipt: &receipt, Runtime: &ControlRuntime{State: "SEALED", Binding: c.Binding, Transcript: &tr, Digest: hash}}
	if err = status.VerifyFinishedControls(); err != nil {
		t.Fatal(err)
	}
	status.Runtime = nil
	if status.VerifyFinishedControls() == nil {
		t.Fatal("missing transcript accepted")
	}
}

func TestHistoricalReadbackDoesNotInventControlAuthority(t *testing.T) {
	raw, err := json.Marshal(Result{})
	if err != nil || strings.Contains(string(raw), "control_transcript_digest") {
		t.Fatal("historical shape was rewritten")
	}
	if (Status{State: Finished, Receipt: &Receipt{Result: Result{}}}).VerifyFinishedControls() == nil {
		t.Fatal("historical receipt acquired current control authority")
	}
}
