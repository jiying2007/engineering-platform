package codexapp

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	runtimeprovider "github.com/jiying2007/engineering-platform/internal/runtime"
)

type Provider struct {
	executable string
}

func NewProvider(executable string) *Provider {
	if executable == "" {
		executable = "codex"
	}
	return &Provider{executable: executable}
}

func (p *Provider) Name() string {
	return "codex-app-server"
}

func (p *Provider) Command(ctx context.Context, spec runtimeprovider.LaunchSpec) (*exec.Cmd, error) {
	if p == nil || p.executable == "" {
		return nil, fmt.Errorf("codex executable is required")
	}
	if spec.Dir == "" {
		return nil, fmt.Errorf("workspace directory is required")
	}
	absolute, err := filepath.Abs(spec.Dir)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace directory: %w", err)
	}
	home, ok := envValue(spec.Env, "HOME")
	if !ok || strings.TrimSpace(home) == "" {
		return nil, fmt.Errorf("isolated HOME is required for codex app-server")
	}
	home, err = filepath.Abs(home)
	if err != nil {
		return nil, fmt.Errorf("resolve isolated HOME: %w", err)
	}
	if filepath.Clean(home) == filepath.Clean(absolute) ||
		strings.HasPrefix(filepath.Clean(home), filepath.Clean(absolute)+string(filepath.Separator)) {
		return nil, fmt.Errorf("isolated HOME must live outside the source worktree")
	}

	args := append([]string{"app-server"}, spec.Args...)
	cmd := exec.CommandContext(ctx, p.executable, args...)
	cmd.Dir = absolute
	cmd.Env = append(cmd.Environ(), spec.Env...)
	return cmd, nil
}

func envValue(env []string, key string) (string, bool) {
	prefix := key + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return strings.TrimPrefix(env[i], prefix), true
		}
	}
	return "", false
}
