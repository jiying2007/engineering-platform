package main

import (
	"fmt"
	"io"
	"os"
)

const maxSteeringTextBytes = 8192

// Steering text is untrusted operator input, not an execution/approval token.
// Fail before the external control POST if the named file changes or is not
// stable. The platform opener must not block on FIFO substitution.
func readSteeringText(path string) (string, error) {
	return readSteeringTextWithOpener(path, openSteeringFile)
}

func readSteeringTextWithOpener(path string, opener func(string) (*os.File, error)) (string, error) {
	if path == "" || opener == nil {
		return "", fmt.Errorf("bounded regular steering text file required")
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() <= 0 || before.Size() > maxSteeringTextBytes {
		return "", fmt.Errorf("bounded regular steering text file required")
	}
	f, err := opener(path)
	if err != nil {
		return "", fmt.Errorf("steering text could not be safely opened: %w", err)
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) ||
		before.Size() != opened.Size() || !before.ModTime().Equal(opened.ModTime()) {
		return "", fmt.Errorf("steering text changed before read")
	}
	raw, err := io.ReadAll(io.LimitReader(f, maxSteeringTextBytes+1))
	if err != nil || len(raw) != int(before.Size()) {
		return "", fmt.Errorf("steering text changed during read")
	}
	onDescriptor, err := f.Stat()
	if err != nil || !onDescriptor.Mode().IsRegular() ||
		!os.SameFile(opened, onDescriptor) || opened.Size() != onDescriptor.Size() ||
		!opened.ModTime().Equal(onDescriptor.ModTime()) {
		return "", fmt.Errorf("steering text descriptor changed during read")
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() ||
		!before.ModTime().Equal(after.ModTime()) {
		return "", fmt.Errorf("steering text path changed during read")
	}
	return string(raw), nil
}
