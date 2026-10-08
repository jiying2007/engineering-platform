package codexexec

import (
	"errors"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

func TestTurnPromptBindingOnLegacyAndTypedTasks(t *testing.T) {
	_, legacy := contractFixture(t)
	for _, p := range []Permit{legacy, typedSkillPermit(t)} {
		prompt, id, err := Prompt(p.Assignment, p.Preparation, "/approved/context")
		if err != nil {
			t.Fatal(err)
		}
		actual := canonical.BytesDigest([]byte(prompt))
		if err := VerifyTurnPromptBinding(p.Assignment, p.Preparation, prompt, id, actual); err != nil {
			t.Fatal("matching prompt was rejected", err)
		}
		changes := []struct {
			prompt, identity, digest string
		}{
			{prompt + " extra", id, actual},
			{prompt, canonical.BytesDigest([]byte("other identity")), actual},
			{prompt, id, canonical.BytesDigest([]byte("other prompt"))},
			{"", id, actual},
			{string(make([]byte, (64<<10)+1)), id, actual},
			{string([]byte{0xff}), id, actual},
			{prompt, id, "invalid"},
		}
		for _, x := range changes {
			if err := VerifyTurnPromptBinding(p.Assignment, p.Preparation, x.prompt, x.identity, x.digest); !errors.Is(err, workerqueue.ErrIdentity) {
				t.Fatalf("changed model input was accepted: %v", err)
			}
		}
		alternate, again, err := Prompt(p.Assignment, p.Preparation, "/other/context")
		if err != nil || id != again || alternate == prompt {
			t.Fatal("host-local locator changed frozen identity", err)
		}
		if err := VerifyTurnPromptBinding(p.Assignment, p.Preparation, alternate, again, canonical.BytesDigest([]byte(alternate))); err != nil {
			t.Fatal("valid alternate locator was denied", err)
		}
	}
}

func TestTurnPromptBindingRejectsStaleTaskWithReboundIdentity(t *testing.T) {
	p := typedSkillPermit(t)
	oldPrompt, oldIdentity, err := Prompt(p.Assignment, p.Preparation, "/approved/context")
	if err != nil {
		t.Fatal(err)
	}
	p.Assignment.Task.AcceptanceCriteria[0] = "changed frozen criterion"
	rebindTypedPermit(t, &p)
	if err := VerifyTurnPromptBinding(p.Assignment, p.Preparation, oldPrompt,
		oldIdentity, canonical.BytesDigest([]byte(oldPrompt))); !errors.Is(err, workerqueue.ErrIdentity) {
		t.Fatalf("stale prompt admitted for changed Task: %v", err)
	}
}
