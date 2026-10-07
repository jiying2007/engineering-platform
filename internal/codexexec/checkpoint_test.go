package codexexec

import (
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
)

func TestSourceCheckpointArchiveSizeUsesCoreBound(t *testing.T) {
	_, permit := contractFixture(t)
	checkpoint := SourceCheckpoint{
		Version: 1,
		Binding: ControlBinding{
			Token:          permit.Token,
			ExecutionEpoch: permit.Assignment.Intent.ExecutionEpoch,
			ThreadID:       "thread",
			TurnID:         "turn",
		},
		TaskDigest:       permit.Assignment.Intent.TaskDigest,
		InputDigest:      permit.Assignment.Intent.InputDigest,
		BaseCommit:       permit.Preparation.Facts.BaseCommit,
		BaselineDigest:   permit.Preparation.Facts.SourceDigest,
		TranscriptDigest: canonical.BytesDigest([]byte("transcript")),
		SnapshotDigest:   canonical.BytesDigest([]byte("snapshot")),
		ArchiveDigest:    canonical.BytesDigest([]byte("archive")),
		ArchiveSize:      core.MaxSourceCheckpointArchiveSize,
	}
	if err := checkpoint.Validate(); err != nil {
		t.Fatal(err)
	}
	checkpoint.ArchiveSize++
	if err := checkpoint.Validate(); err == nil {
		t.Fatal("checkpoint archive above shared Core bound accepted")
	}
}
