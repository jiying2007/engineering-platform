package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestBackupRestoreRealDeliveryAtOriginalPath(t *testing.T) {
	ctx := context.Background()
	store, root := executionStore(t), executionRepository(t)
	owner := executionOwner(t, store)
	check := Check{Name: "verify", Argv: []string{"/bin/sh", "-c", "grep -qx after source.txt && mkdir -p reports && printf captured > reports/result.txt"}, Timeout: "5s"}
	completed := executionTask(t, store, root, "cat > source.txt; ln -s source.txt source-link; printf retained-log", []Check{check}, []string{"reports/*.txt"}, false, "5s")
	completed, err := Execute(ctx, store, completed.ID)
	if err != nil || completed.State != StateDone {
		t.Fatalf("fixture delivery: %+v, %v", completed, err)
	}
	attempt := lastExecutionAttempt(t, store, completed)
	queued := executionTask(t, store, root, "cat >/dev/null; touch should-not-run", nil, nil, false, "5s")
	owner.Close()
	attemptDir := filepath.Dir(attempt.Result.RuntimeStdout)
	if err := os.Symlink("runtime.stdout.log", filepath.Join(attemptDir, "stdout-link")); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(external, []byte("outside bytes are not backup content"), 0600); err != nil {
		t.Fatal(err)
	}
	artifactLink := filepath.Join(filepath.Dir(attempt.Result.Artifacts[0].Path), "outside-link")
	if err := os.Symlink(external, artifactLink); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"projects", "runtimes"} {
		if err := os.Mkdir(filepath.Join(store.DataDir(), kind), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(store.DataDir(), kind, "fixture.yaml"), []byte("# retained local registration\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	backupPath := filepath.Join(t.TempDir(), "snapshot")
	manifest, err := Backup(ctx, store, backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SourceDir != store.DataDir() || manifest.Schema != schemaVersion || len(manifest.Entries) == 0 {
		t.Fatalf("incomplete manifest: %+v", manifest)
	}
	links := 0
	for _, entry := range manifest.Entries {
		if entry.Type == "symlink" {
			links++
		}
		if entry.Path == "eng.db" && (entry.SHA256 == "" || entry.Size == 0) {
			t.Fatal("database snapshot lacks integrity evidence")
		}
	}
	if links < 3 {
		t.Fatalf("symlinks were dereferenced or omitted: %+v", manifest.Entries)
	}
	dataDir := store.DataDir()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dataDir, dataDir+".preserved"); err != nil {
		t.Fatal(err)
	}
	if err := Restore(ctx, backupPath, dataDir); err != nil {
		t.Fatal(err)
	}
	restored, err := OpenStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	got, err := restored.GetTask(ctx, completed.ID)
	if err != nil || got.State != StateDone || got.BaseSHA != completed.BaseSHA || got.Worktree != completed.Worktree {
		t.Fatalf("delivery identity changed: %+v, %v", got, err)
	}
	if err := verifyWorkspace(ctx, got); err != nil {
		t.Fatalf("original Git common directory no longer resolves restored worktree: %v", err)
	}
	head, err := gitOutput(ctx, got.Worktree, "rev-parse", "HEAD")
	if err != nil || head != attempt.Result.ResultCommit {
		t.Fatalf("restored result commit: %s, %v", head, err)
	}
	result := lastExecutionAttempt(t, restored, got).Result
	log, err := os.ReadFile(result.RuntimeStdout)
	if err != nil || string(log) != "retained-log" {
		t.Fatalf("restored log: %q, %v", log, err)
	}
	artifact, err := os.ReadFile(result.Artifacts[0].Path)
	sum := sha256.Sum256(artifact)
	if err != nil || string(artifact) != "captured" || result.Artifacts[0].Digest != "sha256:"+hex.EncodeToString(sum[:]) {
		t.Fatalf("restored artifact lost content or hash: %q, %v", artifact, err)
	}
	if link, err := os.Readlink(filepath.Join(attemptDir, "stdout-link")); err != nil || link != "runtime.stdout.log" {
		t.Fatalf("relative log symlink changed: %q, %v", link, err)
	}
	if link, err := os.Readlink(artifactLink); err != nil || link != external {
		t.Fatalf("external symlink was followed: %q, %v", link, err)
	}
	paused, err := restored.GetTask(ctx, queued.ID)
	if err != nil || paused.State != StateWaiting || paused.CurrentAttempt != 0 {
		t.Fatalf("restored queued work was not paused: %+v, %v", paused, err)
	}
	restoredOwner := executionOwner(t, restored)
	if _, err := Recover(ctx, restored); err != nil {
		t.Fatalf("unstarted restored queue must be a valid waiting item: %v", err)
	}
	attempts, err := restored.ListAttempts(ctx, queued.ID)
	if err != nil || len(attempts) != 0 {
		t.Fatalf("restore/recover started queued work: %+v, %v", attempts, err)
	}
	restoredOwner.Close()
	events, err := restored.ListEvents(ctx, completed.ID)
	if err != nil {
		t.Fatal(err)
	}
	reconciled := false
	for _, event := range events {
		if event.Kind == "task.restored_reconcile" {
			reconciled = true
		}
	}
	if !reconciled {
		t.Fatal("restored deliveries must require publication read-back")
	}
}

func backupRejectionFixture(t *testing.T) (*Store, string) {
	t.Helper()
	store := executionStore(t)
	backupPath := filepath.Join(t.TempDir(), "snapshot")
	if _, err := Backup(context.Background(), store, backupPath); err != nil {
		t.Fatal(err)
	}
	return store, backupPath
}

func preserveBackupSource(t *testing.T, store *Store) string {
	t.Helper()
	path := store.DataDir()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".preserved"); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRestoreRejectsTamperingMissingDatabaseAndExistingTarget(t *testing.T) {
	t.Run("tampered-bytes", func(t *testing.T) {
		store, backupPath := backupRejectionFixture(t)
		destination := preserveBackupSource(t, store)
		file, err := os.OpenFile(filepath.Join(backupPath, "eng.db"), os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, err = file.Write([]byte("changed"))
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		if err := Restore(context.Background(), backupPath, destination); err == nil || !strings.Contains(err.Error(), "manifest") {
			t.Fatalf("tampered bytes accepted: %v", err)
		}
		if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("invalid backup created a destination")
		}
	})
	t.Run("manifest-without-database", func(t *testing.T) {
		store, backupPath := backupRejectionFixture(t)
		destination := preserveBackupSource(t, store)
		if err := os.Remove(filepath.Join(backupPath, "eng.db")); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join(backupPath, "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		var manifest BackupManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			t.Fatal(err)
		}
		manifest.Entries, err = backupInventory(context.Background(), backupPath)
		if err != nil {
			t.Fatal(err)
		}
		raw, err = json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(backupPath, "manifest.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		if err := Restore(context.Background(), backupPath, destination); err == nil || !strings.Contains(err.Error(), "eng.db") {
			t.Fatalf("empty but self-consistent manifest accepted as a database backup: %v", err)
		}
	})
	t.Run("existing-state", func(t *testing.T) {
		store, backupPath := backupRejectionFixture(t)
		before, err := os.ReadFile(filepath.Join(store.DataDir(), "eng.db"))
		if err != nil {
			t.Fatal(err)
		}
		if err := Restore(context.Background(), backupPath, store.DataDir()); err == nil || !strings.Contains(err.Error(), "absent destination") {
			t.Fatalf("restore overwrote existing state: %v", err)
		}
		after, err := os.ReadFile(filepath.Join(store.DataDir(), "eng.db"))
		if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
			t.Fatal("rejected restore changed existing database bytes")
		}
	})
}

func TestBackupRefusesActiveSchedulerAndUnresolvedProcess(t *testing.T) {
	t.Run("active-scheduler", func(t *testing.T) {
		store := executionStore(t)
		executionOwner(t, store)
		destination := filepath.Join(t.TempDir(), "must-not-exist")
		if _, err := Backup(context.Background(), store, destination); err == nil {
			t.Fatal("backup ran concurrently with the scheduler")
		}
		if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("blocked backup created files")
		}
	})
	t.Run("surviving-child", func(t *testing.T) {
		ctx := context.Background()
		store, root := executionStore(t), executionRepository(t)
		submitted := executionTask(t, store, root, "/bin/true", nil, nil, false, "5s")
		_, attempt, err := store.ClaimTask(ctx, submitted.ID)
		if err != nil {
			t.Fatal(err)
		}
		attempt.StartCommit, err = gitOutput(ctx, root, "rev-parse", "HEAD")
		if err != nil {
			t.Fatal(err)
		}
		if err := store.UpdateAttempt(ctx, attempt); err != nil {
			t.Fatal(err)
		}
		child := exec.Command("/bin/sleep", "30")
		child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		identity, err := identifyProcess(child.Process.Pid)
		if err != nil {
			child.Process.Kill()
			child.Wait()
			t.Fatal(err)
		}
		defer func() { stopProcessGroup(identity, 100*time.Millisecond); child.Wait() }()
		attempt.Process, attempt.RuntimeStarted, attempt.Result.LocalProcessStopped = &identity, true, false
		if err := store.UpdateAttempt(ctx, attempt); err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(t.TempDir(), "must-not-exist")
		if _, err := Backup(ctx, store, destination); !errors.Is(err, ErrProcessUnresolved) {
			t.Fatalf("backup accepted a live original group: %v", err)
		}
		if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("unresolved process backup created files")
		}
	})
}
