//go:build linux

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"

	"github.com/jiying2007/engineering-platform/internal/production"
)

type productionObserverDirectoryID struct{ device, inode uint64 }

type productionObserverJournalFile struct {
	file, parent *os.File
	reader       io.Reader
	path         string
	before       syscall.Stat_t
	directories  []productionObserverDirectoryID
}

func observerJournalStat(ctx context.Context, f *os.File) (syscall.Stat_t, error) {
	var zero syscall.Stat_t
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	info, err := f.Stat()
	if err != nil {
		return zero, err
	}
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || (info.Mode() != 0600 && info.Mode() != 0400) || s.Uid != uint32(os.Geteuid()) || s.Nlink != 1 || info.Size() <= 0 || info.Size() > production.ObserverMaxJournalBytes {
		return zero, fmt.Errorf("observer journal requires one bounded, private 0600/0400 regular file owned by the invoking identity")
	}
	return *s, nil
}

func sameObserverJournalStat(a, b syscall.Stat_t) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.Uid == b.Uid && a.Gid == b.Gid && a.Nlink == b.Nlink && a.Mode == b.Mode && a.Size == b.Size && a.Mtim == b.Mtim && a.Ctim == b.Ctim
}

// Every component is opened relative to the preceding directory descriptor.
// NOFOLLOW applies during the actual traversal, not only to a preliminary
// pathname check. NONBLOCK rejects a raced FIFO without waiting for its writer.
func openProductionObserverJournal(ctx context.Context, path string) (productionObserverJournalInput, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(path) > 4096 || strings.Count(path, "/") > 128 || !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" || strings.ContainsRune(path, 0) {
		return nil, fmt.Errorf("absolute canonical observer journal path required")
	}
	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open observer journal root: %w", err)
	}
	parent := os.NewFile(uintptr(fd), "/")
	var directories []productionObserverDirectoryID
	parts := strings.Split(path[1:], "/")
	for i := 0; ; i++ {
		if err := ctx.Err(); err != nil {
			parent.Close()
			return nil, err
		}
		var s syscall.Stat_t
		if err = syscall.Fstat(int(parent.Fd()), &s); err != nil {
			parent.Close()
			return nil, fmt.Errorf("inspect observer journal directory: %w", err)
		}
		directories = append(directories, productionObserverDirectoryID{uint64(s.Dev), s.Ino})
		if i == len(parts)-1 {
			break
		}
		if err := ctx.Err(); err != nil {
			parent.Close()
			return nil, err
		}
		next, openErr := syscall.Openat(int(parent.Fd()), parts[i], syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		closeErr := parent.Close()
		if openErr != nil || closeErr != nil {
			if openErr == nil {
				syscall.Close(next)
			}
			return nil, fmt.Errorf("open observer journal directory: %w", errors.Join(openErr, closeErr))
		}
		parent = os.NewFile(uintptr(next), parts[i])
	}
	if err := ctx.Err(); err != nil {
		parent.Close()
		return nil, err
	}
	leaf, err := syscall.Openat(int(parent.Fd()), parts[len(parts)-1], syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		parent.Close()
		return nil, fmt.Errorf("open observer journal: %w", err)
	}
	file := os.NewFile(uintptr(leaf), path)
	before, err := observerJournalStat(ctx, file)
	if err != nil {
		file.Close()
		parent.Close()
		return nil, err
	}
	return &productionObserverJournalFile{file: file, parent: parent, reader: io.LimitReader(file, before.Size+1), path: path, before: before, directories: directories}, nil
}

func (f *productionObserverJournalFile) Read(p []byte) (int, error) { return f.reader.Read(p) }

func (f *productionObserverJournalFile) Close() error {
	var a, b error
	if f.file != nil {
		a = f.file.Close()
		f.file = nil
	}
	if f.parent != nil {
		b = f.parent.Close()
		f.parent = nil
	}
	return errors.Join(a, b)
}

func (f *productionObserverJournalFile) Verify(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if f.file == nil {
		return fmt.Errorf("observer journal already closed")
	}
	after, err := observerJournalStat(ctx, f.file)
	if err != nil {
		return fmt.Errorf("inspect observer journal after read: %w", err)
	}
	if !sameObserverJournalStat(f.before, after) {
		return fmt.Errorf("observer journal changed during read")
	}
	opened, err := openProductionObserverJournal(ctx, f.path)
	if err != nil {
		return fmt.Errorf("observer journal path changed during read: %w", err)
	}
	now := opened.(*productionObserverJournalFile)
	valid := sameObserverJournalStat(f.before, now.before) && reflect.DeepEqual(f.directories, now.directories)
	closeErr := now.Close()
	if !valid || closeErr != nil {
		return fmt.Errorf("observer journal path or metadata changed during read")
	}
	return ctx.Err()
}
