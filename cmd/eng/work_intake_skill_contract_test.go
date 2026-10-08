package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/routing"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func TestWorkIntakeRejectsTamperedSkillMethodDigestBeforeRun(t *testing.T) {
	spec := workIntakeFixture()
	spec.Subsystem = "SSC305"
	spec.TargetContext = &routing.TargetContext{
		TargetID: spec.Material.TargetID, Platform: routing.PlatformLinuxBSP,
	}
	route, readiness, err := spec.validate()
	if err != nil {
		t.Fatal(err)
	}
	planDigest, err := spec.VerificationPlan.Digest()
	if err != nil {
		t.Fatal(err)
	}
	tampered := core.TaskContract{
		ID: spec.TaskContractID, WorkItemID: spec.WorkItemID,
		TaskType: spec.TaskType, CapabilityIDs: route.CapabilityIDs, SkillIDs: route.SkillIDs,
		SkillContractDigest: "sha256:" + strings.Repeat("0", 64),
		Repository:          spec.Material.Repository, BaseCommit: spec.Material.BaseCommit,
		TargetID: spec.Material.TargetID, TargetPlatform: routing.PlatformLinuxBSP,
		AcceptanceCriteria: spec.Material.AcceptanceCriteria,
		ExpectedOutputs:    spec.ExpectedOutputs, VerificationPlanID: spec.VerificationPlan.ID,
		VerificationPlanDigest: planDigest, Revision: 1,
	}
	tamperedDigest, err := tampered.Digest()
	if err != nil {
		t.Fatal(err)
	}
	var workCalls, taskCalls, runCalls int
	owner := "urn:engineering-platform:operator:workbuddy"
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
			var req workIntakeTaskRequest
			if json.NewDecoder(r.Body).Decode(&req) != nil ||
				req.Contract.SkillContractDigest != "" ||
				req.TargetContext == nil || req.TargetContext.TargetID != spec.Material.TargetID {
				t.Error("client supplied a second Skill authority", req)
			}
			_ = json.NewEncoder(w).Encode(workIntakeTaskResponse{
				Contract: tampered, Digest: tamperedDigest, Readiness: readiness,
			})
		case "/api/v1/runs":
			runCalls++
			t.Error("tampered Skill selection started a Run")
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
	_, err = executeWorkIntake(context.Background(), client, spec, canonical.BytesDigest([]byte("input")))
	var partial *workIntakeError
	if err == nil || !errors.As(err, &partial) || partial.Phase != "TASK_READBACK" ||
		!partial.WorkCreated || !partial.TaskCreated ||
		workCalls != 1 || taskCalls != 1 || runCalls != 0 {
		t.Fatal("tampered-but-self-consistent Task digest was admitted", err, workCalls, taskCalls, runCalls)
	}
}
