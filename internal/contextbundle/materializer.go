// Package contextbundle materializes approved, digest-verified data. It does not
// install skills, execute content, resolve arbitrary URLs or grant Runtime rights.
package contextbundle

import (
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
	"strings"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
)

var (
	ErrDenied     = errors.New("context access denied")
	ErrDigest     = errors.New("context digest mismatch")
	ErrLimit      = errors.New("context size limit exceeded")
	ErrExists     = errors.New("immutable context bundle already exists")
	ErrBusy       = errors.New("context bundle publication is locked")
	ErrUnsafePath = errors.New("unsafe context bundle path")
)

type Resolver interface {
	// Open must honor ctx cancellation, including blocked remote reads. Any URL
	// or file access policy belongs here; no production resolver is implicit.
	Open(context.Context, core.ContextRef) (io.ReadCloser, error)
}

type Subject struct {
	RunID              string
	TaskContractDigest string
	RunInputDigest     string
}

type Authorizer interface {
	// Check authenticated-principal ACL and current approval/revocation state
	// for this exact source/version/digest and frozen input. Trust is not enough.
	Authorize(context.Context, Subject, core.ContextRef) error
}

type Limits struct {
	EntryBytes int64
	TotalBytes int64
}

type Entry struct {
	Ref  core.ContextRef `json:"ref"`
	File string          `json:"file"`
	Size int64           `json:"size"`
}

type Manifest struct {
	SchemaVersion  int     `json:"schema_version"`
	RunInputDigest string  `json:"run_input_digest"`
	Entries        []Entry `json:"entries"`
}

type Bundle struct {
	Path     string // Local locator only; never inserted into RunInputManifest.
	Digest   string
	Manifest Manifest
}

type Materializer struct {
	root       *os.Root
	path       string
	resolver   Resolver
	authorizer Authorizer
	limits     Limits
}

// New requires an existing dedicated directory owned/managed by the host.
// os.Root anchors all child operations against traversal and symlink escapes.
// The parent tree must remain host-controlled for the Materializer's lifetime.
func New(path string, resolver Resolver, authorizer Authorizer, limits Limits) (*Materializer, error) {
	if path == "" || resolver == nil || authorizer == nil {
		return nil, fmt.Errorf("root, resolver and authorizer are required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	if resolved != absolute {
		return nil, ErrUnsafePath
	}
	root, err := os.OpenRoot(resolved)
	if err != nil {
		return nil, err
	}
	info, err := root.Stat(".")
	if err != nil || info.Mode().Perm()&0o022 != 0 {
		_ = root.Close()
		return nil, ErrUnsafePath
	}
	if limits.EntryBytes == 0 {
		limits.EntryBytes = 4 << 20
	}
	if limits.TotalBytes == 0 {
		limits.TotalBytes = 32 << 20
	}
	if limits.EntryBytes < 1 || limits.TotalBytes < 1 || limits.EntryBytes > 1<<30 || limits.TotalBytes > 1<<30 {
		_ = root.Close()
		return nil, fmt.Errorf("context limits must be between 1 byte and 1 GiB")
	}
	return &Materializer{root: root, path: resolved, resolver: resolver, authorizer: authorizer, limits: limits}, nil
}

func (m *Materializer) Close() error { return m.root.Close() }

func (m *Materializer) prepare(ctx context.Context, input core.RunInputManifest) (core.RunInputManifest, string, error) {
	if err := ctx.Err(); err != nil {
		return input, "", err
	}
	// Do not expose the caller's slice to later mutation while resolving bytes.
	input.ContextRefs = append([]core.ContextRef(nil), input.ContextRefs...)
	if input.RunID == "" || !canonical.ValidDigest(input.TaskContractDigest) || input.RuntimeProfile == "" || input.ToolProfile == "" || input.WorkerProfile == "" || input.PolicyProfile == "" {
		return input, "", fmt.Errorf("complete frozen Run input is required")
	}
	digest, err := input.Digest()
	if err != nil {
		return input, "", err
	}
	subject := Subject{RunID: input.RunID, TaskContractDigest: input.TaskContractDigest, RunInputDigest: digest}
	// Authorize the entire input before fetching any source bytes.
	for _, ref := range input.ContextRefs {
		if ref.Trust != core.ContextApproved {
			return input, "", ErrDenied
		}
		if err := ctx.Err(); err != nil {
			return input, "", err
		}
		if err := m.authorizer.Authorize(ctx, subject, ref); err != nil {
			return input, "", fmt.Errorf("%w: source %s", ErrDenied, ref.Source)
		}
	}
	return input, digest, nil
}

func (m *Materializer) Materialize(ctx context.Context, input core.RunInputManifest, worktree string) (bundle Bundle, err error) {
	if worktree == "" {
		return Bundle{}, ErrUnsafePath
	}
	workspace, err := filepath.Abs(worktree)
	if err != nil {
		return Bundle{}, err
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return Bundle{}, err
	}
	info, err := os.Stat(workspace)
	if err != nil || !info.IsDir() {
		return Bundle{}, ErrUnsafePath
	}
	if within(workspace, m.path) || within(m.path, workspace) {
		return Bundle{}, ErrUnsafePath
	}
	input, digest, err := m.prepare(ctx, input)
	if err != nil {
		return Bundle{}, err
	}
	name := strings.TrimPrefix(digest, "sha256:")
	lockName := "." + name + ".lock"
	lock, err := m.root.OpenFile(lockName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return Bundle{}, ErrBusy
	}
	if err != nil {
		return Bundle{}, err
	}
	_ = lock.Close()
	defer func() { err = errors.Join(err, m.root.Remove(lockName)) }()
	if _, statErr := m.root.Lstat(name); !errors.Is(statErr, os.ErrNotExist) {
		if statErr != nil {
			return Bundle{}, statErr
		}
		return Bundle{}, ErrExists
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return Bundle{}, err
	}
	stage := ".staging-" + hex.EncodeToString(nonce[:])
	if err := m.root.Mkdir(stage, 0o700); err != nil {
		return Bundle{}, err
	}
	published := false
	defer func() {
		if !published {
			_ = m.root.Chmod(stage, 0o700)
			err = errors.Join(err, m.root.RemoveAll(stage))
		}
	}()
	manifest := Manifest{SchemaVersion: 1, RunInputDigest: digest, Entries: make([]Entry, 0, len(input.ContextRefs))}
	var total int64
	for i, ref := range input.ContextRefs {
		if err := ctx.Err(); err != nil {
			return Bundle{}, err
		}
		file := entryName(i, ref)
		limit := m.limits.EntryBytes
		if remaining := m.limits.TotalBytes - total; remaining < limit {
			limit = remaining
		}
		size, err := m.writeEntry(ctx, stage+"/"+file, ref, limit)
		if err != nil {
			return Bundle{}, err
		}
		total += size
		manifest.Entries = append(manifest.Entries, Entry{Ref: ref, File: file, Size: size})
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return Bundle{}, err
	}
	if err := m.writeManifest(stage+"/manifest.json", data); err != nil {
		return Bundle{}, err
	}
	if err := ctx.Err(); err != nil {
		return Bundle{}, err
	}
	if err := m.root.Chmod(stage, 0o500); err != nil {
		return Bundle{}, err
	}
	if err := m.root.Rename(stage, name); err != nil {
		return Bundle{}, err
	}
	published = true
	return Bundle{Path: filepath.Join(m.path, name), Digest: canonical.BytesDigest(data), Manifest: manifest}, nil
}

func (m *Materializer) writeEntry(ctx context.Context, name string, ref core.ContextRef, limit int64) (size int64, err error) {
	source, err := m.resolver.Open(ctx, ref)
	if err != nil {
		return 0, fmt.Errorf("resolve context %s: %w", ref.Source, err)
	}
	if source == nil {
		return 0, fmt.Errorf("resolver returned a nil reader")
	}
	defer func() { err = errors.Join(err, source.Close()) }()
	file, err := m.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	hash := sha256.New()
	size, err = io.Copy(io.MultiWriter(file, hash), io.LimitReader(contextReader{ctx, source}, limit+1))
	if err != nil {
		return size, err
	}
	if size > limit {
		return size, ErrLimit
	}
	if "sha256:"+hex.EncodeToString(hash.Sum(nil)) != ref.Digest {
		return size, ErrDigest
	}
	if err := file.Sync(); err != nil {
		return size, err
	}
	return size, file.Chmod(0o400)
}

func (m *Materializer) writeManifest(name string, data []byte) (err error) {
	file, err := m.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return file.Chmod(0o400)
}

func within(parent, path string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func entryName(i int, ref core.ContextRef) string {
	return fmt.Sprintf("%03d-%s.bin", i, strings.TrimPrefix(ref.Digest, "sha256:"))
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
