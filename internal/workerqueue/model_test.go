package workerqueue

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
)

func fixture(t *testing.T) Assignment {
	t.Helper()
	task := core.TaskContract{ID: "task", WorkItemID: "work", Repository: "repo", BaseCommit: strings.Repeat("a", 40), AcceptanceCriteria: []string{"test"}, Revision: 1}
	td, err := task.Digest()
	if err != nil {
		t.Fatal(err)
	}
	input := core.RunInputManifest{RunID: "run", TaskContractDigest: td, RuntimeProfile: "codex", ToolProfile: "read", WorkerProfile: "worker/ubuntu", PolicyProfile: "policy", ContextRefs: []core.ContextRef{{Source: "doc:spec", Type: "DOCUMENT", Version: "r1", Digest: canonical.BytesDigest([]byte("bytes")), Trust: core.ContextApproved}}}
	d, err := input.Digest()
	if err != nil {
		t.Fatal(err)
	}
	intent := Intent{RunID: "run", TaskDigest: td, InputDigest: d, ExecutionEpoch: 1}
	id, err := intent.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return Assignment{Token: Token{InboxID: 1, Generation: 1, Profile: "worker/ubuntu"}, Intent: intent, IntentDigest: id, Task: task, Input: input, LeaseUntil: time.Now().Add(time.Minute)}
}
func TestAdmissionNeverClaimsExecutionOrContextByteVerification(t *testing.T) {
	a := fixture(t)
	v, err := Validate(a)
	if err != nil {
		t.Fatal(err)
	}
	if v.ContextBytesVerified || v.ExecutionStarted || v.ContextCount != 1 || v.Protocol != Protocol {
		t.Fatal("admission overclaimed authority")
	}
	r, err := NewReport(a)
	if err != nil {
		t.Fatal(err)
	}
	again, err := NewReport(a)
	if err != nil || r != again {
		t.Fatal("non-deterministic report")
	}
	receipt := Receipt{Token: a.Token, Kind: Validated, Worker: "urn:engineering-platform:worker", Validation: v, ReceivedAt: time.Now()}
	if err := VerifyReceipt(a, receipt); err != nil {
		t.Fatal(err)
	}
	receipt.Validation.ExecutionStarted = true
	if err := VerifyReceipt(a, receipt); err == nil {
		t.Fatal("forged execution accepted")
	}
}
func TestEveryIdentityBindingIsChecked(t *testing.T) {
	cases := map[string]func(*Assignment){
		"intent": func(a *Assignment) { a.Intent.ExecutionEpoch++ }, "input": func(a *Assignment) { a.Input.ToolProfile = "write" },
		"task": func(a *Assignment) { a.Task.BaseCommit = strings.Repeat("b", 40) }, "profile": func(a *Assignment) { a.Token.Profile = "other" },
		"lease": func(a *Assignment) { a.Token.Generation = 0 }, "context": func(a *Assignment) { a.Input.ContextRefs[0].Digest = canonical.BytesDigest([]byte("other")) },
		"run": func(a *Assignment) { a.Input.RunID = "another" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			a := fixture(t)
			change(&a)
			if _, err := Validate(a); !errors.Is(err, ErrIdentity) {
				t.Fatalf("identity accepted: %v", err)
			}
		})
	}
}
func TestUntrustedReferenceIsNotSilentlyPromoted(t *testing.T) {
	a := fixture(t)
	a.Input.ContextRefs[0].Trust = core.ContextUntrusted
	a.Intent.InputDigest, _ = a.Input.Digest()
	a.IntentDigest, _ = a.Intent.Digest()
	v, err := Validate(a)
	if err != nil {
		t.Fatal(err)
	}
	if v.ContextBytesVerified {
		t.Fatal("untrusted context was promoted")
	}
}
