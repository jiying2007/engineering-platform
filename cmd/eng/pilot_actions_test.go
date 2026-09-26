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

func readPilotActionFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
