//go:build linux

package artifactset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func privateRoot(dir string) (*os.Root, os.FileInfo, error) {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return nil, nil, fmt.Errorf("canonical absolute private directory required")
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil || resolved != dir {
		return nil, nil, fmt.Errorf("aliased directory rejected")
	}
	st, err := os.Lstat(dir)
	if err != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 || !owned(st) {
		return nil, nil, fmt.Errorf("owned private directory required")
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, nil, err
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(st, opened) {
		_ = root.Close()
		return nil, nil, fmt.Errorf("directory changed before open")
	}
	return root, st, nil
}
func owned(st os.FileInfo) bool {
	s, ok := st.Sys().(*syscall.Stat_t)
	return ok && s.Uid == uint32(os.Geteuid())
}
func stable(a, b os.FileInfo) bool {
	if a == nil || b == nil || !os.SameFile(a, b) || a.Mode() != b.Mode() || a.Size() != b.Size() || !a.ModTime().Equal(b.ModTime()) {
		return false
	}
	x, xok := a.Sys().(*syscall.Stat_t)
	y, yok := b.Sys().(*syscall.Stat_t)
	return xok && yok && x.Ctim == y.Ctim && x.Nlink == y.Nlink && x.Uid == y.Uid
}
func rootUnchanged(dir string, st os.FileInfo) error {
	resolved, err := filepath.EvalSymlinks(dir)
	now, e := os.Lstat(dir)
	if err != nil || e != nil || resolved != dir || !os.SameFile(st, now) || !owned(now) || now.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("private directory identity changed")
	}
	return nil
}

type inputFile struct {
	root           *os.Root
	file           *os.File
	name, dir      string
	before, parent os.FileInfo
}

func openInput(p string, limit int64) (*inputFile, error) {
	if !filepath.IsAbs(p) || filepath.Clean(p) != p {
		return nil, fmt.Errorf("canonical absolute file required")
	}
	dir, name := filepath.Dir(p), filepath.Base(p)
	root, parent, err := privateRoot(dir)
	if err != nil {
		return nil, err
	}
	fail := func(e error) (*inputFile, error) { _ = root.Close(); return nil, e }
	before, err := root.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || !owned(before) || before.Mode().Perm()&0022 != 0 || before.Size() < 0 || before.Size() > limit {
		return fail(fmt.Errorf("unsafe or oversized input file"))
	}
	if s, ok := before.Sys().(*syscall.Stat_t); !ok || s.Nlink != 1 {
		return fail(fmt.Errorf("hard-linked input rejected"))
	}
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return fail(err)
	}
	opened, err := f.Stat()
	if err != nil || !stable(before, opened) {
		_ = f.Close()
		return fail(fmt.Errorf("input changed before read"))
	}
	return &inputFile{root: root, file: f, name: name, dir: dir, before: before, parent: parent}, nil
}
func (f *inputFile) Close() { _ = f.file.Close(); _ = f.root.Close() }
func (f *inputFile) check() error {
	after, err := f.file.Stat()
	now, e := f.root.Lstat(f.name)
	if err != nil || e != nil || !stable(f.before, after) || !stable(f.before, now) {
		return fmt.Errorf("input changed during read")
	}
	return rootUnchanged(f.dir, f.parent)
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(b []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(b)
}
func copyExact(ctx context.Context, r io.Reader, w io.Writer, size int64, digest string) error {
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(contextReader{ctx, r}, size+1))
	if err != nil {
		return err
	}
	if n != size || "sha256:"+hex.EncodeToString(h.Sum(nil)) != digest {
		return fmt.Errorf("artifact byte identity mismatch")
	}
	return nil
}
func readInput(ctx context.Context, p string, limit int64) ([]byte, error) {
	f, err := openInput(p, limit)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(contextReader{ctx, f.file}, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, fmt.Errorf("bounded input read failed")
	}
	return b, f.check()
}
func syncRoot(root *os.Root) error {
	d, err := root.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(d.Sync(), d.Close())
}
func hashReader(ctx context.Context, r io.Reader) (string, error) {
	h := sha256.New()
	_, err := io.Copy(h, contextReader{ctx, r})
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), err
}
