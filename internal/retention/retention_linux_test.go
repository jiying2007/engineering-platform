//go:build linux

package retention

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/artifactset"
	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func retentionMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func retentionFixture(t *testing.T) (Config, string, string, artifactset.Report) {
	t.Helper()
	source := t.TempDir()
	primary := t.TempDir()
	replica := t.TempDir()
	for _, dir := range []string{source, primary, replica} {
		retentionMust(t, os.Chmod(dir, 0700))
	}
	payload := []byte("TEST-ONLY retained bytes\x00\xff")
	member := filepath.Join(source, "result.bundle")
	retentionMust(t, os.WriteFile(member, payload, 0600))
	plan := artifactset.Plan{
		Version: 1,
		Subject: artifactset.Subject{
			RunID: "retention-run", ExecutionID: strings.Repeat("a", 64),
			TaskDigest:  canonical.BytesDigest([]byte("task")),
			InputDigest: canonical.BytesDigest([]byte("input")),
			BaseCommit:  strings.Repeat("b", 40),
		},
		Members: []artifactset.Input{{
			Entry: artifactset.Entry{ID: "result.bundle", Kind: "git-bundle", Size: int64(len(payload)), Digest: canonical.BytesDigest(payload)},
			Path:  member,
		}},
	}
	planRaw, err := json.Marshal(plan)
	retentionMust(t, err)
	planPath := filepath.Join(source, "plan.json")
	retentionMust(t, os.WriteFile(planPath, planRaw, 0600))
	archive := filepath.Join(primary, "run-a.tar")
	packed, err := artifactset.Pack(context.Background(), planPath, canonical.BytesDigest(planRaw), archive)
	retentionMust(t, err)
	config := Config{
		Version: 1, PrimaryRoot: primary, ReplicaRoot: replica,
		MaxTotalBytes: packed.ArchiveSize + 4096,
		Items:         []Item{{Archive: "run-a.tar", RunID: plan.Subject.RunID, ArchiveDigest: packed.ArchiveDigest}},
	}
	configRaw, err := json.Marshal(config)
	retentionMust(t, err)
	configPath := filepath.Join(source, "retention.json")
	retentionMust(t, os.WriteFile(configPath, configRaw, 0600))
	return config, configPath, canonical.BytesDigest(configRaw), packed
}

func TestReplicateCopiesVerifiesAndIsIdempotent(t *testing.T) {
	config, path, digest, packed := retentionFixture(t)
	first, err := Replicate(context.Background(), path, digest)
	retentionMust(t, err)
	if first.Status != "RETENTION_REPLICA_BYTES_VERIFIED" || first.ItemCount != 1 ||
		first.CopiedCount != 1 || first.ExistingCount != 0 || first.VerifiedBytes != packed.ArchiveSize ||
		first.DeletionPerformed || first.ExecutionAuthorized || first.ProductionQualified || first.SecondSiteQualified {
		t.Fatal("retention report promoted authority or lost facts", first)
	}
	replica, err := artifactset.Verify(context.Background(), filepath.Join(config.ReplicaRoot, "run-a.tar"), packed.ArchiveDigest, config.Items[0].RunID)
	retentionMust(t, err)
	if !sameArchive(replica, packed) {
		t.Fatal("replica differs from primary")
	}
	second, err := Replicate(context.Background(), path, digest)
	retentionMust(t, err)
	if second.CopiedCount != 0 || second.ExistingCount != 1 || second.VerifiedBytes != packed.ArchiveSize {
		t.Fatal("idempotent readback changed", second)
	}
	if _, err := os.Stat(filepath.Join(config.PrimaryRoot, "run-a.tar")); err != nil {
		t.Fatal("retention deleted primary", err)
	}
	entries, err := os.ReadDir(config.ReplicaRoot)
	retentionMust(t, err)
	if len(entries) != 1 || entries[0].Name() != "run-a.tar" {
		t.Fatal("temporary or extra replica files remain", entries)
	}
}

func TestReplicateRejectsUntrustedConfigAndUnsafeStateBeforeCopy(t *testing.T) {
	for _, mode := range []string{"wrong-config-digest", "budget", "overlap", "existing-mismatch", "primary-drift", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			config, path, digest, packed := retentionFixture(t)
			ctx := context.Background()
			switch mode {
			case "wrong-config-digest":
				digest = "sha256:" + strings.Repeat("0", 64)
			case "budget":
				config.MaxTotalBytes = packed.ArchiveSize - 1
			case "overlap":
				nested := filepath.Join(config.PrimaryRoot, "replica")
				retentionMust(t, os.Mkdir(nested, 0700))
				config.ReplicaRoot = nested
			case "existing-mismatch":
				retentionMust(t, os.WriteFile(filepath.Join(config.ReplicaRoot, "run-a.tar"), []byte("wrong"), 0600))
			case "primary-drift":
				primary := filepath.Join(config.PrimaryRoot, "run-a.tar")
				retentionMust(t, os.WriteFile(primary, []byte("changed"), 0600))
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if mode == "budget" || mode == "overlap" {
				raw, err := json.Marshal(config)
				retentionMust(t, err)
				retentionMust(t, os.WriteFile(path, raw, 0600))
				digest = canonical.BytesDigest(raw)
			}
			if _, err := Replicate(ctx, path, digest); err == nil {
				t.Fatal("unsafe retention state accepted", mode)
			}
			if mode != "existing-mismatch" && mode != "overlap" {
				if _, err := os.Lstat(filepath.Join(config.ReplicaRoot, "run-a.tar")); !os.IsNotExist(err) {
					t.Fatal("failed retention published replica", mode, err)
				}
			}
		})
	}
}

func TestRetentionConfigRequiresSortedBoundedExplicitItems(t *testing.T) {
	config, _, _, packed := retentionFixture(t)
	base := config.Items[0]
	tests := []Config{
		{Version: 1, PrimaryRoot: config.PrimaryRoot, ReplicaRoot: config.ReplicaRoot, MaxTotalBytes: packed.ArchiveSize, Items: nil},
		{Version: 2, PrimaryRoot: config.PrimaryRoot, ReplicaRoot: config.ReplicaRoot, MaxTotalBytes: packed.ArchiveSize, Items: []Item{base}},
		{Version: 1, PrimaryRoot: config.PrimaryRoot, ReplicaRoot: config.ReplicaRoot, MaxTotalBytes: MaxSweepBytes + 1, Items: []Item{base}},
		{Version: 1, PrimaryRoot: config.PrimaryRoot, ReplicaRoot: config.ReplicaRoot, MaxTotalBytes: packed.ArchiveSize, Items: []Item{base, base}},
	}
	badLeaf := config
	badLeaf.Items = []Item{{Archive: "../escape", RunID: base.RunID, ArchiveDigest: base.ArchiveDigest}}
	tests = append(tests, badLeaf)
	for i, candidate := range tests {
		if validateConfig(candidate) == nil {
			t.Fatal("invalid config accepted", i)
		}
	}
}
