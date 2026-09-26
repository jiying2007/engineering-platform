package preparation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/contextbundle"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
	"github.com/jiying2007/engineering-platform/internal/workspace"
)

// FinalizeChangedWorkspace converts one authorized prepared slot into an
// independent result commit/bundle. It deliberately does not call Recheck after
// execution because the source bytes are expected to have changed. Frozen
// task/context/approval identities are still revalidated before Git mutation.
func (p *Preparer) FinalizeChangedWorkspace(ctx context.Context, subject string, a workerqueue.Assignment, prepared Result, preparationDigest, artifactID string) (workspace.Finalized, error) {
	var result workspace.Finalized
	if p == nil || subject != p.worker || preparationDigest == "" {
		return result, contextbundle.ErrDenied
	}
	if _, err := workerqueue.Validate(a); err != nil {
		return result, err
	}
	if err := prepared.Facts.Check(a); err != nil {
		return result, err
	}
	actual, err := canonical.Digest(prepared.Facts)
	if err != nil || actual != preparationDigest ||
		prepared.Facts.SourceDigest != prepared.Workspace.SourceDigest ||
		prepared.Facts.ConfigDigest != prepared.Workspace.ConfigDigest ||
		prepared.Facts.TreeCommit != prepared.Workspace.TreeCommit {
		return result, workerqueue.ErrIdentity
	}
	bundle, err := p.bundles.Verify(ctx, a.Input)
	if err != nil {
		return result, err
	}
	if bundle.Path != prepared.BundlePath || bundle.Digest != prepared.Facts.BundleDigest {
		return result, workerqueue.ErrIdentity
	}
	head, err := p.manager.Head(ctx, prepared.Workspace)
	if err != nil || head != prepared.Workspace.BaseCommit {
		return result, workerqueue.ErrIdentity
	}
	artifacts := filepath.Join(p.root, "artifacts")
	finalized, err := p.manager.Finalize(ctx, prepared.Workspace, artifacts, artifactID)
	if err != nil {
		return result, err
	}
	if finalized.Facts.BaseCommit != prepared.Facts.BaseCommit ||
		finalized.Facts.BaseTree != prepared.Facts.TreeCommit ||
		finalized.Facts.BaseSourceDigest != prepared.Facts.SourceDigest {
		_ = os.Remove(finalized.BundlePath)
		return result, workerqueue.ErrIdentity
	}
	return finalized, nil
}

// SaveCodex retains local reconciliation bytes after the workspace may have
// changed. The caller-supplied ID is fixed alphabet and cannot escape the owned
// workspace slot.
func (p *Preparer) SaveCodex(a workerqueue.Assignment, prepared Result, id string, value any) error {
	if p == nil || len(id) != 64 || strings.Trim(id, "0123456789abcdef") != "" {
		return workerqueue.ErrIdentity
	}
	if _, err := workerqueue.Validate(a); err != nil {
		return err
	}
	if prepared.Facts.TaskDigest != a.Intent.TaskDigest || prepared.Facts.InputDigest != a.Intent.InputDigest ||
		prepared.Workspace.ID == "" || prepared.Workspace.WorktreePath == "" {
		return workerqueue.ErrIdentity
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data) > 4<<20 {
		return fmt.Errorf("Codex reconciliation record exceeds limit")
	}
	return saveOfflineRecord(filepath.Join(p.root, "workspaces", prepared.Workspace.ID), "codex-"+id+".json", data)
}
