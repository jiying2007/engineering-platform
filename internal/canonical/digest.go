package canonical

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// Digest returns a deterministic SHA-256 digest for the JSON representation of v.
// Core schemas intentionally avoid ambiguous float/map semantics until the canonical
// serialization profile is frozen further in M0.
func Digest(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
