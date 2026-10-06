//go:build linux

package distribution

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

var manifestDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type ReleaseReadbackResult struct {
	Version             int    `json:"version"`
	Status              string `json:"status"`
	SourceCommit        string `json:"source_commit"`
	ManifestDigest      string `json:"manifest_digest"`
	FileCount           int    `json:"file_count"`
	BinaryCount         int    `json:"binary_count"`
	ServicesStarted     bool   `json:"services_started"`
	DatabaseChanged     bool   `json:"database_changed"`
	ExecutionAuthorized bool   `json:"execution_authorized"`
	ProductionQualified bool   `json:"production_qualified"`
}

// VerifyInstalledRelease reads an immutable installed release using an
// externally retained installation-manifest digest. Unlike VerifyInstallation,
// it does not compare templates against the current executable's embedded
// assets, so it can read back a previous source version without weakening the
// byte identity. The supplied manifest digest is authority input: callers must
// retain it from the original authenticated installation receipt rather than
// recompute it from an arbitrary tree during recovery.
func VerifyInstalledRelease(ctx context.Context, dir, expectedSource, expectedManifestDigest string) (ReleaseReadbackResult, error) {
	var zero ReleaseReadbackResult
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if !sourceCommit.MatchString(expectedSource) || !manifestDigestPattern.MatchString(expectedManifestDigest) {
		return zero, fmt.Errorf("exact source commit and retained manifest digest required")
	}
	root, err := controlledDirectory(dir, false)
	if err != nil {
		return zero, err
	}
	rootStat, ok := root.Sys().(*syscall.Stat_t)
	if !ok {
		return zero, fmt.Errorf("Unix installation ownership required")
	}
	owner := rootStat.Uid
	manifestPath := filepath.Join(dir, "installation-manifest.json")
	info, err := os.Lstat(manifestPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0444 || info.Size() <= 0 || info.Size() > 1<<20 {
		return zero, fmt.Errorf("canonical installation manifest required")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != owner || stat.Nlink != 1 {
		return zero, fmt.Errorf("installation manifest ownership/link identity changed")
	}
	raw, err := readFile(manifestPath, 1<<20, false)
	if err != nil || rawDigest(raw) != expectedManifestDigest {
		return zero, fmt.Errorf("installation manifest differs from retained digest")
	}
	var manifest InstallationManifest
	if err := strictjson.Decode(raw, &manifest); err != nil {
		return zero, fmt.Errorf("invalid installation manifest")
	}
	if manifest.Version != 1 || manifest.SourceCommit != expectedSource || len(manifest.Files) == 0 || len(manifest.Files) > 256 {
		return zero, fmt.Errorf("installation manifest identity mismatch")
	}
	wanted := make(map[string]InstalledFile, len(manifest.Files)+1)
	dirs := map[string]bool{".": true}
	last := ""
	for _, fact := range manifest.Files {
		if !fs.ValidPath(fact.Path) || strings.ContainsAny(fact.Path, "\\\x00\r\n") || fact.Path == "installation-manifest.json" ||
			fact.Path <= last || fact.Size <= 0 || fact.Size > 128<<20 || !manifestDigestPattern.MatchString(fact.Digest) ||
			(fact.Mode != 0444 && fact.Mode != 0555) {
			return zero, fmt.Errorf("invalid or unordered installed release member")
		}
		last = fact.Path
		wanted[fact.Path] = fact
		for p := filepath.ToSlash(filepath.Dir(fact.Path)); p != "."; p = filepath.ToSlash(filepath.Dir(p)) {
			dirs[p] = true
		}
	}
	for _, name := range append(Names(), "file-manifest.json", "SHA256SUMS") {
		path := "bin/" + name
		if _, ok := wanted[path]; !ok {
			return zero, fmt.Errorf("installed release omits required binary member")
		}
	}
	seen := map[string]bool{}
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		current, err := os.Lstat(path)
		if err != nil {
			return err
		}
		st, ok := current.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != owner {
			return fmt.Errorf("installed release owner changed")
		}
		if entry.IsDir() {
			if !dirs[rel] || current.Mode().Perm() != 0755 || current.Mode()&os.ModeSetgid != 0 {
				return fmt.Errorf("unexpected or unsafe installed release directory")
			}
			return nil
		}
		if rel == "installation-manifest.json" {
			seen[rel] = true
			return nil
		}
		fact, ok := wanted[rel]
		if !ok || !current.Mode().IsRegular() || current.Mode().Perm() != os.FileMode(fact.Mode) ||
			current.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 || st.Nlink != 1 {
			return fmt.Errorf("unexpected or unsafe installed release file")
		}
		content, err := readFile(path, 128<<20, fact.Mode == 0555)
		if err != nil || int64(len(content)) != fact.Size || rawDigest(content) != fact.Digest {
			return fmt.Errorf("installed release bytes differ from retained manifest")
		}
		seen[rel] = true
		return nil
	})
	if err != nil {
		return zero, err
	}
	if len(seen) != len(wanted)+1 {
		return zero, fmt.Errorf("installed release is incomplete")
	}
	binary, err := Verify(filepath.Join(dir, "bin"))
	if err != nil || binary.SourceCommit != expectedSource {
		return zero, fmt.Errorf("installed release binary identity mismatch")
	}
	after, err := controlledDirectory(dir, false)
	if err != nil || !os.SameFile(root, after) {
		return zero, fmt.Errorf("installed release root changed during readback")
	}
	// Preserve deterministic member ordering as a contract check even though the
	// map is used for the filesystem walk.
	keys := make([]string, 0, len(wanted))
	for path := range wanted {
		keys = append(keys, path)
	}
	sort.Strings(keys)
	if len(keys) != len(manifest.Files) {
		return zero, fmt.Errorf("duplicate installed release members")
	}
	return ReleaseReadbackResult{
		Version: 1, Status: "INSTALLATION_RECEIPT_BYTES_VERIFIED",
		SourceCommit: expectedSource, ManifestDigest: expectedManifestDigest,
		FileCount: len(manifest.Files), BinaryCount: binary.BinaryCount,
	}, nil
}
