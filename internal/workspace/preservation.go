package workspace

import (
	"context"
	"errors"
	"os"
	"strings"
)

// PreservationHead observes only a stopped, owned source slot. Unlike Head it
// also permits the one detached child commit Finalize may have written before
// failing. This grants neither execution/reopen nor successful delivery. The
// caller must hold the sealed kernel stop proof before attempting a capture.
func (m *Manager) PreservationHead(ctx context.Context, w Workspace) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	root, err := m.validateManaged(w)
	if err != nil {
		return "", err
	}
	defer root.Close()
	info, err := root.Lstat(w.ID + "/source/.git")
	if err != nil || !info.IsDir() {
		return "", ErrUnmanagedWorkspace
	}
	headFile := w.ID + "/source/.git/HEAD"
	before, err := boundedRootFile(root, headFile, 64)
	head := strings.TrimSuffix(string(before), "\n")
	if err != nil || !fullCommitPattern.MatchString(head) || string(before) != head+"\n" {
		return "", ErrDirtyWorkspace
	}
	config, err := boundedRootFile(root, w.ID+"/source/.git/config", 64<<10)
	if err != nil || rawDigest(config) != w.ConfigDigest {
		return "", ErrDirtyWorkspace
	}
	for _, name := range []string{"commondir", "objects/info/alternates", "info/attributes", "hooks", "info/grafts"} {
		if _, err := root.Lstat(w.ID + "/source/.git/" + name); !errors.Is(err, os.ErrNotExist) {
			return "", ErrDirtyWorkspace
		}
	}
	// Create intentionally fetches depth=1. Only its original shallow boundary
	// may remain; never let altered graph metadata make a child look admissible.
	shallow, shallowErr := boundedRootFile(root, w.ID+"/source/.git/shallow", 64)
	if shallowErr != nil {
		if _, e := root.Lstat(w.ID + "/source/.git/shallow"); !errors.Is(e, os.ErrNotExist) {
			return "", ErrDirtyWorkspace
		}
	} else if string(shallow) != w.BaseCommit+"\n" {
		return "", ErrDirtyWorkspace
	}
	if head != w.BaseCommit {
		// Raw commit headers, not revision traversal, establish exact direct parent.
		value, err := m.gitOutput(ctx, w.WorktreePath, w.HomePath, "cat-file", "commit", head)
		if err != nil {
			return "", err
		}
		headers, _, ok := strings.Cut(value, "\n\n")
		parents := []string{}
		for _, line := range strings.Split(headers, "\n") {
			if strings.HasPrefix(line, "parent ") {
				parents = append(parents, strings.TrimPrefix(line, "parent "))
			}
		}
		if !ok || len(parents) != 1 || parents[0] != w.BaseCommit {
			return "", ErrDirtyWorkspace
		}
	}
	after, err := boundedRootFile(root, headFile, 64)
	configAfter, configErr := boundedRootFile(root, w.ID+"/source/.git/config", 64<<10)
	if err != nil || string(after) != string(before) || configErr != nil || rawDigest(configAfter) != w.ConfigDigest {
		return "", ErrDirtyWorkspace
	}
	return head, nil
}
