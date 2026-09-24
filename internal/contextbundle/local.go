package contextbundle

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
)

// LocalApproval is a trusted operator input, never a field accepted from a Run
// request. It pins the full frozen input and the exact refs it may materialize.
// This is a restart-loaded local approval snapshot, not an identity/approval UI.
type LocalApproval struct {
	Subject Subject
	Refs    []core.ContextRef
}
type localKey struct {
	subject Subject
	ref     core.ContextRef
}
type LocalSource struct {
	root     *os.Root
	approved map[localKey]bool
	refs     map[core.ContextRef]bool
}

// NewLocalSource opens a pre-populated, host-owned content-addressed directory.
// Files are <64 lowercase digest hex>.bin. There are no caller paths or URLs.
func NewLocalSource(path string, approvals []LocalApproval) (*LocalSource, error) {
	if !filepath.IsAbs(path) || len(approvals) == 0 || len(approvals) > 256 {
		return nil, ErrDenied
	}
	clean := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil || resolved != clean {
		return nil, ErrUnsafePath
	}
	info, err := os.Stat(clean)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o022 != 0 {
		return nil, ErrUnsafePath
	}
	s := &LocalSource{approved: make(map[localKey]bool), refs: make(map[core.ContextRef]bool)}
	for _, a := range approvals {
		if a.Subject.RunID == "" || !canonical.ValidDigest(a.Subject.TaskContractDigest) || !canonical.ValidDigest(a.Subject.RunInputDigest) || core.ValidateContextRefs(a.Refs) != nil {
			return nil, ErrDenied
		}
		for _, ref := range a.Refs {
			if ref.Trust != core.ContextApproved {
				return nil, ErrDenied
			}
			key := localKey{a.Subject, ref}
			if s.approved[key] {
				return nil, fmt.Errorf("duplicate local approval")
			}
			s.approved[key] = true
			s.refs[ref] = true
		}
	}
	s.root, err = os.OpenRoot(clean)
	if err != nil {
		return nil, err
	}
	return s, nil
}
func (s *LocalSource) Close() error { return s.root.Close() }
func (s *LocalSource) Authorize(ctx context.Context, subject Subject, ref core.ContextRef) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || !s.approved[localKey{subject, ref}] {
		return ErrDenied
	}
	return nil
}
func (s *LocalSource) Open(ctx context.Context, ref core.ContextRef) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.root == nil || !s.refs[ref] {
		return nil, ErrDenied
	}
	name := strings.TrimPrefix(ref.Digest, "sha256:") + ".bin"
	info, err := s.root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o222 != 0 {
		return nil, ErrUnsafePath
	}
	f, err := s.root.Open(name)
	if err != nil {
		return nil, err
	}
	actual, err := f.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(info, actual) || actual.Mode().Perm()&0o222 != 0 {
		_ = f.Close()
		return nil, ErrUnsafePath
	}
	// Materializer still streams within limits and recomputes the raw-byte digest.
	return f, nil
}
