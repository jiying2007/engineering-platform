//go:build linux

package distribution

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type ReleaseIdentity struct {
	SourceCommit   string `json:"source_commit"`
	ManifestDigest string `json:"manifest_digest"`
}

type ReleaseSwitchResult struct {
	Version             int             `json:"version"`
	Status              string          `json:"status"`
	Active              string          `json:"active"`
	Previous            string          `json:"previous"`
	ActiveIdentity      ReleaseIdentity `json:"active_identity"`
	PreviousIdentity    ReleaseIdentity `json:"previous_identity"`
	ServicesStarted     bool            `json:"services_started"`
	DatabaseChanged     bool            `json:"database_changed"`
	ExecutionAuthorized bool            `json:"execution_authorized"`
	ProductionQualified bool            `json:"production_qualified"`
}

func releaseChild(parent, path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && filepath.Dir(path) == parent &&
		path != parent && !strings.ContainsAny(path, "\x00\r\n")
}

func releaseRootOwner(path string) (uint32, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return 0, fmt.Errorf("release directory required")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("Unix release ownership required")
	}
	return stat.Uid, nil
}

func processUsesRelease(procRoot string, self int, active, candidate string) error {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return fmt.Errorf("cannot inspect running release processes")
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		exe, err := os.Readlink(filepath.Join(procRoot, entry.Name(), "exe"))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("cannot inspect running release process")
		}
		exe = strings.TrimSuffix(exe, " (deleted)")
		in := func(root string) bool { return exe == root || strings.HasPrefix(exe, root+string(filepath.Separator)) }
		if in(active) || in(candidate) {
			if pid == self && in(candidate) && !in(active) {
				continue
			}
			return fmt.Errorf("release has a running executable")
		}
	}
	return nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

// SwitchInstalledRelease performs only an immutable directory cutover. The
// caller must stop services first. This function independently verifies both
// releases from externally retained identities, rejects running executables,
// serializes switches on the common parent, preserves the old active release at
// previous, and never starts services or touches the database. Rollback uses
// the same operation with the retained previous release as candidate.
//
// A host crash between the two directory renames can leave active absent while
// previous and candidate remain. That is an explicit reconciliation state, not
// success; installation-readback can identify both surviving releases before
// an operator restores one. No automatic database downgrade is attempted.
func SwitchInstalledRelease(ctx context.Context, active, candidate, previous string, activeID, candidateID ReleaseIdentity) (ReleaseSwitchResult, error) {
	if os.Geteuid() != 0 {
		return ReleaseSwitchResult{}, fmt.Errorf("host administrator/root required for release switch")
	}
	return switchInstalledRelease(ctx, active, candidate, previous, activeID, candidateID, "/proc", os.Getpid())
}

func switchInstalledRelease(ctx context.Context, active, candidate, previous string, activeID, candidateID ReleaseIdentity, procRoot string, self int) (ReleaseSwitchResult, error) {
	var zero ReleaseSwitchResult
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if activeID == candidateID || activeID.SourceCommit == candidateID.SourceCommit ||
		!sourceCommit.MatchString(activeID.SourceCommit) || !sourceCommit.MatchString(candidateID.SourceCommit) ||
		!manifestDigestPattern.MatchString(activeID.ManifestDigest) || !manifestDigestPattern.MatchString(candidateID.ManifestDigest) {
		return zero, fmt.Errorf("distinct retained active and candidate identities required")
	}
	parent := filepath.Dir(active)
	if !releaseChild(parent, active) || !releaseChild(parent, candidate) || !releaseChild(parent, previous) ||
		active == candidate || active == previous || candidate == previous {
		return zero, fmt.Errorf("three distinct direct release children of one parent required")
	}
	parentInfo, err := controlledDirectory(parent, true)
	if err != nil {
		return zero, err
	}
	parentStat := parentInfo.Sys().(*syscall.Stat_t)
	for _, path := range []string{active, candidate} {
		uid, err := releaseRootOwner(path)
		if err != nil || uid != parentStat.Uid {
			return zero, fmt.Errorf("release ownership differs from controlled parent")
		}
	}
	if _, err := os.Lstat(previous); !errors.Is(err, os.ErrNotExist) {
		return zero, fmt.Errorf("previous release destination must not exist")
	}
	if _, err := VerifyInstalledRelease(ctx, active, activeID.SourceCommit, activeID.ManifestDigest); err != nil {
		return zero, fmt.Errorf("active release: %w", err)
	}
	if _, err := VerifyInstalledRelease(ctx, candidate, candidateID.SourceCommit, candidateID.ManifestDigest); err != nil {
		return zero, fmt.Errorf("candidate release: %w", err)
	}
	parentHandle, err := os.Open(parent)
	if err != nil {
		return zero, err
	}
	defer parentHandle.Close()
	if err := syscall.Flock(int(parentHandle.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return zero, fmt.Errorf("another release switch holds the parent lock")
	}
	defer syscall.Flock(int(parentHandle.Fd()), syscall.LOCK_UN)
	// Re-read all mutable preconditions after acquiring the serialization lock.
	if _, err := os.Lstat(previous); !errors.Is(err, os.ErrNotExist) {
		return zero, fmt.Errorf("previous release destination appeared")
	}
	if _, err := VerifyInstalledRelease(ctx, active, activeID.SourceCommit, activeID.ManifestDigest); err != nil {
		return zero, fmt.Errorf("active release changed before switch")
	}
	if _, err := VerifyInstalledRelease(ctx, candidate, candidateID.SourceCommit, candidateID.ManifestDigest); err != nil {
		return zero, fmt.Errorf("candidate release changed before switch")
	}
	if err := processUsesRelease(procRoot, self, active, candidate); err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if err := os.Rename(active, previous); err != nil {
		return zero, err
	}
	if err := syncDirectory(parent); err != nil {
		_ = os.Rename(previous, active)
		_ = syncDirectory(parent)
		return zero, fmt.Errorf("release switch parent sync failed")
	}
	if err := os.Rename(candidate, active); err != nil {
		restoreErr := os.Rename(previous, active)
		syncErr := syncDirectory(parent)
		return zero, errors.Join(fmt.Errorf("candidate activation failed"), restoreErr, syncErr)
	}
	if err := syncDirectory(parent); err != nil {
		return zero, fmt.Errorf("candidate activated but parent sync failed; reconciliation required")
	}
	_, activeErr := VerifyInstalledRelease(ctx, active, candidateID.SourceCommit, candidateID.ManifestDigest)
	_, previousErr := VerifyInstalledRelease(ctx, previous, activeID.SourceCommit, activeID.ManifestDigest)
	if activeErr != nil || previousErr != nil {
		// Best-effort byte-preserving revert. Failure remains explicit.
		moveCandidate := os.Rename(active, candidate)
		restoreActive := os.Rename(previous, active)
		syncErr := syncDirectory(parent)
		return zero, errors.Join(fmt.Errorf("post-switch readback failed; revert attempted"), activeErr, previousErr, moveCandidate, restoreActive, syncErr)
	}
	after, err := controlledDirectory(parent, true)
	if err != nil || !os.SameFile(parentInfo, after) {
		return zero, fmt.Errorf("release parent changed during switch")
	}
	return ReleaseSwitchResult{
		Version: 1, Status: "INSTALLATION_SWITCH_BYTES_VERIFIED",
		Active: active, Previous: previous,
		ActiveIdentity: candidateID, PreviousIdentity: activeID,
	}, nil
}
