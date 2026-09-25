package codexapp

import (
	"context"
	runtimeprovider "github.com/jiying2007/engineering-platform/internal/runtime"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func launchFixture(t *testing.T) (*Provider, runtimeprovider.LaunchSpec) {
	t.Helper()
	root := t.TempDir()
	work, home := filepath.Join(root, "work"), filepath.Join(root, "home")
	for _, p := range []string{work, home} {
		if err := os.Mkdir(p, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatal(err)
	}
	return NewProvider(exe), runtimeprovider.LaunchSpec{Dir: work, Env: []string{"HOME=" + home}}
}
func TestProviderDoesNotInheritHostEnvironment(t *testing.T) {
	p, s := launchFixture(t)
	t.Setenv("EP_HOST_SECRET", "must-not-leak")
	t.Setenv("CODEX_HOME", "/host-config")
	t.Setenv("LD_PRELOAD", "/host-injection")
	t.Setenv("HTTPS_PROXY", "http://host-proxy")
	s.Env = append(s.Env, "OPENAI_API_KEY=explicit-test-value", "OPENAI_BASE_URL=https://provider.example/v1")
	cmd, err := p.Command(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cmd.Args[1:], " ") != "app-server --stdio" || cmd.Dir != s.Dir {
		t.Fatal(cmd.Args, cmd.Dir)
	}
	joined := strings.Join(cmd.Env, "\n")
	for _, bad := range []string{"EP_HOST_SECRET=", "/host-config", "LD_PRELOAD=", "HTTPS_PROXY="} {
		if strings.Contains(joined, bad) {
			t.Fatal("inherited host config", bad)
		}
	}
	if !strings.Contains(joined, "OPENAI_API_KEY=explicit-test-value") {
		t.Fatal("explicit credential missing")
	}
}
func TestProviderRejectsUnsafeLaunchInputs(t *testing.T) {
	for _, kind := range []string{"args", "env", "duplicate", "relative", "nested", "alias", "config", "endpoint", "executable"} {
		t.Run(kind, func(t *testing.T) {
			p, s := launchFixture(t)
			home := strings.TrimPrefix(s.Env[0], "HOME=")
			switch kind {
			case "args":
				s.Args = []string{"--dangerously-bypass-approvals-and-sandbox"}
			case "env":
				s.Env = append(s.Env, "GIT_CONFIG_GLOBAL=/host")
			case "duplicate":
				s.Env = append(s.Env, s.Env[0])
			case "relative":
				s.Dir = "."
			case "nested":
				s.Env = []string{"HOME=" + s.Dir}
			case "alias":
				link := filepath.Join(filepath.Dir(home), "alias")
				if err := os.Symlink(home, link); err != nil {
					t.Fatal(err)
				}
				s.Env = []string{"HOME=" + link}
			case "config":
				if err := os.Mkdir(filepath.Join(home, ".codex"), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(home, ".codex", "config.toml"), []byte("unreviewed"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "endpoint":
				s.Env = append(s.Env, "OPENAI_BASE_URL=http://plain.invalid")
			case "executable":
				s.Executable = "/other"
			}
			if _, err := p.Command(context.Background(), s); err == nil {
				t.Fatal("unsafe launch accepted", kind)
			}
		})
	}
	if _, err := NewProvider("codex").Command(context.Background(), runtimeprovider.LaunchSpec{}); err == nil {
		t.Fatal("PATH executable accepted")
	}
}

func launchWIFFixture(t *testing.T) (*Provider, runtimeprovider.LaunchSpec) {
	t.Helper()
	base, spec := launchFixture(t)
	digest, err := executableDigest(base.executable)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewPinnedWIFProvider(base.executable, digest)
	if err != nil {
		t.Fatal(err)
	}
	return provider, spec
}

func workloadIdentityEnv(t *testing.T, spec runtimeprovider.LaunchSpec) []string {
	t.Helper()
	home := strings.TrimPrefix(spec.Env[0], "HOME=")
	secretDir := filepath.Join(filepath.Dir(home), "identity")
	if err := os.Mkdir(secretDir, 0o700); err != nil {
		t.Fatal(err)
	}
	token := filepath.Join(secretDir, "token")
	if err := os.WriteFile(token, []byte("eyJhbGciOiJub25lIn0.fixture.signature"), 0o600); err != nil {
		t.Fatal(err)
	}
	return []string{
		"OPENAI_FEDERATION_RULE_ID=rule-engineering-platform-test",
		"OPENAI_IDENTITY_TOKEN_FILE=" + token,
		`OPENAI_WORKLOAD_IDENTITY_CONTEXT={"run_id":"test-run","worker":"test-worker"}`,
	}
}

func TestProviderAcceptsPrivateWorkloadIdentityWithoutReadingSecret(t *testing.T) {
	p, spec := launchWIFFixture(t)
	spec.Env = append(spec.Env, workloadIdentityEnv(t, spec)...)
	cmd, err := p.Command(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(cmd.Env, "\n")
	for _, required := range []string{
		"OPENAI_FEDERATION_RULE_ID=rule-engineering-platform-test",
		"OPENAI_IDENTITY_TOKEN_FILE=",
		"OPENAI_WORKLOAD_IDENTITY_CONTEXT=",
	} {
		if !strings.Contains(joined, required) {
			t.Fatal("missing workload identity configuration", required)
		}
	}
	if strings.Contains(joined, "eyJhbGciOiJub25lIn0.fixture.signature") {
		t.Fatal("identity token contents leaked into environment")
	}
	if strings.Contains(joined, "OPENAI_API_KEY=") || strings.Contains(joined, "OPENAI_BASE_URL=") {
		t.Fatal("workload identity carried alternate provider credentials")
	}
}

func TestProviderRejectsIncompleteOrUnsafeWorkloadIdentity(t *testing.T) {
	for _, kind := range []string{"rule-only", "token-only", "context-only", "api-key", "base-url", "bad-rule", "public-file", "public-parent", "relative-token", "worktree-token", "bad-context"} {
		t.Run(kind, func(t *testing.T) {
			p, spec := launchWIFFixture(t)
			wif := workloadIdentityEnv(t, spec)
			switch kind {
			case "rule-only":
				spec.Env = append(spec.Env, wif[0])
			case "token-only":
				spec.Env = append(spec.Env, wif[1])
			case "context-only":
				spec.Env = append(spec.Env, wif[2])
			case "api-key":
				spec.Env = append(spec.Env, wif...)
				spec.Env = append(spec.Env, "OPENAI_API_KEY=must-not-coexist")
			case "base-url":
				spec.Env = append(spec.Env, wif...)
				spec.Env = append(spec.Env, "OPENAI_BASE_URL=https://provider.example/v1")
			case "bad-rule":
				spec.Env = append(spec.Env, "OPENAI_FEDERATION_RULE_ID= bad-rule", wif[1])
			case "public-file":
				token := strings.TrimPrefix(wif[1], "OPENAI_IDENTITY_TOKEN_FILE=")
				if err := os.Chmod(token, 0o644); err != nil {
					t.Fatal(err)
				}
				spec.Env = append(spec.Env, wif[:2]...)
			case "public-parent":
				token := strings.TrimPrefix(wif[1], "OPENAI_IDENTITY_TOKEN_FILE=")
				if err := os.Chmod(filepath.Dir(token), 0o755); err != nil {
					t.Fatal(err)
				}
				spec.Env = append(spec.Env, wif[:2]...)
			case "relative-token":
				spec.Env = append(spec.Env, wif[0], "OPENAI_IDENTITY_TOKEN_FILE=relative.jwt")
			case "worktree-token":
				token := filepath.Join(spec.Dir, "identity-token")
				if err := os.WriteFile(token, []byte("eyJhbGciOiJub25lIn0.fixture.signature"), 0o600); err != nil {
					t.Fatal(err)
				}
				spec.Env = append(spec.Env, wif[0], "OPENAI_IDENTITY_TOKEN_FILE="+token)
			case "bad-context":
				spec.Env = append(spec.Env, wif[:2]...)
				spec.Env = append(spec.Env, `OPENAI_WORKLOAD_IDENTITY_CONTEXT={"Bad-Key":"x"}`)
			}
			_, err := p.Command(context.Background(), spec)
			if err == nil {
				t.Fatal("unsafe workload identity accepted", kind)
			}
			if strings.Contains(err.Error(), "eyJhbGciOiJub25l") {
				t.Fatal("identity token contents leaked in error")
			}
		})
	}
}

func TestRejectedCredentialConfigurationLeavesRuntimeHomeFresh(t *testing.T) {
	p, spec := launchWIFFixture(t)
	home := strings.TrimPrefix(spec.Env[0], "HOME=")
	spec.Env = append(spec.Env, "OPENAI_FEDERATION_RULE_ID=rule-incomplete")
	if _, err := p.Command(context.Background(), spec); err == nil {
		t.Fatal("incomplete workload identity accepted")
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("rejected launch polluted runtime HOME: %#v", entries)
	}
}


func TestNormalProviderRejectsWorkloadIdentity(t *testing.T) {
	p, spec := launchFixture(t)
	spec.Env = append(spec.Env, workloadIdentityEnv(t, spec)...)
	if _, err := p.Command(context.Background(), spec); err == nil || !strings.Contains(err.Error(), "credential-safe") {
		t.Fatalf("normal provider accepted workload identity: %v", err)
	}
}

func TestWIFProviderWritesOnlyFixedCredentialSafeConfig(t *testing.T) {
	p, spec := launchWIFFixture(t)
	spec.Env = append(spec.Env, workloadIdentityEnv(t, spec)...)
	if _, err := p.Command(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	home := strings.TrimPrefix(spec.Env[0], "HOME=")
	path := filepath.Join(home, ".codex", "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != credentialSafeConfig {
		t.Fatalf("unexpected credential-safe config: %q", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credential-safe config mode=%#o", info.Mode().Perm())
	}
	if strings.Contains(string(data), "OPENAI_") || strings.Contains(string(data), "identity") {
		t.Fatal("credential data leaked into Codex config")
	}
}
