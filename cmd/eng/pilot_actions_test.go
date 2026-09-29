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
	liveWorkflow := readPilotActionFile(t, filepath.Join(root, ".github", "workflows", "codex-wif-live.yml"))
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

	for _, required := range []string{
		"verify-github-oidc.py",
		"GITHUB_REPOSITORY_ID",
		"GITHUB_REPOSITORY_OWNER_ID",
		"codex-wif-live.yml@refs/heads/main",
	} {
		if !strings.Contains(liveWorkflow, required) {
			t.Fatalf("live WIF workflow missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"subject.startswith",
	} {
		if strings.Contains(liveWorkflow, forbidden) {
			t.Fatalf("live WIF workflow retains legacy subject-format assumption %q", forbidden)
		}
	}

	for _, forbidden := range []string{"GITHUB_TOKEN", "GH_TOKEN", "PUBLISH_TOKEN"} {
		if strings.Contains(model, forbidden) {
			t.Fatalf("model-phase script contains publisher credential name %q", forbidden)
		}
	}
	for _, required := range []string{
		"retained-pilot-engineer.yml@refs/heads/main",
		"verify-github-oidc.py",
		"GITHUB_REPOSITORY_ID",
		"GITHUB_REPOSITORY_OWNER_ID",
		"--max-lifetime-seconds 600",
		"--min-remaining-seconds 120",
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
	if strings.Contains(model, "\n\"$ROOT/examples/pilots/local-stack/bootstrap.sh\" ") {
		t.Fatal("self-hosted path must not depend on executable bit for bootstrap.sh")
	}
	if strings.Contains(model, "PG_NAME=\"engineering-platform-retained-$PPID-$\"") {
		t.Fatal("self-hosted postgres container name retains malformed literal dollar suffix")
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
		"engineering_recovery_run_id:",
		"ENGINEERING_RECOVERY_RUN_ID: ${{ inputs.engineering_recovery_run_id }}",
		"name: Resume exact retained subject and produce Verification",
		"retained-pilot-verification-${{ inputs.pilot }}-${{ inputs.engineering_run_id }}-${{ github.run_id }}",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("verification workflow missing %q", required)
		}
	}

	if got := strings.Count(verify, "expired|tostring"); got < 2 {
		t.Fatalf("verification must safely parse false expiry at both source and CI artifact boundaries; got %d guarded reads", got)
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
		"ENGINEERING_RECOVERY_RUN_ID",
		"expired|tostring",
		"Retained M1 pilot engineering recovery",
		".model_replay==false",
		".publication==\"CONFIRMED\"",
		"pg_restore",
		"worktree add --detach",
		"trusted-ci-evidence-$RESULT_COMMIT",
		"engineering-binaries-$RESULT_COMMIT",
		"codex-compatibility-qualification-$RESULT_COMMIT",
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

func TestRetainedPilotEngineeringRecoveryWorkflowBoundary(t *testing.T) {
	root := filepath.Join("..", "..")
	workflowPath := filepath.Join(root, ".github", "workflows", "retained-pilot-recover.yml")
	recoveryPath := filepath.Join(root, "examples", "pilots", "actions", "recover-retained-engineering.sh")
	workflow := readPilotActionFile(t, workflowPath)
	recovery := readPilotActionFile(t, recoveryPath)

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	if out, err := exec.Command(bash, "-n", recoveryPath).CombinedOutput(); err != nil {
		t.Fatalf("recover-retained-engineering.sh syntax: %v: %s", err, out)
	}

	for _, required := range []string{
		"permissions:",
		"contents: read",
		"actions: read",
		"pull-requests: read",
		"persist-credentials: false",
		"GH_TOKEN: ${{ github.token }}",
		"engineering_run_id:",
		"name: Validate retained FINISHED subject without model replay",
		"retained-pilot-engineering-recovery-${{ inputs.pilot }}-${{ inputs.engineering_run_id }}-${{ github.run_id }}",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("engineering recovery workflow missing %q", required)
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
			t.Fatalf("engineering recovery workflow unexpectedly contains %q", forbidden)
		}
	}

	for _, required := range []string{
		"Retained M1 pilot engineering",
		".github/workflows/retained-pilot-self-hosted-engineer.yml",
		`test "$(printf '%s' "$run_json" | jq -er .conclusion)" = "failure"`,
		"engineering-state.json",
		"model-phase.json",
		"codex-status.json",
		"publication-receipt.json",
		"preflight-before-publication.json",
		"expired|tostring",
		"result.bundle",
		"core.dump",
		`test "$(jq -er .model_phase "$MODEL_PHASE")" = FINISHED`,
		`test "$(jq -er .result "$PUBLICATION_RECEIPT")" = CONFIRMED`,
		"gh api \"repos/$GITHUB_REPOSITORY/pulls/$PR_NUMBER\"",
		"model_replay:false",
		"next_gate:\"PASS_EXACT_PR_HEAD_CI\"",
	} {
		if !strings.Contains(recovery, required) {
			t.Fatalf("engineering recovery script missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"--execute-codex",
		"PUBLISH_TOKEN",
		"OPENAI_IDENTITY_TOKEN_FILE",
		"/api/v1/runs/$RUN_ID/actions",
	} {
		if strings.Contains(recovery, forbidden) {
			t.Fatalf("engineering recovery crosses no-replay/read-only boundary with %q", forbidden)
		}
	}
}

func TestRetainedDebugFirmwareIdentityReproductionWorkflowBoundary(t *testing.T) {
	root := filepath.Join("..", "..")
	workflow := readPilotActionFile(t, filepath.Join(root, ".github", "workflows", "retained-debug-firmware-identity-reproduction.yml"))
	scriptPath := filepath.Join(root, "examples", "pilots", "debug-firmware-identity", "capture-reproduction.sh")
	script := readPilotActionFile(t, scriptPath)

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	if out, err := exec.Command(bash, "-n", scriptPath).CombinedOutput(); err != nil {
		t.Fatalf("capture-reproduction.sh syntax: %v: %s", err, out)
	}

	for _, required := range []string{
		"workflow_dispatch:",
		"contents: read",
		"GITHUB_REF_PROTECTED",
		"persist-credentials: false",
		"Capture authoritative failing reproduction",
		"retained-debug-firmware-identity-reproduction-${{ github.sha }}-${{ github.run_id }}",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("Debug reproduction workflow missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"contents: write",
		"pull-requests: write",
		"id-token: write",
		"--execute-codex",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Fatalf("Debug reproduction workflow unexpectedly contains %q", forbidden)
		}
	}

	for _, required := range []string{
		"FirmwareIdentity:   \"not-a-digest\"",
		"expected BLOCKED for noncanonical firmware identity, got %s",
		"test \"$test_rc\" -ne 0",
		"got READY",
		"reproduction_confirmed:true",
		"observed_status \"READY\"",
		"rm -f \"$TEST_FILE\"",
		"status --porcelain=v1 --untracked-files=all",
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("Debug reproduction script missing %q", required)
		}
	}
	if strings.Contains(script, "git commit") || strings.Contains(script, "git push") {
		t.Fatal("Debug reproduction capture must not mutate repository history")
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
		"issues: read",
		"pull-requests: read",
		"decision_comment_id:",
		"REVIEW_DECISION_COMMENT_ID: ${{ inputs.decision_comment_id }}",
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
		"engineering-platform.retained-independent-review.v1",
		"review decision comment predates successful verification",
		"independent review requires a different GitHub decision actor from engineering execution",
		"author_association",
		"reviewer-actor.txt",
		"review-decision-comment-id.txt",
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
	if strings.Index(reviewScript, "REVIEWER_ACTOR") > strings.Index(reviewScript, "/api/v1/reviews") {
		t.Fatal("review is submitted before bound GitHub decision actor independence is checked")
	}
	if !strings.Contains(reviewScript, `dispatcher_actor:$dispatcher_actor`) {
		t.Fatal("review state must retain dispatcher actor separately from decision actor")
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

func TestPilotControlHealthRequiresMTLS(t *testing.T) {
	root := filepath.Join("..", "..")
	healthPath := filepath.Join(root, "examples", "pilots", "local-stack", "health.sh")
	health := readPilotActionFile(t, healthPath)

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}

	for _, required := range []string{
		"--cacert \"$CA\"",
		"--cert \"$CERT\"",
		"--key \"$KEY\"",
		"https://127.0.0.1:18443/healthz",
	} {
		if !strings.Contains(health, required) {
			t.Fatalf("mTLS health helper missing %q", required)
		}
	}

	scripts := map[string][]string{
		filepath.Join(root, "examples", "pilots", "self-hosted", "engineer-model.sh"): {
			"local-stack/health.sh\" \"$STACK_ROOT\" owner",
			"local-stack/health.sh\" \"$STACK_ROOT\" publisher",
		},
		filepath.Join(root, "examples", "pilots", "actions", "engineer-model.sh"): {
			"local-stack/health.sh\" \"$STACK_ROOT\" \"$health_client\"",
			"start_control \"$STACK_ROOT/operator/control-plane.env\" owner",
		},
		filepath.Join(root, "examples", "pilots", "actions", "engineer-publish.sh"): {
			"local-stack/health.sh\" \"$STACK_ROOT\" publisher",
		},
		filepath.Join(root, "examples", "pilots", "actions", "verify-retained.sh"): {
			"local-stack/health.sh\" \"$STACK_ROOT\" verifier",
		},
		filepath.Join(root, "examples", "pilots", "actions", "review-close-retained.sh"): {
			"local-stack/health.sh\" \"$STACK_ROOT\" reviewer",
		},
		filepath.Join(root, "examples", "pilots", "local-stack", "status.sh"): {
			"health.sh\" \"$ROOT\" owner",
		},
	}

	for path, required := range scripts {
		body := readPilotActionFile(t, path)
		if out, err := exec.Command(bash, "-n", path).CombinedOutput(); err != nil {
			t.Fatalf("%s syntax: %v: %s", path, err, out)
		}
		for _, value := range required {
			if !strings.Contains(body, value) {
				t.Fatalf("%s missing authenticated health contract %q", path, value)
			}
		}
		if strings.Contains(body, "--cacert \"$STACK_ROOT/pki/ca.crt\" https://127.0.0.1:18443/healthz") {
			t.Fatalf("%s retains CA-only health probe", path)
		}
	}
}

func TestPilotControlEnvironmentIsIsolatedFromOrchestrators(t *testing.T) {
	root := filepath.Join("..", "..")
	runnerPath := filepath.Join(root, "examples", "pilots", "local-stack", "run-control.sh")

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	if out, err := exec.Command(bash, "-n", runnerPath).CombinedOutput(); err != nil {
		t.Fatalf("run-control.sh syntax: %v: %s", err, out)
	}

	runner := readPilotActionFile(t, runnerPath)
	for _, required := range []string{
		". \"$ENV_FILE\"",
		"exec \"$CONTROL\"",
		"export AUTO_MIGRATE=\"$AUTO_MIGRATE_OVERRIDE\"",
	} {
		if !strings.Contains(runner, required) {
			t.Fatalf("run-control helper missing %q", required)
		}
	}

	scripts := map[string][]string{
		filepath.Join(root, "examples", "pilots", "self-hosted", "engineer-model.sh"): {
			"local-stack/run-control.sh",
			"operator/control-plane.env",
			"operator/control-plane-with-publisher.env",
		},
		filepath.Join(root, "examples", "pilots", "actions", "engineer-model.sh"): {
			"local health_client=\"$2\"",
			"local-stack/run-control.sh",
			"start_control \"$STACK_ROOT/operator/control-plane.env\" owner",
		},
		filepath.Join(root, "examples", "pilots", "actions", "engineer-publish.sh"): {
			"local-stack/run-control.sh",
			"operator/control-plane-with-publisher.env",
		},
		filepath.Join(root, "examples", "pilots", "actions", "verify-retained.sh"): {
			"local-stack/run-control.sh",
			"operator/control-plane.env",
			"  0 \\",
		},
		filepath.Join(root, "examples", "pilots", "actions", "review-close-retained.sh"): {
			"local-stack/run-control.sh",
			"operator/control-plane.env",
			"  0 \\",
		},
	}

	for path, required := range scripts {
		body := readPilotActionFile(t, path)
		if out, err := exec.Command(bash, "-n", path).CombinedOutput(); err != nil {
			t.Fatalf("%s syntax: %v: %s", path, err, out)
		}
		for _, value := range required {
			if !strings.Contains(body, value) {
				t.Fatalf("%s missing isolated control contract %q", path, value)
			}
		}
		for _, forbidden := range []string{
			". \"$STACK_ROOT/operator/control-plane.env\"",
			". \"$STACK_ROOT/operator/control-plane-with-publisher.env\"",
			"export DATABASE_URL=",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s leaks control-plane environment through %q", path, forbidden)
			}
		}
	}
}

func TestPilotRepositorySourceSecurity(t *testing.T) {
	root := filepath.Join("..", "..")
	helperPath := filepath.Join(root, "examples", "pilots", "local-stack", "secure-repository-source.sh")

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	if out, err := exec.Command(bash, "-n", helperPath).CombinedOutput(); err != nil {
		t.Fatalf("secure-repository-source.sh syntax: %v: %s", err, out)
	}

	repository := t.TempDir()
	if err := os.Chmod(repository, 0o777); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(bash, helperPath, repository).CombinedOutput(); err != nil {
		t.Fatalf("secure repository source: %v: %s", err, out)
	}
	info, err := os.Stat(repository)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o022 != 0 {
		t.Fatalf("repository source remains group/world writable: %o", info.Mode().Perm())
	}

	realRepository := t.TempDir()
	aliasRoot := t.TempDir()
	alias := filepath.Join(aliasRoot, "repository-link")
	if err := os.Symlink(realRepository, alias); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(bash, helperPath, alias).CombinedOutput(); err == nil {
		t.Fatalf("symlink repository source accepted: %s", out)
	}

	scripts := map[string]string{
		filepath.Join(root, "examples", "pilots", "self-hosted", "engineer-model.sh"): "secure-repository-source.sh\" \"$ROOT\"",
		filepath.Join(root, "examples", "pilots", "actions", "engineer-model.sh"):     "secure-repository-source.sh\" \"$GITHUB_WORKSPACE\"",
	}
	for path, required := range scripts {
		body := readPilotActionFile(t, path)
		if out, err := exec.Command(bash, "-n", path).CombinedOutput(); err != nil {
			t.Fatalf("%s syntax: %v: %s", path, err, out)
		}
		if !strings.Contains(body, required) {
			t.Fatalf("%s missing repository source hardening %q", path, required)
		}
	}
}

func TestTrustedSelfHostedSavedLoginStaging(t *testing.T) {
	root := filepath.Join("..", "..")
	helperPath := filepath.Join(root, "examples", "pilots", "self-hosted", "stage-saved-login.sh")

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	if out, err := exec.Command(bash, "-n", helperPath).CombinedOutput(); err != nil {
		t.Fatalf("stage-saved-login.sh syntax: %v: %s", err, out)
	}
	helper := readPilotActionFile(t, helperPath)
	if !strings.Contains(helper, "chmod go-w \"$SOURCE_PARENT\"") {
		t.Fatal("saved-login staging helper must harden the owned source parent before copy")
	}

	sourceParent := t.TempDir()
	if err := os.Chmod(sourceParent, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceParent, "auth.json")
	if err := os.WriteFile(source, []byte("{\"auth\":\"0123456789abcdef\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	stageParent := t.TempDir()
	if err := os.Chmod(stageParent, 0o700); err != nil {
		t.Fatal(err)
	}
	stageRoot := filepath.Join(stageParent, "staged-login")
	out, err := exec.Command(bash, helperPath, source, stageRoot).CombinedOutput()
	if err != nil {
		t.Fatalf("stage saved login: %v: %s", err, out)
	}
	staged := strings.TrimSpace(string(out))
	if staged != filepath.Join(stageRoot, "auth.json") {
		t.Fatalf("unexpected staged path %q", staged)
	}
	info, err := os.Stat(staged)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("staged login is not owner-private regular file: %v", info.Mode())
	}
	parentInfo, err := os.Stat(stageRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !parentInfo.IsDir() || parentInfo.Mode().Perm()&0o077 != 0 {
		t.Fatalf("staged login parent is not owner-private: %v", parentInfo.Mode())
	}

	if err := os.Chmod(sourceParent, 0o777); err != nil {
		t.Fatal(err)
	}
	hardenedStage := filepath.Join(stageParent, "hardened-parent")
	if out, err := exec.Command(bash, helperPath, source, hardenedStage).CombinedOutput(); err != nil {
		t.Fatalf("owned writable source parent was not hardened: %v: %s", err, out)
	}
	sourceParentInfo, err := os.Stat(sourceParent)
	if err != nil {
		t.Fatal(err)
	}
	if sourceParentInfo.Mode().Perm()&0o022 != 0 {
		t.Fatalf("source parent remains group/world writable: %o", sourceParentInfo.Mode().Perm())
	}

	alias := filepath.Join(sourceParent, "auth-link.json")
	if err := os.Symlink(source, alias); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(bash, helperPath, alias, filepath.Join(stageParent, "bad-link")).CombinedOutput(); err == nil {
		t.Fatalf("symlink saved-login source accepted: %s", out)
	}
}

func TestTrustedSelfHostedPilotModelPhaseCredentialBoundary(t *testing.T) {
	root := filepath.Join("..", "..")
	workflowPath := filepath.Join(root, ".github", "workflows", "retained-pilot-self-hosted-engineer.yml")
	qualifyPath := filepath.Join(root, "examples", "pilots", "self-hosted", "qualify-login.sh")
	modelPath := filepath.Join(root, "examples", "pilots", "self-hosted", "engineer-model.sh")
	workflow := readPilotActionFile(t, workflowPath)
	qualify := readPilotActionFile(t, qualifyPath)
	model := readPilotActionFile(t, modelPath)

	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	stagePath := filepath.Join(root, "examples", "pilots", "self-hosted", "stage-saved-login.sh")
	for _, script := range []string{stagePath, qualifyPath, modelPath} {
		if out, err := exec.Command(bash, "-n", script).CombinedOutput(); err != nil {
			t.Fatalf("%s syntax: %v: %s", script, err, out)
		}
	}

	for _, required := range []string{
		"login status",
		"Logged in using ChatGPT",
		"codex-saved-login-live",
		"credential_bootstrap_removed_before_turn==true",
		"test ! -e \"$HOME_DIR/.codex/auth.json\"",
		"stage-saved-login.sh",
		"--saved-login-file \"$STAGED_LOGIN_FILE\"",
	} {
		if !strings.Contains(qualify, required) {
			t.Fatalf("saved-login qualification missing %q", required)
		}
	}
	for _, required := range []string{
		"runner_label:",
		"reproduction_run_id:",
		"DEBUG_REPRODUCTION_RUN_ID: ${{ inputs.reproduction_run_id }}",
		"runs-on: ${{ inputs.runner_label }}",
		"engineering-platform-codex-[0-9a-f]{32}",
		"GITHUB_REF_PROTECTED",
		"persist-credentials: false",
		"Retain resumable engineering state",
		"retained-pilot-engineering-${{ inputs.pilot }}-${{ github.run_id }}",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("self-hosted workflow missing %q", required)
		}
	}
	for _, required := range []string{
		".protected==true",
		"Logged in using ChatGPT",
		"credential_mode:\"saved_chatgpt_login\"",
		"publication==\"BLOCKED_EXTERNAL_PUBLISHER\"",
		"--execute-codex",
		"credential_bootstrap_removed_before_turn==true",
		"core-pre-publication.dump",
		"result.bundle",
		"model_phase:\"FINISHED\"",
		"gh auth token --hostname github.com > \"$TOKEN_FILE\"",
		"control-plane-with-publisher.env",
		"/api/v1/runs/$RUN_ID/actions",
		"test \"$RESULT\" = \"CONFIRMED\"",
		"core.dump",
		"engineering-state.json",
		"no model replay",
		"preflight-checks.log",
		"gh auth status --hostname github.com",
		"gh auth token --help",
		"gh auth status --help",
		"--show-token",
		"gh api user --jq '.login == \"jiying2007\"'",
		"gh api repos/jiying2007/engineering-platform/branches/main",
		"bash \"$ROOT/examples/pilots/local-stack/bootstrap.sh\"",
		"stage-saved-login.sh",
		"jq --arg login \"$STAGED_LOGIN_FILE\"",
		"rm -rf -- \"$LOGIN_STAGE_ROOT\"",
		"PG_NAME=\"engineering-platform-retained-${GITHUB_RUN_ID}-${PPID}\"",
		"starting debug-reproduction-binding",
		"Retained Debug firmware identity reproduction",
		"reproduction_confirmed==true",
		"expected_status==\"BLOCKED\"",
		"observed_status==\"READY\"",
		"debug-reproduction-binding.json",
		"debug_reproduction_run_id:",
		"debug_reproduction_receipt_digest:",
	} {
		if !strings.Contains(model, required) {
			t.Fatalf("self-hosted engineer-to-PR phase missing %q", required)
		}
	}
	if strings.Index(model, "core-pre-publication.dump") > strings.Index(model, "model_phase:\"FINISHED\"") {
		t.Fatal("self-hosted model state is marked FINISHED before pre-publication snapshot")
	}
	if strings.Index(model, "starting debug-reproduction-binding") > strings.Index(model, "api POST /api/v1/task-contracts") {
		t.Fatal("Debug reproduction is not validated before retained Task creation")
	}
	if strings.Index(model, "starting debug-reproduction-binding") > strings.Index(model, "--execute-codex") {
		t.Fatal("Debug reproduction is not validated before model execution")
	}
	if strings.Index(model, "gh auth status --hostname github.com --show-token") < strings.Index(model, "model_phase:\"FINISHED\"") {
		t.Fatal("legacy token export occurs before retained model FINISHED state")
	}
	if strings.Index(model, "gh auth token --hostname github.com > \"$TOKEN_FILE\"") < strings.Index(model, "model_phase:\"FINISHED\"") {
		t.Fatal("publisher token is materialized before retained model FINISHED state")
	}
	for _, forbidden := range []string{
		`cp "$PREPARATION_FILE" "$STATE_ROOT/worker-preparation.json"`,
		`cp "$WORKER_CODEX_FILE" "$STATE_ROOT/worker-codex.json"`,
	} {
		if strings.Contains(model, forbidden) {
			t.Fatalf("self-hosted retained state must not copy a file onto itself: %q", forbidden)
		}
	}
	for _, required := range []string{
		`test "$PREPARATION_FILE" = "$STATE_ROOT/worker-preparation.json"`,
		`test "$WORKER_CODEX_FILE" = "$STATE_ROOT/worker-codex.json"`,
	} {
		if !strings.Contains(model, required) {
			t.Fatalf("self-hosted retained state identity guard missing %q", required)
		}
	}
	if strings.Contains(workflow, "${{ github.token }}") {
		t.Fatal("self-hosted workflow must not inject job-scoped GitHub token into the saved-login chain")
	}
	if strings.Contains(workflow, "runs-on: [self-hosted") ||
		strings.Contains(workflow, "runs-on: self-hosted") ||
		strings.Contains(workflow, "engineering-platform-codex]") {
		t.Fatal("public-repository self-hosted workflow must not target persistent/default runner labels")
	}
}
