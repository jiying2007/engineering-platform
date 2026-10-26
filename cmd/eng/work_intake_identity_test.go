package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func TestWorkIntakeRejectsResponseDriftBeforeRunCreation(t *testing.T) {
	cases := []struct {
		name string
		mutateWork func(*core.WorkItem)
		mutateTask func(*core.TaskContract)
		phase string
	}{
		{name: "source-ref", phase: "WORK_READBACK", mutateWork: func(w *core.WorkItem) { w.SourceRef = "another-source" }},
		{name: "target-id", phase: "WORK_READBACK", mutateWork: func(w *core.WorkItem) { w.TargetID = "another-target" }},
		{name: "assurance", phase: "WORK_READBACK", mutateWork: func(w *core.WorkItem) { w.AssuranceClass = "changed-assurance" }},
		{name: "task-type", phase: "TASK_READBACK", mutateTask: func(c *core.TaskContract) { c.TaskType = "BRINGUP" }},
		{name: "allowed-actions", phase: "TASK_READBACK", mutateTask: func(c *core.TaskContract) { c.AllowedActions = append(c.AllowedActions, "unexpected-action") }},
		{name: "expected-outputs", phase: "TASK_READBACK", mutateTask: func(c *core.TaskContract) { c.ExpectedOutputs = []string{"different-output"} }},
		{name: "verification-plan", phase: "TASK_READBACK", mutateTask: func(c *core.TaskContract) { c.VerificationPlanID = "other-plan" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := workIntakeFixture()
			route, readiness, err := spec.validate()
			if err != nil { t.Fatal(err) }
			planDigest, err := spec.VerificationPlan.Digest()
			if err != nil { t.Fatal(err) }
			actor := "urn:engineering-platform:operator:drift-test"
			var workPosts, taskPosts, runPosts int
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/work-items":
					workPosts++
					var work core.WorkItem
					if err := json.NewDecoder(r.Body).Decode(&work); err != nil { t.Error(err); return }
					work.State, work.Version, work.CreatedAt = core.WorkDraft, 1, time.Now().UTC()
					if tc.mutateWork != nil { tc.mutateWork(&work) }
					_ = json.NewEncoder(w).Encode(work)
				case "/api/v1/task-contracts":
					taskPosts++
					var request workIntakeTaskRequest
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil { t.Error(err); return }
					c := request.Contract
					c.Repository, c.BaseCommit, c.TargetID = request.Material.Repository, request.Material.BaseCommit, request.Material.TargetID
					c.CapabilityIDs, c.SkillIDs = route.CapabilityIDs, route.SkillIDs
					c.AcceptanceCriteria = append([]string(nil), request.Material.AcceptanceCriteria...)
					c.VerificationPlanID, c.VerificationPlanDigest = request.VerificationPlan.ID, planDigest
					c.Revision = 1
					if tc.mutateTask != nil { tc.mutateTask(&c) }
					digest, err := c.Digest()
					if err != nil { t.Error(err); return }
					_ = json.NewEncoder(w).Encode(workIntakeTaskResponse{Contract: c, Digest: digest, Readiness: readiness})
				case "/api/v1/runs":
					runPosts++
					t.Error("unbound response created a Run")
					http.Error(w, "unexpected Run creation", http.StatusConflict)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
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
				Certificates: []tls.Certificate{pki.ClientCertificate(t, actor)},
			})
			if err != nil { t.Fatal(err) }
			defer client.Close()
			_, err = executeWorkIntake(context.Background(), client, spec, canonical.BytesDigest([]byte("intake-input")))
			var partial *workIntakeError
			if !errors.As(err, &partial) || partial.Phase != tc.phase {
				t.Fatalf("drift not rejected at expected phase: got=%v want=%s", err, tc.phase)
			}
			expectedTaskPosts := 1
			if tc.mutateWork != nil { expectedTaskPosts = 0 }
			if workPosts != 1 || taskPosts != expectedTaskPosts || runPosts != 0 {
				t.Fatalf("extra side effects: work=%d task=%d run=%d", workPosts, taskPosts, runPosts)
			}
		})
	}
}
