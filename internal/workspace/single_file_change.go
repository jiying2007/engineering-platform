package workspace

import (
	"context"
	"fmt"
	"path"
	"strings"
)

// VerifySingleFileChange compares immutable Git objects in two managed snapshots.
// It proves only the requested source change, not the execution or approval of it.
// Worktree paths are never opened to read the marker's contents.
func (m *Manager) VerifySingleFileChange(ctx context.Context, base, result Workspace, name, before, after string) error {
	if name == "" || path.IsAbs(name) || path.Clean(name) != name || name == "." ||
		name == ".." || strings.HasPrefix(name, "../") || strings.ContainsRune(name, '\x00') || before == after {
		return fmt.Errorf("exact relative file path and distinct bytes required")
	}
	baseEntries, err := m.gitLeafEntries(ctx, base)
	if err != nil {
		return err
	}
	resultEntries, err := m.gitLeafEntries(ctx, result)
	if err != nil {
		return err
	}
	if len(baseEntries) != len(resultEntries) {
		return fmt.Errorf("single-file change must preserve the complete Git leaf set")
	}
	for file, entry := range baseEntries {
		if err := ctx.Err(); err != nil {
			return err
		}
		other, ok := resultEntries[file]
		if !ok {
			return fmt.Errorf("single-file change must preserve every Git path")
		}
		// Compare mode, type and object ID at every other exact path.
		if file != name && entry != other {
			return fmt.Errorf("Git leaf changed outside the required file")
		}
	}
	baseMarker, baseOK := baseEntries[name]
	resultMarker, resultOK := resultEntries[name]
	if !baseOK || !resultOK || baseMarker.kind != "blob" || resultMarker.kind != "blob" ||
		(baseMarker.mode != "100644" && baseMarker.mode != "100755") ||
		baseMarker.mode != resultMarker.mode || baseMarker.oid == resultMarker.oid {
		return fmt.Errorf("required file must be a changed regular blob with unchanged mode")
	}
	for _, item := range []struct {
		workspace Workspace
		oid       string
		want      string
	}{{base, baseMarker.oid, before}, {result, resultMarker.oid, after}} {
		data, err := m.gitOutput(ctx, item.workspace.WorktreePath, item.workspace.HomePath, "cat-file", "blob", item.oid)
		if err != nil {
			return err
		}
		if data != item.want {
			return fmt.Errorf("required file does not have the exact frozen bytes")
		}
	}
	return ctx.Err()
}

type gitLeafEntry struct {
	mode string
	kind string
	oid  string
}

func (m *Manager) gitLeafEntries(ctx context.Context, w Workspace) (map[string]gitLeafEntry, error) {
	commit, err := m.Head(ctx, w)
	if err != nil {
		return nil, err
	}
	data, err := m.gitOutput(ctx, w.WorktreePath, w.HomePath, "ls-tree", "-r", "-z", "--full-tree", commit)
	if err != nil {
		return nil, err
	}
	entries := make(map[string]gitLeafEntry)
	for data != "" {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		record, rest, ok := strings.Cut(data, "\x00")
		if !ok {
			return nil, fmt.Errorf("unterminated Git leaf record")
		}
		data = rest
		header, file, ok := strings.Cut(record, "\t")
		fields := strings.Split(header, " ")
		if !ok || file == "" || len(fields) != 3 || !fullCommitPattern.MatchString(fields[2]) {
			return nil, fmt.Errorf("invalid Git leaf record")
		}
		if _, duplicate := entries[file]; duplicate {
			return nil, fmt.Errorf("duplicate Git leaf path")
		}
		entries[file] = gitLeafEntry{mode: fields[0], kind: fields[1], oid: fields[2]}
	}
	return entries, nil
}
