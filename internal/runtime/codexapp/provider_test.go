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
