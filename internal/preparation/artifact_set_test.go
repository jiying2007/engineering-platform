//go:build linux

package preparation_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/artifactset"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/workeragent"
)

// Uses the actual Worker/Preparer/process-scope and its on-disk records. Only
// the model/Core transport is TEST-only. No source or credential upload.
func TestActualWorkerArtifactSetRestoresAfterOriginalRemoval(t *testing.T) {
	for _, mode := range []string{"report-lost", "ignored"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			q, c, _ := readbackFixture(t, mode)
			q.Archive = filepath.Join(c.root, "artifacts", q.ExecutionID+".source-checkpoint.tar")
			if mode != "ignored" {
				q.Bundle = filepath.Join(c.root, "artifacts", q.ExecutionID+".bundle")
			}
			before, err := workeragent.InspectPostTurn(ctx, q)
			mustCheckpoint(t, err)
			p := artifactset.Plan{Version: 1, Subject: artifactset.Subject{RunID: q.RunID, ExecutionID: q.ExecutionID, TaskDigest: c.permit.Assignment.Intent.TaskDigest, InputDigest: c.permit.Assignment.Intent.InputDigest, BaseCommit: c.permit.Preparation.Facts.BaseCommit}}
			for _, f := range before.Files {
				p.Members = append(p.Members, artifactset.Input{Entry: artifactset.Entry{ID: f.Name, Kind: "runtime-record", Size: int64(f.Size), Digest: f.Digest}, Path: filepath.Join(q.Records, f.Name)})
			}
			appendArtifact := func(id, kind, path string) {
				b, e := os.ReadFile(path)
				mustCheckpoint(t, e)
				p.Members = append(p.Members, artifactset.Input{Entry: artifactset.Entry{ID: id, Kind: kind, Size: int64(len(b)), Digest: canonical.BytesDigest(b)}, Path: path})
			}
			appendArtifact("source-checkpoint.tar", "source", q.Archive)
			if q.Bundle != "" {
				appendArtifact("result.bundle", "git-bundle", q.Bundle)
			}
			sort.Slice(p.Members, func(i, j int) bool { return p.Members[i].ID < p.Members[j].ID })
			raw, err := json.Marshal(p)
			mustCheckpoint(t, err)
			store := t.TempDir()
			mustCheckpoint(t, os.Chmod(store, 0700))
			plan := filepath.Join(store, "plan.json")
			mustCheckpoint(t, os.WriteFile(plan, raw, 0600))
			archive := filepath.Join(store, "private-set.tar")
			retained, err := artifactset.Pack(ctx, plan, canonical.BytesDigest(raw), archive)
			mustCheckpoint(t, err)
			calls, _ := canonical.Digest(c.calls)
			// Delete this test's original workspace/HOME/record directory and original
			// byte artifacts. A metadata pointer or surviving input cannot save the test.
			mustCheckpoint(t, os.RemoveAll(q.Records))
			mustCheckpoint(t, os.Remove(q.Archive))
			if q.Bundle != "" {
				mustCheckpoint(t, os.Remove(q.Bundle))
			}
			mustCheckpoint(t, os.Remove(plan))
			target := filepath.Join(store, "restored")
			restored, err := artifactset.Restore(ctx, archive, retained.ArchiveDigest, q.RunID, target)
			mustCheckpoint(t, err)
			if restored.ExecutionAuthorized || restored.ProductionQualified || restored.ProducerSemanticsVerified {
				t.Fatal("invented authority")
			}
			q.Records = filepath.Join(target, "files")
			q.Archive = filepath.Join(q.Records, "source-checkpoint.tar")
			if q.Bundle != "" {
				q.Bundle = filepath.Join(q.Records, "result.bundle")
			}
			after, err := workeragent.InspectPostTurn(ctx, q)
			mustCheckpoint(t, err)
			if after.LocalObservation != before.LocalObservation || after.TranscriptDigest != before.TranscriptDigest || after.ExpectedResultDigest != before.ExpectedResultDigest || after.SourceBytes != before.SourceBytes || after.BundleBytes != before.BundleBytes || after.CheckpointDigest != before.CheckpointDigest {
				t.Fatal("restored semantic readback drift")
			}
			finalCalls, _ := canonical.Digest(c.calls)
			if finalCalls != calls {
				t.Fatal("archive/restore called Core")
			}
		})
	}
}
