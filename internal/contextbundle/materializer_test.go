package contextbundle

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
)

type resolverFunc func(context.Context, core.ContextRef) (io.ReadCloser, error)

func (f resolverFunc) Open(ctx context.Context, ref core.ContextRef) (io.ReadCloser, error) {
	return f(ctx, ref)
}

type authorizerFunc func(context.Context, Subject, core.ContextRef) error

func (f authorizerFunc) Authorize(ctx context.Context, subject Subject, ref core.ContextRef) error {
	return f(ctx, subject, ref)
}

var allow = authorizerFunc(func(context.Context, Subject, core.ContextRef) error { return nil })

func inputFixture(data string) core.RunInputManifest {
	return core.RunInputManifest{
		RunID: "run-1", TaskContractDigest: canonical.BytesDigest([]byte("task")),
		RuntimeProfile: "codex-test-v1", ToolProfile: "read-only-v1", WorkerProfile: "test-v1", PolicyProfile: "test-v1",
		ContextRefs: []core.ContextRef{{Source: "docs:spec", Type: "DOCUMENT", Version: "r1", Digest: canonical.BytesDigest([]byte(data)), Trust: core.ContextApproved}},
	}
}

func byteResolver(data string) Resolver {
	return resolverFunc(func(context.Context, core.ContextRef) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(data)), nil
	})
}

func fixture(t *testing.T, resolver Resolver, auth Authorizer, limits Limits) (*Materializer, string, string) {
	t.Helper()
	base := t.TempDir()
	root, worktree := filepath.Join(base, "bundles"), filepath.Join(base, "worktree")
	for _, dir := range []string{root, worktree} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// Read-only bundles need directory write permission restored for test cleanup.
	t.Cleanup(func() {
		_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				return os.Chmod(path, 0o700)
			}
			return err
		})
	})
	m, err := New(root, resolver, auth, limits)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m, root, worktree
}

func assertEmpty(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed operation left context state: %v %v", entries, err)
	}
}

func TestMaterializeAndVerifyStableAcrossMachines(t *testing.T) {
	input := inputFixture("spec\n")
	m, root, worktree := fixture(t, byteResolver("spec\n"), allow, Limits{})
	bundle, err := m.Materialize(context.Background(), input, worktree)
	if err != nil {
		t.Fatal(err)
	}
	if !within(root, bundle.Path) || within(worktree, bundle.Path) {
		t.Fatalf("bad bundle path: %s", bundle.Path)
	}
	info, err := os.Stat(bundle.Path)
	if err != nil || info.Mode().Perm() != 0o500 {
		t.Fatalf("bundle is not read-only: %v %v", info, err)
	}
	for _, name := range []string{"manifest.json", bundle.Manifest.Entries[0].File} {
		info, err := os.Stat(filepath.Join(bundle.Path, name))
		if err != nil || info.Mode().Perm() != 0o400 {
			t.Fatalf("entry is not read-only: %v %v", info, err)
		}
	}
	reopened, err := New(root, byteResolver("must not be fetched by Verify"), allow, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	verified, err := reopened.Verify(context.Background(), input)
	if err != nil || verified.Digest != bundle.Digest {
		t.Fatalf("restart verify failed: %v", err)
	}
	if _, err := m.Materialize(context.Background(), input, worktree); !errors.Is(err, ErrExists) {
		t.Fatalf("overwrote immutable bundle: %v", err)
	}
	other, _, otherWorktree := fixture(t, byteResolver("spec\n"), allow, Limits{})
	copy, err := other.Materialize(context.Background(), input, otherWorktree)
	if err != nil || copy.Path == bundle.Path || copy.Digest != bundle.Digest {
		t.Fatalf("machine path contaminated identity: %v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(bundle.Path, "manifest.json"))
	if err != nil || bytes.Contains(manifest, []byte(root)) || bytes.Contains(manifest, []byte(worktree)) {
		t.Fatalf("manifest contains local locator: %v", err)
	}
}

func TestApprovalRequiresIndependentAuthorizationBeforeAnyFetch(t *testing.T) {
	input := inputFixture("spec")
	second := input.ContextRefs[0]
	second.Source = "docs:private"
	input.ContextRefs = append(input.ContextRefs, second)
	fetches, checks := 0, 0
	digest, _ := input.Digest()
	resolver := resolverFunc(func(context.Context, core.ContextRef) (io.ReadCloser, error) {
		fetches++
		return io.NopCloser(strings.NewReader("spec")), nil
	})
	auth := authorizerFunc(func(_ context.Context, subject Subject, ref core.ContextRef) error {
		checks++
		if subject.RunID != input.RunID || subject.RunInputDigest != digest || subject.TaskContractDigest != input.TaskContractDigest {
			t.Fatal("authorization subject is not frozen")
		}
		if ref.Source == second.Source {
			return errors.New("revoked")
		}
		return nil
	})
	m, root, worktree := fixture(t, resolver, auth, Limits{})
	if _, err := m.Materialize(context.Background(), input, worktree); !errors.Is(err, ErrDenied) {
		t.Fatalf("self-declared APPROVED bypassed authorization: %v", err)
	}
	if fetches != 0 || checks != 2 {
		t.Fatalf("fetched before all refs authorized: fetch=%d check=%d", fetches, checks)
	}
	assertEmpty(t, root)
	input = inputFixture("spec")
	input.ContextRefs[0].Trust = core.ContextUntrusted
	if _, err := m.Materialize(context.Background(), input, worktree); !errors.Is(err, ErrDenied) {
		t.Fatalf("untrusted content materialized: %v", err)
	}
	if fetches != 0 {
		t.Fatal("untrusted content fetched")
	}
	if _, err := New(root, resolver, nil, Limits{}); err == nil {
		t.Fatal("missing authorizer accepted")
	}
}

type closeFailure struct{ io.Reader }

func (closeFailure) Close() error { return errors.New("resolver close failure") }

type cancelReader struct{ cancel context.CancelFunc }

func (r cancelReader) Read(p []byte) (int, error) { r.cancel(); p[0] = 's'; return 1, nil }
func (cancelReader) Close() error                 { return nil }

func TestMaterializeFailuresLeaveNoPublishedOrStagedBytes(t *testing.T) {
	cases := []struct {
		name     string
		resolver Resolver
		limits   Limits
		want     error
	}{
		{"digest mismatch", byteResolver("wrong"), Limits{}, ErrDigest},
		{"entry limit", byteResolver("too large"), Limits{EntryBytes: 3}, ErrLimit},
		{"resolve failure", resolverFunc(func(context.Context, core.ContextRef) (io.ReadCloser, error) { return nil, errors.New("offline") }), Limits{}, nil},
		{"nil reader", resolverFunc(func(context.Context, core.ContextRef) (io.ReadCloser, error) { return nil, nil }), Limits{}, nil},
		{"close failure", resolverFunc(func(context.Context, core.ContextRef) (io.ReadCloser, error) {
			return closeFailure{strings.NewReader("spec")}, nil
		}), Limits{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, root, worktree := fixture(t, tc.resolver, allow, tc.limits)
			_, err := m.Materialize(context.Background(), inputFixture("spec"), worktree)
			if err == nil || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Fatalf("failure accepted: %v", err)
			}
			assertEmpty(t, root)
		})
	}
	t.Run("total limit", func(t *testing.T) {
		input := inputFixture("spec")
		second := input.ContextRefs[0]
		second.Source = "docs:second"
		input.ContextRefs = append(input.ContextRefs, second)
		m, root, worktree := fixture(t, byteResolver("spec"), allow, Limits{TotalBytes: 7})
		if _, err := m.Materialize(context.Background(), input, worktree); !errors.Is(err, ErrLimit) {
			t.Fatalf("total limit bypassed: %v", err)
		}
		assertEmpty(t, root)
	})
	t.Run("cancel during read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		m, root, worktree := fixture(t, resolverFunc(func(context.Context, core.ContextRef) (io.ReadCloser, error) { return cancelReader{cancel}, nil }), allow, Limits{})
		if _, err := m.Materialize(ctx, inputFixture("spec"), worktree); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation lost: %v", err)
		}
		assertEmpty(t, root)
	})
}

func TestUnsafeRootsAndOverlappingWorkspaceAreRejected(t *testing.T) {
	m, root, worktree := fixture(t, byteResolver("spec"), allow, Limits{})
	if _, err := m.Materialize(context.Background(), inputFixture("spec"), filepath.Dir(root)); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("bundle nested in source accepted: %v", err)
	}
	if _, err := m.Materialize(context.Background(), inputFixture("spec"), root); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("bundle root as source accepted: %v", err)
	}
	alias := filepath.Join(worktree, "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := New(alias, byteResolver("spec"), allow, Limits{}); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("symlink root accepted: %v", err)
	}
	if err := os.Chmod(root, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := New(root, byteResolver("spec"), allow, Limits{}); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("shared writable root accepted: %v", err)
	}
}

func TestVerifyDetectsTamperingAndRevocation(t *testing.T) {
	for _, attack := range []string{"content", "manifest", "extra", "symlink", "writable", "revoked"} {
		t.Run(attack, func(t *testing.T) {
			denied := false
			auth := authorizerFunc(func(context.Context, Subject, core.ContextRef) error {
				if denied {
					return ErrDenied
				}
				return nil
			})
			m, _, worktree := fixture(t, byteResolver("spec"), auth, Limits{})
			input := inputFixture("spec")
			bundle, err := m.Materialize(context.Background(), input, worktree)
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(bundle.Path, bundle.Manifest.Entries[0].File)
			switch attack {
			case "content", "manifest":
				if attack == "manifest" {
					file = filepath.Join(bundle.Path, "manifest.json")
				}
				if err := os.Chmod(file, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte("tampered"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(file, 0o400); err != nil {
					t.Fatal(err)
				}
			case "extra", "symlink":
				if err := os.Chmod(bundle.Path, 0o700); err != nil {
					t.Fatal(err)
				}
				if attack == "extra" {
					err = os.WriteFile(filepath.Join(bundle.Path, "injected"), []byte("unexpected"), 0o400)
				} else {
					if err := os.Remove(file); err != nil {
						t.Fatal(err)
					}
					external := filepath.Join(worktree, "external")
					if err := os.WriteFile(external, []byte("spec"), 0o400); err != nil {
						t.Fatal(err)
					}
					err = os.Symlink(external, file)
				}
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(bundle.Path, 0o500); err != nil {
					t.Fatal(err)
				}
			case "writable":
				if err := os.Chmod(file, 0o600); err != nil {
					t.Fatal(err)
				}
			case "revoked":
				denied = true
			}
			if _, err := m.Verify(context.Background(), input); err == nil {
				t.Fatalf("%s was not detected", attack)
			}
		})
	}
}

func TestConcurrentPublicationHasOneOwnerAndNoPartialBundle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	resolver := resolverFunc(func(ctx context.Context, _ core.ContextRef) (io.ReadCloser, error) {
		close(entered)
		select {
		case <-release:
			return io.NopCloser(strings.NewReader("spec")), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	m, root, worktree := fixture(t, resolver, allow, Limits{})
	input := inputFixture("spec")
	done := make(chan error, 1)
	go func() { _, err := m.Materialize(ctx, input, worktree); done <- err }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	digest, _ := input.Digest()
	if _, err := os.Stat(filepath.Join(root, strings.TrimPrefix(digest, "sha256:"))); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("partial destination exposed: %v", err)
	}
	if _, err := m.Materialize(ctx, input, worktree); !errors.Is(err, ErrBusy) {
		t.Errorf("concurrent writer was not fenced: %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := m.Verify(ctx, input); err != nil {
		t.Fatal(err)
	}
}
