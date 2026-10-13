package githubpublish

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

const bundleReadChunk = 64 << 10

// Bind the opened directory descriptor back to the startup identity as well as
// the pre-open path. A replacement between Lstat and OpenRoot is not admitted.
func openBundleRoot(path string, identity os.FileInfo) (*os.Root, error) {
	current, err := os.Lstat(path)
	if err != nil || identity == nil || !current.IsDir() || !os.SameFile(current, identity) {
		return nil, fmt.Errorf("publisher artifact root changed")
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(current, opened) {
		root.Close()
		return nil, fmt.Errorf("publisher artifact root changed while opening")
	}
	return root, nil
}

// Bundle files belong to the Worker, not the Publisher. Reuse only the held-
// directory, single-leaf no-follow/nonblocking open primitive, NOT the private
// witness ownership rule. A previous Lstat is not authority to follow a link or
// wait for a FIFO which replaced that leaf before Open.
func openBundleRead(root *os.Root, name string, before os.FileInfo, size int64) (*os.File, error) {
	if !validBundleFile(before, size) {
		return nil, fmt.Errorf("retained result bundle metadata invalid")
	}
	file, err := openWitnessRead(root, name)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !validBundleFile(opened, size) || !os.SameFile(before, opened) {
		_ = file.Close()
		return nil, fmt.Errorf("retained result bundle changed before read")
	}
	return file, nil
}

func validBundleFile(info os.FileInfo, size int64) bool {
	return info != nil && info.Mode().IsRegular() &&
		info.Mode().Perm()&0022 == 0 &&
		info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) == 0 &&
		size > 0 && size <= 1<<30 && info.Size() == size
}

type bundleContextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r bundleContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) > bundleReadChunk {
		p = p[:bundleReadChunk]
	}
	n, err := r.source.Read(p)
	if cancelled := r.ctx.Err(); cancelled != nil {
		// Do not forward a newly read chunk after observing cancellation.
		return 0, cancelled
	}
	return n, err
}

// Cancellation is cooperative between bounded reads. No goroutine, short
// deadline, or claim of interrupting a blocked regular-file kernel syscall.
// The extra byte detects a growing source without reading an arbitrary 1GiB.
func copyBundle(ctx context.Context, dst io.Writer, source io.Reader, size int64) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if size <= 0 || size > 1<<30 {
		return 0, fmt.Errorf("invalid retained bundle size")
	}
	n, err := io.CopyBuffer(dst, io.LimitReader(bundleContextReader{ctx, source}, size+1), make([]byte, min(int64(bundleReadChunk), size+1)))
	return n, errors.Join(err, ctx.Err())
}
