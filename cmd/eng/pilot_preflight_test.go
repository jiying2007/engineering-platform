package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
)

func TestPilotPreflightReportsOnlyExternalBlockers(t *testing.T) {
	options, _, _ := pilotPreflightFixture(t)
	result, err := checkPilotPreflight(options)
	if err != nil {
		t.Fatal(err)
	}
	if result.Internal != "READY" ||
		result.ModelExecution != "BLOCKED_EXTERNAL_WIF" ||
		result.Publication != "BLOCKED_EXTERNAL_PUBLISHER" {
		t.Fatalf("unexpected partial readiness: %#v", result)
	}
	want := map[string]bool{
		"worker_codex_federation_rule":          true,
		"managed_workspace_wif_qualification":   true,
		"publisher_configuration_or_credential": true,
	}
	if len(result.Blockers) != len(want) {
		t.Fatalf("unexpected blockers: %#v", result.Blockers)
	}
	for _, blocker := range result.Blockers {
		if !want[blocker] {
			t.Fatalf("unexpected blocker %q", blocker)
		}
	}
}

func TestPilotPreflightFullyReadyWithBoundWIFAndPublisher(t *testing.T) {
	options, profile, rule := pilotPreflightFixture(t)
	root := filepath.Dir(options.ProfileFile)

	workerCodex := map[string]any{
		"version":            1,
		"codex_executable":   profile.CodexExecutable,
		"federation_rule_id": rule,
		"profile":            profile.Profile,
	}
	options.WorkerCodexFile = writePilotJSON(t, root, "worker-codex.json", workerCodex)

	wif := codexapp.LiveReceipt{
		SchemaVersion:              1,
		CLI:                        "codex-cli",
		Version:                    codexapp.QualifiedCodexVersion,
		BinaryDigest:               profile.Profile.BinaryDigest,
		CredentialSafeConfigDigest: canonical.BytesDigest([]byte("[features]\nshell_tool = false\nview_image = false\n")),
		CredentialMode:             "workload_identity",
		FederationRuleID:           rule,
		Model:                      profile.Profile.Model,
		PromptDigest:               canonical.BytesDigest([]byte(codexapp.LiveProbePrompt)),
		ThreadID:                   "thread-preflight",
		TurnID:                     "turn-preflight",
		TurnStatus:                 "completed",
		Output:                     codexapp.LiveProbeExpected,
		OutputDigest:               canonical.BytesDigest([]byte(codexapp.LiveProbeExpected)),
		ApprovalRequests:           0,
		UnexpectedToolUse:          false,
		AssertionRemovedBeforeTurn: true,
	}
	if err := wif.Validate(); err != nil {
		t.Fatal(err)
	}
	options.WIFReceiptFile = writePilotJSON(t, root, "wif-receipt.json", wif)

	artifactRoot := filepath.Join(root, "preparation-root", "artifacts")
	tokenFile := filepath.Join(root, "publisher-token")
	if err := os.WriteFile(tokenFile, []byte("test-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	git, err = filepath.EvalSymlinks(git)
	if err != nil {
		t.Fatal(err)
	}
	publisher := map[string]any{
		"version":        1,
		"artifact_root":  artifactRoot,
		"git_executable": git,
		"token_file":     tokenFile,
		"targets": []any{
			map[string]any{
				"repository":    "jiying2007/engineering-platform",
				"base_ref":      "main",
				"branch_prefix": "engineering-platform/",
			},
		},
	}
	options.PublisherFile = writePilotJSON(t, root, "publisher.json", publisher)

	result, err := checkPilotPreflight(options)
	if err != nil {
		t.Fatal(err)
	}
	if result.Internal != "READY" || result.ModelExecution != "READY" ||
		result.Publication != "READY" || len(result.Blockers) != 0 {
		t.Fatalf("unexpected full readiness: %#v", result)
	}
}

func TestPilotPreflightRejectsPublisherArtifactViewDrift(t *testing.T) {
	options, profile, rule := pilotPreflightFixture(t)
	root := filepath.Dir(options.ProfileFile)

	workerCodex := map[string]any{
		"version":            1,
		"codex_executable":   profile.CodexExecutable,
		"federation_rule_id": rule,
		"profile":            profile.Profile,
	}
	options.WorkerCodexFile = writePilotJSON(t, root, "worker-codex.json", workerCodex)

	wif := codexapp.LiveReceipt{
		SchemaVersion:              1,
		CLI:                        "codex-cli",
		Version:                    codexapp.QualifiedCodexVersion,
		BinaryDigest:               profile.Profile.BinaryDigest,
		CredentialSafeConfigDigest: canonical.BytesDigest([]byte("[features]\nshell_tool = false\nview_image = false\n")),
		CredentialMode:             "workload_identity",
		FederationRuleID:           rule,
		Model:                      profile.Profile.Model,
		PromptDigest:               canonical.BytesDigest([]byte(codexapp.LiveProbePrompt)),
		ThreadID:                   "thread-preflight-drift",
		TurnID:                     "turn-preflight-drift",
		TurnStatus:                 "completed",
		Output:                     codexapp.LiveProbeExpected,
		OutputDigest:               canonical.BytesDigest([]byte(codexapp.LiveProbeExpected)),
		AssertionRemovedBeforeTurn: true,
	}
	if err := wif.Validate(); err != nil {
		t.Fatal(err)
	}
	options.WIFReceiptFile = writePilotJSON(t, root, "wif-receipt.json", wif)

	wrongRoot := filepath.Join(root, "publisher-artifacts")
	if err := os.Mkdir(wrongRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	tokenFile := filepath.Join(root, "publisher-token")
	if err := os.WriteFile(tokenFile, []byte("test-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	git, err = filepath.EvalSymlinks(git)
	if err != nil {
		t.Fatal(err)
	}
	options.PublisherFile = writePilotJSON(t, root, "publisher-drift.json", map[string]any{
		"version":        1,
		"artifact_root":  wrongRoot,
		"git_executable": git,
		"token_file":     tokenFile,
		"targets": []any{map[string]any{
			"repository":    "jiying2007/engineering-platform",
			"base_ref":      "main",
			"branch_prefix": "engineering-platform/",
		}},
	})

	if _, err := checkPilotPreflight(options); err == nil ||
		!strings.Contains(err.Error(), "retained Worker artifact root") {
		t.Fatalf("publisher artifact-view drift accepted: %v", err)
	}
}

func TestPilotPreflightRejectsMainDrift(t *testing.T) {
	options, _, _ := pilotPreflightFixture(t)
	options.Base = strings.Repeat("f", 40)
	if _, err := checkPilotPreflight(options); err == nil {
		t.Fatal("main/base drift accepted")
	}
}

func pilotPreflightFixture(t *testing.T) (pilotPreflightOptions, codexProfileOutput, string) {
	t.Helper()
	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	git, err = filepath.EvalSymlinks(git)
	if err != nil {
		t.Fatal(err)
	}
	pilotGit(t, git, repository, "init", "-b", "main")
	pilotGit(t, git, repository, "config", "user.email", "pilot@example.invalid")
	pilotGit(t, git, repository, "config", "user.name", "Pilot")
	if err := os.WriteFile(filepath.Join(repository, "fixture.txt"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	pilotGit(t, git, repository, "add", "fixture.txt")
	pilotGit(t, git, repository, "commit", "-m", "fixture")
	base := strings.TrimSpace(pilotGit(t, git, repository, "rev-parse", "HEAD"))

	codex := filepath.Join(root, "codex")
	if err := os.WriteFile(codex, []byte("#!/bin/sh\nprintf '%s\\n' 'codex-cli 0.155.0'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	profile, err := buildCodexProfile(codex, "gpt-5.6-sol")
	if err != nil {
		t.Fatal(err)
	}
	profileFile := writePilotJSON(t, root, "codex-profile.json", profile)

	workerProfile := "worker/codex-pilot"
	policyTemplate := readPilotFixture(t, "access-policy.json.tmpl")
	policyText := strings.ReplaceAll(policyTemplate, "__PROFILE_DIGEST__", profile.ProfileDigest)
	policyText = strings.ReplaceAll(policyText, "__WORKER_PROFILE__", workerProfile)
	policyFile := filepath.Join(root, "access-policy.json")
	if err := os.WriteFile(policyFile, []byte(policyText), 0o600); err != nil {
		t.Fatal(err)
	}

	preparationRoot := filepath.Join(root, "preparation-root")
	contextSource := filepath.Join(root, "context-source")
	for _, dir := range []string{preparationRoot, filepath.Join(preparationRoot, "artifacts"), contextSource} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	contextBytes := []byte("frozen pilot requirement\n")
	contextDigest := canonical.BytesDigest(contextBytes)
	contextName := strings.TrimPrefix(contextDigest, "sha256:") + ".bin"
	if err := os.WriteFile(filepath.Join(contextSource, contextName), contextBytes, 0o400); err != nil {
		t.Fatal(err)
	}

	prep := readPilotFixture(t, "worker-preparation.json.tmpl")
	replacements := map[string]string{
		"__PREPARATION_ROOT__": preparationRoot,
		"__GIT_EXECUTABLE__":   git,
		"__CONTEXT_SOURCE__":   contextSource,
		"__RUN_ID__":           "m1-feature-routing-run",
		"__TASK_DIGEST__":      "sha256:" + strings.Repeat("a", 64),
		"__RUN_INPUT_DIGEST__": "sha256:" + strings.Repeat("b", 64),
		"__REPOSITORY_PATH__":  repository,
		"__CONTEXT_DIGEST__":   contextDigest,
	}
	for key, value := range replacements {
		prep = strings.ReplaceAll(prep, key, value)
	}
	preparationFile := filepath.Join(root, "preparation.json")
	if err := os.WriteFile(preparationFile, []byte(prep), 0o600); err != nil {
		t.Fatal(err)
	}

	return pilotPreflightOptions{
		Repository:      repository,
		Base:            base,
		ProfileFile:     profileFile,
		PolicyFile:      policyFile,
		PreparationFile: preparationFile,
		WorkerProfile:   workerProfile,
	}, profile, "rule-pilot-test"
}

func readPilotFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "pilots", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writePilotJSON(t *testing.T, root, name string, value any) string {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func pilotGit(t *testing.T, git, repository string, args ...string) string {
	t.Helper()
	commandArgs := append([]string{"-C", repository}, args...)
	cmd := exec.Command(git, commandArgs...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}
