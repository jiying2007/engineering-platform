//go:build linux

package distribution

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestInstalledReleaseReadbackRequiresExternalManifestIdentity(t *testing.T) {
	from, sha := installFixture(t)
	dest := filepath.Join(t.TempDir(), "installed")
	installed, err := Install(context.Background(), from, dest, sha)
	installOK(t, err)
	installOK(t, os.RemoveAll(from))
	result, err := VerifyInstalledRelease(context.Background(), dest, sha, installed.ManifestDigest)
	installOK(t, err)
	if result.Status != "INSTALLATION_RECEIPT_BYTES_VERIFIED" || result.SourceCommit != sha ||
		result.ManifestDigest != installed.ManifestDigest || result.BinaryCount != len(Names()) ||
		result.ServicesStarted || result.DatabaseChanged || result.ExecutionAuthorized || result.ProductionQualified {
		t.Fatal("readback promoted authority or lost identity", result)
	}
	for _, mutate := range []struct {
		name   string
		source string
		digest string
	}{
		{"wrong-source", strings.Repeat("0", 40), installed.ManifestDigest},
		{"wrong-digest", sha, "sha256:" + strings.Repeat("0", 64)},
		{"bare-digest", sha, strings.TrimPrefix(installed.ManifestDigest, "sha256:")},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			if _, err := VerifyInstalledRelease(context.Background(), dest, mutate.source, mutate.digest); err == nil {
				t.Fatal("untrusted release identity accepted")
			}
		})
	}
}

func TestInstalledReleaseReadbackSupportsRetainedOlderTemplateSet(t *testing.T) {
	from, sha := installFixture(t)
	dest := filepath.Join(t.TempDir(), "installed")
	installed, err := Install(context.Background(), from, dest, sha)
	installOK(t, err)
	installOK(t, os.RemoveAll(from))
	manifestPath := filepath.Join(dest, "installation-manifest.json")
	raw, err := os.ReadFile(manifestPath)
	installOK(t, err)
	var manifest InstallationManifest
	installOK(t, json.Unmarshal(raw, &manifest))
	oldPath := "templates/control.env.example"
	legacyPath := "templates/legacy-control.env"
	var changed bool
	for i := range manifest.Files {
		if manifest.Files[i].Path == oldPath {
			installOK(t, os.Rename(filepath.Join(dest, filepath.FromSlash(oldPath)), filepath.Join(dest, filepath.FromSlash(legacyPath))))
			manifest.Files[i].Path = legacyPath
			changed = true
		}
	}
	if !changed {
		t.Fatal("fixture template missing")
	}
	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
	raw, err = json.MarshalIndent(manifest, "", "  ")
	installOK(t, err)
	raw = append(raw, '\n')
	installOK(t, os.Chmod(manifestPath, 0644))
	installOK(t, os.WriteFile(manifestPath, raw, 0444))
	installOK(t, os.Chmod(manifestPath, 0444))
	retained := rawDigest(raw)
	if retained == installed.ManifestDigest {
		t.Fatal("synthetic older release did not change manifest identity")
	}
	if _, err := VerifyInstallation(context.Background(), dest, sha); err == nil {
		t.Fatal("current-source verifier accepted a different retained template set")
	}
	if _, err := VerifyInstalledRelease(context.Background(), dest, sha, retained); err != nil {
		t.Fatal("retained cross-version manifest could not be independently read back", err)
	}
}

func TestInstalledReleaseReadbackRejectsTreeMutationAndAliases(t *testing.T) {
	from, sha := installFixture(t)
	dest := filepath.Join(t.TempDir(), "installed")
	installed, err := Install(context.Background(), from, dest, sha)
	installOK(t, err)
	installOK(t, os.RemoveAll(from))
	template := filepath.Join(dest, "templates", "control.env.example")
	original, err := os.ReadFile(template)
	installOK(t, err)
	for _, mode := range []string{"extra", "bytes", "hardlink", "manifest-mode", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			var cleanup func()
			switch mode {
			case "extra":
				path := filepath.Join(dest, "unexpected")
				installOK(t, os.WriteFile(path, []byte("x"), 0444))
				cleanup = func() { _ = os.Remove(path) }
			case "bytes":
				installOK(t, os.Chmod(template, 0644))
				installOK(t, os.WriteFile(template, []byte("forged"), 0444))
				installOK(t, os.Chmod(template, 0444))
				cleanup = func() {
					_ = os.Chmod(template, 0644)
					_ = os.WriteFile(template, original, 0444)
					_ = os.Chmod(template, 0444)
				}
			case "hardlink":
				alias := filepath.Join(t.TempDir(), "alias")
				installOK(t, os.Link(template, alias))
				cleanup = func() { _ = os.Remove(alias) }
			case "manifest-mode":
				path := filepath.Join(dest, "installation-manifest.json")
				installOK(t, os.Chmod(path, 0644))
				cleanup = func() { _ = os.Chmod(path, 0444) }
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
				cleanup = func() {}
			}
			defer cleanup()
			if _, err := VerifyInstalledRelease(ctx, dest, sha, installed.ManifestDigest); err == nil {
				t.Fatal("mutated installed release accepted", mode)
			}
		})
	}
	if _, err := VerifyInstalledRelease(context.Background(), dest, sha, installed.ManifestDigest); err != nil {
		t.Fatal(err)
	}
}
