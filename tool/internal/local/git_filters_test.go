package local

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInspectRepositoryRejectsExecutableGitFiltersBeforeCheckout(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, output)
		}
	}
	git("init", "-b", "main")
	git("config", "user.name", "fixture")
	git("config", "user.email", "fixture@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "input.txt"), []byte("hello\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "input.txt")
	git("commit", "-m", "base")
	marker := filepath.Join(t.TempDir(), "filter-started")
	filter := "sh -c 'touch " + marker + "; cat'"
	git("config", "filter.executable.smudge", filter)
	if err := os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("*.txt filter=executable\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := InspectRepository(ctx, repo, "main")
	if err == nil || !strings.Contains(err.Error(), "executable Git filters") {
		t.Fatalf("expected fail-closed Git filter rejection, got %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("filter command ran during repository inspection: %v", err)
	}
}
