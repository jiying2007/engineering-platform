// Package sourcecheckpoint retains exact quiescent source bytes, including
// untracked/ignored files. It never carries .git, HOME, credentials from HOME,
// process memory or authority to restart a model. Archives are private local
// artifacts; they are NOT automatically uploaded to public Git or Actions.
package sourcecheckpoint

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

const MaxSource int64 = 256 << 20
const MaxFile int64 = 64 << 20
const MaxArchive int64 = 320 << 20
const MaxEntries = 10000
const maxManifest int64 = 16 << 20

type Entry struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Size       int64  `json:"size"`
	Executable bool   `json:"executable"`
	Digest     string `json:"digest,omitempty"`
	Target     string `json:"target,omitempty"`
}
type Manifest struct {
	Version        int                         `json:"version"`
	TaskDigest     string                      `json:"task_contract_digest"`
	InputDigest    string                      `json:"run_input_manifest_digest"`
	BaseCommit     string                      `json:"base_commit"`
	BaselineDigest string                      `json:"baseline_source_digest"`
	Transcript     codexexec.ControlTranscript `json:"control_transcript"`
	SnapshotDigest string                      `json:"snapshot_digest"`
	CapturedAt     time.Time                   `json:"captured_at"`
	Entries        []Entry                     `json:"entries"`
}
type Artifact struct {
	Facts codexexec.SourceCheckpoint `json:"facts"`
	Path  string                     `json:"archive_path"`
}

func validPath(p string) bool {
	if p == "" || len(p) > 2048 || !utf8.ValidString(p) || path.IsAbs(p) || path.Clean(p) != p || strings.ContainsAny(p, "\\\x00\r\n") {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." || part == ".git" {
			return false
		}
	}
	return p != "."
}
func validTarget(name, target string) bool {
	if target == "" || path.IsAbs(target) || len(target) > 2048 || strings.ContainsAny(target, "\\\x00\r\n") || !utf8.ValidString(target) {
		return false
	}
	resolved := path.Clean(path.Join(path.Dir(name), target))
	return resolved == "." || validPath(resolved)
}
func (m Manifest) descriptor(hash string, size int64) (codexexec.SourceCheckpoint, error) {
	td, err := m.Transcript.Digest()
	if err != nil || m.Version != 1 || !m.Transcript.Close.ProcessScope.Quiescent() || m.CapturedAt.Before(m.Transcript.Close.ProcessScope.ReapedAt) || m.CapturedAt.IsZero() {
		return codexexec.SourceCheckpoint{}, fmt.Errorf("quiescent sealed transcript required")
	}
	facts := codexexec.SourceCheckpoint{Version: 1, Binding: m.Transcript.Close.Binding, TaskDigest: m.TaskDigest, InputDigest: m.InputDigest, BaseCommit: m.BaseCommit, BaselineDigest: m.BaselineDigest, TranscriptDigest: td, SnapshotDigest: m.SnapshotDigest, ArchiveDigest: hash, ArchiveSize: size}
	if err := facts.Validate(); err != nil {
		return facts, err
	}
	if len(m.Entries) > MaxEntries {
		return facts, fmt.Errorf("too many checkpoint entries")
	}
	seen := map[string]string{}
	last := ""
	var total int64
	for _, e := range m.Entries {
		if !validPath(e.Path) || e.Path <= last {
			return facts, fmt.Errorf("invalid checkpoint path/order")
		}
		last = e.Path
		for parent := path.Dir(e.Path); parent != "."; parent = path.Dir(parent) {
			if seen[parent] != "directory" {
				return facts, fmt.Errorf("missing or non-directory ancestor")
			}
		}
		switch e.Kind {
		case "directory":
			if e.Size != 0 || e.Executable || e.Digest != "" || e.Target != "" {
				return facts, fmt.Errorf("invalid directory")
			}
		case "symlink":
			if !validTarget(e.Path, e.Target) || e.Size != 0 || e.Executable || e.Digest != canonical.BytesDigest([]byte(e.Target)) {
				return facts, fmt.Errorf("unsafe checkpoint symlink")
			}
		case "file":
			if e.Size < 0 || e.Size > MaxFile || !canonical.ValidDigest(e.Digest) || e.Target != "" {
				return facts, fmt.Errorf("invalid checkpoint file")
			}
			total += e.Size
		default:
			return facts, fmt.Errorf("special checkpoint entry")
		}
		if total > MaxSource {
			return facts, fmt.Errorf("checkpoint source byte limit")
		}
		seen[e.Path] = e.Kind
	}
	if err := safeLinks(m.Entries); err != nil {
		return facts, err
	}
	sd, err := canonical.Digest(m.Entries)
	if err != nil || sd != m.SnapshotDigest {
		return facts, fmt.Errorf("checkpoint snapshot identity mismatch")
	}
	return facts, nil
}

// Resolve complete link chains, not just lexical targets: d/link/../outside
// can escape even when each separately cleaned target looks in-tree.
func safeLinks(entries []Entry) error {
	links := map[string]string{}
	for _, e := range entries {
		if e.Kind == "symlink" {
			links[e.Path] = e.Target
		}
	}
	for name, target := range links {
		prefix := []string{}
		if path.Dir(name) != "." {
			prefix = strings.Split(path.Dir(name), "/")
		}
		pending := strings.Split(target, "/")
		traversals := 0
		for len(pending) > 0 {
			part := pending[0]
			pending = pending[1:]
			if part == "" || part == "." {
				continue
			}
			if part == ".." {
				if len(prefix) == 0 {
					return fmt.Errorf("link chain escapes checkpoint")
				}
				prefix = prefix[:len(prefix)-1]
				continue
			}
			if part == ".git" {
				return fmt.Errorf("link targets excluded metadata")
			}
			candidate := strings.Join(append(append([]string(nil), prefix...), part), "/")
			if next, ok := links[candidate]; ok {
				traversals++
				if traversals > 40 {
					return fmt.Errorf("cyclic or excessive link chain")
				}
				pending = append(strings.Split(next, "/"), pending...)
			} else {
				prefix = append(prefix, part)
			}
		}
	}
	return nil
}
func privateDir(p string) error {
	if !filepath.IsAbs(p) || filepath.Clean(p) != p {
		return fmt.Errorf("canonical absolute directory required")
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil || resolved != p {
		return fmt.Errorf("aliased directory")
	}
	st, err := os.Lstat(p)
	if err != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("owner-private directory required")
	}
	return nil
}
func stable(a, b os.FileInfo) bool {
	return a != nil && b != nil && os.SameFile(a, b) && a.Mode() == b.Mode() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

type reader struct {
	ctx context.Context
	r   io.Reader
}

func (r reader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
func fileBytes(ctx context.Context, root *os.Root, name string, w io.Writer) (int64, string, error) {
	before, err := root.Lstat(name)
	if err != nil || !before.Mode().IsRegular() || before.Size() < 0 || before.Size() > MaxFile {
		return 0, "", fmt.Errorf("unsafe checkpoint source file")
	}
	f, err := root.Open(name)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !stable(before, opened) {
		return 0, "", fmt.Errorf("source changed before read")
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w, h), io.LimitReader(reader{ctx, f}, MaxFile+1))
	if err != nil {
		return 0, "", err
	}
	after, err := f.Stat()
	now, e2 := root.Lstat(name)
	if err != nil || e2 != nil || !stable(before, after) || !stable(before, now) || n != before.Size() {
		return 0, "", fmt.Errorf("source changed during read")
	}
	return n, "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
func collect(ctx context.Context, root *os.Root) ([]Entry, error) {
	entries := []Entry{}
	var total int64
	var walk func(string, int) error
	walk = func(dir string, depth int) error {
		if depth > 64 {
			return fmt.Errorf("checkpoint depth limit")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		f, err := root.Open(dir)
		if err != nil {
			return err
		}
		items, err := f.ReadDir(MaxEntries + 1)
		_ = f.Close()
		if err != nil && err != io.EOF {
			return err
		}
		if len(items) > MaxEntries {
			return fmt.Errorf("checkpoint entry limit")
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Name() < items[j].Name() })
		for _, item := range items {
			if dir == "." && item.Name() == ".git" {
				continue
			}
			name := path.Join(dir, item.Name())
			if !validPath(name) || len(entries) >= MaxEntries {
				return fmt.Errorf("invalid or excessive checkpoint paths")
			}
			st, err := root.Lstat(name)
			if err != nil {
				return err
			}
			e := Entry{Path: name}
			switch {
			case st.IsDir():
				e.Kind = "directory"
				entries = append(entries, e)
				if err := walk(name, depth+1); err != nil {
					return err
				}
				continue
			case st.Mode().IsRegular():
				e.Kind = "file"
				e.Executable = st.Mode().Perm()&0111 != 0
				e.Size, e.Digest, err = fileBytes(ctx, root, name, io.Discard)
				if err != nil {
					return err
				}
				total += e.Size
				if total > MaxSource {
					return fmt.Errorf("checkpoint byte limit")
				}
			case st.Mode()&os.ModeSymlink != 0:
				e.Kind = "symlink"
				e.Target, err = root.Readlink(name)
				if err != nil || !validTarget(name, e.Target) {
					return fmt.Errorf("external checkpoint symlink")
				}
				e.Digest = canonical.BytesDigest([]byte(e.Target))
			default:
				return fmt.Errorf("special file in checkpoint source")
			}
			entries = append(entries, e)
		}
		return nil
	}
	if err := walk(".", 0); err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries, nil
}

// Capture accepts only a validated immutable permit and sealed same-run controls.
// The preparation owner validates the source slot before invoking it. Output is
// installed without replacement and read back by its exact externally held hash.
func Capture(ctx context.Context, source, output string, p codexexec.Permit, t codexexec.ControlTranscript) (Artifact, error) {
	var empty Artifact
	if err := p.Check(p.Preparation.Admission.Worker, codexexec.Start{RunID: p.Token.RunID, WorkerProfile: p.Token.WorkerProfile, Profile: p.Profile}); err != nil {
		return empty, err
	}
	if t.Close.Binding.Token != p.Token || t.Close.Binding.ExecutionEpoch != p.Assignment.Intent.ExecutionEpoch {
		return empty, fmt.Errorf("checkpoint permit/control identity mismatch")
	}
	if _, err := t.Digest(); err != nil || !t.Close.ProcessScope.Quiescent() {
		return empty, fmt.Errorf("unconfirmed runtime termination")
	}
	if err := privateDir(output); err != nil {
		return empty, err
	}
	if !filepath.IsAbs(source) || filepath.Clean(source) != source {
		return empty, fmt.Errorf("invalid source path")
	}
	resolved, err := filepath.EvalSymlinks(source)
	if err != nil || resolved != source {
		return empty, fmt.Errorf("aliased source")
	}
	if output == source || strings.HasPrefix(output, source+string(os.PathSeparator)) || strings.HasPrefix(source, output+string(os.PathSeparator)) {
		return empty, fmt.Errorf("checkpoint storage overlaps source")
	}
	root, err := os.OpenRoot(source)
	if err != nil {
		return empty, err
	}
	defer root.Close()
	entries, err := collect(ctx, root)
	if err != nil {
		return empty, err
	}
	sd, err := canonical.Digest(entries)
	if err != nil {
		return empty, err
	}
	m := Manifest{Version: 1, TaskDigest: p.Assignment.Intent.TaskDigest, InputDigest: p.Assignment.Intent.InputDigest, BaseCommit: p.Preparation.Facts.BaseCommit, BaselineDigest: p.Preparation.Facts.SourceDigest, Transcript: t, SnapshotDigest: sd, CapturedAt: time.Now().UTC(), Entries: entries}
	if _, err := m.descriptor(canonical.BytesDigest([]byte("validation-only")), 1); err != nil {
		return empty, err
	}
	metadata, err := json.Marshal(m)
	if err != nil || int64(len(metadata)) > maxManifest {
		return empty, fmt.Errorf("checkpoint manifest too large")
	}
	dir, err := os.OpenRoot(output)
	if err != nil {
		return empty, err
	}
	defer dir.Close()
	name := p.Token.ID + ".source-checkpoint.tar"
	f, err := dir.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return empty, err
	}
	// A failed capture remains an unaccepted private artifact. Never delete a path
	// that another process might have substituted and never overwrite on retry.
	defer f.Close()
	h := sha256.New()
	tw := tar.NewWriter(io.MultiWriter(f, h))
	if err := tw.WriteHeader(&tar.Header{Name: "checkpoint.json", Mode: 0600, Size: int64(len(metadata)), Typeflag: tar.TypeReg}); err != nil {
		return empty, err
	}
	if _, err := tw.Write(metadata); err != nil {
		return empty, err
	}
	for _, e := range entries {
		header := &tar.Header{Name: "source/" + e.Path, Mode: 0600, Size: e.Size, Typeflag: tar.TypeReg}
		switch e.Kind {
		case "directory":
			header.Typeflag = tar.TypeDir
			header.Mode = 0700
		case "symlink":
			header.Typeflag = tar.TypeSymlink
			header.Linkname = e.Target
			header.Mode = 0777
		case "file":
			if e.Executable {
				header.Mode = 0700
			}
		}
		if err := tw.WriteHeader(header); err != nil {
			return empty, err
		}
		if e.Kind == "file" {
			n, d, e2 := fileBytes(ctx, root, e.Path, tw)
			if e2 != nil {
				return empty, e2
			}
			if n != e.Size || d != e.Digest {
				return empty, fmt.Errorf("source changed while capturing")
			}
		}
	}
	if err := tw.Close(); err != nil {
		return empty, err
	}
	if err := f.Sync(); err != nil {
		return empty, err
	}
	st, err := f.Stat()
	if err != nil || st.Size() > MaxArchive {
		return empty, fmt.Errorf("checkpoint archive exceeds limit")
	}
	if err := f.Close(); err != nil {
		return empty, err
	}
	after, err := collect(ctx, root)
	if err != nil {
		return empty, err
	}
	ad, err := canonical.Digest(after)
	if err != nil || ad != sd {
		return empty, fmt.Errorf("source snapshot changed during capture")
	}
	hash := "sha256:" + hex.EncodeToString(h.Sum(nil))
	filename := filepath.Join(output, name)
	verified, err := Verify(ctx, filename, hash, p.Token.RunID)
	if err != nil {
		return empty, err
	}
	df, err := dir.Open(".")
	if err != nil {
		return empty, err
	}
	err = df.Sync()
	_ = df.Close()
	if err != nil {
		return empty, err
	}
	return Artifact{Facts: verified, Path: filename}, nil
}

func openArchive(ctx context.Context, filename, expected string) (*os.File, error) {
	if !canonical.ValidDigest(expected) || !filepath.IsAbs(filename) || filepath.Clean(filename) != filename {
		return nil, fmt.Errorf("explicit archive digest and canonical path required")
	}
	resolved, err := filepath.EvalSymlinks(filename)
	if err != nil || resolved != filename {
		return nil, fmt.Errorf("aliased archive")
	}
	before, err := os.Lstat(filename)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0 || before.Size() <= 0 || before.Size() > MaxArchive {
		return nil, fmt.Errorf("private bounded archive required")
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	fail := func(e error) (*os.File, error) { _ = f.Close(); return nil, e }
	opened, err := f.Stat()
	if err != nil || !stable(before, opened) {
		return fail(fmt.Errorf("archive changed before open"))
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(reader{ctx, f}, MaxArchive+1))
	if err != nil {
		return fail(err)
	}
	after, err := f.Stat()
	if err != nil || !stable(before, after) || n != before.Size() || "sha256:"+hex.EncodeToString(h.Sum(nil)) != expected {
		return fail(fmt.Errorf("archive anchor mismatch"))
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return fail(err)
	}
	return f, nil
}

// readArchive validates even the unconsumed trailer and can optionally restore
// through a fresh os.Root. No caller-provided tar path is ever extracted directly.
func readArchive(ctx context.Context, f *os.File, expected, run string, dest *os.Root) (codexexec.SourceCheckpoint, error) {
	var empty codexexec.SourceCheckpoint
	before, err := f.Stat()
	if err != nil {
		return empty, err
	}
	h := sha256.New()
	stream := io.TeeReader(reader{ctx, f}, h)
	tr := tar.NewReader(stream)
	first, err := tr.Next()
	if err != nil || first.Name != "checkpoint.json" || first.Typeflag != tar.TypeReg || first.Size <= 0 || first.Size > maxManifest {
		return empty, fmt.Errorf("checkpoint manifest missing")
	}
	raw, err := io.ReadAll(io.LimitReader(tr, maxManifest+1))
	if err != nil {
		return empty, err
	}
	var m Manifest
	if err := strictjson.Decode(raw, &m); err != nil {
		return empty, err
	}
	canonicalJSON, _ := json.Marshal(m)
	if !bytes.Equal(raw, canonicalJSON) {
		return empty, fmt.Errorf("non-canonical checkpoint manifest")
	}
	facts, err := m.descriptor(expected, before.Size())
	if err != nil {
		return empty, err
	}
	if run == "" || facts.Binding.Token.RunID != run {
		return empty, fmt.Errorf("checkpoint run mismatch")
	}
	for _, e := range m.Entries {
		header, err := tr.Next()
		if err != nil || header.Name != "source/"+e.Path || header.Size != e.Size || header.Linkname != e.Target {
			return empty, fmt.Errorf("archive member mismatch")
		}
		typ := byte(tar.TypeReg)
		mode := int64(0600)
		switch e.Kind {
		case "directory":
			typ = tar.TypeDir
			mode = 0700
		case "symlink":
			typ = tar.TypeSymlink
			mode = 0777
		case "file":
			if e.Executable {
				mode = 0700
			}
		}
		if header.Typeflag != typ || header.Mode != mode || header.Uid != 0 || header.Gid != 0 {
			return empty, fmt.Errorf("archive member type/mode mismatch")
		}
		if e.Kind == "file" {
			var output io.Writer = io.Discard
			var target *os.File
			if dest != nil {
				target, err = dest.OpenFile(e.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, os.FileMode(mode))
				if err != nil {
					return empty, err
				}
				output = target
			}
			fileHash := sha256.New()
			n, copyErr := io.Copy(io.MultiWriter(fileHash, output), tr)
			if target != nil {
				syncErr := target.Sync()
				closeErr := target.Close()
				copyErr = errors.Join(copyErr, syncErr, closeErr)
			}
			if copyErr != nil {
				return empty, copyErr
			}
			if n != e.Size || "sha256:"+hex.EncodeToString(fileHash.Sum(nil)) != e.Digest {
				return empty, fmt.Errorf("checkpoint member digest mismatch")
			}
		} else if dest != nil && e.Kind == "directory" {
			if err := dest.Mkdir(e.Path, 0700); err != nil {
				return empty, err
			}
		}
	}
	if _, err = tr.Next(); err != io.EOF {
		return empty, fmt.Errorf("extra checkpoint member")
	}
	// No extra tar after end markers, concatenation, or arbitrary trailing bytes.
	trailer := make([]byte, 4096)
	for {
		n, e := stream.Read(trailer)
		for _, b := range trailer[:n] {
			if b != 0 {
				return empty, fmt.Errorf("nonzero checkpoint trailer")
			}
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return empty, e
		}
	}
	after, err := f.Stat()
	if err != nil || !stable(before, after) || "sha256:"+hex.EncodeToString(h.Sum(nil)) != expected {
		return empty, fmt.Errorf("archive changed during readback")
	}
	if dest != nil {
		// Links are created last: even internal link chains can never redirect writes.
		for _, e := range m.Entries {
			if e.Kind == "symlink" {
				if err := dest.Symlink(e.Target, e.Path); err != nil {
					return empty, err
				}
			}
		}
		for i := len(m.Entries) - 1; i >= 0; i-- {
			if m.Entries[i].Kind == "directory" {
				if err := syncDir(dest, m.Entries[i].Path); err != nil {
					return empty, err
				}
			}
		}
		if err := syncDir(dest, "."); err != nil {
			return empty, err
		}
		entries, err := collect(ctx, dest)
		if err != nil {
			return empty, err
		}
		sd, err := canonical.Digest(entries)
		if err != nil || sd != m.SnapshotDigest {
			return empty, fmt.Errorf("restored source digest mismatch")
		}
	}
	return facts, nil
}
func Verify(ctx context.Context, filename, expected, run string) (codexexec.SourceCheckpoint, error) {
	f, err := openArchive(ctx, filename, expected)
	if err != nil {
		return codexexec.SourceCheckpoint{}, err
	}
	defer f.Close()
	return readArchive(ctx, f, expected, run, nil)
}

// Restore creates a NEW owner-private recovery directory. It never overwrites
// or grants Worker/Human ownership of a live slot. Failed restore leaves the
// INCOMPLETE marker; no model execution or source publishing happens here.
func Restore(ctx context.Context, filename, expected, run, destination string) (codexexec.SourceCheckpoint, error) {
	var empty codexexec.SourceCheckpoint
	if !filepath.IsAbs(destination) || filepath.Clean(destination) != destination {
		return empty, fmt.Errorf("canonical recovery destination required")
	}
	if err := privateDir(filepath.Dir(destination)); err != nil {
		return empty, err
	}
	f, err := openArchive(ctx, filename, expected)
	if err != nil {
		return empty, err
	}
	defer f.Close()
	// Validate all bytes before creating any recovery files; read again while writing.
	if _, err := readArchive(ctx, f, expected, run, nil); err != nil {
		return empty, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return empty, err
	}
	parent, err := os.OpenRoot(filepath.Dir(destination))
	if err != nil {
		return empty, err
	}
	defer parent.Close()
	name := filepath.Base(destination)
	if err := parent.Mkdir(name, 0700); err != nil {
		return empty, err
	}
	out, err := parent.OpenRoot(name)
	if err != nil {
		return empty, err
	}
	defer out.Close()
	if err := out.WriteFile("INCOMPLETE", []byte("not accepted\n"), 0600); err != nil {
		return empty, err
	}
	if err := out.Mkdir("source", 0700); err != nil {
		return empty, err
	}
	src, err := out.OpenRoot("source")
	if err != nil {
		return empty, err
	}
	defer src.Close()
	facts, err := readArchive(ctx, f, expected, run, src)
	if err != nil {
		return empty, err
	}
	report, _ := json.Marshal(struct {
		Status              string                     `json:"status"`
		Facts               codexexec.SourceCheckpoint `json:"facts"`
		ExecutionAuthorized bool                       `json:"execution_authorized"`
	}{Status: "SOURCE_BYTES_RESTORED", Facts: facts})
	rf, err := out.OpenFile("RESTORED.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return empty, err
	}
	_, err = rf.Write(report)
	err = errors.Join(err, rf.Sync(), rf.Close())
	if err != nil {
		return empty, err
	}
	if err := out.Remove("INCOMPLETE"); err != nil {
		return empty, err
	}
	df, err := out.Open(".")
	if err != nil {
		return empty, err
	}
	err = df.Sync()
	_ = df.Close()
	if err != nil {
		return empty, err
	}
	if err := syncDir(parent, "."); err != nil {
		return empty, err
	}
	return facts, nil
}

func syncDir(root *os.Root, name string) error {
	f, err := root.Open(name)
	if err != nil {
		return err
	}
	err = f.Sync()
	return errors.Join(err, f.Close())
}
