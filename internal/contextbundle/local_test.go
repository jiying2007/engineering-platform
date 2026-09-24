package contextbundle

import (
	"context"
	"errors"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalSourceBindsApprovalAndRechecksBytes(t *testing.T) {
	input := inputFixture("approved bytes")
	digest, _ := input.Digest()
	subject := Subject{input.RunID, input.TaskContractDigest, digest}
	root := t.TempDir()
	file := filepath.Join(root, strings.TrimPrefix(input.ContextRefs[0].Digest, "sha256:")+".bin")
	if err := os.WriteFile(file, []byte("approved bytes"), 0o400); err != nil {
		t.Fatal(err)
	}
	approvals := []LocalApproval{{Subject: subject, Refs: input.ContextRefs}}
	source, err := NewLocalSource(root, approvals)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	// Caller-owned approval data cannot mutate the compiled snapshot.
	saved := input.ContextRefs[0]
	approvals[0].Refs[0].Version = "changed"
	input.ContextRefs[0] = saved
	m, bundleRoot, work := fixture(t, source, source, Limits{})
	bundle, err := m.Materialize(context.Background(), input, work)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Verify(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	other := subject
	other.RunID = "other"
	if !errors.Is(source.Authorize(context.Background(), other, saved), ErrDenied) {
		t.Fatal("wrong Run authorized")
	}
	other = subject
	other.RunInputDigest = canonical.BytesDigest([]byte("other"))
	if !errors.Is(source.Authorize(context.Background(), other, saved), ErrDenied) {
		t.Fatal("wrong input authorized")
	}
	if !within(bundleRoot, bundle.Path) {
		t.Fatal("unmanaged bundle")
	}
	if err := os.Chmod(file, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Open(context.Background(), saved); !errors.Is(err, ErrUnsafePath) {
		t.Fatal("writable source accepted", err)
	}
}
func TestLocalSourceRejectsSymlinksAndUnapprovedRefs(t *testing.T) {
	input := inputFixture("bytes")
	digest, _ := input.Digest()
	root := t.TempDir()
	ref := input.ContextRefs[0]
	source, err := NewLocalSource(root, []LocalApproval{{Subject: Subject{input.RunID, input.TaskContractDigest, digest}, Refs: input.ContextRefs}})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	other := ref
	other.Source = "unapproved"
	if _, err := source.Open(context.Background(), other); !errors.Is(err, ErrDenied) {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(target, []byte("bytes"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, strings.TrimPrefix(ref.Digest, "sha256:")+".bin")); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Open(context.Background(), ref); !errors.Is(err, ErrUnsafePath) {
		t.Fatal("symlink source accepted", err)
	}
}
