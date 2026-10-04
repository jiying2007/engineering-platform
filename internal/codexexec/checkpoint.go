package codexexec

import (
	"context"
	"fmt"

	"github.com/jiying2007/engineering-platform/internal/canonical"
)

// SourceCheckpoint is a retained artifact observation, never a new execution,
// successful result, process-memory snapshot, or permission to resume/take over.
type SourceCheckpoint struct {
	Version          int            `json:"version"`
	Binding          ControlBinding `json:"binding"`
	TaskDigest       string         `json:"task_contract_digest"`
	InputDigest      string         `json:"run_input_manifest_digest"`
	BaseCommit       string         `json:"base_commit"`
	BaselineDigest   string         `json:"baseline_source_digest"`
	TranscriptDigest string         `json:"control_transcript_digest"`
	SnapshotDigest   string         `json:"snapshot_digest"`
	ArchiveDigest    string         `json:"archive_digest"`
	ArchiveSize      int64          `json:"archive_size"`
}

func (c SourceCheckpoint) Validate() error {
	if c.Version != 1 || c.Binding.Validate() != nil || len(c.BaseCommit) != 40 || c.ArchiveSize <= 0 || c.ArchiveSize > 320<<20 {
		return fmt.Errorf("invalid source checkpoint")
	}
	for _, ch := range c.BaseCommit {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			return fmt.Errorf("invalid checkpoint base")
		}
	}
	for _, d := range []string{c.TaskDigest, c.InputDigest, c.BaselineDigest, c.TranscriptDigest, c.SnapshotDigest, c.ArchiveDigest} {
		if !canonical.ValidDigest(d) {
			return fmt.Errorf("invalid checkpoint identity")
		}
	}
	return nil
}
func (c SourceCheckpoint) Digest() (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	return canonical.Digest(c)
}

type CheckpointRepository interface {
	SaveCodexCheckpoint(context.Context, string, SourceCheckpoint) (SourceCheckpoint, error)
}
