package preparation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
)

// CheckpointPaths revalidates ownership/frozen preparation, not changed source
// bytes. Callers additionally require a sealed namespace-termination proof.
func (p *Preparer) CheckpointPaths(ctx context.Context, subject string, a workerqueue.Assignment, prepared Result, preparationDigest string) (string, string, error) {
	if p == nil || subject != p.worker || prepared.Facts.Check(a) != nil {
		return "", "", workerqueue.ErrIdentity
	}
	d, err := canonical.Digest(prepared.Facts)
	if err != nil || d != preparationDigest || prepared.Facts.SourceDigest != prepared.Workspace.SourceDigest || prepared.Facts.ConfigDigest != prepared.Workspace.ConfigDigest || prepared.Facts.TreeCommit != prepared.Workspace.TreeCommit {
		return "", "", workerqueue.ErrIdentity
	}
	head, err := p.manager.Head(ctx, prepared.Workspace)
	if err != nil || head != prepared.Workspace.BaseCommit {
		return "", "", workerqueue.ErrIdentity
	}
	bundle, err := p.bundles.Verify(ctx, a.Input)
	if err != nil || bundle.Path != prepared.BundlePath || bundle.Digest != prepared.Facts.BundleDigest {
		return "", "", workerqueue.ErrIdentity
	}
	artifactRoot := filepath.Join(p.root, "artifacts")
	st, err := os.Lstat(artifactRoot)
	if err != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return "", "", fmt.Errorf("private artifact root required")
	}
	return prepared.Workspace.WorktreePath, artifactRoot, nil
}
