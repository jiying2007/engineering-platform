// Package artifactset preserves an explicitly declared private set of raw files.
// It does not run producers, follow paths stored in artifacts, or grant authority.
package artifactset

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

const MaxMembers = 128
const MaxFile int64 = 1 << 30
const MaxTotal int64 = 4 << 30
const MaxMetadata int64 = 256 << 10
const MaxArchive int64 = MaxTotal + MaxMetadata + (MaxMembers+4)*1024
const Coverage = "EXPLICIT_DECLARED_MEMBERS_ONLY"

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,89}$`)
var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var executionPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Subject struct {
	RunID       string `json:"run_id"`
	ExecutionID string `json:"execution_id"`
	TaskDigest  string `json:"task_contract_digest"`
	InputDigest string `json:"run_input_manifest_digest"`
	BaseCommit  string `json:"base_commit"`
}
type Entry struct {
	ID     string `json:"artifact_id"`
	Kind   string `json:"kind"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}
type Input struct {
	Entry
	Path string `json:"source_path"`
}
type Plan struct {
	Version int     `json:"version"`
	Subject Subject `json:"subject"`
	Members []Input `json:"members"`
}
type Manifest struct {
	Version    int     `json:"version"`
	Coverage   string  `json:"coverage"`
	Subject    Subject `json:"subject"`
	PlanDigest string  `json:"plan_digest"`
	Members    []Entry `json:"members"`
}
type Report struct {
	Status                    string  `json:"status"`
	ArchiveDigest             string  `json:"archive_digest"`
	ArchiveSize               int64   `json:"archive_size"`
	ManifestDigest            string  `json:"manifest_digest"`
	Coverage                  string  `json:"coverage"`
	Subject                   Subject `json:"subject"`
	Members                   int     `json:"member_count"`
	Bytes                     int64   `json:"payload_bytes"`
	ProducerSemanticsVerified bool    `json:"producer_semantics_verified"`
	ExecutionAuthorized       bool    `json:"execution_authorized"`
	ProductionQualified       bool    `json:"production_qualified"`
}

func (m Manifest) Validate() error {
	s := m.Subject
	if m.Version != 1 || m.Coverage != Coverage || !canonical.ValidDigest(m.PlanDigest) ||
		len(s.RunID) == 0 || len(s.RunID) > 256 || !executionPattern.MatchString(s.ExecutionID) ||
		!canonical.ValidDigest(s.TaskDigest) || !canonical.ValidDigest(s.InputDigest) || !commitPattern.MatchString(s.BaseCommit) ||
		len(m.Members) == 0 || len(m.Members) > MaxMembers {
		return fmt.Errorf("invalid artifact set identity or bounds")
	}
	for _, ch := range s.RunID {
		if ch < 0x21 || ch > 0x7e {
			return fmt.Errorf("invalid run identity")
		}
	}
	last := ""
	var total int64
	for _, e := range m.Members {
		if !idPattern.MatchString(e.ID) || e.ID <= last || e.Size < 0 || e.Size > MaxFile || !canonical.ValidDigest(e.Digest) {
			return fmt.Errorf("invalid, unordered or duplicate artifact")
		}
		switch e.Kind {
		case "source", "git-bundle", "runtime-record", "runtime-binary", "build-output", "database-backup", "distribution", "qualification", "test-evidence", "context":
		default:
			return fmt.Errorf("unknown artifact kind")
		}
		total += e.Size
		if total > MaxTotal {
			return fmt.Errorf("artifact set exceeds byte limit")
		}
		last = e.ID
	}
	return nil
}
func decodeManifest(raw []byte) (Manifest, error) {
	var m Manifest
	if int64(len(raw)) > MaxMetadata {
		return m, fmt.Errorf("manifest exceeds limit")
	}
	if err := strictjson.Decode(raw, &m); err != nil {
		return m, err
	}
	if err := m.Validate(); err != nil {
		return m, err
	}
	encoded, err := json.Marshal(m)
	if err != nil || string(encoded) != string(raw) {
		return m, fmt.Errorf("noncanonical manifest")
	}
	return m, nil
}
func report(m Manifest, raw []byte, digest string, size int64, status string) Report {
	r := Report{Status: status, ArchiveDigest: digest, ArchiveSize: size, ManifestDigest: canonical.BytesDigest(raw), Coverage: Coverage, Subject: m.Subject, Members: len(m.Members)}
	for _, e := range m.Members {
		r.Bytes += e.Size
	}
	return r
}
func tarSize(m Manifest, raw []byte) int64 {
	n := int64(512 + ((len(raw)+511)/512)*512 + 1024)
	for _, e := range m.Members {
		n += 512 + ((e.Size+511)/512)*512
	}
	return n
}
