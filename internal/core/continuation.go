package core

import (
	"fmt"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"strings"
)

// ContinuationRef identifies reviewed source bytes, not a model-session token or
// permission to replay the old execution. Only the atomic continuation operation
// may create a Run carrying this reference.
// MaxSourceCheckpointArchiveSize is the single Core/Worker contract bound for
// a retained source checkpoint archive. Archive encoding v2 uses the larger
// budget to include exact stopped source plus its self-contained Git-base graph.
const MaxSourceCheckpointArchiveSize int64 = 800 << 20

type ContinuationRef struct {
	SourceRunID      string `json:"source_run_id"`
	CheckpointDigest string `json:"checkpoint_digest"`
	ArchiveDigest    string `json:"archive_digest"`
	ArchiveSize      int64  `json:"archive_size"`
	SnapshotDigest   string `json:"snapshot_digest"`
}

func (c ContinuationRef) Validate(runID string) error {
	if c.SourceRunID == "" || len(c.SourceRunID) > 256 || c.SourceRunID == runID || strings.ContainsAny(c.SourceRunID, "\x00\r\n") || c.ArchiveSize <= 0 || c.ArchiveSize > MaxSourceCheckpointArchiveSize {
		return fmt.Errorf("invalid source continuation identity")
	}
	for _, d := range []string{c.CheckpointDigest, c.ArchiveDigest, c.SnapshotDigest} {
		if !canonical.ValidDigest(d) {
			return fmt.Errorf("invalid source continuation digest")
		}
	}
	return nil
}
