package githubpublish

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func TestPublisherNewBranchPushHasAtomicAbsentLease(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	dir := t.TempDir()
	gitPath := filepath.Join(dir, "git")
	argumentsPath := filepath.Join(dir, "arguments")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argumentsPath + "\n"
	if err := os.WriteFile(gitPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	remote := &githubRemote{git: gitPath}
	plan := Plan{Repository: "test/repository", Branch: "test/race"}
	if err := remote.push(context.Background(), "test-only-token", dir, plan); err != nil {
		t.Fatal(err)
	}
	arguments, err := os.ReadFile(argumentsPath)
	if err != nil {
		t.Fatal(err)
	}
	expected := "--force-with-lease=refs/heads/test/race:"
	if !strings.Contains(string(arguments), expected+"\n") {
		t.Fatalf("branch creation lacks exact absent lease: %s", string(arguments))
	}
	if strings.Contains(string(arguments), "test-only-token") {
		t.Fatal("publisher token appeared in Git arguments")
	}
}

func TestPublisherGitLeaseRejectsConcurrentBranchCreation(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git binary unavailable")
	}
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	source := filepath.Join(root, "source")
	run := func(directory string, args ...string) string {
		t.Helper()
		cmd := exec.Command(git, args...)
		cmd.Dir = directory
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	run(root, "init", "--bare", bare)
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	run(source, "init")
	run(source, "config", "user.name", "Fixture")
	run(source, "config", "user.email", "fixture@example.invalid")
	file := filepath.Join(source, "content")
	if err := os.WriteFile(file, []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(source, "add", "content")
	run(source, "commit", "-m", "initial")
	first := run(source, "rev-parse", "HEAD")
	if err := os.WriteFile(file, []byte("second\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(source, "commit", "-am", "second")
	second := run(source, "rev-parse", "HEAD")
	run(source, "push", bare, first+":refs/heads/test-race")
	rejected := exec.Command(git, "-C", source, "push",
		"--force-with-lease=refs/heads/test-race:", bare, second+":refs/heads/test-race")
	if err := rejected.Run(); err == nil {
		t.Fatal("concurrent branch was updated despite absent-lease")
	}
	actual := run(root, "--git-dir="+bare, "rev-parse", "refs/heads/test-race")
	if actual != first {
		t.Fatalf("conflicting ref moved from %s to %s", first, actual)
	}
	run(source, "push", "--force-with-lease=refs/heads/test-new:", bare, second+":refs/heads/test-new")
	if actual := run(root, "--git-dir="+bare, "rev-parse", "refs/heads/test-new"); actual != second {
		t.Fatalf("valid new branch rejected or changed: %s", actual)
	}
}
