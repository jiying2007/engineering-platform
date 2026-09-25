package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type snapshotEntry struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Executable bool   `json:"executable"`
	Digest     string `json:"digest"`
}

// Snapshot the checkout bytes without executing repository-configured filters,
// hooks or filesystem monitors. Symlinks are hashed as link text, never followed.
func snapshotDigest(ctx context.Context, path string) (string, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return "", err
	}
	defer root.Close()
	var entries []snapshotEntry
	var total int64
	var walk func(string, int) error
	walk = func(dir string, depth int) error {
		if depth > 64 {
			return fmt.Errorf("workspace nesting limit exceeded")
		}
		f, err := root.Open(dir)
		if err != nil {
			return err
		}
		defer f.Close()
		for {
			batch, readErr := f.ReadDir(256)
			if readErr != nil && readErr != io.EOF {
				return readErr
			}
			for _, item := range batch {
				if dir == "." && item.Name() == ".git" {
					continue
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				name := filepath.Join(dir, item.Name())
				if len(entries) >= 100000 || len(name) > 4096 {
					return fmt.Errorf("workspace entry limit exceeded")
				}
				info, err := root.Lstat(name)
				if err != nil {
					return err
				}
				entry := snapshotEntry{Path: filepath.ToSlash(name)}
				switch {
				case info.IsDir():
					entry.Kind = "directory"
					entries = append(entries, entry)
					if err := walk(name, depth+1); err != nil {
						return err
					}
					continue
				case info.Mode()&os.ModeSymlink != 0:
					entry.Kind = "symlink"
					target, err := root.Readlink(name)
					if err != nil {
						return err
					}
					hash := sha256.Sum256([]byte(target))
					entry.Digest = hex.EncodeToString(hash[:])
				case info.Mode().IsRegular():
					entry.Kind = "file"
					entry.Executable = info.Mode().Perm()&0o111 != 0
					if info.Size() > 256<<20 {
						return fmt.Errorf("workspace file limit exceeded")
					}
					file, err := root.Open(name)
					if err != nil {
						return err
					}
					actual, err := file.Stat()
					if err != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) {
						_ = file.Close()
						return ErrUnmanagedWorkspace
					}
					hash := sha256.New()
					size, copyErr := io.Copy(hash, io.LimitReader(contextReader{ctx, file}, (256<<20)+1))
					closeErr := file.Close()
					if copyErr != nil {
						return copyErr
					}
					if closeErr != nil {
						return closeErr
					}
					total += size
					if size > 256<<20 || total > 1<<30 {
						return fmt.Errorf("workspace byte limit exceeded")
					}
					entry.Digest = hex.EncodeToString(hash.Sum(nil))
				default:
					return fmt.Errorf("unsupported workspace file type")
				}
				entries = append(entries, entry)
			}
			if readErr == io.EOF {
				break
			}
		}
		return nil
	}
	if err := walk(".", 0); err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	for _, entry := range entries {
		if err := encoder.Encode(entry); err != nil {
			return "", err
		}
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
