package core

import (
	"fmt"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"strings"
)

// ContinuationRef identifies reviewed source bytes, not a model-session token or
// permission to replay the old execution. Only the atomic continuation operation
// may create a Run carrying this reference.
type ContinuationRef struct {
	SourceRunID      string `json:"source_run_id"`
	CheckpointDigest string `json:"checkpoint_digest"`
	ArchiveDigest    string `json:"archive_digest"`
	ArchiveSize      int64  `json:"archive_size"`
	SnapshotDigest   string `json:"snapshot_digest"`
}

func (c ContinuationRef) Validate(runID string) error {
	if c.SourceRunID == "" || len(c.SourceRunID) > 256 || c.SourceRunID == runID || strings.ContainsAny(c.SourceRunID, "\x00\r\n") || c.ArchiveSize <= 0 || c.ArchiveSize > 320<<20 {
		return fmt.Errorf("invalid source continuation identity")
	}
	for _, d := range []string{c.CheckpointDigest, c.ArchiveDigest, c.SnapshotDigest} {
		if !canonical.ValidDigest(d) {
			return fmt.Errorf("invalid source continuation digest")
		}
	}
	return nil
}
