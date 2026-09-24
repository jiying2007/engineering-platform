package canonical

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
)

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// BytesDigest hashes original bytes, not their JSON/base64 representation.
func BytesDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ValidDigest(value string) bool {
	return digestPattern.MatchString(value)
}
