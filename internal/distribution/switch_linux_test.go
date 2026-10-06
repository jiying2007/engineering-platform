//go:build linux

package distribution

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func installedReleaseFixture(t *testing.T, parent, name string) (string, ReleaseIdentity) {
	t.Helper()
	from, sha := installFixture(t, name)
	dest := filepath.Join(parent, name)
	result, err := Install(context.Background(), from, dest, sha)
	installOK(t, err)
	installOK(t, os.RemoveAll(from))
	return dest, ReleaseIdentity{SourceCommit: sha, ManifestDigest: result.ManifestDigest}
}

func emptyProc(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "proc")
	installOK(t, os.Mkdir(root, 0755))
	return root
}

func TestInstalledReleaseSwitchAndRollbackPreserveBothIdentities(t *testing.T) {
	parent := t.TempDir()
	proc := emptyProc(t)
	active, oldID := installedReleaseFixture(t, parent, "active")
	candidate, newID := installedReleaseFixture(t, parent, "candidate")
	previous := filepath.Join(parent, "previous")
	result, err := switchInstalledRelease(context.Background(), active, candidate, previous, oldID, newID, proc, 999999)
	installOK(t, err)
	if result.Status != "INSTALLATION_SWITCH_BYTES_VERIFIED" || result.ActiveIdentity != newID || result.PreviousIdentity != oldID ||
		result.ServicesStarted || result.DatabaseChanged || result.ExecutionAuthorized || result.ProductionQualified {
		t.Fatal("switch result promoted authority or lost identity", result)
	}
	if _, err := VerifyInstalledRelease(context.Background(), active, newID.SourceCommit, newID.ManifestDigest); err != nil {
		t.Fatal("new release not active", err)
	}
	if _, err := VerifyInstalledRelease(context.Background(), previous, oldID.SourceCommit, oldID.ManifestDigest); err != nil {
		t.Fatal("old release not retained", err)
	}
	failed := filepath.Join(parent, "failed-new")
	rollback, err := switchInstalledRelease(context.Background(), active, previous, failed, newID, oldID, proc, 999999)
	installOK(t, err)
	if rollback.ActiveIdentity != oldID || rollback.PreviousIdentity != newID {
		t.Fatal("rollback identities drifted", rollback)
	}
	if _, err := VerifyInstalledRelease(context.Background(), active, oldID.SourceCommit, oldID.ManifestDigest); err != nil {
		t.Fatal("old release not restored", err)
	}
	if _, err := VerifyInstalledRelease(context.Background(), failed, newID.SourceCommit, newID.ManifestDigest); err != nil {
		t.Fatal("replaced candidate was not retained", err)
	}
}

func TestInstalledReleaseSwitchRejectsUnsafePreconditionsWithoutMutation(t *testing.T) {
	for _, mode := range []string{"same-id", "previous-exists", "wrong-digest", "cross-parent", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			parent := t.TempDir()
			active, oldID := installedReleaseFixture(t, parent, "active")
			candidate, newID := installedReleaseFixture(t, parent, "candidate")
			previous := filepath.Join(parent, "previous")
			ctx := context.Background()
			switch mode {
			case "same-id":
				newID = oldID
			case "previous-exists":
				installOK(t, os.Mkdir(previous, 0755))
			case "wrong-digest":
				newID.ManifestDigest = "sha256:" + string(make([]byte, 64))
			case "cross-parent":
				other := t.TempDir()
				moved := filepath.Join(other, "candidate")
				installOK(t, os.Rename(candidate, moved))
				candidate = moved
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if _, err := switchInstalledRelease(ctx, active, candidate, previous, oldID, newID, emptyProc(t), 999999); err == nil {
				t.Fatal("unsafe switch accepted", mode)
			}
			if mode != "cross-parent" {
				if _, err := VerifyInstalledRelease(context.Background(), active, oldID.SourceCommit, oldID.ManifestDigest); err != nil {
					t.Fatal("rejected switch changed active release", err)
				}
			}
		})
	}
}

func TestReleaseProcessUseRejectsActiveAndOtherCandidateProcesses(t *testing.T) {
	root := t.TempDir()
	active := filepath.Join(root, "active")
	candidate := filepath.Join(root, "candidate")
	for _, path := range []string{active, candidate} {
		installOK(t, os.Mkdir(path, 0755))
	}
	proc := filepath.Join(root, "proc")
	installOK(t, os.Mkdir(proc, 0755))
	add := func(pid int, target string) {
		t.Helper()
		dir := filepath.Join(proc, strconv.Itoa(pid))
		installOK(t, os.Mkdir(dir, 0755))
		installOK(t, os.Symlink(target, filepath.Join(dir, "exe")))
	}
	add(101, filepath.Join(active, "bin", "worker"))
	if err := processUsesRelease(proc, 999, active, candidate); err == nil {
		t.Fatal("running active release accepted")
	}
	installOK(t, os.RemoveAll(filepath.Join(proc, "101")))
	add(999, filepath.Join(candidate, "bin", "eng"))
	if err := processUsesRelease(proc, 999, active, candidate); err != nil {
		t.Fatal("switch process in candidate should be allowed", err)
	}
	add(102, filepath.Join(candidate, "bin", "worker"))
	if err := processUsesRelease(proc, 999, active, candidate); err == nil {
		t.Fatal("other running candidate process accepted")
	}
}

func TestInstalledReleaseSwitchPublicEntryRequiresHostAdministrator(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test runner is root; host-admin requirement is exercised by native upgrade regression")
	}
	_, err := SwitchInstalledRelease(context.Background(), "/x/active", "/x/candidate", "/x/previous",
		ReleaseIdentity{SourceCommit: strings.Repeat("a", 40), ManifestDigest: "sha256:" + strings.Repeat("a", 64)},
		ReleaseIdentity{SourceCommit: strings.Repeat("b", 40), ManifestDigest: "sha256:" + strings.Repeat("b", 64)})
	if err == nil {
		t.Fatal("non-root release switch accepted")
	}
}
