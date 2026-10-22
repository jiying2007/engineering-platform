package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/api"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/production"
	"github.com/jiying2007/engineering-platform/internal/review"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

type terminalCLIStore struct {
	*store.Memory
	forgeEvidence bool
}

func (s *terminalCLIStore) GetEvidenceContext(ctx context.Context, id string) (core.EvidenceRef, error) {
	item, err := s.Memory.GetEvidenceContext(ctx, id)
	if s.forgeEvidence {
		item.Issuer = "SECRET-forged-issuer"
	}
	return item, err
}

// Synthetic fixture insertion is confined to tests. The executable below uses
// actual TLS, access.Read admission, immutable lookup routes and Core stores.
// The reviewer identity is a test string; it never grants human acceptance.
func terminalCLIHistory(t *testing.T) (*terminalCLIStore, production.TerminalReadbackRequest) {
	t.Helper()
	backend := &terminalCLIStore{Memory: store.NewMemory()}
	ctx := context.Background()
	raw, err := os.ReadFile("../../examples/production/terminal-maintenance/task.json.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	var template struct {
		Contract core.TaskContract `json:"contract"`
		Material struct {
			Repository         string   `json:"repository"`
			TargetID           string   `json:"target_id"`
			AcceptanceCriteria []string `json:"acceptance_criteria"`
		} `json:"material"`
		Plan verification.Plan `json:"verification_plan"`
	}
	if err := json.Unmarshal(raw, &template); err != nil {
		t.Fatal(err)
	}
	task, plan := template.Contract, template.Plan
	task.Repository, task.TargetID, task.AcceptanceCriteria = template.Material.Repository, template.Material.TargetID, template.Material.AcceptanceCriteria
	task.BaseCommit, task.Revision, task.VerificationPlanID = strings.Repeat("a", 40), 1, plan.ID
	task.VerificationPlanDigest, _ = plan.Digest()
	taskDigest, _ := task.Digest()
	if err := backend.CreateTask(task, plan); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(-time.Minute)
	input := core.RunInputManifest{RunID: "terminal-cli-run", TaskContractDigest: taskDigest, RuntimeProfile: "test-runtime", ToolProfile: "test-tool", WorkerProfile: "test-worker", PolicyProfile: "test-policy"}
	inputDigest, _ := input.Digest()
	value := run.Run{ID: input.RunID, TaskContractDigest: taskDigest, RunInputManifestDigest: inputDigest, State: run.Completed, Version: 2, CurrentEpoch: 1, CurrentAttemptID: "test-attempt", ControlOwner: "RUNTIME"}
	if err := backend.CreateExecution(value, session.Session{RunID: input.RunID, ExecutionEpoch: 1, Owner: session.Runtime}, input); err != nil {
		t.Fatal(err)
	}
	work := core.WorkItem{ID: task.WorkItemID, HumanOwner: "urn:engineering-platform:test:owner", State: core.WorkVerifying, ActiveTaskContractDigest: taskDigest, ActiveRunID: input.RunID, Version: 1, CreatedAt: now.Add(-time.Hour)}
	if err := backend.CreateWorkContext(ctx, work); err != nil {
		t.Fatal(err)
	}
	delivery := core.DeliveryReceipt{ID: "terminal-cli-delivery", WorkItemID: work.ID, TaskContractDigest: taskDigest, RunID: input.RunID, TargetID: task.TargetID, BaseCommit: task.BaseCommit, ResultCommit: strings.Repeat("b", 40), Artifacts: []core.ArtifactRef{{ID: "test-artifact", Digest: canonical.BytesDigest([]byte("test artifact"))}}, CreatedAt: now}
	delivery.SubjectDigest, _ = delivery.CalculateSubjectDigest()
	if err := backend.CreateDeliveryContext(ctx, delivery); err != nil {
		t.Fatal(err)
	}
	var evidence []core.EvidenceRef
	for _, criterion := range plan.Criteria {
		req := criterion.Requirements[0]
		item := core.EvidenceRef{ID: "evidence-" + req.ID, DeliveryReceiptID: delivery.ID, RequirementID: req.ID, SubjectDigest: delivery.SubjectDigest, Issuer: req.Issuer, Procedure: req.Procedure, Result: "PASS", Applicable: true, ArtifactRefs: []string{"test-artifact"}}
		if err := backend.CreateEvidenceContext(ctx, item); err != nil {
			t.Fatal(err)
		}
		evidence = append(evidence, item)
	}
	verified := verification.Evaluate(plan, delivery.SubjectDigest, evidence)
	verified.ID, verified.DeliveryReceiptID, verified.VerificationPlanDigest = "terminal-cli-verification", delivery.ID, task.VerificationPlanDigest
	verified.Verifier, verified.CreatedAt = "urn:engineering-platform:test:verifier", now.Add(time.Second)
	if err := backend.CreateVerificationContext(ctx, verified); err != nil {
		t.Fatal(err)
	}
	reviewed := review.Report{ID: "terminal-cli-review", DeliveryReceiptID: delivery.ID, VerificationReportID: verified.ID, TaskContractDigest: taskDigest, SubjectDigest: delivery.SubjectDigest, Reviewer: "urn:engineering-platform:test:reviewer", Result: review.ResultPass, CreatedAt: now.Add(2 * time.Second)}
	work.State = core.WorkReviewing
	if err := backend.CreateReviewAndUpdateWorkContext(ctx, reviewed, 1, work); err != nil {
		t.Fatal(err)
	}
	closure := core.ClosureReceipt{ID: "terminal-cli-closure", WorkItemID: work.ID, TaskContractDigest: taskDigest, RunID: input.RunID, DeliveryReceiptID: delivery.ID, VerificationReportID: verified.ID, ReviewReportID: reviewed.ID, SubjectDigest: delivery.SubjectDigest, Result: "CLOSED", CreatedAt: now.Add(3 * time.Second)}
	work.State = core.WorkClosed
	if err := backend.CreateClosureAndUpdateWorkContext(ctx, closure, 2, work); err != nil {
		t.Fatal(err)
	}
	newTask := task
	newTask.Revision = 2
	newTask.BaseCommit = strings.Repeat("c", 40)
	if err := backend.CreateTask(newTask, plan); err != nil {
		t.Fatal(err)
	}
	newDigest, _ := newTask.Digest()
	work.State, work.ActiveTaskContractDigest, work.ActiveRunID = core.WorkExecuting, newDigest, "newer-run"
	if err := backend.UpdateWorkContext(ctx, work.ID, 3, work); err != nil {
		t.Fatal(err)
	}
	return backend, production.TerminalReadbackRequest{ClosureID: closure.ID, RunID: input.RunID, SubjectDigest: delivery.SubjectDigest}
}

func TestProductionTerminalReadbackCLIThroughAuthenticatedCore(t *testing.T) {
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	bin := filepath.Join(t.TempDir(), "eng")
	build := exec.CommandContext(ctx, goBinary, "build", "-trimpath", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build actual CLI: %v %s", err, out)
	}
	for _, scenario := range []string{"historical-core", "denied-reader", "wrong-subject", "forged-evidence", "invalid-id", "qualification-override", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			backend, request := terminalCLIHistory(t)
			backend.forgeEvidence = scenario == "forged-evidence"
			readerSubject, deniedSubject := "urn:engineering-platform:test:terminal-reader", "urn:engineering-platform:test:denied-reader"
			policy, err := access.New(access.Document{Version: 1, Principals: []access.PrincipalSpec{
				{Subject: readerSubject, Scope: "platform", Capabilities: []string{access.Read}},
				{Subject: deniedSubject, Scope: "platform", Capabilities: []string{access.WorkCreate}},
			}})
			if err != nil {
				t.Fatal(err)
			}
			handler, err := api.NewAuthenticatedHandler(backend, nil, policy, api.AuthenticatedOptions{})
			if err != nil {
				t.Fatal(err)
			}
			pki := testsupport.NewPKI(t)
			var calls atomic.Int32
			started := make(chan struct{}, 1)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || r.ContentLength > 0 {
					t.Error("terminal readback issued a mutation or body")
				}
				if scenario == "canceled" {
					started <- struct{}{}
					<-r.Context().Done()
					return
				}
				handler.ServeHTTP(w, r)
			}))
			server.TLS = pki.ServerTLS()
			server.StartTLS()
			defer server.Close()
			subject := readerSubject
			if scenario == "denied-reader" {
				subject = deniedSubject
			}
			cert := pki.ClientCertificate(t, subject)
			key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if err := os.Chmod(dir, 0700); err != nil {
				t.Fatal(err)
			}
			for name, raw := range map[string][]byte{"ca.pem": pki.CAPEM, "client.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}), "client.key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})} {
				if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "wrong-subject" {
				request.SubjectDigest = canonical.BytesDigest([]byte("wrong"))
			}
			if scenario == "invalid-id" {
				request.ClosureID = "../unrelated"
			}
			args := []string{"production-terminal-readback", "--closure", request.ClosureID, "--run", request.RunID, "--subject", request.SubjectDigest}
			if scenario == "qualification-override" {
				args = append(args, "--accept-human")
			}
			command := exec.CommandContext(ctx, bin, args...)
			command.Env = []string{"PATH=/usr/bin:/bin", "CONTROL_ENDPOINT=" + server.URL, "CONTROL_CLIENT_CERT_FILE=" + filepath.Join(dir, "client.pem"), "CONTROL_CLIENT_KEY_FILE=" + filepath.Join(dir, "client.key"), "CONTROL_SERVER_CA_FILE=" + filepath.Join(dir, "ca.pem"), "HTTPS_PROXY=http://127.0.0.1:1"}
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			if scenario == "canceled" {
				if err := command.Start(); err != nil {
					t.Fatal(err)
				}
				defer command.Process.Kill()
				select {
				case <-started:
				case <-ctx.Done():
					t.Fatal("CLI did not start authenticated request")
				}
				if err := command.Process.Signal(syscall.SIGTERM); err != nil {
					t.Fatal(err)
				}
				err = command.Wait()
			} else {
				err = command.Run()
			}
			wantCalls := int32(1)
			if scenario == "invalid-id" || scenario == "qualification-override" {
				wantCalls = 0
			}
			if scenario == "forged-evidence" {
				wantCalls = 8
			}
			if scenario == "historical-core" {
				wantCalls = 12
				if err != nil {
					t.Fatalf("actual CLI failed: %v %s", err, stderr.String())
				}
				var report production.TerminalReadbackReport
				if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
					t.Fatal(err)
				}
				if report.Status != production.TerminalCoreVerified || report.ReaderSubject != readerSubject || report.QualificationGranted || report.Ready || report.ProductionQualified || report.HumanPresenceEstablished || len(report.Gates) != 14 || len(report.Procedures) != 3 || len(report.Records) != 12 {
					t.Fatal("incorrect real CLI readback or qualification boundary")
				}
				for _, record := range report.Records {
					if canonical.BytesDigest(record.SourceBytes) != record.SourceDigest {
						t.Fatal("CLI output changed original source bytes")
					}
				}
			} else {
				if err == nil || stdout.Len() != 0 {
					t.Fatalf("negative CLI emitted report: err=%v stdout=%s", err, stdout.String())
				}
				if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
					t.Fatalf("wrong CLI failure: %v", err)
				}
			}
			if calls.Load() != wantCalls {
				t.Fatalf("calls=%d want=%d stderr=%s", calls.Load(), wantCalls, stderr.String())
			}
			if strings.Contains(stderr.String(), "SECRET") || strings.Contains(stderr.String(), "client.key") {
				t.Fatal("raw source or credential path leaked")
			}
		})
	}
}
