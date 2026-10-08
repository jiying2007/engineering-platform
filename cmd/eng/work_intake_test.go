package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/embedded"
	"github.com/jiying2007/engineering-platform/internal/material"
	"github.com/jiying2007/engineering-platform/internal/routing"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"github.com/jiying2007/engineering-platform/internal/verification"
)

func workIntakeFixture() workIntakeSpec {
	return workIntakeSpec{
		Version:    1,
		WorkItemID: "wb-work", TaskContractID: "wb-task", RunID: "wb-run", AttemptID: "attempt-1",
		Title: "Add bounded embedded diagnostic", SourceRef: "workbuddy:req-42",
		TaskType: "FEATURE", Subsystem: "linux-bsp",
		ExpectedOutputs: []string{"source-change"},
		Material: material.Manifest{
			Repository: "repo", BaseCommit: strings.Repeat("a", 40), TargetID: "ssc305",
			AcceptanceCriteria: []string{"build passes"},
		},
		VerificationPlan: verification.Plan{
			ID: "wb-plan",
			Criteria: []verification.Criterion{{
				ID: "criterion-1", Statement: "build passes",
				Requirements: []verification.EvidenceRequirement{{ID: "req-1", Procedure: "ci.test"}},
			}},
		},
		ContextRefs: []core.ContextRef{{
			Source: "workbuddy:req-42", Type: "DOCUMENT", Version: "v1",
			Digest: canonical.BytesDigest([]byte("requirement")), Trust: core.ContextApproved,
		}},
		RuntimeProfile: "codex/runtime", ToolProfile: "codex/tool",
		WorkerProfile: "worker/codex-production", PolicyProfile: "policy/default",
	}
}

func TestWorkIntakeUsesCertificateOwnerAndExistingCoreAPIs(t *testing.T) {
	spec := workIntakeFixture()
	// Use an opaque chip label: only the explicit platform, tied to material,
	// may select the Linux/BSP Skill. Both client and Core re-derive the route.
	spec.Subsystem = "SSC305"
	spec.TargetContext = &routing.TargetContext{TargetID: spec.Material.TargetID, Platform: routing.PlatformLinuxBSP}
	route, readiness, err := spec.validate()
	if err != nil {
		t.Fatal(err)
	}
	contract := core.TaskContract{
		ID: spec.TaskContractID, WorkItemID: spec.WorkItemID, TaskType: spec.TaskType,
		CapabilityIDs: route.CapabilityIDs, SkillIDs: route.SkillIDs,
		Repository: spec.Material.Repository, BaseCommit: spec.Material.BaseCommit,
		TargetID: spec.Material.TargetID, AcceptanceCriteria: spec.Material.AcceptanceCriteria,
		TargetPlatform:  routing.PlatformLinuxBSP,
		ExpectedOutputs: spec.ExpectedOutputs, VerificationPlanID: spec.VerificationPlan.ID, Revision: 1,
	}
	selectedSkillDigest, err := embedded.RoutedSkillContractDigest(route.SkillIDs)
	if err != nil {
		t.Fatal(err)
	}
	contract.SkillContractDigest = selectedSkillDigest
	contract.SkillGuidanceVersion = 1
	planDigest, _ := spec.VerificationPlan.Digest()
	contract.VerificationPlanDigest = planDigest
	taskDigest, _ := contract.Digest()
	input := core.RunInputManifest{
		RunID: spec.RunID, TaskContractDigest: taskDigest, ContextRefs: spec.ContextRefs,
		RuntimeProfile: spec.RuntimeProfile, ToolProfile: spec.ToolProfile,
		WorkerProfile: spec.WorkerProfile, PolicyProfile: spec.PolicyProfile,
	}
	inputDigest, _ := input.Digest()
	owner := "urn:engineering-platform:operator:workbuddy"
	var mu sync.Mutex
	var calls []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/work-items":
			var got core.WorkItem
			if json.NewDecoder(r.Body).Decode(&got) != nil || got.HumanOwner != owner || got.TargetID != spec.Material.TargetID {
				t.Error("work owner was not certificate identity", got)
			}
			got.State, got.Version, got.CreatedAt = core.WorkDraft, 1, time.Now().UTC()
			_ = json.NewEncoder(w).Encode(got)
		case "/api/v1/task-contracts":
			var got workIntakeTaskRequest
			if json.NewDecoder(r.Body).Decode(&got) != nil ||
				len(got.Contract.CapabilityIDs) != 0 || len(got.Contract.SkillIDs) != 0 ||
				got.TargetContext == nil || got.TargetContext.TargetID != spec.Material.TargetID ||
				got.TargetContext.Platform != routing.PlatformLinuxBSP ||
				got.Contract.SkillContractDigest != "" || got.Contract.SkillGuidanceVersion != 0 {
				t.Error("client bypassed server-side routing", got)
			}
			_ = json.NewEncoder(w).Encode(workIntakeTaskResponse{
				Contract: contract, Digest: taskDigest, Readiness: readiness,
			})
		case "/api/v1/runs":
			var got workIntakeRunRequest
			if json.NewDecoder(r.Body).Decode(&got) != nil ||
				got.TaskContractDigest != taskDigest || got.RunInput.TaskContractDigest != taskDigest {
				t.Error("run did not bind returned task digest", got)
			}
			_ = json.NewEncoder(w).Encode(workIntakeRunResponse{
				Run: run.Run{
					ID: spec.RunID, TaskContractDigest: taskDigest,
					RunInputManifestDigest: inputDigest, State: run.Running,
					Version: 1, CurrentEpoch: 1, CurrentAttemptID: spec.AttemptID,
					ControlOwner: "RUNTIME",
				},
				Attempt:  run.Attempt{ID: spec.AttemptID, Epoch: 1, StartedAt: time.Now().UTC()},
				Session:  session.Session{RunID: spec.RunID, ExecutionEpoch: 1, Owner: session.Runtime},
				RunInput: input,
			})
		default:
			http.NotFound(w, r)
		}
	})
	pki := testsupport.NewPKI(t)
	server := httptest.NewUnstartedServer(handler)
	server.TLS = pki.ServerTLS()
	server.StartTLS()
	defer server.Close()
	client, err := controlclient.New(server.URL, &tls.Config{
		MinVersion: tls.VersionTLS13, RootCAs: pki.Roots,
		Certificates: []tls.Certificate{pki.ClientCertificate(t, owner)},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	receipt, err := executeWorkIntake(context.Background(), client, spec, canonical.BytesDigest([]byte("input")))
	if err != nil {
		t.Fatal(err)
	}
	if receipt.HumanOwner != owner || receipt.TaskContractDigest != taskDigest ||
		receipt.RunInputManifestDigest != inputDigest ||
		receipt.WorkerExecutionStarted || receipt.ModelTurnExecuted || receipt.ProductionQualified {
		t.Fatal("intake promoted authority", receipt)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(calls, ",") != "POST /api/v1/work-items,POST /api/v1/task-contracts,POST /api/v1/runs" {
		t.Fatal("unexpected API sequence", calls)
	}
}

func TestWorkIntakeNeverRetriesPartialCreation(t *testing.T) {
	spec := workIntakeFixture()
	owner := "urn:engineering-platform:operator:workbuddy"
	var workCalls, taskCalls, runCalls int
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/work-items":
			workCalls++
			var work core.WorkItem
			_ = json.NewDecoder(r.Body).Decode(&work)
			work.State, work.Version, work.CreatedAt = core.WorkDraft, 1, time.Now().UTC()
			_ = json.NewEncoder(w).Encode(work)
		case "/api/v1/task-contracts":
			taskCalls++
			w.WriteHeader(http.StatusConflict)
		case "/api/v1/runs":
			runCalls++
		}
	})
	pki := testsupport.NewPKI(t)
	server := httptest.NewUnstartedServer(handler)
	server.TLS = pki.ServerTLS()
	server.StartTLS()
	defer server.Close()
	client, err := controlclient.New(server.URL, &tls.Config{
		MinVersion: tls.VersionTLS13, RootCAs: pki.Roots,
		Certificates: []tls.Certificate{pki.ClientCertificate(t, owner)},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, err = executeWorkIntake(context.Background(), client, spec, canonical.BytesDigest([]byte("input")))
	var partial *workIntakeError
	if err == nil || !errors.As(err, &partial) || partial.Phase != "TASK" ||
		!partial.WorkCreated || partial.TaskCreated ||
		workCalls != 1 || taskCalls != 1 || runCalls != 0 {
		t.Fatal("partial intake retried or advanced", err, workCalls, taskCalls, runCalls)
	}
}

func TestWorkIntakeRejectsDuplicateTaskAndTargetAuthorities(t *testing.T) {
	spec := workIntakeFixture()
	spec.Material.TaskType = spec.TaskType
	if _, _, err := spec.validate(); err == nil {
		t.Fatal("duplicate material task_type accepted")
	}
}

func TestWorkIntakeExplicitTargetCannotDriftFromMaterial(t *testing.T) {
	spec := workIntakeFixture()
	spec.Subsystem = "opaque SSC305 board"
	spec.TargetContext = &routing.TargetContext{TargetID: "ssc305", Platform: routing.PlatformLinuxBSP}
	route, _, err := spec.validate()
	if err != nil || route.SkillIDs[len(route.SkillIDs)-1] != "linux-bsp-integration" {
		t.Fatalf("valid explicit routing failed: %#v %v", route, err)
	}
	spec.TargetContext.TargetID = "another-board"
	if _, _, err := spec.validate(); err == nil {
		t.Fatal("accepted mismatched material target identity")
	}
	spec.TargetContext.TargetID = "ssc305"
	spec.Subsystem = "MCU motor"
	if _, _, err := spec.validate(); err == nil {
		t.Fatal("accepted contradictory subsystem classification")
	}
}
