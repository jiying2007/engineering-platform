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
	source, root, _, err := p.checkpointPaths(ctx, subject, a, prepared, preparationDigest, false)
	return source, root, err
}

// PostFinalizeCheckpointPaths is preservation-only: the original base or one
// detached Finalize child is allowed, while normal Reopen/Head remain strict.
// It requires the caller to supply an already sealed, quiescent runtime proof.
func (p *Preparer) PostFinalizeCheckpointPaths(ctx context.Context, subject string, a workerqueue.Assignment, prepared Result, preparationDigest string) (string, string, error) {
	source, root, _, err := p.checkpointPaths(ctx, subject, a, prepared, preparationDigest, true)
	return source, root, err
}

func (p *Preparer) CheckpointGitBundle(ctx context.Context, subject string, a workerqueue.Assignment, prepared Result, preparationDigest, artifactID string, finalized bool) (workspace.PreservationBundle, error) {
	_, root, head, err := p.checkpointPaths(ctx, subject, a, prepared, preparationDigest, finalized)
	if err != nil {
		return workspace.PreservationBundle{}, err
	}
	return p.manager.RetainPreservationBundle(ctx, prepared.Workspace, root, artifactID, head)
}

func (p *Preparer) checkpointPaths(ctx context.Context, subject string, a workerqueue.Assignment, prepared Result, preparationDigest string, finalized bool) (string, string, string, error) {
	if p == nil || subject != p.worker || prepared.Facts.Check(a) != nil || prepared.Facts.BaseCommit != prepared.Workspace.BaseCommit {
		return "", "", "", workerqueue.ErrIdentity
	}
	d, err := canonical.Digest(prepared.Facts)
	if err != nil || d != preparationDigest || prepared.Facts.SeedSourceDigest != prepared.Workspace.SeedSourceDigest || prepared.Facts.SourceDigest != prepared.Workspace.SourceDigest || prepared.Facts.ConfigDigest != prepared.Workspace.ConfigDigest || prepared.Facts.TreeCommit != prepared.Workspace.TreeCommit {
		return "", "", "", workerqueue.ErrIdentity
	}
	var head string
	if finalized {
		head, err = p.manager.PreservationHead(ctx, prepared.Workspace)
	} else {
		head, err = p.manager.Head(ctx, prepared.Workspace)
	}
	if err != nil || head == "" {
		return "", "", "", workerqueue.ErrIdentity
	}
	bundle, err := p.bundles.Verify(ctx, a.Input)
	if err != nil || bundle.Path != prepared.BundlePath || bundle.Digest != prepared.Facts.BundleDigest {
		return "", "", "", workerqueue.ErrIdentity
	}
	artifactRoot := filepath.Join(p.root, "artifacts")
	st, err := os.Lstat(artifactRoot)
	if err != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return "", "", "", fmt.Errorf("private artifact root required")
	}
	return prepared.Workspace.WorktreePath, artifactRoot, head, nil
}
