package codexapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	runtimeprovider "github.com/jiying2007/engineering-platform/internal/runtime"
)

type Provider struct {
	executable string
	digest     string
}

func NewProvider(executable string) *Provider { return &Provider{executable: executable} }

// NewPinnedProvider binds every launch to the exact bytes qualified by the host.
// This does not replace executable ownership or OS isolation checks.
func NewPinnedProvider(executable, digest string) (*Provider, error) {
	if !canonical.ValidDigest(digest) {
		return nil, fmt.Errorf("valid executable digest required")
	}
	return &Provider{executable: executable, digest: digest}, nil
}
func (p *Provider) Name() string { return "codex-app-server" }

// Command is a narrow launch policy, not an OS sandbox. Host-controlled absolute
// paths, separate identities/read-only mounts and resource limits remain required.
// Credentials are explicit inputs; no parent environment is inherited.
func (p *Provider) Command(ctx context.Context, spec runtimeprovider.LaunchSpec) (*exec.Cmd, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p == nil || !filepath.IsAbs(p.executable) || len(spec.Args) != 0 || spec.Executable != "" {
		return nil, fmt.Errorf("absolute configured executable required; launch overrides are forbidden")
	}
	executable, err := canonicalPath(p.executable, false)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(executable)
	if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0o022 != 0 || fi.Mode().Perm()&0o111 == 0 {
		return nil, fmt.Errorf("trusted executable required")
	}
	if p.digest != "" {
		actual, err := executableDigest(executable)
		if err != nil {
			return nil, err
		}
		if actual != p.digest {
			return nil, fmt.Errorf("qualified executable bytes changed")
		}
	}
	work, err := canonicalPath(spec.Dir, true)
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, value := range spec.Env {
		key, val, ok := strings.Cut(value, "=")
		if !ok || strings.ContainsRune(val, 0) {
			return nil, fmt.Errorf("invalid runtime environment")
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("duplicate runtime environment key")
		}
		switch key {
		case "HOME", "OPENAI_API_KEY", "OPENAI_BASE_URL":
		default:
			return nil, fmt.Errorf("runtime environment key is not allowlisted")
		}
		if strings.TrimSpace(val) == "" {
			return nil, fmt.Errorf("empty runtime environment value")
		}
		values[key] = val
	}
	home, err := canonicalPath(values["HOME"], true)
	if err != nil {
		return nil, err
	}
	if overlaps(home, work) {
		return nil, fmt.Errorf("isolated HOME and worktree must be disjoint")
	}
	info, err := os.Stat(home)
	if err != nil || info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("isolated HOME must be owner-only")
	}
	// A freshly allocated HOME cannot import stale provider/XDG configuration.
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		return nil, fmt.Errorf("fresh empty isolated HOME required")
	}
	env := []string{"PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C.UTF-8", "TZ=UTC", "HOME=" + home, "CODEX_HOME=" + filepath.Join(home, ".codex"), "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache")}
	if key, ok := values["OPENAI_API_KEY"]; ok {
		env = append(env, "OPENAI_API_KEY="+key)
	}
	if base, ok := values["OPENAI_BASE_URL"]; ok {
		u, e := url.Parse(base)
		if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, fmt.Errorf("explicit provider endpoint must be HTTPS without credentials or query")
		}
		env = append(env, "OPENAI_BASE_URL="+base)
	}
	// Current Codex documents --stdio as the explicit equivalent of
	// --listen stdio://. A new process is launched for every qualified session;
	// the managed daemon/proxy path is intentionally not used.
	cmd := exec.CommandContext(ctx, executable, "app-server", "--stdio")
	cmd.Dir = work
	cmd.Env = env
	return cmd, nil
}

func executableDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
func canonicalPath(path string, directory bool) (string, error) {
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute existing path required")
	}
	clean := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return "", err
	}
	if resolved != clean {
		return "", fmt.Errorf("symlink path alias is not allowed")
	}
	info, err := os.Stat(clean)
	if err != nil {
		return "", err
	}
	if directory && !info.IsDir() {
		return "", fmt.Errorf("directory required")
	}
	return clean, nil
}
func overlaps(a, b string) bool { return inside(a, b) || inside(b, a) }
func inside(a, b string) bool {
	rel, err := filepath.Rel(a, b)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}
