// Package sandbox runs bounded offline commands on an explicitly trusted local
// Docker Engine. The container never receives the socket or Worker credentials.
package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"strings"
)

const Recipe = "docker-offline-readonly-v1"
const OutputLimit = 128 << 10

var ErrPolicy = errors.New("invalid offline sandbox policy")
var ErrUnknown = errors.New("sandbox outcome or cleanup is unknown")
var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type Profile struct {
	Image       string   `json:"image_id"`
	GuardDigest string   `json:"guard_digest"`
	Argv        []string `json:"argv"`
	Seconds     int      `json:"timeout_seconds"`
}

func (p Profile) Validate() error {
	if !digestPattern.MatchString(p.Image) || !digestPattern.MatchString(p.GuardDigest) || p.Seconds < 1 || p.Seconds > 45 || len(p.Argv) < 1 || len(p.Argv) > 32 {
		return ErrPolicy
	}
	if !path.IsAbs(p.Argv[0]) || path.Clean(p.Argv[0]) != p.Argv[0] || p.Argv[0] == "/ep-guard" {
		return ErrPolicy
	}
	size := 0
	for _, arg := range p.Argv {
		size += len(arg)
		if strings.ContainsRune(arg, 0) {
			return ErrPolicy
		}
	}
	if size > 8192 {
		return ErrPolicy
	}
	return nil
}
func Hash(data []byte) string { h := sha256.Sum256(data); return "sha256:" + hex.EncodeToString(h[:]) }
func (p Profile) Digest() (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(p)
	return Hash(data), err
}

type Result struct {
	Recipe        string `json:"recipe"`
	ProfileDigest string `json:"profile_digest"`
	ContainerID   string `json:"container_id"`
	ExitCode      int    `json:"exit_code"`
	Stdout        []byte `json:"stdout"`
	Stderr        []byte `json:"stderr"`
	StdoutDigest  string `json:"stdout_digest"`
	StderrDigest  string `json:"stderr_digest"`
	UserID        int    `json:"user_id"`
}

func (r Result) Validate(p Profile) error {
	digest, err := p.Digest()
	// 122 is reserved by our pinned guard for output overflow. Never retain a
	// truncated Docker log as though it were a complete execution transcript.
	if err != nil || r.Recipe != Recipe || r.ProfileDigest != digest || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(r.ContainerID) || r.ExitCode < 0 || r.ExitCode > 255 || r.ExitCode == 122 || r.UserID < 1 || len(r.Stdout) > OutputLimit || len(r.Stderr) > OutputLimit || Hash(r.Stdout) != r.StdoutDigest || Hash(r.Stderr) != r.StderrDigest {
		return ErrPolicy
	}
	return nil
}
