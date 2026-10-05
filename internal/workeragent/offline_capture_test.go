package workeragent

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/artifactset"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/contextbundle"
	"github.com/jiying2007/engineering-platform/internal/offline"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
	"github.com/jiying2007/engineering-platform/internal/workerqueue"
	"github.com/jiying2007/engineering-platform/internal/workspace"
)

func captureOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// Synthetic producer records isolate the byte/identity contract. Actual compiled
// Worker/Core/container capture is additionally required in command integration.
func offlineCaptureFixture(t *testing.T) (OfflineCaptureRequest, offline.Permit, offline.Report) {
	t.Helper()
	dir, store := t.TempDir(), t.TempDir()
	captureOK(t, os.Chmod(dir, 0700))
	captureOK(t, os.Chmod(store, 0700))
	profile := sandbox.Profile{Image: sandbox.Hash([]byte("TEST image")), GuardDigest: sandbox.Hash([]byte("TEST guard")), Argv: []string{"/probe"}, Seconds: 5, Outputs: []sandbox.OutputSpec{{Name: "APP.bin", MaxBytes: 64}, {Name: "app.map", MaxBytes: 64}}}
	pd, err := profile.Digest()
	captureOK(t, err)
	a := newTransport(t).a
	a.Task.AllowedActions = []string{offline.Action}
	a.Intent.TaskDigest, err = a.Task.Digest()
	captureOK(t, err)
	a.Input.TaskContractDigest = a.Intent.TaskDigest
	a.Input.ToolProfile = "offline/" + pd
	a.Intent.InputDigest, err = a.Input.Digest()
	captureOK(t, err)
	a.IntentDigest, err = a.Intent.Digest()
	captureOK(t, err)
	validation, err := workerqueue.Validate(a)
	captureOK(t, err)
	manifest := contextbundle.Manifest{SchemaVersion: 1, RunInputDigest: a.Intent.InputDigest, Entries: []contextbundle.Entry{}}
	contextRaw, err := json.Marshal(manifest)
	captureOK(t, err)
	facts := preparation.Facts{Version: 1, IntentDigest: a.IntentDigest, InputDigest: a.Intent.InputDigest, TaskDigest: a.Intent.TaskDigest, ApprovalDigest: sandbox.Hash([]byte("TEST approval")), BaseCommit: a.Task.BaseCommit, TreeCommit: strings.Repeat("b", 40), WorkspaceRecipe: workspace.Recipe, SourceDigest: sandbox.Hash([]byte("TEST source")), ConfigDigest: sandbox.Hash([]byte("TEST config")), BundleDigest: sandbox.Hash(contextRaw), Context: manifest}
	fd, err := canonical.Digest(facts)
	captureOK(t, err)
	now := time.Unix(1700000000, 0).UTC()
	p := offline.Permit{Token: offline.Token{ID: strings.Repeat("c", 64), RunID: a.Input.RunID, WorkerProfile: a.Input.WorkerProfile, ProfileDigest: pd}, Assignment: a, Profile: profile, LeaseUntil: now.Add(time.Minute), Preparation: preparation.Receipt{Kind: preparation.Kind, Admission: workerqueue.Receipt{Token: a.Token, Worker: "TEST-worker", Kind: workerqueue.Validated, Validation: validation, ReceivedAt: now}, Facts: facts, FactsDigest: fd, ReceivedAt: now}}
	cd, err := sandbox.OutputContractDigest(profile.Outputs)
	captureOK(t, err)
	b := &sandbox.BuildOutputs{Contract: profile.Outputs, ContractDigest: cd, State: "COLLECTED", ChildrenReaped: true, Files: []sandbox.OutputFile{}}
	for i, spec := range profile.Outputs {
		data := []byte("TEST build bytes\x00\xff")
		if i == 1 {
			data = []byte{}
		}
		b.Files = append(b.Files, sandbox.OutputFile{Name: spec.Name, Size: len(data), Digest: sandbox.Hash(data), Bytes: data})
	}
	r := offline.Report{Token: p.Token, Result: sandbox.Result{Recipe: sandbox.Recipe, ProfileDigest: pd, ContainerID: strings.Repeat("d", 64), ExitCode: 0, UserID: 1000, Stdout: []byte("TEST stdout"), StdoutDigest: sandbox.Hash([]byte("TEST stdout")), Stderr: []byte{}, StderrDigest: sandbox.Hash(nil), BuildOutputs: b}}
	q := OfflineCaptureRequest{Records: dir, RunID: p.Token.RunID, ExecutionID: p.Token.ID, Destination: filepath.Join(store, "retained.tar")}
	writeOfflineFixture(t, &q, p, r)
	return q, p, r
}

func writeOfflineFixture(t *testing.T, q *OfflineCaptureRequest, p offline.Permit, r offline.Report) {
	t.Helper()
	permitRaw, err := json.Marshal(p)
	captureOK(t, err)
	reportRaw, err := json.Marshal(r)
	captureOK(t, err)
	q.PermitDigest = canonical.BytesDigest(permitRaw)
	q.ReportDigest = canonical.BytesDigest(reportRaw)
	captureOK(t, os.WriteFile(filepath.Join(q.Records, "offline-"+q.ExecutionID+".json"), permitRaw, 0600))
	captureOK(t, os.WriteFile(filepath.Join(q.Records, "offline-"+offlineReportID(q.ExecutionID)+".json"), reportRaw, 0600))
}

func TestOfflineCaptureRestoresOriginalRecordsAndEveryOutput(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "collected", true: "failed-command"}[failed], func(t *testing.T) {
			q, p, r := offlineCaptureFixture(t)
			if failed {
				r.Result.ExitCode = 7
				r.Result.BuildOutputs.State = "NOT_COLLECTED_EXIT_NONZERO"
				r.Result.BuildOutputs.Files = nil
				writeOfflineFixture(t, &q, p, r)
			}
			_, _, before, err := readOfflineCapture(context.Background(), q)
			captureOK(t, err)
			report, err := CaptureOfflineBuildArtifacts(context.Background(), q)
			captureOK(t, err)
			if report.Archive.Members != 2+len(r.Result.BuildOutputs.Files) || report.OutputCount != len(r.Result.BuildOutputs.Files) || report.CommandExitCode != r.Result.ExitCode || report.OutputState != r.Result.BuildOutputs.State || report.CoreObservation != "NOT_OBSERVED" || report.FullRunBackup || report.ExecutionAuthorized || report.ProductionQualified || report.Archive.ProducerSemanticsVerified {
				t.Fatal("incorrect scope/result", report)
			}
			encoded, err := json.Marshal(report)
			captureOK(t, err)
			if bytes.Contains(encoded, []byte(q.Records)) || bytes.Contains(encoded, []byte("TEST stdout")) {
				t.Fatal("private content disclosed")
			}
			captureOK(t, os.RemoveAll(q.Records))
			if _, err := os.Stat(q.Records); !os.IsNotExist(err) {
				t.Fatal("original records survive")
			}
			into := filepath.Join(filepath.Dir(q.Destination), "restored")
			_, err = artifactset.Restore(context.Background(), q.Destination, report.Archive.ArchiveDigest, q.RunID, into)
			captureOK(t, err)
			q.Records = filepath.Join(into, "files")
			_, afterReport, after, err := readOfflineCapture(context.Background(), q)
			captureOK(t, err)
			captureOK(t, afterReport.Result.Validate(p.Profile))
			for name, raw := range before {
				if !bytes.Equal(raw, after[name]) {
					t.Fatal("raw record changed")
				}
			}
			for i, output := range r.Result.BuildOutputs.Files {
				name := filepath.Join(q.Records, fmtOutput(i))
				raw, err := os.ReadFile(name)
				captureOK(t, err)
				st, err := os.Stat(name)
				captureOK(t, err)
				if !bytes.Equal(raw, output.Bytes) || st.Mode().Perm() != 0600 {
					t.Fatal("restored output or permission drift")
				}
			}
			leaks, err := filepath.Glob(filepath.Join(filepath.Dir(q.Destination), ".offline-capture-*"))
			captureOK(t, err)
			if len(leaks) != 0 {
				t.Fatal("staging leaked")
			}
		})
	}
}

func fmtOutput(i int) string { return []string{"output-000.bin", "output-001.bin"}[i] }

func TestOfflineCaptureRejectsMalformedOrUnanchoredRecords(t *testing.T) {
	for _, mode := range []string{"permit-anchor", "report-anchor", "missing-permit", "missing-report", "changed-token", "changed-bytes", "wrong-contract", "not-reaped", "missing-output", "extra-output", "wrong-size", "no-output-contract", "failed-with-files", "duplicate-key", "oversized", "symlink", "hardlink", "directory-alias", "directory-permission", "record-permission", "output-overlap", "output-exists", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			q, p, r := offlineCaptureFixture(t)
			permitName := filepath.Join(q.Records, "offline-"+q.ExecutionID+".json")
			reportName := filepath.Join(q.Records, "offline-"+offlineReportID(q.ExecutionID)+".json")
			ctx := context.Background()
			switch mode {
			case "permit-anchor":
				q.PermitDigest = sandbox.Hash([]byte("other"))
			case "report-anchor":
				q.ReportDigest = sandbox.Hash([]byte("other"))
			case "missing-permit":
				captureOK(t, os.Remove(permitName))
			case "missing-report":
				captureOK(t, os.Remove(reportName))
			case "changed-token":
				r.Token.RecoveryEpoch++
				writeOfflineFixture(t, &q, p, r)
			case "changed-bytes":
				r.Result.BuildOutputs.Files[0].Bytes[0] ^= 1
				writeOfflineFixture(t, &q, p, r)
			case "wrong-contract":
				r.Result.BuildOutputs.Contract = append([]sandbox.OutputSpec(nil), p.Profile.Outputs...)
				r.Result.BuildOutputs.Contract[0].MaxBytes++
				r.Result.BuildOutputs.ContractDigest, _ = sandbox.OutputContractDigest(r.Result.BuildOutputs.Contract)
				writeOfflineFixture(t, &q, p, r)
			case "not-reaped":
				r.Result.BuildOutputs.ChildrenReaped = false
				writeOfflineFixture(t, &q, p, r)
			case "missing-output":
				r.Result.BuildOutputs.Files = r.Result.BuildOutputs.Files[:1]
				writeOfflineFixture(t, &q, p, r)
			case "extra-output":
				r.Result.BuildOutputs.Files = append(r.Result.BuildOutputs.Files, r.Result.BuildOutputs.Files[0])
				writeOfflineFixture(t, &q, p, r)
			case "wrong-size":
				r.Result.BuildOutputs.Files[0].Size++
				writeOfflineFixture(t, &q, p, r)
			case "no-output-contract":
				r.Result.BuildOutputs = nil
				writeOfflineFixture(t, &q, p, r)
			case "failed-with-files":
				r.Result.ExitCode = 7
				writeOfflineFixture(t, &q, p, r)
			case "duplicate-key":
				raw, err := os.ReadFile(reportName)
				captureOK(t, err)
				raw = append([]byte(`{"token":{},`), raw[1:]...)
				captureOK(t, os.WriteFile(reportName, raw, 0600))
				q.ReportDigest = sandbox.Hash(raw)
			case "oversized":
				raw := bytes.Repeat([]byte("x"), (1<<20)+1)
				captureOK(t, os.WriteFile(reportName, raw, 0600))
				q.ReportDigest = sandbox.Hash(raw)
			case "symlink":
				other := reportName + ".original"
				captureOK(t, os.Rename(reportName, other))
				captureOK(t, os.Symlink(other, reportName))
			case "hardlink":
				captureOK(t, os.Link(reportName, reportName+".link"))
			case "directory-alias":
				alias := filepath.Join(filepath.Dir(q.Destination), "alias")
				captureOK(t, os.Symlink(q.Records, alias))
				q.Records = alias
			case "directory-permission":
				captureOK(t, os.Chmod(q.Records, 0777))
			case "record-permission":
				captureOK(t, os.Chmod(reportName, 0644))
			case "output-overlap":
				q.Destination = filepath.Join(q.Records, "new.tar")
			case "output-exists":
				captureOK(t, os.WriteFile(q.Destination, []byte("DO NOT OVERWRITE"), 0600))
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if _, err := CaptureOfflineBuildArtifacts(ctx, q); err == nil {
				t.Fatal("accepted", mode)
			}
			if mode == "output-exists" {
				raw, err := os.ReadFile(q.Destination)
				captureOK(t, err)
				if string(raw) != "DO NOT OVERWRITE" {
					t.Fatal("overwritten")
				}
			} else if _, err := os.Lstat(q.Destination); !os.IsNotExist(err) {
				t.Fatal("rejected preflight published output", err)
			}
		})
	}
}

func TestOfflineCaptureConcurrentPublicationDoesNotOverwrite(t *testing.T) {
	q, _, _ := offlineCaptureFixture(t)
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := CaptureOfflineBuildArtifacts(context.Background(), q); results <- err }()
	}
	wins := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatal("unexpected publications", wins)
	}
	bytes, err := os.ReadFile(q.Destination)
	captureOK(t, err)
	_, err = artifactset.Verify(context.Background(), q.Destination, sandbox.Hash(bytes), q.RunID)
	captureOK(t, err)
}
