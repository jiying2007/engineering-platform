package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// readRelayInput reads one exact bounded host-owned file. The shared steering
// opener provides Linux no-follow, close-on-exec and nonblocking semantics:
// a path switched to a symlink/FIFO after Lstat cannot redirect or stall us.
// Unsupported non-Linux openers fail closed, not as an unsafe os.Open fallback.
// This is offline provenance checking, not authentication of the file's author.
func readRelayInput(path string, maxBytes int64, requirePrivate bool) (data []byte, err error) {
	if maxBytes <= 0 || maxBytes > 2<<20 {
		return nil, fmt.Errorf("invalid relay input size limit")
	}
	before, statErr := os.Lstat(path)
	if statErr != nil || !relayInputFileOK(before, maxBytes, requirePrivate) {
		return nil, fmt.Errorf("bounded regular relay input required")
	}
	file, openErr := openSteeringFile(path)
	if openErr != nil {
		return nil, fmt.Errorf("open relay input safely: %w", openErr)
	}
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
	after, statErr := os.Lstat(path)
	if statErr != nil || !os.SameFile(before, after) || !relayInputFileOK(after, maxBytes, requirePrivate) ||
		after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || after.Mode() != before.Mode() {
		return nil, fmt.Errorf("relay input changed during read")
	}
	return data, nil
}

func relayInputFileOK(info os.FileInfo, maxBytes int64, requirePrivate bool) bool {
	return info != nil && info.Mode().IsRegular() && info.Size() > 0 && info.Size() <= maxBytes &&
		(!requirePrivate || info.Mode().Perm() == 0o600)
}
