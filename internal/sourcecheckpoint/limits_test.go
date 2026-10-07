package sourcecheckpoint

import (
	"testing"

	"github.com/jiying2007/engineering-platform/internal/core"
)

func TestArchiveLimitMatchesCoreCheckpointContract(t *testing.T) {
	if MaxArchive != core.MaxSourceCheckpointArchiveSize {
		t.Fatalf("source checkpoint archive bound %d differs from Core contract %d", MaxArchive, core.MaxSourceCheckpointArchiveSize)
	}
	if MaxArchive != MaxSource+MaxGitBundle+(32<<20) {
		t.Fatal("archive budget no longer covers bounded source, Git graph and metadata overhead")
	}
}
