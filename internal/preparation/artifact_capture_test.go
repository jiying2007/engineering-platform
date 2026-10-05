//go:build linux

package preparation_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/artifactset"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/workeragent"
)

func captureFixture(t *testing.T, mode string) (workeragent.ExecutionCaptureRequest, *completedTransport) {
	t.Helper()
	q, c, _ := readbackFixture(t, mode)
	if mode != "success" {
		q.Archive = filepath.Join(c.root, "artifacts", q.ExecutionID+".source-checkpoint.tar")
	}
	if mode != "ignored" {
		q.Bundle = filepath.Join(c.root, "artifacts", q.ExecutionID+".bundle")
	}
	store := t.TempDir()
	mustCheckpoint(t, os.Chmod(store, 0700))
	return workeragent.ExecutionCaptureRequest{Readback: q, ContextDirectory: filepath.Join(c.root, "bundles", strings.TrimPrefix(c.permit.Assignment.Intent.InputDigest, "sha256:")), Destination: filepath.Join(store, "execution.tar")}, c
}

func TestFrozenExecutionCaptureRestoresContextAndProducerRecords(t *testing.T) {
	for _, mode := range []string{"success", "ignored", "report-lost", "checkpoint-lost"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			q, c := captureFixture(t, mode)
			before, err := workeragent.InspectPostTurn(ctx, q.Readback)
			mustCheckpoint(t, err)
			calls, _ := canonical.Digest(c.calls)
			r, err := workeragent.CaptureExecutionArtifacts(ctx, q)
			mustCheckpoint(t, err)
			if r.FullRunBackup || r.ExecutionAuthorized || r.ProductionQualified || r.Archive.ProducerSemanticsVerified || r.Archive.ExecutionAuthorized || r.Archive.ProductionQualified {
				t.Fatal("invented authority or coverage", r)
			}
			if r.Selection != "EXECUTION_RECORDS_AND_ALL_FROZEN_CONTEXT" || r.RecordCount != len(before.Files) || r.ContextCount != len(c.permit.Assignment.Input.ContextRefs) || r.ContextManifestDigest != c.permit.Preparation.Facts.BundleDigest || r.PermitDigest != q.Readback.PermitDigest {
				t.Fatal("wrong derived coverage", r)
			}
			// Same source identities/bytes produce the same archive regardless of new
			// temporary staging names or output path. No timestamp goes into the plan.
			copyQ := q
			copyQ.Destination = filepath.Join(filepath.Dir(q.Destination), "second.tar")
			other, err := workeragent.CaptureExecutionArtifacts(ctx, copyQ)
			mustCheckpoint(t, err)
			if r.Archive.ArchiveDigest != other.Archive.ArchiveDigest {
				t.Fatal("nondeterministic producer selection")
			}
			encoded, err := json.Marshal(r)
			mustCheckpoint(t, err)
			for _, private := range []string{c.root, c.work, "approved context", "fixture_only", "completed model source preserved"} {
				if strings.Contains(string(encoded), private) {
					t.Fatal("raw private content in report")
				}
			}
			// Remove ALL original prepared state: context, source, HOME, records, raw
			// result bundle and source archive. Only the retained archive can save this.
			mustCheckpoint(t, filepath.WalkDir(c.root, func(path string, d os.DirEntry, e error) error {
				if e == nil && d.IsDir() {
					return os.Chmod(path, 0700)
				}
				return e
			}))
			mustCheckpoint(t, os.RemoveAll(c.root))
			if _, err := os.Stat(c.root); !os.IsNotExist(err) {
				t.Fatal("original survived")
			}
			into := filepath.Join(filepath.Dir(q.Destination), "restored")
			_, err = artifactset.Restore(ctx, q.Destination, r.Archive.ArchiveDigest, q.Readback.RunID, into)
			mustCheckpoint(t, err)
			files := filepath.Join(into, "files")
			read := q.Readback
			read.Records = files
			if read.Archive != "" {
				read.Archive = filepath.Join(files, "source-checkpoint.tar")
			}
			if read.Bundle != "" {
				read.Bundle = filepath.Join(files, "result.bundle")
			}
			after, err := workeragent.InspectPostTurn(ctx, read)
			mustCheckpoint(t, err)
			if !reflect.DeepEqual(before.Files, after.Files) || before.LocalObservation != after.LocalObservation || before.TranscriptDigest != after.TranscriptDigest {
				t.Fatal("restored producer semantics drift")
			}
			manifest, err := os.ReadFile(filepath.Join(files, "context-manifest.json"))
			mustCheckpoint(t, err)
			if canonical.BytesDigest(manifest) != c.permit.Preparation.Facts.BundleDigest {
				t.Fatal("missing original context manifest")
			}
			for _, entry := range c.permit.Preparation.Facts.Context.Entries {
				raw, err := os.ReadFile(filepath.Join(files, entry.File))
				mustCheckpoint(t, err)
				if int64(len(raw)) != entry.Size || canonical.BytesDigest(raw) != entry.Ref.Digest {
					t.Fatal("frozen context byte mismatch")
				}
			}
			afterCalls, _ := canonical.Digest(c.calls)
			if calls != afterCalls {
				t.Fatal("capture or restore invoked transport")
			}
			found, err := filepath.Glob(filepath.Join(filepath.Dir(q.Destination), ".execution-capture-*"))
			mustCheckpoint(t, err)
			if len(found) != 0 {
				t.Fatal("staging leaked on success")
			}
		})
	}
}

func TestFrozenExecutionCaptureRejectsOmissionsAndUnsafeInputs(t *testing.T) {
	for _, bad := range []string{"missing-context", "extra-context", "changed-context", "context-link", "manifest-changed", "missing-source", "missing-bundle", "wrong-anchor", "output-in-context", "output-in-records", "output-exists", "context-alias", "world-writable-context", "hard-linked-context", "cancelled"} {
		t.Run(bad, func(t *testing.T) {
			q, c := captureFixture(t, "report-lost")
			ctx := context.Background()
			file := filepath.Join(q.ContextDirectory, c.permit.Preparation.Facts.Context.Entries[0].File)
			// Test-only mutation: not a producer API and never uploaded.
			mustCheckpoint(t, os.Chmod(q.ContextDirectory, 0700))
			switch bad {
			case "missing-context":
				mustCheckpoint(t, os.Remove(file))
			case "extra-context":
				mustCheckpoint(t, os.WriteFile(filepath.Join(q.ContextDirectory, "undeclared"), []byte("TEST private extra"), 0600))
			case "changed-context":
				mustCheckpoint(t, os.Chmod(file, 0600))
				mustCheckpoint(t, os.WriteFile(file, []byte("changed"), 0600))
			case "context-link":
				mustCheckpoint(t, os.Rename(file, file+"-target"))
				mustCheckpoint(t, os.Symlink(file+"-target", file))
			case "manifest-changed":
				m := filepath.Join(q.ContextDirectory, "manifest.json")
				mustCheckpoint(t, os.Chmod(m, 0600))
				mustCheckpoint(t, os.WriteFile(m, []byte("{}"), 0600))
			case "missing-source":
				q.Readback.Archive = ""
			case "missing-bundle":
				q.Readback.Bundle = ""
			case "wrong-anchor":
				q.Readback.PermitDigest = "sha256:" + strings.Repeat("0", 64)
			case "output-in-context":
				q.Destination = filepath.Join(q.ContextDirectory, "output.tar")
			case "output-in-records":
				q.Destination = filepath.Join(q.Readback.Records, "output.tar")
			case "output-exists":
				mustCheckpoint(t, os.WriteFile(q.Destination, []byte("KEEP"), 0600))
			case "context-alias":
				alias := filepath.Join(filepath.Dir(q.Destination), "alias")
				mustCheckpoint(t, os.Symlink(q.ContextDirectory, alias))
				q.ContextDirectory = alias
			case "world-writable-context":
				mustCheckpoint(t, os.Chmod(file, 0666))
			case "hard-linked-context":
				mustCheckpoint(t, os.Link(file, filepath.Join(filepath.Dir(q.Destination), "extra-link")))
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			calls, _ := canonical.Digest(c.calls)
			if _, err := workeragent.CaptureExecutionArtifacts(ctx, q); err == nil {
				t.Fatal("invalid capture accepted")
			}
			afterCalls, _ := canonical.Digest(c.calls)
			if calls != afterCalls {
				t.Fatal("failed capture contacted Core")
			}
			if bad == "output-exists" {
				raw, err := os.ReadFile(q.Destination)
				mustCheckpoint(t, err)
				if string(raw) != "KEEP" {
					t.Fatal("existing output overwritten")
				}
			} else if _, err := os.Lstat(q.Destination); !os.IsNotExist(err) {
				t.Fatal("failed preflight published an archive")
			}
			found, err := filepath.Glob(filepath.Join(filepath.Dir(q.Destination), ".execution-capture-*"))
			mustCheckpoint(t, err)
			if len(found) != 0 {
				t.Fatal("staging remains")
			}
		})
	}
}
