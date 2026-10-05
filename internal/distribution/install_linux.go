//go:build linux

package distribution

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	productionassets "github.com/jiying2007/engineering-platform/examples/production"
)

func rawDigest(b []byte) string { h := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(h[:]) }

func controlledDirectory(path string, requireCaller bool) (os.FileInfo, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "\x00\r\n") {
		return nil, fmt.Errorf("canonical absolute directory required")
	}
	resolved, e := filepath.EvalSymlinks(path)
	st, err := os.Lstat(path)
	if e != nil || err != nil || resolved != path || !st.IsDir() || st.Mode().Perm()&0022 != 0 || st.Mode()&os.ModeSetgid != 0 {
		return nil, fmt.Errorf("unaliased non-shared-writable directory required")
	}
	s, ok := st.Sys().(*syscall.Stat_t)
	if !ok || (s.Uid != uint32(os.Geteuid()) && (requireCaller || s.Uid != 0)) {
		return nil, fmt.Errorf("directory is not controlled by installer or host administrator")
	}
	return st, nil
}

func installFiles(from string) ([]InstalledFile, map[string][]byte, error) {
	names := append(Names(), "file-manifest.json", "SHA256SUMS")
	out := make([]InstalledFile, 0, len(names)+32)
	for _, name := range names {
		executable := name != "file-manifest.json" && name != "SHA256SUMS"
		raw, err := readFile(filepath.Join(from, name), 128<<20, executable)
		if err != nil {
			return nil, nil, err
		}
		mode := uint32(0444)
		if executable {
			mode = 0555
		}
		out = append(out, InstalledFile{Path: "bin/" + name, Size: int64(len(raw)), Digest: rawDigest(raw), Mode: mode})
	}
	assets, err := productionassets.Files()
	if err != nil || len(assets) == 0 || len(assets) > 128 {
		return nil, nil, fmt.Errorf("invalid embedded deployment assets")
	}
	for name, raw := range assets {
		if !fs.ValidPath(name) || strings.ContainsAny(name, "\\\x00\r\n") || len(raw) > 1<<20 {
			return nil, nil, fmt.Errorf("invalid embedded asset")
		}
		out = append(out, InstalledFile{Path: "templates/" + name, Size: int64(len(raw)), Digest: rawDigest(raw), Mode: 0444})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, assets, nil
}

func installationBytes(expected string, files []InstalledFile) ([]byte, error) {
	raw, err := json.MarshalIndent(InstallationManifest{Version: 1, SourceCommit: expected, Files: files}, "", "  ")
	return append(raw, '\n'), err
}

func syncInstallDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(d.Sync(), d.Close())
}

func install(ctx context.Context, from, into, expected string) (result InstallationResult, err error) {
	var zero InstallationResult
	if err = ctx.Err(); err != nil {
		return zero, err
	}
	verified, err := Verify(from)
	if err != nil {
		return zero, err
	}
	if verified.SourceCommit != expected {
		return zero, fmt.Errorf("distribution source differs from expected commit")
	}
	if !filepath.IsAbs(into) || filepath.Clean(into) != into || strings.ContainsAny(into, "\x00\r\n") {
		return zero, fmt.Errorf("canonical new installation path required")
	}
	for _, pair := range [][2]string{{from, into}, {into, from}} {
		rel, e := filepath.Rel(pair[0], pair[1])
		if e != nil || rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return zero, fmt.Errorf("installation and source overlap")
		}
	}
	parent := filepath.Dir(into)
	before, err := controlledDirectory(parent, true)
	if err != nil {
		return zero, err
	}
	if _, err = os.Lstat(into); !errors.Is(err, os.ErrNotExist) {
		return zero, fmt.Errorf("installation destination already exists or is inaccessible")
	}
	files, assets, err := installFiles(from)
	if err != nil {
		return zero, err
	}
	manifest, err := installationBytes(expected, files)
	if err != nil {
		return zero, err
	}
	if err = ctx.Err(); err != nil {
		return zero, err
	}
	// Exclusive root creation serializes concurrent attempts. No recursive
	// deletion: partial installs remain visible for explicit reconciliation.
	if err = os.Mkdir(into, 0700); err != nil {
		return zero, err
	}
	dirs := map[string]bool{into: true}
	write := func(name string, raw []byte, mode os.FileMode) error {
		if e := ctx.Err(); e != nil {
			return e
		}
		path := filepath.Join(into, filepath.FromSlash(name))
		dir := filepath.Dir(path)
		if e := os.MkdirAll(dir, 0700); e != nil {
			return e
		}
		for p := dir; p != into; p = filepath.Dir(p) {
			dirs[p] = true
		}
		f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write(raw)
		if e == nil {
			e = f.Chmod(mode)
		}
		return errors.Join(e, f.Sync(), f.Close())
	}
	for _, f := range files {
		var raw []byte
		if strings.HasPrefix(f.Path, "templates/") {
			raw = assets[strings.TrimPrefix(f.Path, "templates/")]
		} else {
			raw, err = readFile(filepath.Join(from, strings.TrimPrefix(f.Path, "bin/")), 128<<20, f.Mode == 0555)
			if err != nil {
				return zero, err
			}
		}
		if int64(len(raw)) != f.Size || rawDigest(raw) != f.Digest {
			return zero, fmt.Errorf("distribution changed before copy")
		}
		if err = write(f.Path, raw, os.FileMode(f.Mode)); err != nil {
			return zero, err
		}
	}
	if _, err = Verify(from); err != nil {
		return zero, err
	}
	afterFiles, _, err := installFiles(from)
	if err != nil {
		return zero, err
	}
	afterManifest, err := installationBytes(expected, afterFiles)
	if err != nil || !bytes.Equal(manifest, afterManifest) {
		return zero, fmt.Errorf("source changed during installation")
	}
	if err = write("installation-manifest.json", manifest, 0444); err != nil {
		return zero, err
	}
	ordered := make([]string, 0, len(dirs))
	for d := range dirs {
		ordered = append(ordered, d)
	}
	sort.Slice(ordered, func(i, j int) bool { return len(ordered[i]) > len(ordered[j]) })
	for _, d := range ordered {
		if err = os.Chmod(d, 0755); err != nil {
			return zero, err
		}
		if err = syncInstallDir(d); err != nil {
			return zero, err
		}
	}
	after, err := controlledDirectory(parent, true)
	if err != nil || !os.SameFile(before, after) {
		return zero, fmt.Errorf("installation parent changed")
	}
	if err = syncInstallDir(parent); err != nil {
		return zero, err
	}
	result, err = verifyInstallation(ctx, into, expected)
	if err != nil {
		return zero, err
	}
	result.Status = "INSTALLED_BYTES_VERIFIED"
	return result, nil
}

func verifyInstallation(ctx context.Context, dir, expected string) (InstallationResult, error) {
	var zero InstallationResult
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	root, err := controlledDirectory(dir, false)
	if err != nil {
		return zero, err
	}
	owner := root.Sys().(*syscall.Stat_t).Uid
	verified, err := Verify(filepath.Join(dir, "bin"))
	if err != nil {
		return zero, err
	}
	if verified.SourceCommit != expected {
		return zero, fmt.Errorf("installed source differs from expected commit")
	}
	files, assets, err := installFiles(filepath.Join(dir, "bin"))
	if err != nil {
		return zero, err
	}
	manifest, err := installationBytes(expected, files)
	if err != nil {
		return zero, err
	}
	wanted := map[string]InstalledFile{"installation-manifest.json": {Path: "installation-manifest.json", Size: int64(len(manifest)), Digest: rawDigest(manifest), Mode: 0444}}
	dirs := map[string]bool{".": true}
	for _, f := range files {
		wanted[f.Path] = f
		for p := filepath.ToSlash(filepath.Dir(f.Path)); p != "."; p = filepath.ToSlash(filepath.Dir(p)) {
			dirs[p] = true
		}
	}
	seen := make(map[string]bool)
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		rel, e := filepath.Rel(dir, path)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		st, e := os.Lstat(path)
		if e != nil {
			return e
		}
		s, ok := st.Sys().(*syscall.Stat_t)
		if !ok || s.Uid != owner {
			return fmt.Errorf("installed owner changed")
		}
		if d.IsDir() {
			if !dirs[rel] || st.Mode().Perm() != 0755 || st.Mode()&os.ModeSetgid != 0 {
				return fmt.Errorf("unexpected or unsafe installed directory")
			}
			return nil
		}
		f, ok := wanted[rel]
		if !ok || !st.Mode().IsRegular() || st.Mode().Perm() != os.FileMode(f.Mode) || st.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 || s.Nlink != 1 {
			return fmt.Errorf("unexpected or unsafe installed file")
		}
		raw, e := readFile(path, 128<<20, f.Mode == 0555)
		if e != nil {
			return e
		}
		if int64(len(raw)) != f.Size || rawDigest(raw) != f.Digest {
			return fmt.Errorf("installed bytes differ from canonical source")
		}
		seen[rel] = true
		return nil
	})
	if err != nil {
		return zero, err
	}
	if len(seen) != len(wanted) {
		return zero, fmt.Errorf("installation is incomplete")
	}
	after, e := controlledDirectory(dir, false)
	if e != nil || !os.SameFile(root, after) {
		return zero, fmt.Errorf("installation root changed")
	}
	return InstallationResult{Version: 1, Status: "INSTALLATION_BYTES_VERIFIED", SourceCommit: expected, ManifestDigest: rawDigest(manifest), BinaryCount: verified.BinaryCount, TemplateCount: len(assets)}, nil
}
