package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"time"
)

type BackupEntry struct {
	Path   string `json:"path"`
	Type   string `json:"type"`
	Mode   uint32 `json:"mode"`
	Size   int64  `json:"size,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Link   string `json:"link,omitempty"`
}

type BackupManifest struct {
	Format    int           `json:"format"`
	Version   string        `json:"eng_version"`
	Schema    int           `json:"schema"`
	SourceDir string        `json:"source_dir"`
	CreatedAt time.Time     `json:"created_at"`
	Scope     string        `json:"scope"`
	Entries   []BackupEntry `json:"entries"`
}

// Backup takes a SQLite snapshot only while execution is quiescent. A worktree
// backup still depends on its original Git common directory; it is not a clone.
func Backup(ctx context.Context, s *Store, destination string) (BackupManifest, error) {
	var manifest BackupManifest
	lock, err := AcquireScheduler(s.DataDir())
	if err != nil {
		return manifest, fmt.Errorf("stop the scheduler before backup: %w", err)
	}
	defer lock.Close()
	if _, err := Recover(ctx, s); err != nil {
		return manifest, err
	}
	if err := ensurePublicationStopped(ctx, s); err != nil {
		return manifest, err
	}
	tasks, err := s.ListTasks(ctx)
	if err != nil {
		return manifest, err
	}
	for _, task := range tasks {
		attempts, err := s.ListAttempts(ctx, task.ID)
		if err != nil {
			return manifest, err
		}
		for _, attempt := range attempts {
			if attempt.Process == nil || attempt.Result.LocalProcessStopped {
				continue
			}
			alive, err := ProcessGroupAlive(*attempt.Process)
			if err != nil || alive {
				return manifest, fmt.Errorf("cannot back up unresolved task %s attempt %d: %w", task.ID, attempt.Number, ErrProcessUnresolved)
			}
		}
	}
	dest, err := canonicalNewDirectory(destination)
	if err != nil {
		return manifest, err
	}
	if withinDirectory(s.DataDir(), dest) {
		return manifest, errors.New("backup destination must be outside the data directory")
	}
	if err := os.Mkdir(dest, 0700); err != nil {
		return manifest, fmt.Errorf("backup needs a new directory: %w", err)
	}
	marker := filepath.Join(dest, ".backup-incomplete")
	if err := writeExclusive(marker, []byte("Do not restore an incomplete backup.\n")); err != nil {
		return manifest, err
	}
	// VACUUM INTO includes committed WAL data and never copies an inconsistent
	// live database file. Parameters prevent a path from becoming SQL syntax.
	if _, err := s.DB().ExecContext(ctx, "VACUUM INTO ?", filepath.Join(dest, "eng.db")); err != nil {
		return manifest, err
	}
	if err := os.Chmod(filepath.Join(dest, "eng.db"), 0600); err != nil {
		return manifest, err
	}
	if err := syncFile(filepath.Join(dest, "eng.db")); err != nil {
		return manifest, err
	}
	for _, name := range []string{"projects", "runtimes", "tasks"} {
		source := filepath.Join(s.DataDir(), name)
		if _, err := os.Lstat(source); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return manifest, err
		}
		if err := copyBackupTree(ctx, source, filepath.Join(dest, name)); err != nil {
			return manifest, err
		}
	}
	entries, err := backupInventory(ctx, dest)
	if err != nil {
		return manifest, err
	}
	manifest = BackupManifest{Format: 1, Version: Version, Schema: schemaVersion, SourceDir: s.DataDir(), CreatedAt: time.Now().UTC(), Scope: "same-host-original-path; original Git repositories and credentials are separate", Entries: entries}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return manifest, err
	}
	if err := writeExclusive(filepath.Join(dest, "manifest.json"), append(raw, '\n')); err != nil {
		return manifest, err
	}
	if err := os.Remove(marker); err != nil {
		return manifest, err
	}
	return manifest, syncDirectory(dest)
}

// Restore never overwrites current state or translates Git worktree pointers.
// Restored queued tasks are paused before the directory is marked usable.
func Restore(ctx context.Context, source, destination string) error {
	src, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	src, err = filepath.EvalSymlinks(src)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(filepath.Join(src, ".backup-incomplete")); err == nil {
		return errors.New("backup is incomplete")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	raw, err := readRegularFile(filepath.Join(src, "manifest.json"), 64<<20)
	if err != nil {
		return err
	}
	var manifest BackupManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return fmt.Errorf("invalid backup manifest: %w", err)
	}
	if manifest.Format != 1 || manifest.Schema != schemaVersion {
		return errors.New("backup format/schema requires its matching eng version")
	}
	dest, err := canonicalNewDirectory(destination)
	if err != nil {
		return err
	}
	if manifest.SourceDir != dest {
		return fmt.Errorf("restore requires original data directory %s; Git worktree pointers are not portable", manifest.SourceDir)
	}
	entries, err := backupInventory(ctx, src)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(entries, manifest.Entries) {
		return errors.New("backup contents do not match the manifest")
	}
	hasDatabase := false
	for _, entry := range entries {
		if entry.Path == "eng.db" && entry.Type == "file" {
			hasDatabase = true
		}
	}
	if !hasDatabase {
		return errors.New("backup must contain a regular eng.db snapshot")
	}
	if err := os.Mkdir(dest, 0700); err != nil {
		return fmt.Errorf("preserve existing state first; restore requires an absent destination: %w", err)
	}
	marker := filepath.Join(dest, ".restore-incomplete")
	if err := writeExclusive(marker, []byte("Restore has not completed; do not run tasks from this directory.\n")); err != nil {
		return err
	}
	for _, name := range []string{"eng.db", "projects", "runtimes", "tasks"} {
		path := filepath.Join(src, name)
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		if err := copyBackupTree(ctx, path, filepath.Join(dest, name)); err != nil {
			return err
		}
	}
	copied, err := backupInventory(ctx, dest)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(copied, manifest.Entries) {
		return errors.New("restored files failed integrity verification; incomplete directory retained for inspection")
	}
	s, err := OpenStore(dest)
	if err != nil {
		return err
	}
	lock, err := AcquireScheduler(dest)
	if err != nil {
		s.Close()
		return err
	}
	markErr := s.MarkRestored(ctx)
	closeErr := s.Close()
	lockErr := lock.Close()
	if markErr != nil {
		return markErr
	}
	if closeErr != nil {
		return closeErr
	}
	if lockErr != nil {
		return lockErr
	}
	if err := os.Remove(marker); err != nil {
		return err
	}
	return syncDirectory(dest)
}

func canonicalNewDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", fmt.Errorf("destination parent must exist: %w", err)
	}
	return filepath.Join(parent, filepath.Base(abs)), nil
}

func withinDirectory(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func backupInventory(ctx context.Context, root string) ([]BackupEntry, error) {
	entries := []BackupEntry{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "manifest.json" || rel == ".backup-incomplete" || rel == ".restore-incomplete" {
			return nil
		}
		top := strings.Split(rel, string(filepath.Separator))[0]
		if top != "eng.db" && top != "projects" && top != "runtimes" && top != "tasks" {
			return fmt.Errorf("unexpected backup entry %s", rel)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		item := BackupEntry{Path: filepath.ToSlash(rel), Mode: uint32(info.Mode().Perm())}
		switch {
		case info.IsDir():
			item.Type = "directory"
		case info.Mode()&os.ModeSymlink != 0:
			item.Type = "symlink"
			item.Link, err = os.Readlink(path)
		case info.Mode().IsRegular():
			item.Type, item.Size = "file", info.Size()
			file, openErr := openRegularFile(path)
			if openErr != nil {
				return openErr
			}
			hash := sha256.New()
			_, copyErr := io.Copy(hash, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			item.SHA256 = hex.EncodeToString(hash.Sum(nil))
		default:
			return fmt.Errorf("unsupported special file in backup: %s", rel)
		}
		if err != nil {
			return err
		}
		entries = append(entries, item)
		return nil
	})
	return entries, err
}

func copyBackupTree(ctx context.Context, source, destination string) error {
	var directories []string
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case info.IsDir():
			if err := os.Mkdir(target, info.Mode().Perm()); err != nil {
				return err
			}
			if err := os.Chmod(target, info.Mode().Perm()); err != nil {
				return err
			}
			directories = append(directories, target)
			return nil
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.Mode().IsRegular():
			in, err := openRegularFile(path)
			if err != nil {
				return err
			}
			defer in.Close()
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, info.Mode().Perm())
			if err != nil {
				return err
			}
			_, err = io.Copy(out, in)
			if err == nil {
				err = out.Chmod(info.Mode().Perm())
			}
			if err == nil {
				err = out.Sync()
			}
			closeErr := out.Close()
			if err != nil {
				return err
			}
			return closeErr
		default:
			return fmt.Errorf("cannot back up special file %s", path)
		}
	})
	if err != nil {
		return err
	}
	for i := len(directories) - 1; i >= 0; i-- {
		if err := syncDirectory(directories[i]); err != nil {
			return err
		}
	}
	return nil
}

func openRegularFile(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, errors.New("expected a regular file")
	}
	return file, nil
}

func readRegularFile(path string, limit int64) ([]byte, error) {
	file, err := openRegularFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("file exceeds size limit")
	}
	return data, nil
}

func syncFile(path string) error {
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
