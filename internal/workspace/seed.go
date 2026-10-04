package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// AdoptRestoredSource is usable only before preparation is published. The caller
// must have independently verified/restored the source checkpoint into this
// exact owned slot. Git authority is from Create(), never from checkpoint bytes.
// No caller path, live workspace overlay, or Git commit is accepted here.
func (m *Manager) AdoptRestoredSource(ctx context.Context, w Workspace) (Workspace, error) {
	if w.SeedSourceDigest != "" {
		return w, ErrDirtyWorkspace
	}
	clean, err := m.IsClean(ctx, w)
	if err != nil || !clean {
		return w, ErrDirtyWorkspace
	}
	root, err := m.validateManaged(w)
	if err != nil {
		return w, err
	}
	defer root.Close()
	if _, err = root.Lstat(w.ID + "/prepared.json"); !errors.Is(err, os.ErrNotExist) {
		return w, ErrWorkspaceExists
	}
	source := filepath.Join(m.root, w.ID, "seed-restore", "source")
	for _, dir := range []string{filepath.Dir(source), source} {
		info, e := canonicalDirectory(dir)
		if e != nil || info.Mode().Perm()&0077 != 0 {
			return w, ErrUnmanagedWorkspace
		}
	}
	if _, err = root.Lstat(w.ID + "/seed-restore/INCOMPLETE"); !errors.Is(err, os.ErrNotExist) {
		return w, ErrDirtyWorkspace
	}
	if _, err = boundedRootFile(root, w.ID+"/seed-restore/RESTORED.json", 64<<10); err != nil {
		return w, err
	}
	if _, err = root.Lstat(w.ID + "/seed-restore/source/.git"); !errors.Is(err, os.ErrNotExist) {
		return w, ErrUnmanagedWorkspace
	}
	before, err := snapshotDigest(ctx, source)
	if err != nil {
		return w, err
	}
	// All destinations lie in a fresh unexposed slot. A partial failure never
	// yields a preparation receipt; the owning Preparer removes the failed slot.
	if err = root.Rename(w.ID+"/source", w.ID+"/base-source"); err != nil {
		return w, err
	}
	if err = root.Rename(w.ID+"/base-source/.git", w.ID+"/seed-restore/source/.git"); err != nil {
		return w, err
	}
	if err = root.Rename(w.ID+"/seed-restore/source", w.ID+"/source"); err != nil {
		return w, err
	}
	after, err := snapshotDigest(ctx, w.WorktreePath)
	if err != nil || before != after {
		return w, ErrDirtyWorkspace
	}
	if err = root.RemoveAll(w.ID + "/base-source"); err != nil {
		return w, err
	}
	updated := w
	updated.SeedSourceDigest = after
	raw, err := json.Marshal(updated)
	if err != nil {
		return w, err
	}
	file, err := root.OpenFile(w.ID+"/owner.next.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return w, err
	}
	_, we := file.Write(raw)
	se := file.Sync()
	ce := file.Close()
	if err = errors.Join(we, se, ce); err != nil {
		return w, err
	}
	if err = root.Rename(w.ID+"/owner.next.json", w.ID+"/owner.json"); err != nil {
		return w, err
	}
	dir, err := root.Open(w.ID)
	if err != nil {
		return updated, err
	}
	err = dir.Sync()
	closeErr := dir.Close()
	return updated, errors.Join(err, closeErr)
}
