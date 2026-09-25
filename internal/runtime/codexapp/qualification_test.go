package codexapp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	runtimeprovider "github.com/jiying2007/engineering-platform/internal/runtime"
)

func writeSchemaFixture(t *testing.T, experimental bool) string {
	t.Helper()
	root := t.TempDir()
	thread := `{"type":"object","properties":{"approvalPolicy":{},"sandbox":{}`
	if experimental {
		thread += `,"permissions":{}`
	}
	thread += `}}`
	if err := os.WriteFile(filepath.Join(root, "ThreadStartParams.json"), []byte(thread), 0o600); err != nil {
		t.Fatal(err)
	}
	enums := `{"sandbox":["read-only","workspace-write","danger-full-access"],"approval":["never","on-request"]}`
	if err := os.WriteFile(filepath.Join(root, "Enums.json"), []byte(enums), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestValidateSchemaTreeSeparatesStableAndExperimental(t *testing.T) {
	stable := writeSchemaFixture(t, false)
	digest, err := validateSchemaTree(stable, false)
	if err != nil || !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("stable schema rejected: %s %v", digest, err)
	}
	if _, err := validateSchemaTree(stable, true); err == nil {
		t.Fatal("stable schema accepted as experimental")
	}
	experimental := writeSchemaFixture(t, true)
	if _, err := validateSchemaTree(experimental, true); err != nil {
		t.Fatal(err)
	}
	if _, err := validateSchemaTree(experimental, false); err == nil {
		t.Fatal("experimental permissions leaked into stable surface")
	}
}

func TestPinnedProviderRejectsExecutableByteDrift(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "codex")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	digest, err := executableDigest(executable)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewPinnedProvider(executable, digest)
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(root, "work")
	home := filepath.Join(root, "home")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Command(context.Background(), runtimeprovider.LaunchSpec{Dir: work, Env: []string{"HOME=" + home}}); err != nil {
		t.Fatalf("unchanged executable rejected: %v", err)
	}
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Command(context.Background(), runtimeprovider.LaunchSpec{Dir: work, Env: []string{"HOME=" + home}}); err == nil {
		t.Fatal("changed executable bytes accepted")
	}
}

func TestQualificationReceiptMarshalHasNoHostLocator(t *testing.T) {
	data, err := MarshalQualification(QualificationReceipt{
		SchemaVersion:              1,
		CLI:                         "codex-cli",
		Version:                     QualifiedCodexVersion,
		ReleaseTag:                  QualifiedCodexReleaseTag,
		ReleaseCommit:               QualifiedCodexReleaseCommit,
		BinaryDigest:                "sha256:" + strings.Repeat("a", 64),
		StableSchemaDigest:          "sha256:" + strings.Repeat("b", 64),
		ExperimentalSchemaDigest:    "sha256:" + strings.Repeat("c", 64),
		Transport:                   "stdio",
		FreshProcess:                true,
		InitializePassed:            true,
		ThreadStartPassed:           true,
		ThreadStartModel:            "gpt-5.6-sol",
		StableSchemaContractChecked: true,
		ExperimentalSurfaceChecked:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), t.TempDir()) {
		t.Fatal("qualification receipt contains machine locator")
	}
}
