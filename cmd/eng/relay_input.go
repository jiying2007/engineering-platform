package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// readRelayInput reads one exact bounded host-owned file. The shared steering
// opener provides Linux no-follow, close-on-exec and nonblocking semantics:
// a path switched to a symlink/FIFO after Lstat cannot redirect or stall us.
// Unsupported non-Linux openers fail closed, not as an unsafe os.Open fallback.
// This is offline provenance checking, not authentication of the file's author.
func readRelayInput(path string, maxBytes int64, requirePrivate bool) ([]byte, error) {
	if err := relayInputLimit(maxBytes); err != nil {
		return nil, err
	}
	before, err := os.Lstat(path)
	if err != nil || !relayInputFileOK(before, maxBytes, requirePrivate) {
		return nil, fmt.Errorf("bounded regular relay input required")
	}
	file, err := openSteeringFile(path)
	if err != nil {
		return nil, fmt.Errorf("open relay input safely: %w", err)
	}
	return readRelayOpenedInput(file, before, func() (os.FileInfo, error) { return os.Lstat(path) }, maxBytes, requirePrivate)
}

// readRelayRootInput resolves only one named member relative to the opened
// qualification-kit directory. A rename of the original pathname cannot
// redirect this read to another directory or outside the original root.
func readRelayRootInput(root *os.Root, name string, maxBytes int64, requirePrivate bool) ([]byte, error) {
	if err := relayInputLimit(maxBytes); err != nil {
		return nil, err
	}
	if root == nil || name == "" || name == "." || filepath.Base(name) != name {
		return nil, fmt.Errorf("exact relay kit member name required")
	}
	before, err := root.Lstat(name)
	if err != nil || !relayInputFileOK(before, maxBytes, requirePrivate) {
		return nil, fmt.Errorf("bounded regular relay kit member required")
	}
	file, err := openRelayRootInput(root, name)
	if err != nil {
		return nil, fmt.Errorf("open anchored relay input safely: %w", err)
	}
	return readRelayOpenedInput(file, before, func() (os.FileInfo, error) { return root.Lstat(name) }, maxBytes, requirePrivate)
}

func relayInputLimit(maxBytes int64) error {
	if maxBytes <= 0 || maxBytes > 2<<20 {
		return fmt.Errorf("invalid relay input size limit")
	}
	return nil
}

// Both ordinary policy files and anchored kit members share exact descriptor
// and post-read identity validation. No path-only read occurs after preflight.
func readRelayOpenedInput(file *os.File, before os.FileInfo, reread func() (os.FileInfo, error), maxBytes int64, requirePrivate bool) (data []byte, err error) {
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			data = nil
			err = errors.Join(err, closeErr)
		}
	}()
	opened, statErr := file.Stat()
	if statErr != nil || !os.SameFile(before, opened) || !relayInputFileOK(opened, maxBytes, requirePrivate) ||
		opened.Size() != before.Size() || opened.Mode() != before.Mode() {
		return nil, fmt.Errorf("relay input changed before read")
	}
	data, err = io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("relay input read failed: %w", err)
	}
	if len(data) == 0 || int64(len(data)) != before.Size() || int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("relay input size changed or exceeds bound")
	}
	after, statErr := reread()
	openedAfter, openedErr := file.Stat()
	if statErr != nil || openedErr != nil || !os.SameFile(before, after) || !os.SameFile(before, openedAfter) ||
		!relayInputFileOK(after, maxBytes, requirePrivate) || !relayInputFileOK(openedAfter, maxBytes, requirePrivate) ||
		after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || after.Mode() != before.Mode() ||
		openedAfter.Size() != before.Size() || !openedAfter.ModTime().Equal(before.ModTime()) || openedAfter.Mode() != before.Mode() {
		return nil, fmt.Errorf("relay input changed during read")
	}
	return data, nil
}

func relayInputFileOK(info os.FileInfo, maxBytes int64, requirePrivate bool) bool {
	return info != nil && info.Mode().IsRegular() && info.Size() > 0 && info.Size() <= maxBytes &&
		(!requirePrivate || info.Mode().Perm() == 0o600)
}
