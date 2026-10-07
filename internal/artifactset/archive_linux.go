//go:build linux

package artifactset

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

func header(name string, size int64) *tar.Header {
	return &tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0600, Size: size, ModTime: time.Unix(0, 0), Format: tar.FormatUSTAR}
}
func checkHeader(h *tar.Header, name string, size int64) error {
	if h == nil || h.Name != name || h.Size != size || h.Typeflag != tar.TypeReg || h.Mode != 0600 || h.Uid != 0 || h.Gid != 0 || h.Uname != "" || h.Gname != "" || h.Linkname != "" || h.ModTime.Unix() != 0 || h.Format != tar.FormatUSTAR || len(h.PAXRecords) != 0 || len(h.Xattrs) != 0 {
		return fmt.Errorf("noncanonical or unexpected archive member")
	}
	return nil
}

// Pack requires the raw plan digest from an external authority. Paths in the
// plan are explicit private inputs, never discovered by recursively scanning.
func Pack(ctx context.Context, planPath, expectedPlan, out string) (Report, error) {
	var zero Report
	if !canonical.ValidDigest(expectedPlan) {
		return zero, fmt.Errorf("externally pinned plan digest required")
	}
	planRaw, err := readInput(ctx, planPath, MaxMetadata)
	if err != nil {
		return zero, err
	}
	if canonical.BytesDigest(planRaw) != expectedPlan {
		return zero, fmt.Errorf("plan digest mismatch")
	}
	var p Plan
	if err := strictjson.Decode(planRaw, &p); err != nil {
		return zero, err
	}
	m := Manifest{Version: p.Version, Coverage: Coverage, Subject: p.Subject, PlanDigest: expectedPlan}
	seen := map[string]bool{planPath: true}
	for _, in := range p.Members {
		if seen[in.Path] || in.Path == out {
			return zero, fmt.Errorf("duplicate or overlapping input path")
		}
		seen[in.Path] = true
		m.Members = append(m.Members, in.Entry)
	}
	if err := m.Validate(); err != nil {
		return zero, err
	}
	raw, err := json.Marshal(m)
	if err != nil || int64(len(raw)) > MaxMetadata {
		return zero, fmt.Errorf("manifest encoding exceeds bound")
	}
	if !filepath.IsAbs(out) || filepath.Clean(out) != out || seen[out] {
		return zero, fmt.Errorf("canonical new archive path required")
	}
	dir, name := filepath.Dir(out), filepath.Base(out)
	root, parent, err := privateRoot(dir)
	if err != nil {
		return zero, err
	}
	defer root.Close()
	if _, err := root.Lstat(name); !errors.Is(err, os.ErrNotExist) {
		return zero, fmt.Errorf("archive destination already exists or cannot be inspected")
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return zero, err
	}
	tempName := ".artifact-set-" + hex.EncodeToString(nonce)
	temp, err := root.OpenFile(tempName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return zero, err
	}
	defer func() { _ = temp.Close(); _ = root.Remove(tempName) }()
	if err := rootUnchanged(dir, parent); err != nil {
		return zero, err
	}
	tw := tar.NewWriter(temp)
	if err := tw.WriteHeader(header("manifest.json", int64(len(raw)))); err != nil {
		return zero, err
	}
	if _, err := tw.Write(raw); err != nil {
		return zero, err
	}
	for _, in := range p.Members {
		f, err := openInput(in.Path, MaxFile)
		if err != nil {
			return zero, err
		}
		if f.before.Size() != in.Size {
			f.Close()
			return zero, fmt.Errorf("declared artifact size mismatch")
		}
		err = tw.WriteHeader(header("files/"+in.ID, in.Size))
		if err == nil {
			err = copyExact(ctx, f.file, tw, in.Size, in.Digest)
		}
		if err == nil {
			err = f.check()
		}
		f.Close()
		if err != nil {
			return zero, err
		}
	}
	if err := tw.Close(); err != nil {
		return zero, err
	}
	if err := temp.Sync(); err != nil {
		return zero, err
	}
	if err := temp.Close(); err != nil {
		return zero, err
	}
	// Re-read the declaration and every source after capture. A changed source
	// is rejected; this never turns a live database copy into a valid backup.
	again, err := readInput(ctx, planPath, MaxMetadata)
	if err != nil || !bytes.Equal(again, planRaw) {
		return zero, fmt.Errorf("plan changed during capture")
	}
	for _, in := range p.Members {
		f, err := openInput(in.Path, MaxFile)
		if err != nil {
			return zero, err
		}
		err = copyExact(ctx, f.file, io.Discard, in.Size, in.Digest)
		if err == nil {
			err = f.check()
		}
		f.Close()
		if err != nil {
			return zero, err
		}
	}
	sum, size, err := fileDigest(ctx, filepath.Join(dir, tempName))
	if err != nil {
		return zero, err
	}
	r, err := Verify(ctx, filepath.Join(dir, tempName), sum, m.Subject.RunID)
	if err != nil {
		return zero, fmt.Errorf("archive independent readback failed: %w", err)
	}
	if size != tarSize(m, raw) {
		return zero, fmt.Errorf("archive canonical size mismatch")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if err := rootUnchanged(dir, parent); err != nil {
		return zero, err
	}
	// Hard-link publication is atomic and fails if any destination exists.
	// The temporary link is removed before reporting a durable final archive.
	if err := root.Link(tempName, name); err != nil {
		return zero, err
	}
	if err := root.Remove(tempName); err != nil {
		return zero, err
	}
	if err := syncRoot(root); err != nil {
		return zero, err
	}
	if err := rootUnchanged(dir, parent); err != nil {
		return zero, err
	}
	r, err = Verify(ctx, out, sum, m.Subject.RunID)
	if err != nil {
		return zero, err
	}
	r.Status = "ARTIFACT_SET_PACKED_BYTES_VERIFIED"
	return r, nil
}

func fileDigest(ctx context.Context, p string) (string, int64, error) {
	f, err := openInput(p, MaxArchive)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	d, err := hashReader(ctx, f.file)
	if err != nil {
		return "", 0, err
	}
	return d, f.before.Size(), f.check()
}

// walk verifies the entire archive before success. Extraction writes into a
// freshly created private root only; executable permissions are never restored.
func walk(ctx context.Context, p, digest, run string, dest *os.Root) (Report, error) {
	var zero Report
	if !canonical.ValidDigest(digest) || run == "" {
		return zero, fmt.Errorf("external archive digest and exact run required")
	}
	f, err := openInput(p, MaxArchive)
	if err != nil {
		return zero, err
	}
	defer f.Close()
	if err := copyExact(ctx, f.file, io.Discard, f.before.Size(), digest); err != nil {
		return zero, err
	}
	if _, err := f.file.Seek(0, io.SeekStart); err != nil {
		return zero, err
	}
	tr := tar.NewReader(contextReader{ctx, f.file})
	h, err := tr.Next()
	if err != nil || h.Size < 1 || h.Size > MaxMetadata {
		return zero, fmt.Errorf("bounded manifest first required")
	}
	if err := checkHeader(h, "manifest.json", h.Size); err != nil {
		return zero, err
	}
	raw, err := io.ReadAll(tr)
	if err != nil {
		return zero, err
	}
	m, err := decodeManifest(raw)
	if err != nil {
		return zero, err
	}
	if m.Subject.RunID != run || f.before.Size() != tarSize(m, raw) {
		return zero, fmt.Errorf("archive run or canonical size mismatch")
	}
	canonicalHash := sha256.New()
	canonicalTar := tar.NewWriter(canonicalHash)
	if err := canonicalTar.WriteHeader(header("manifest.json", int64(len(raw)))); err != nil {
		return zero, err
	}
	if _, err := canonicalTar.Write(raw); err != nil {
		return zero, err
	}
	for _, e := range m.Members {
		h, err = tr.Next()
		if err != nil {
			return zero, err
		}
		if err := checkHeader(h, "files/"+e.ID, e.Size); err != nil {
			return zero, err
		}
		var out *os.File
		var target io.Writer = io.Discard
		if dest != nil {
			out, err = dest.OpenFile("files/"+e.ID, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				return zero, err
			}
			target = out
		}
		if err := canonicalTar.WriteHeader(header("files/"+e.ID, e.Size)); err != nil {
			if out != nil {
				_ = out.Close()
			}
			return zero, err
		}
		err = copyExact(ctx, tr, io.MultiWriter(target, canonicalTar), e.Size, e.Digest)
		if out != nil {
			err = errors.Join(err, out.Sync(), out.Close())
		}
		if err != nil {
			return zero, err
		}
	}
	if _, err = tr.Next(); err != io.EOF {
		return zero, fmt.Errorf("extra or invalid archive member")
	}
	if err = canonicalTar.Close(); err != nil {
		return zero, err
	}
	if "sha256:"+hex.EncodeToString(canonicalHash.Sum(nil)) != digest {
		return zero, fmt.Errorf("noncanonical archive encoding")
	}
	if _, err = f.file.Seek(0, io.SeekStart); err != nil {
		return zero, err
	}
	if err = copyExact(ctx, f.file, io.Discard, f.before.Size(), digest); err != nil {
		return zero, err
	}
	if err = f.check(); err != nil {
		return zero, err
	}
	if dest != nil {
		files, err := dest.OpenRoot("files")
		if err != nil {
			return zero, err
		}
		err = syncRoot(files)
		_ = files.Close()
		if err != nil {
			return zero, err
		}
		mf, err := dest.OpenFile("manifest.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return zero, err
		}
		_, writeErr := mf.Write(raw)
		err = errors.Join(writeErr, mf.Sync(), mf.Close())
		if err != nil {
			return zero, err
		}
		if err = syncRoot(dest); err != nil {
			return zero, err
		}
	}
	return report(m, raw, digest, f.before.Size(), "ARTIFACT_SET_BYTES_VERIFIED"), nil
}
func Verify(ctx context.Context, p, digest, run string) (Report, error) {
	return walk(ctx, p, digest, run, nil)
}

func filesystemDevice(info os.FileInfo) (uint64, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return stat.Dev, ok
}

// Mirror copies one already-verified private archive to a NEW destination on a
// different filesystem. It is a durability primitive, not a claim of geographic
// separation, backup scheduling, encryption, retention policy or second-site
// qualification. The exact externally anchored archive digest/Run remain the
// authority input. No overwrite, repair or execution occurs.
func Mirror(ctx context.Context, p, digest, run, out string) (Report, error) {
	var zero Report
	before, err := Verify(ctx, p, digest, run)
	if err != nil {
		return zero, err
	}
	if !filepath.IsAbs(out) || filepath.Clean(out) != out || out == p {
		return zero, fmt.Errorf("canonical new mirror path required")
	}
	sourceInfo, err := os.Lstat(p)
	if err != nil || !sourceInfo.Mode().IsRegular() {
		return zero, fmt.Errorf("verified source archive disappeared")
	}
	sourceDevice, ok := filesystemDevice(sourceInfo)
	if !ok {
		return zero, fmt.Errorf("source filesystem identity unavailable")
	}
	dir, name := filepath.Dir(out), filepath.Base(out)
	root, parent, err := privateRoot(dir)
	if err != nil {
		return zero, err
	}
	defer root.Close()
	destinationDevice, ok := filesystemDevice(parent)
	if !ok || sourceDevice == destinationDevice {
		return zero, fmt.Errorf("mirror requires a distinct destination filesystem")
	}
	if _, err := root.Lstat(name); !errors.Is(err, os.ErrNotExist) {
		return zero, fmt.Errorf("mirror destination already exists or cannot be inspected")
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return zero, err
	}
	tempName := ".artifact-mirror-" + hex.EncodeToString(nonce)
	temp, err := root.OpenFile(tempName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return zero, err
	}
	defer func() { _ = temp.Close(); _ = root.Remove(tempName) }()
	if err := rootUnchanged(dir, parent); err != nil {
		return zero, err
	}
	source, err := openInput(p, MaxArchive)
	if err != nil {
		return zero, err
	}
	if source.before.Size() != before.ArchiveSize {
		source.Close()
		return zero, fmt.Errorf("source archive size changed")
	}
	err = copyExact(ctx, source.file, temp, before.ArchiveSize, digest)
	if err == nil {
		err = source.check()
	}
	source.Close()
	if err != nil {
		return zero, err
	}
	if err := temp.Sync(); err != nil {
		return zero, err
	}
	if err := temp.Close(); err != nil {
		return zero, err
	}
	tempPath := filepath.Join(dir, tempName)
	mirrored, err := Verify(ctx, tempPath, digest, run)
	if err != nil || mirrored != before {
		return zero, fmt.Errorf("mirror independent readback failed")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if err := rootUnchanged(dir, parent); err != nil {
		return zero, err
	}
	if err := root.Link(tempName, name); err != nil {
		return zero, err
	}
	if err := root.Remove(tempName); err != nil {
		return zero, err
	}
	if err := syncRoot(root); err != nil {
		return zero, err
	}
	if err := rootUnchanged(dir, parent); err != nil {
		return zero, err
	}
	final, err := Verify(ctx, out, digest, run)
	if err != nil || final != before {
		return zero, fmt.Errorf("published mirror readback failed")
	}
	if err := rootUnchanged(dir, parent); err != nil {
		return zero, err
	}
	final.Status = "ARTIFACT_SET_INDEPENDENT_FILESYSTEM_MIRROR_BYTES_VERIFIED"
	return final, nil
}

// Restore never overwrites anything. On failure a private partial destination
// can remain for explicit operator disposition; it is NOT a verified restore.
func Restore(ctx context.Context, p, digest, run, destination string) (Report, error) {
	var zero Report
	before, err := Verify(ctx, p, digest, run)
	if err != nil {
		return zero, err
	}
	if !filepath.IsAbs(destination) || filepath.Clean(destination) != destination {
		return zero, fmt.Errorf("canonical new destination required")
	}
	dir, name := filepath.Dir(destination), filepath.Base(destination)
	parent, identity, err := privateRoot(dir)
	if err != nil {
		return zero, err
	}
	defer parent.Close()
	if err := parent.Mkdir(name, 0700); err != nil {
		return zero, err
	}
	target, err := parent.OpenRoot(name)
	if err != nil {
		return zero, err
	}
	defer target.Close()
	targetInfo, err := target.Stat(".")
	if err != nil {
		return zero, err
	}
	if err = target.Mkdir("files", 0700); err != nil {
		return zero, err
	}
	r, err := walk(ctx, p, digest, run, target)
	if err != nil {
		return zero, err
	}
	if r != before {
		return zero, fmt.Errorf("archive identity changed during restore")
	}
	// Independently verify each restored byte and reject extra paths.
	raw, err := readInput(ctx, filepath.Join(destination, "manifest.json"), MaxMetadata)
	if err != nil {
		return zero, err
	}
	m, err := decodeManifest(raw)
	if err != nil {
		return zero, err
	}
	if canonical.BytesDigest(raw) != r.ManifestDigest {
		return zero, fmt.Errorf("restored manifest mismatch")
	}
	top, err := os.ReadDir(destination)
	if err != nil || len(top) != 2 || top[0].Name() != "files" || top[1].Name() != "manifest.json" {
		return zero, fmt.Errorf("unexpected restored members")
	}
	entries, err := os.ReadDir(filepath.Join(destination, "files"))
	if err != nil || len(entries) != len(m.Members) {
		return zero, fmt.Errorf("incomplete restored set")
	}
	for i, e := range m.Members {
		if entries[i].Name() != e.ID {
			return zero, fmt.Errorf("unexpected restored artifact")
		}
		f, err := openInput(filepath.Join(destination, "files", e.ID), MaxFile)
		if err != nil {
			return zero, err
		}
		err = copyExact(ctx, f.file, io.Discard, e.Size, e.Digest)
		if err == nil && f.before.Mode().Perm() != 0600 {
			err = fmt.Errorf("unexpected restored permission")
		}
		if err == nil {
			err = f.check()
		}
		f.Close()
		if err != nil {
			return zero, err
		}
	}
	if err = syncRoot(parent); err != nil {
		return zero, err
	}
	if err = rootUnchanged(dir, identity); err != nil {
		return zero, err
	}
	if err = rootUnchanged(destination, targetInfo); err != nil {
		return zero, err
	}
	r.Status = "ARTIFACT_SET_RESTORED_BYTES_VERIFIED"
	return r, nil
}
