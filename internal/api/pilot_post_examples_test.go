package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/review"
	"github.com/jiying2007/engineering-platform/internal/store"
)

func TestRetainedPilotPostModelTemplatesReachClosure(t *testing.T) {
	const (
		baseCommit = "0123456789abcdef0123456789abcdef01234567"
		resultCommit = "cccccccccccccccccccccccccccccccccccccccc"
		profileDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		workerProfile = "worker/codex-pilot"
		humanOwner = "urn:engineering-platform:operator:pilot-owner"
		codexResultDigest = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		codexReceiptDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
		bundleDigest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
		gitManifestDigest = "sha256:3333333333333333333333333333333333333333333333333333333333333333"
		ciEnvelopeDigest = "sha256:4444444444444444444444444444444444444444444444444444444444444444"
	)

	for _, pilot := range []struct {
		name string
		dir string
		prefix string
		workID string
		runID string
		reqCodex string
		reqGit string
		reqCI string
	}{
		{
			name: "feature",
			dir: "feature-routing",
			prefix: "feature",
			workID: "m1-feature-routing-work",
			runID: "m1-feature-routing-run",
			reqCodex: "feature-codex",
			reqGit: "feature-git",
			reqCI: "feature-ci",
		},
		{
			name: "debug",
			dir: "debug-firmware-identity",
			prefix: "debug",
			workID: "m1-debug-firmware-identity-work",
			runID: "m1-debug-firmware-identity-run",
			reqCodex: "debug-codex",
			reqGit: "debug-git",
			reqCI: "debug-ci",
		},
	} {
		t.Run(pilot.name, func(t *testing.T) {
			server := NewServer(store.NewMemory())
			h := server.Handler()

			work := decodePilotTemplate(t, pilot.dir, "work.json.tmpl", map[string]string{
				"__HUMAN_OWNER__": humanOwner,
			})
			mustRequest(t, h, http.MethodPost, "/api/v1/work-items", work, http.StatusCreated)

			task := decodePilotTemplate(t, pilot.dir, "task.json.tmpl", map[string]string{
				"__BASE_COMMIT__": baseCommit,
			})
			taskBody := mustRequest(t, h, http.MethodPost, "/api/v1/task-contracts", task, http.StatusCreated)
			var taskResponse struct {
				Digest string `json:"digest"`
			}
			mustJSON(t, taskBody, &taskResponse)

			runRequest := decodePilotTemplate(t, pilot.dir, "run.json.tmpl", map[string]string{
				"__TASK_DIGEST__": taskResponse.Digest,
				"__PROFILE_DIGEST__": profileDigest,
				"__WORKER_PROFILE__": workerProfile,
			})
			mustRequest(t, h, http.MethodPost, "/api/v1/runs", runRequest, http.StatusCreated)

			publish := decodePilotTemplate(t, pilot.dir, "publish.json.tmpl", map[string]string{
				"__EXECUTION_EPOCH__": "1",
				"__RECOVERY_EPOCH__": "0",
				"__CODEX_RESULT_DIGEST__": codexResultDigest,
			})
			publishBytes, err := json.Marshal(publish)
			if err != nil {
				t.Fatal(err)
			}
			var actionReq createActionRequest
			if err := json.Unmarshal(publishBytes, &actionReq); err != nil {
				t.Fatal(err)
			}
			if actionReq.ExecutionEpoch != 1 || actionReq.RecoveryEpoch == nil || *actionReq.RecoveryEpoch != 0 ||
				actionReq.Action != "github.publish-pr" || actionReq.RiskClass != "CONTROLLED_MUTATION" ||
				actionReq.Capability != "github.publish-pr" || actionReq.ParametersDigest != codexResultDigest ||
				actionReq.RequestedBy != "urn:engineering-platform:operator:pilot-publisher" {
				t.Fatalf("unexpected publication template: %#v", actionReq)
			}

			complete := decodePilotTemplate(t, pilot.dir, "complete.json.tmpl", map[string]string{
				"__EXECUTION_EPOCH__": "1",
			})
			mustRequest(t, h, http.MethodPost, "/api/v1/runs/"+pilot.runID+"/complete", complete, http.StatusOK)

			deliveryRequest := decodePilotTemplate(t, pilot.dir, "delivery.json.tmpl", map[string]string{
				"__RESULT_COMMIT__": resultCommit,
				"__CODEX_RECEIPT_DIGEST__": codexReceiptDigest,
				"__BUNDLE_DIGEST__": bundleDigest,
				"__GIT_MANIFEST_DIGEST__": gitManifestDigest,
				"__CI_ENVELOPE_DIGEST__": ciEnvelopeDigest,
			})
			deliveryBody := mustRequest(t, h, http.MethodPost, "/api/v1/deliveries", deliveryRequest, http.StatusCreated)
			var delivery core.DeliveryReceipt
			mustJSON(t, deliveryBody, &delivery)
			if delivery.ResultCommit != resultCommit || delivery.BaseCommit != baseCommit ||
				len(delivery.Artifacts) != 4 || delivery.SubjectDigest == "" {
				t.Fatalf("unexpected retained-pilot Delivery: %#v", delivery)
			}

			registerPilotEvidence(t, h, delivery.ID, pilot.prefix+"-codex-evidence", pilot.reqCodex,
				"codex-execution-importer", "codex.core.execution.v1",
				[]string{"codex-execution-receipt", "codex-result-bundle"})
			registerPilotEvidence(t, h, delivery.ID, pilot.prefix+"-git-evidence", pilot.reqGit,
				"git-change-importer", "git.changed-tree.v1",
				[]string{"git-change-manifest"})
			registerPilotEvidence(t, h, delivery.ID, pilot.prefix+"-ci-evidence", pilot.reqCI,
				"github-actions-importer", "github.actions.trusted-ci.v1",
				[]string{"ci-provenance"})

			verification := decodePilotTemplate(t, pilot.dir, "verification.json", nil)
			verificationBody := mustRequest(t, h, http.MethodPost, "/api/v1/verifications", verification, http.StatusCreated)
			var verificationReport struct {
				ID string `json:"verification_report_id"`
				Result string `json:"result"`
				SubjectDigest string `json:"subject_digest"`
			}
			mustJSON(t, verificationBody, &verificationReport)
			if verificationReport.Result != "PASS" || verificationReport.SubjectDigest != delivery.SubjectDigest {
				t.Fatalf("unexpected pilot Verification: %#v", verificationReport)
			}

			reviewID := strings.Replace(pilot.runID, "-run", "-review", 1)
			reviewBody := mustRequest(t, h, http.MethodPost, "/api/v1/reviews", map[string]any{
				"review_report_id": reviewID,
				"delivery_receipt_id": delivery.ID,
				"verification_report_id": verificationReport.ID,
				"reviewer": "urn:engineering-platform:reviewer:pilot",
				"result": review.ResultPass,
				"findings": []any{
					map[string]any{
						"finding_id": pilot.prefix + "-review-info",
						"severity": review.SeverityInfo,
						"summary": "dry-run only: real pilot reviewer must independently inspect the retained subject",
					},
				},
			}, http.StatusCreated)
			var reviewReport review.Report
			mustJSON(t, reviewBody, &reviewReport)
			if reviewReport.Result != review.ResultPass || reviewReport.SubjectDigest != delivery.SubjectDigest {
				t.Fatalf("unexpected pilot Review: %#v", reviewReport)
			}

			closure := decodePilotTemplate(t, pilot.dir, "closure.json", nil)
			mustRequest(t, h, http.MethodPost, "/api/v1/closures", closure, http.StatusCreated)

			workBody := mustRequest(t, h, http.MethodGet, "/api/v1/work-items/"+pilot.workID, nil, http.StatusOK)
			var closed core.WorkItem
			mustJSON(t, workBody, &closed)
			if closed.State != core.WorkClosed {
				t.Fatalf("pilot post-model templates did not reach CLOSED: %#v", closed)
			}
		})
	}
}

func registerPilotEvidence(t *testing.T, h http.Handler, deliveryID, evidenceID, requirementID, issuer, procedure string, artifacts []string) {
	t.Helper()
	mustRequest(t, h, http.MethodPost, "/api/v1/evidence", map[string]any{
		"delivery_receipt_id": deliveryID,
		"evidence": map[string]any{
			"evidence_id": evidenceID,
			"requirement_id": requirementID,
			"issuer": issuer,
			"procedure": procedure,
			"result": "PASS",
			"artifact_refs": artifacts,
			"applicable": true,
		},
	}, http.StatusCreated)
}
