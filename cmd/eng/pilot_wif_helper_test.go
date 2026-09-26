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

func TestWIFQualificationHelperHandoffWithFakeGitHub(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq unavailable")
	}

	root := t.TempDir()
	repository := filepath.Join(root, "repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	git, err := exec.LookPath("git")
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
	mainSHA := strings.TrimSpace(pilotGit(t, git, repository, "rev-parse", "HEAD"))

	codex := filepath.Join(root, "codex")
	if err := os.WriteFile(codex, []byte("#!/bin/sh\nprintf '%s\\n' 'codex-cli 0.155.0'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	profile, err := buildCodexProfile(codex, "gpt-5.6-sol")
	if err != nil {
		t.Fatal(err)
	}
	profileFile := writePilotJSON(t, root, "codex-profile.json", profile)

	const rule = "rule-pilot-test"
	receipt := codexapp.LiveReceipt{
		SchemaVersion:              1,
		CLI:                        "codex-cli",
		Version:                    codexapp.QualifiedCodexVersion,
		BinaryDigest:               profile.Profile.BinaryDigest,
		CredentialSafeConfigDigest: canonical.BytesDigest([]byte("[features]\nshell_tool = false\nview_image = false\n")),
		CredentialMode:             "workload_identity",
		FederationRuleID:           rule,
		Model:                      profile.Profile.Model,
		PromptDigest:               canonical.BytesDigest([]byte(codexapp.LiveProbePrompt)),
		ThreadID:                   "thread-wif-helper",
		TurnID:                     "turn-wif-helper",
		TurnStatus:                 "completed",
		Output:                     codexapp.LiveProbeExpected,
		OutputDigest:               canonical.BytesDigest([]byte(codexapp.LiveProbeExpected)),
		ApprovalRequests:           0,
		UnexpectedToolUse:          false,
		AssertionRemovedBeforeTurn: true,
	}
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
	receiptFile := writePilotJSON(t, root, "fake-live-receipt.json", receipt)

	fakeBin := filepath.Join(root, "bin")
	if err := os.Mkdir(fakeBin, 0o700); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(root, "gh.log")
	gh := filepath.Join(fakeBin, "gh")
	fake := `#!/usr/bin/env bash
set -euo pipefail
printf '%s
' "$*" >> "$FAKE_GH_LOG"

if [ "$1" = api ]; then
  printf '%s
' "$FAKE_MAIN_SHA"
  exit 0
fi
if [ "$1" = variable ] && [ "$2" = set ]; then
  exit 0
fi
if [ "$1" = workflow ] && [ "$2" = run ]; then
  exit 0
fi
if [ "$1" = run ] && [ "$2" = list ]; then
  printf '%s
' "424242"
  exit 0
fi
if [ "$1" = run ] && [ "$2" = watch ]; then
  exit 0
fi
if [ "$1" = run ] && [ "$2" = download ]; then
  dest=""
  while [ "$#" -gt 0 ]; do
    if [ "$1" = --dir ]; then
      dest="$2"
      break
    fi
    shift
  done
  test -n "$dest"
  mkdir -p "$dest"
  cp "$FAKE_WIF_RECEIPT" "$dest/codex-wif-live-receipt.json"
  exit 0
fi

echo "unexpected fake gh invocation: $*" >&2
exit 1
`
	if err := os.WriteFile(gh, []byte(fake), 0o700); err != nil {
		t.Fatal(err)
	}

	script, err := filepath.Abs(filepath.Join("..", "..", "examples", "pilots", "wif", "qualify.sh"))
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(root, "output")
	cmd := exec.Command(bash, script, "audience-pilot-test", rule, profileFile, outputDir, "gpt-5.6-sol")
	cmd.Dir = repository
	cmd.Env = append(os.Environ(),
		"PATH="+fakeBin+":"+os.Getenv("PATH"),
		"FAKE_GH_LOG="+logFile,
		"FAKE_MAIN_SHA="+mainSHA,
		"FAKE_WIF_RECEIPT="+receiptFile,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("qualification helper failed: %v: %s", err, out)
	}

	gotReceipt := filepath.Join(outputDir, "codex-wif-live-receipt.json")
	data, err := os.ReadFile(gotReceipt)
	if err != nil {
		t.Fatal(err)
	}
	var got codexapp.LiveReceipt
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.FederationRuleID != rule || got.Model != "gpt-5.6-sol" || got.BinaryDigest != profile.Profile.BinaryDigest {
		t.Fatalf("unexpected retained helper receipt: %#v", got)
	}

	workerData, err := os.ReadFile(filepath.Join(outputDir, "worker-codex.json"))
	if err != nil {
		t.Fatal(err)
	}
	var worker pilotWorkerCodexConfig
	if err := json.Unmarshal(workerData, &worker); err != nil {
		t.Fatal(err)
	}
	if worker.Version != 1 || worker.FederationRuleID != rule ||
		worker.Executable != profile.CodexExecutable || worker.Profile != profile.Profile {
		t.Fatalf("unexpected rendered Worker Codex config: %#v", worker)
	}

	logData, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logData)
	for _, want := range []string{
		"variable set OPENAI_WIF_AUDIENCE",
		"variable set OPENAI_CODEX_FEDERATION_RULE_ID",
		"workflow run codex-wif-live.yml",
		"run list",
		"run watch 424242",
		"run download 424242",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("fake gh log missing %q: %s", want, logText)
		}
	}
}

func TestWIFQualificationHelperShellSyntax(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	path := filepath.Join("..", "..", "examples", "pilots", "wif", "qualify.sh")
	if out, err := exec.Command(bash, "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("qualify.sh syntax: %v: %s", err, out)
	}
}
