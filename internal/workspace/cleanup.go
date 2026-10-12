package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// Cooperative budgets do not make a blocking kernel filesystem call interruptible.
// Never detach a goroutine and report success while it continues deleting.
const cleanupBudget = 5 * time.Second
const cleanupMaxEntries = 100000
const cleanupMaxDepth = 128

var ErrCleanupIncomplete = errors.New("owned workspace cleanup incomplete; retain and inspect original slot")

// CleanupAfterFailure owns one separately bounded cleanup lifecycle, limited to
// trusted preparation before a Runtime starts, never general artifact GC.
func (m *Manager) CleanupAfterFailure(ctx context.Context, w Workspace) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupBudget)
	defer cancel()
	return m.Cleanup(cleanup, w)
}

// Keep owner.json until all other contents are removed, including on cancellation.
func (m *Manager) cleanupManaged(ctx context.Context, w Workspace) error {
	ctx, cancel := context.WithTimeout(ctx, cleanupBudget)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return errors.Join(ErrCleanupIncomplete, err)
	}
	root, err := m.validateManaged(w)
	if err != nil {
		return errors.Join(ErrCleanupIncomplete, err)
	}
	defer root.Close()
	slot, err := root.OpenRoot(w.ID)
	if err != nil {
		return errors.Join(ErrCleanupIncomplete, err)
	}
	defer slot.Close()
	before, err := slot.Stat(".")
	if err != nil {
		return errors.Join(ErrCleanupIncomplete, err)
	}
	owner, err := boundedRootFile(slot, "owner.json", 16<<10)
	if err != nil {
		return errors.Join(ErrCleanupIncomplete, err)
	}
	remaining := cleanupMaxEntries
	if err := cleanupContents(ctx, slot, 0, &remaining, true); err != nil {
		return errors.Join(ErrCleanupIncomplete, err)
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(ErrCleanupIncomplete, err)
	}
	current, err := root.Lstat(w.ID)
	if err != nil || !os.SameFile(before, current) {
		return errors.Join(ErrCleanupIncomplete, ErrUnmanagedWorkspace, err)
	}
	// Only the owner record and empty slot remain. No recursive final removal:
	// a concurrent writer must cause failure, not deletion of newly written data.
	if err := slot.Remove("owner.json"); err != nil {
		return errors.Join(ErrCleanupIncomplete, err)
	}
	if err := root.Remove(w.ID); err != nil {
		// Synchronous best effort: do not hide failure to restore ownership either.
		restoreErr := restoreCleanupOwner(slot, owner)
		return errors.Join(ErrCleanupIncomplete, err, restoreErr)
	}
	return nil
}

// Never follows links or opens special files. Each opened directory is compared
// with its lstat identity; traversal stays under its anchored Root descriptor.
func cleanupContents(ctx context.Context, root *os.Root, depth int, remaining *int, preserveOwner bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if depth > cleanupMaxDepth {
		return fmt.Errorf("cleanup depth budget exhausted")
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, readErr := dir.ReadDir(64)
		for _, entry := range entries {
			if preserveOwner && entry.Name() == "owner.json" {
				continue
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if *remaining <= 0 {
				return fmt.Errorf("cleanup entry budget exhausted")
			}
			*remaining--
			name := entry.Name()
			info, err := root.Lstat(name)
			if err != nil {
				return err
			}
			if info.IsDir() {
				child, err := root.OpenRoot(name)
				if err != nil {
					return err
				}
				opened, statErr := child.Stat(".")
				if statErr != nil || !os.SameFile(info, opened) {
					_ = child.Close()
					return errors.Join(ErrUnmanagedWorkspace, statErr)
				}
				walkErr := cleanupContents(ctx, child, depth+1, remaining, false)
				closeErr := child.Close()
				if err := errors.Join(walkErr, closeErr); err != nil {
					return err
				}
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			current, err := root.Lstat(name)
			if err != nil || !os.SameFile(info, current) {
				return errors.Join(ErrUnmanagedWorkspace, err)
			}
			if err := root.Remove(name); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

// Create exclusively allocated this pre-Runtime slot before owner.json exists.
// Failure retains an explicit incomplete slot, never deletes another slot or repo.
func cleanupIncompleteCreate(ctx context.Context, root *os.Root, id string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupBudget)
	defer cancel()
	child, err := root.OpenRoot(id)
	if err != nil {
		return errors.Join(ErrCleanupIncomplete, err)
	}
	defer child.Close()
	before, err := child.Stat(".")
	if err != nil {
		return errors.Join(ErrCleanupIncomplete, err)
	}
	remaining := cleanupMaxEntries
	if err := cleanupContents(ctx, child, 0, &remaining, false); err != nil {
		return errors.Join(ErrCleanupIncomplete, err)
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(ErrCleanupIncomplete, err)
	}
	current, err := root.Lstat(id)
	if err != nil || !os.SameFile(before, current) {
		return errors.Join(ErrCleanupIncomplete, ErrUnmanagedWorkspace, err)
	}
	if err := root.Remove(id); err != nil {
		return errors.Join(ErrCleanupIncomplete, err)
	}
	return nil
}

// Never overwrite a concurrent replacement of the ownership record.
func restoreCleanupOwner(slot *os.Root, owner []byte) error {
	file, err := slot.OpenFile("owner.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	n, writeErr := file.Write(owner)
	if n != len(owner) && writeErr == nil {
		writeErr = io.ErrShortWrite
	}
	return errors.Join(writeErr, file.Sync(), file.Close())
}
