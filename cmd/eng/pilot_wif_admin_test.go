package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWIFAdminAPIHelperCreatesExactProviderAndRule(t *testing.T) {
	root, output, errText := runWIFAdminHelper(t, "create", true)
	if errText != "" {
		t.Fatal(errText)
	}

	data, err := os.ReadFile(filepath.Join(output, "wif-admin-receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt map[string]any
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt["version"] != float64(1) ||
		receipt["provider_id"] != "idp_test" ||
		receipt["federation_rule_id"] != "idpm_test" ||
		receipt["workspace_id"] != "ws_test" ||
		receipt["principal_id"] != "usr_test" ||
		receipt["audience"] != "aud-pilot" ||
		receipt["repository"] != "jiying2007/engineering-platform" ||
		receipt["ref"] != "refs/heads/main" {
		t.Fatalf("unexpected WIF admin receipt: %#v", receipt)
	}
	condition, _ := receipt["condition"].(string)
	if !strings.Contains(condition, "codex-wif-live.yml@refs/heads/main") ||
		!strings.Contains(condition, "retained-pilot-engineer.yml@refs/heads/main") {
		t.Fatalf("unexpected workflow allow-list: %q", condition)
	}

	if _, err := os.Stat(filepath.Join(output, ".openai-admin-header")); !os.IsNotExist(err) {
		t.Fatal("temporary Admin API header file was not removed")
	}
	curlLog, err := os.ReadFile(filepath.Join(root, "curl.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(curlLog), "test-admin-key") {
		t.Fatal("Admin API key leaked into curl argv")
	}
	count, err := os.ReadFile(filepath.Join(root, "curl.count"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(count)) != "4" {
		t.Fatalf("unexpected Admin API call count %q", count)
	}
	ghLog, err := os.ReadFile(filepath.Join(root, "gh.log"))
	if err != nil {
		t.Fatal(err)
	}
	ghText := string(ghLog)
	if !strings.Contains(ghText, "variable set OPENAI_WIF_AUDIENCE") ||
		!strings.Contains(ghText, "variable set OPENAI_CODEX_FEDERATION_RULE_ID") {
		t.Fatalf("GitHub variable handoff missing: %s", ghText)
	}
}

func TestWIFAdminAPIHelperReusesExactResources(t *testing.T) {
	root, output, errText := runWIFAdminHelper(t, "existing", false)
	if errText != "" {
		t.Fatal(errText)
	}
	if _, err := os.Stat(filepath.Join(output, "wif-admin-receipt.json")); err != nil {
		t.Fatal(err)
	}
	count, err := os.ReadFile(filepath.Join(root, "curl.count"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(count)) != "2" {
		t.Fatalf("idempotent reuse unexpectedly mutated Admin API: calls=%q", count)
	}
}

func TestWIFAdminAPIHelperRejectsExistingTrustDrift(t *testing.T) {
	_, _, errText := runWIFAdminHelper(t, "drift", false)
	if errText == "" || !strings.Contains(errText, "does not match required trust policy") {
		t.Fatalf("existing trust drift was not rejected: %s", errText)
	}
}

func TestWIFAdminAPIHelperShellSyntax(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	path := filepath.Join("..", "..", "examples", "pilots", "wif", "configure-admin-api.sh")
	if out, err := exec.Command(bash, "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("configure-admin-api.sh syntax: %v: %s", err, out)
	}
}

func runWIFAdminHelper(t *testing.T, mode string, setGitHubVariables bool) (string, string, string) {
	t.Helper()
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash unavailable")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq unavailable")
	}

	root := t.TempDir()
	fakeBin := filepath.Join(root, "bin")
	if err := os.Mkdir(fakeBin, 0o700); err != nil {
		t.Fatal(err)
	}
	for sourceName, targetName := range map[string]string{
		"fake-wif-admin-curl.sh": "curl",
		"fake-wif-admin-gh.sh":   "gh",
	} {
		source := filepath.Join("testdata", sourceName)
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fakeBin, targetName), data, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	script, err := filepath.Abs(filepath.Join("..", "..", "examples", "pilots", "wif", "configure-admin-api.sh"))
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "output")
	env := append(os.Environ(),
		"PATH="+fakeBin+":"+os.Getenv("PATH"),
		"OPENAI_ADMIN_KEY=test-admin-key",
		"WORKSPACE_ID=ws_test",
		"PRINCIPAL_ID=usr_test",
		"OPENAI_WIF_AUDIENCE=aud-pilot",
		"FAKE_CURL_LOG="+filepath.Join(root, "curl.log"),
		"FAKE_CURL_COUNT="+filepath.Join(root, "curl.count"),
		"FAKE_GH_LOG="+filepath.Join(root, "gh.log"),
		"FAKE_WIF_MODE="+mode,
	)
	if setGitHubVariables {
		env = append(env, "SET_GITHUB_VARIABLES=1")
	}
	cmd := exec.Command(bash, script, output)
	cmd.Env = env
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return root, output, string(out)
	}
	return root, output, ""
}
