package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

var ErrInvalidContext = errors.New("invalid context reference")

const (
	ContextApproved  = "APPROVED"
	ContextUntrusted = "UNTRUSTED"
)

// ContextRef is a stable logical identity. Locators, credentials, machine paths
// and signed URLs belong to a Resolver, never to the frozen Run identity.
// Trust is a recorded classification, NOT an authorization grant.
type ContextRef struct {
	Source  string `json:"source"`
	Type    string `json:"type"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
	Trust   string `json:"trust"`
}

var (
	contextSource  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,255}$`)
	contextVersion = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,127}$`)
)

func (r ContextRef) Validate() error {
	if !contextSource.MatchString(r.Source) || !contextVersion.MatchString(r.Version) {
		return fmt.Errorf("%w: source/version must be bounded logical identifiers, not locators", ErrInvalidContext)
	}
	switch r.Type {
	case "DOCUMENT", "SKILL", "LOG", "EVIDENCE":
	default:
		return fmt.Errorf("%w: unknown type", ErrInvalidContext)
	}
	if !canonical.ValidDigest(r.Digest) {
		return fmt.Errorf("%w: canonical raw-byte SHA-256 is required", ErrInvalidContext)
	}
	if r.Trust != ContextApproved && r.Trust != ContextUntrusted {
		return fmt.Errorf("%w: explicit trust classification is required", ErrInvalidContext)
	}
	return nil
}

// Context order is significant: changing prompt/material order changes Run identity.
// Two versions of the same logical source cannot silently compete in one input.
func ValidateContextRefs(refs []ContextRef) error {
	if len(refs) > 64 {
		return fmt.Errorf("%w: at most 64 context references", ErrInvalidContext)
	}
	seen := make(map[string]bool, len(refs))
	for i, ref := range refs {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("context[%d]: %w", i, err)
		}
		key := ref.Type + ":" + ref.Source
		if seen[key] {
			return fmt.Errorf("%w: duplicate source/type at context[%d]", ErrInvalidContext, i)
		}
		seen[key] = true
	}
	return nil
}

// UnmarshalJSON rejects legacy strings, unknown locator fields and invalid refs
// at the HTTP/JSON boundary, rather than returning a server error after hashing.
func (m *RunInputManifest) UnmarshalJSON(data []byte) error {
	type wire RunInputManifest
	var value wire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidContext, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("%w: trailing JSON", ErrInvalidContext)
	}
	if err := ValidateContextRefs(value.ContextRefs); err != nil {
		return err
	}
	*m = RunInputManifest(value)
	return nil
}
