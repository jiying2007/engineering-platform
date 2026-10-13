//go:build linux

package githubpublish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestPublisherBundleReadRejectsReplacedLeaf(t *testing.T) {
	for _, kind := range []string{"same-bytes-new-inode", "symlink-same-inode", "fifo", "setuid", "group-write"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "source.bundle")
			if err := os.WriteFile(path, []byte("frozen"), 0600); err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if err := os.Rename(path, filepath.Join(dir, "original")); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "same-bytes-new-inode":
				err = os.WriteFile(path, []byte("frozen"), 0600)
			case "symlink-same-inode":
				err = os.Symlink("original", path)
			case "fifo":
				err = syscall.Mkfifo(path, 0600)
			case "setuid", "group-write":
				err = os.Rename(filepath.Join(dir, "original"), path)
				if err == nil {
					mode := os.FileMode(0600 | os.ModeSetuid)
					if kind == "group-write" {
						mode = 0660
					}
					err = os.Chmod(path, mode)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			file, err := openBundleRead(root, "source.bundle", before, before.Size())
			if err == nil {
				file.Close()
				t.Fatalf("replaced bundle leaf accepted: %s", kind)
			}
		})
	}
}

type bundleCancelReader struct {
	cancel context.CancelFunc
	reads  int
}

func (r *bundleCancelReader) Read(p []byte) (int, error) {
	r.reads++
	copy(p, "read")
	r.cancel()
	return 4, nil
}

type bundleCancelWriter struct {
	cancel context.CancelFunc
	writes int
	size   int
}

func (w *bundleCancelWriter) Write(p []byte) (int, error) {
	w.writes++
	w.size += len(p)
	w.cancel()
	return len(p), nil
}

type bundleCountReader struct {
	source  *bytes.Reader
	read    int
	maximum int
}

func (r *bundleCountReader) Read(p []byte) (int, error) {
	if len(p) > r.maximum {
		r.maximum = len(p)
	}
	n, e := r.source.Read(p)
	r.read += n
	return n, e
}

func TestPublisherBundleCopyCooperativeCancellation(t *testing.T) {
	t.Run("during-read", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		source := &bundleCancelReader{cancel: cancel}
		var dst bytes.Buffer
		n, err := copyBundle(ctx, &dst, source, 10)
		if !errors.Is(err, context.Canceled) || n != 0 || dst.Len() != 0 || source.reads != 1 {
			t.Fatalf("cancelled chunk forwarded: n=%d len=%d reads=%d err=%v", n, dst.Len(), source.reads, err)
		}
	})
	t.Run("between-chunks", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		source := &bundleCountReader{source: bytes.NewReader(make([]byte, 3*bundleReadChunk))}
		dst := &bundleCancelWriter{cancel: cancel}
		n, err := copyBundle(ctx, dst, source, 3*bundleReadChunk)
		if !errors.Is(err, context.Canceled) || n != bundleReadChunk || dst.writes != 1 || source.read != bundleReadChunk {
			t.Fatalf("copy continued after cancellation: n=%d writes=%d read=%d err=%v", n, dst.writes, source.read, err)
		}
	})
}

func TestPublisherBundleCopyBoundedToFrozenSize(t *testing.T) {
	source := &bundleCountReader{source: bytes.NewReader(make([]byte, 3*bundleReadChunk))}
	n, err := copyBundle(context.Background(), io.Discard, source, 9)
	if err != nil || n != 10 || source.read != 10 || source.maximum > bundleReadChunk {
		t.Fatalf("copy ignored frozen-size bound: n=%d read=%d chunk=%d err=%v", n, source.read, source.maximum, err)
	}
	for _, size := range []int64{-1, 0, (1 << 30) + 1} {
		if _, err := copyBundle(context.Background(), io.Discard, bytes.NewReader(nil), size); err == nil {
			t.Fatal("invalid size accepted", size)
		}
	}
}

func TestPublisherBundlePreCancelledUsesOriginalContext(t *testing.T) {
	service, plan := publisherServiceFixture(t)
	provider := &Provider{artifactRoot: service.artifactRoot, artifactIdentity: service.artifactIdentity}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A missing root would return a filesystem error if either lane dropped ctx.
	if err := os.RemoveAll(service.artifactRoot); err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.verifyBundle(ctx, plan); !errors.Is(err, context.Canceled) {
		t.Fatalf("service lost cancellation: %v", err)
	}
	if _, err := provider.verifyBundle(ctx, plan.ExecutionID, plan.BundleDigest, plan.BundleSize); !errors.Is(err, context.Canceled) {
		t.Fatalf("provider lost cancellation: %v", err)
	}
}

// The cancellation is triggered at a deterministic check inside the actual
// copy, not with a timing-dependent goroutine racing a tiny fixture file.
type bundleCheckContext struct {
	context.Context
	cancel context.CancelFunc
	checks int
}

func (c *bundleCheckContext) Err() error {
	c.checks++
	if c.checks == 4 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestPublisherBundleMidReadCancellationLeavesNoPublication(t *testing.T) {
	for _, lane := range []string{"service", "provider"} {
		t.Run(lane, func(t *testing.T) {
			service, plan := publisherServiceFixture(t)
			privateTemp := t.TempDir()
			t.Setenv("TMPDIR", privateTemp)
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx := &bundleCheckContext{Context: parent, cancel: cancel}
			if lane == "service" {
				path, cleanup, err := service.verifyBundle(ctx, plan)
				if cleanup != nil {
					cleanup()
				}
				if !errors.Is(err, context.Canceled) || path != "" {
					t.Fatalf("service returned cancelled snapshot: %q %v", path, err)
				}
			} else {
				provider := &Provider{artifactRoot: service.artifactRoot, artifactIdentity: service.artifactIdentity}
				path, err := provider.verifyBundle(ctx, plan.ExecutionID, plan.BundleDigest, plan.BundleSize)
				if !errors.Is(err, context.Canceled) || path != "" {
					t.Fatalf("provider returned cancelled source: %q %v", path, err)
				}
			}
			entries, err := os.ReadDir(privateTemp)
			if err != nil || len(entries) != 0 {
				t.Fatalf("cancelled snapshot left staging: %v %v", entries, err)
			}
			entries, err = os.ReadDir(service.journal.path)
			if err != nil || len(entries) != 0 {
				t.Fatalf("cancelled snapshot reserved witness: %v %v", entries, err)
			}
			if service.remote.(*serviceRemote).publishCalls != 0 {
				t.Fatal("cancelled snapshot reached Publish")
			}
		})
	}
}

func TestPublisherCancelledBundleRequestStartsNothing(t *testing.T) {
	service, plan := publisherServiceFixture(t)
	data, err := json.Marshal(publishRequest{Plan: plan, Identity: testDispatchIdentity(plan)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := authorizedPublisherRequest(t, http.MethodPost, "/v1/publish", data).WithContext(ctx)
	response := httptest.NewRecorder()
	service.Handler().ServeHTTP(response, req)
	entries, err := os.ReadDir(service.journal.path)
	if response.Code < 400 || service.remote.(*serviceRemote).publishCalls != 0 || err != nil || len(entries) != 0 {
		t.Fatalf("cancelled request advanced: code=%d entries=%v err=%v", response.Code, entries, err)
	}
}

// Even a destination's ReaderFrom optimization may not enlarge a source read.
type bundleGreedyWriter struct{ maximum int }

func (*bundleGreedyWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *bundleGreedyWriter) ReadFrom(r io.Reader) (int64, error) {
	buf := make([]byte, 4*bundleReadChunk)
	var total int64
	for {
		n, err := r.Read(buf)
		if n > w.maximum {
			w.maximum = n
		}
		total += int64(n)
		if err == io.EOF {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
}
func TestPublisherBundleReaderOptimizationKeepsChunkBound(t *testing.T) {
	source := &bundleCountReader{source: bytes.NewReader(make([]byte, 3*bundleReadChunk))}
	writer := &bundleGreedyWriter{}
	n, err := copyBundle(context.Background(), writer, source, 3*bundleReadChunk)
	if err != nil || n != 3*bundleReadChunk || source.maximum > bundleReadChunk || writer.maximum > bundleReadChunk {
		t.Fatalf("ReaderFrom bypassed chunk bound: n=%d source=%d writer=%d err=%v", n, source.maximum, writer.maximum, err)
	}
}
func TestPublisherBundleReadPreservesReadableWorkerMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.bundle")
	if err := os.WriteFile(path, []byte("bytes"), 0640); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := os.Lstat(dir)
	if err != nil {
		t.Fatal(err)
	}
	root, err := openBundleRoot(dir, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	file, err := openBundleRead(root, "source.bundle", before, 5)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	file.Close()
	if err != nil || string(data) != "bytes" {
		t.Fatalf("valid Worker bundle rejected: %s %v", data, err)
	}
	if err := os.Rename(dir, dir+"-old"); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir + "-old")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if unexpected, err := openBundleRoot(dir, identity); err == nil {
		unexpected.Close()
		t.Fatal("replaced artifact directory admitted")
	}
}
