package core

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func contextFixture() ContextRef {
	return ContextRef{Source: "docs:spec", Type: "DOCUMENT", Version: "r1", Digest: canonical.BytesDigest([]byte("spec")), Trust: ContextApproved}
}

func TestContextIdentityValidation(t *testing.T) {
	base := contextFixture()
	if err := ValidateContextRefs([]ContextRef{base}); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*ContextRef){
		"missing source":  func(r *ContextRef) { r.Source = "" },
		"URL":             func(r *ContextRef) { r.Source = "https://example.org/context?token=x" },
		"path":            func(r *ContextRef) { r.Source = "../../etc/passwd" },
		"missing version": func(r *ContextRef) { r.Version = "" },
		"bad digest":      func(r *ContextRef) { r.Digest = "sha256:bad" },
		"bad type":        func(r *ContextRef) { r.Type = "EXECUTABLE" },
		"missing trust":   func(r *ContextRef) { r.Trust = "" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			ref := base
			change(&ref)
			_, err := (RunInputManifest{ContextRefs: []ContextRef{ref}}).Digest()
			if !errors.Is(err, ErrInvalidContext) {
				t.Fatalf("invalid typed input was digestible: %v", err)
			}
		})
	}
	other := base
	other.Version = "r2"
	if !errors.Is(ValidateContextRefs([]ContextRef{base, other}), ErrInvalidContext) {
		t.Fatal("ambiguous versions of same source accepted")
	}
	if !errors.Is(ValidateContextRefs(make([]ContextRef, 65)), ErrInvalidContext) {
		t.Fatal("unbounded context refs accepted")
	}
}

func TestContextFieldsAndOrderBindRunIdentity(t *testing.T) {
	base := contextFixture()
	original, _ := (RunInputManifest{ContextRefs: []ContextRef{base}}).Digest()
	changes := []func(*ContextRef){
		func(r *ContextRef) { r.Source = "docs:other" },
		func(r *ContextRef) { r.Type = "SKILL" },
		func(r *ContextRef) { r.Version = "r2" },
		func(r *ContextRef) { r.Digest = canonical.BytesDigest([]byte("changed")) },
		func(r *ContextRef) { r.Trust = ContextUntrusted },
	}
	for _, change := range changes {
		ref := base
		change(&ref)
		got, err := (RunInputManifest{ContextRefs: []ContextRef{ref}}).Digest()
		if err != nil || got == original {
			t.Fatalf("context identity did not bind all fields: %v", err)
		}
	}
	second := base
	second.Source = "docs:second"
	a, _ := (RunInputManifest{ContextRefs: []ContextRef{base, second}}).Digest()
	b, _ := (RunInputManifest{ContextRefs: []ContextRef{second, base}}).Digest()
	if a == b {
		t.Fatal("context order is significant and must not be silently sorted")
	}
}

func TestContextJSONRejectsLegacyAndHiddenFields(t *testing.T) {
	valid, err := json.Marshal(RunInputManifest{ContextRefs: []ContextRef{contextFixture()}})
	if err != nil {
		t.Fatal(err)
	}
	var decoded RunInputManifest
	if err := json.Unmarshal(valid, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		`{"context_refs":["legacy-context"]}`,
		`{"context_refs":[{}]}`,
		strings.Replace(string(valid), `"source":"docs:spec"`, `"path":"/tmp/spec","source":"docs:spec"`, 1),
		strings.Replace(string(valid), `"run_id":""`, `"locator":"https://temporary.invalid","run_id":""`, 1),
	} {
		if err := json.Unmarshal([]byte(bad), &decoded); err == nil {
			t.Fatalf("legacy/unknown/invalid input accepted: %s", bad)
		}
	}
}
