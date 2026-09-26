package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetainedPilotEngineerWorkflowCredentialBoundary(t *testing.T) {
	root := filepath.Join("..", "..")
	workflow := readPilotActionFile(t, filepath.Join(root, ".github", "workflows", "retained-pilot-engineer.yml"))
	model := readPilotActionFile(t, filepath.Join(root, "examples", "pilots", "actions", "engineer-model.sh"))
	publish := readPilotActionFile(t, filepath.Join(root, "examples", "pilots", "actions", "engineer-publish.sh"))

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	for _, script := range []string{
		filepath.Join(root, "examples", "pilots", "actions", "engineer-model.sh"),
		filepath.Join(root, "examples", "pilots", "actions", "engineer-publish.sh"),
	} {
		if out, err := exec.Command(bash, "-n", script).CombinedOutput(); err != nil {
			t.Fatalf("%s syntax: %v: %s", script, err, out)
		}
	}

	requiredWorkflow := []string{
		"persist-credentials: false",
		"id-token: write",
		"contents: write",
		"pull-requests: write",
		"name: Real WIF qualification and retained engineering turn",
		"name: Publish retained result only after model process exits",
		"PUBLISH_TOKEN: ${{ github.token }}",
		"if: ${{ always() }}",
		"retained-pilot-engineering-${{ inputs.pilot }}-${{ github.run_id }}",
	}
	for _, value := range requiredWorkflow {
		if !strings.Contains(workflow, value) {
			t.Fatalf("retained workflow missing %q", value)
		}
	}
	if strings.Count(workflow, "${{ github.token }}") != 1 {
		t.Fatal("job-scoped GitHub token must appear only in the publisher step")
	}
	start := strings.Index(workflow, "name: Real WIF qualification and retained engineering turn")
	end := strings.Index(workflow, "name: Publish retained result only after model process exits")
	if start < 0 || end <= start {
		t.Fatal("workflow model/publisher step order is invalid")
	}
	modelStep := workflow[start:end]
	if strings.Contains(modelStep, "github.token") || strings.Contains(modelStep, "GITHUB_TOKEN") ||
		strings.Contains(modelStep, "GH_TOKEN") || strings.Contains(modelStep, "PUBLISH_TOKEN") {
		t.Fatal("model workflow step exposes GitHub publication credential")
	}

	for _, forbidden := range []string{"GITHUB_TOKEN", "GH_TOKEN", "PUBLISH_TOKEN"} {
		if strings.Contains(model, forbidden) {
			t.Fatalf("model-phase script contains publisher credential name %q", forbidden)
		}
	}
	for _, required := range []string{
		"retained-pilot-engineer.yml@refs/heads/main",
		"codex-wif-live",
		"pilot-preflight",
		"core-pre-publication.dump",
		"result.bundle",
		"jq -e '.state==\"FINISHED\"",
	} {
		if !strings.Contains(model, required) {
			t.Fatalf("model-phase script missing %q", required)
		}
	}
	if strings.Index(model, "core-pre-publication.dump") > strings.Index(model, "model_phase:\"FINISHED\"") {
		t.Fatal("model state is marked FINISHED before pre-publication snapshot")
	}

	for _, required := range []string{
		"printf '%s' \"$PUBLISH_TOKEN\"",
		"control-plane-with-publisher.env",
		"preflight-before-publication.json",
		"test \"$RESULT\" = \"CONFIRMED\"",
		"core.dump",
		"engineering-state.json",
		"APPROVE_AND_PASS_EXACT_PR_HEAD_CI",
	} {
		if !strings.Contains(publish, required) {
			t.Fatalf("publisher-phase script missing %q", required)
		}
	}
	if strings.Index(publish, "printf '%s' \"$PUBLISH_TOKEN\"") > strings.Index(publish, "control-plane-with-publisher.env") {
		t.Fatal("publisher control-plane starts before credential is staged")
	}
	if strings.Contains(publish, "--execute-codex") || strings.Contains(publish, "OPENAI_IDENTITY_TOKEN_FILE") {
		t.Fatal("publisher phase contains model execution authority")
	}
}

func TestRetainedPilotVerificationWorkflowAuthorityBoundary(t *testing.T) {
	root := filepath.Join("..", "..")
	workflow := readPilotActionFile(t, filepath.Join(root, ".github", "workflows", "retained-pilot-verify.yml"))
	verify := readPilotActionFile(t, filepath.Join(root, "examples", "pilots", "actions", "verify-retained.sh"))

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	if out, err := exec.Command(bash, "-n", filepath.Join(root, "examples", "pilots", "actions", "verify-retained.sh")).CombinedOutput(); err != nil {
		t.Fatalf("verify-retained.sh syntax: %v: %s", err, out)
	}

	for _, required := range []string{
		"permissions:",
		"contents: read",
		"actions: read",
		"pull-requests: read",
		"persist-credentials: false",
		"GH_TOKEN: ${{ github.token }}",
		"name: Resume exact retained subject and produce Verification",
		"retained-pilot-verification-${{ inputs.pilot }}-${{ inputs.engineering_run_id }}-${{ github.run_id }}",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("verification workflow missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"contents: write",
		"pull-requests: write",
		"id-token: write",
		"PUBLISH_TOKEN",
		"--execute-codex",
		"OPENAI_IDENTITY_TOKEN_FILE",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("verification workflow unexpectedly contains %q", forbidden)
		}
	}

	for _, required := range []string{
		"core.dump",
		"pg_restore",
		"worktree add --detach",
		"trusted-ci-evidence-$RESULT_COMMIT",
		"engineering-binaries-$RESULT_COMMIT",
		"codex-0.155.0-qualification-$RESULT_COMMIT",
		"import-codex-evidence",
		"import-git-change-evidence",
		"import-ci-evidence",
		"clients/codex-evidence.env",
		"clients/git-evidence.env",
		"clients/ci-evidence.env",
		"clients/verifier.env",
		"/api/v1/verifications",
		"core-verification.dump",
		"next_gate:\"INDEPENDENT_REVIEW\"",
	} {
		if !strings.Contains(verify, required) {
			t.Fatalf("verification script missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"/api/v1/reviews",
		"/api/v1/closures",
		"clients/reviewer.env",
		"clients/closure.env",
		"--execute-codex",
		"PUBLISH_TOKEN",
	} {
		if strings.Contains(verify, forbidden) {
			t.Fatalf("verification script crosses independent-review boundary with %q", forbidden)
		}
	}
	if strings.Index(verify, "gh run list") > strings.Index(verify, "/api/v1/runs/$RUN_ID/complete") {
		t.Fatal("Run is completed before exact successful PR-head CI is discovered")
	}
	if strings.Index(verify, "import-codex-evidence") > strings.Index(verify, "/api/v1/verifications") ||
		strings.Index(verify, "import-git-change-evidence") > strings.Index(verify, "/api/v1/verifications") ||
		strings.Index(verify, "import-ci-evidence") > strings.Index(verify, "/api/v1/verifications") {
		t.Fatal("Verification is requested before all three Evidence imports")
	}
}

func TestRetainedPilotIndependentReviewWorkflowBoundary(t *testing.T) {
	root := filepath.Join("..", "..")
	workflow := readPilotActionFile(t, filepath.Join(root, ".github", "workflows", "retained-pilot-review.yml"))
	reviewScript := readPilotActionFile(t, filepath.Join(root, "examples", "pilots", "actions", "review-close-retained.sh"))

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	if out, err := exec.Command(bash, "-n", filepath.Join(root, "examples", "pilots", "actions", "review-close-retained.sh")).CombinedOutput(); err != nil {
		t.Fatalf("review-close-retained.sh syntax: %v: %s", err, out)
	}

	for _, required := range []string{
		"contents: read",
		"actions: read",
		"result:",
		"- PASS",
		"- FAIL",
		"GH_TOKEN: ${{ github.token }}",
		"name: Restore verified subject and record independent review",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("review workflow missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"contents: write",
		"pull-requests: write",
		"id-token: write",
		"PUBLISH_TOKEN",
		"OPENAI_IDENTITY_TOKEN_FILE",
		"--execute-codex",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("review workflow unexpectedly contains %q", forbidden)
		}
	}

	for _, required := range []string{
		"independent review requires a different GitHub actor from engineering execution",
		"clients/reviewer.env",
		"/api/v1/reviews",
		"if [ \"$REVIEW_RESULT\" = PASS ]",
		"clients/closure.env",
		"/api/v1/closures",
		"core-review.dump",
		"review-state.json",
	} {
		if !strings.Contains(reviewScript, required) {
			t.Fatalf("review script missing %q", required)
		}
	}
	if strings.Index(reviewScript, "GITHUB_ACTOR") > strings.Index(reviewScript, "/api/v1/reviews") {
		t.Fatal("review is submitted before GitHub actor independence is checked")
	}
	if strings.Index(reviewScript, "if [ \"$REVIEW_RESULT\" = PASS ]") > strings.Index(reviewScript, "/api/v1/closures") {
		t.Fatal("Closure is not guarded by PASS review")
	}
	for _, forbidden := range []string{
		"worker.codex-execute",
		"github.publish-pr",
		"--execute-codex",
		"import-ci-evidence",
		"/api/v1/verifications",
	} {
		if strings.Contains(reviewScript, forbidden) {
			t.Fatalf("independent review phase crosses earlier authority with %q", forbidden)
		}
	}
}

func readPilotActionFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
