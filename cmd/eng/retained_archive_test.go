package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRetainedReviewArchiveOfflineReadback(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("python3 is required to validate retained terminal archives")
	}
	cmd := exec.Command(python, "-B", filepath.Join("..", "..", "scripts", "test_retained_review_archive.py"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("retained archive regression: %v\n%s", err, output)
	}
}
