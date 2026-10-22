package engineeringevidence

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureSingleFileGitChangeReadsExactGitObjects(t *testing.T) {
	change := GitFileChange{Path: "marker.txt", Before: "before\n", After: "after\n"}
	for _, mode := range []string{
		"exact", "other-content", "other-mode", "other-type", "other-renamed", "other-deleted", "extra-path",
		"wrong-before", "missing-newline", "crlf", "trailing-bytes", "marker-mode", "marker-symlink", "marker-deleted", "marker-renamed",
	} {
		t.Run(mode, func(t *testing.T) {
			repo := t.TempDir()
			git := func(args ...string) string {
				t.Helper()
				command := exec.Command("git", append([]string{"-C", repo}, args...)...)
				command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
				out, err := command.CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %v: %s", args, err, out)
				}
				return strings.TrimSpace(string(out))
			}
			write := func(name, data string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(repo, name), []byte(data), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err)
				}
			}
			git("init", "--object-format=sha1")
			git("config", "user.name", "Git evidence test")
			git("config", "user.email", "test@example.invalid")
			git("config", "core.filemode", "true")
			write(change.Path, change.Before)
			if mode == "wrong-before" {
				write(change.Path, strings.TrimSuffix(change.Before, "\n"))
			}
			// Both tabs and newlines are legal path bytes and must not split records.
			const other = "unrelated\nfile\t.txt"
			write(other, "unchanged\n")
			git("add", ".")
			git("commit", "-m", "base")
			base := git("rev-parse", "HEAD")
			write(change.Path, change.After)
			switch mode {
			case "other-content":
				write(other, "changed\n")
			case "other-mode":
				must(os.Chmod(filepath.Join(repo, other), 0o755))
			case "other-type":
				must(os.Remove(filepath.Join(repo, other)))
				must(os.Symlink(change.Path, filepath.Join(repo, other)))
			case "other-renamed":
				must(os.Rename(filepath.Join(repo, other), filepath.Join(repo, "renamed\nfile")))
			case "other-deleted":
				must(os.Remove(filepath.Join(repo, other)))
			case "extra-path":
				write("new\npath", "extra\n")
			case "missing-newline":
				write(change.Path, "after")
			case "crlf":
				write(change.Path, "after\r\n")
			case "trailing-bytes":
				write(change.Path, change.After+"extra")
			case "marker-mode":
				must(os.Chmod(filepath.Join(repo, change.Path), 0o755))
			case "marker-symlink":
				must(os.Remove(filepath.Join(repo, change.Path)))
				must(os.Symlink(other, filepath.Join(repo, change.Path)))
			case "marker-deleted":
				must(os.Remove(filepath.Join(repo, change.Path)))
			case "marker-renamed":
				must(os.Rename(filepath.Join(repo, change.Path), filepath.Join(repo, "renamed-marker")))
			}
			git("add", "-A")
			git("commit", "-m", "result")
			result := git("rev-parse", "HEAD")
			// An unrelated dirty caller checkout cannot stand in for the Git objects.
			write("caller-only.txt", "not in either commit\n")
			observed, err := CaptureSingleFileGitChange(context.Background(), repo, base, result, change)
			if mode != "exact" {
				if err == nil {
					t.Fatal("nonconforming Git change accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			ordinary, err := CaptureGitChange(context.Background(), repo, base, result)
			if err != nil || observed != ordinary || observed.SchemaVersion != 1 {
				t.Fatalf("single-file check changed the manifest contract: %#v %#v %v", observed, ordinary, err)
			}
		})
	}
}
