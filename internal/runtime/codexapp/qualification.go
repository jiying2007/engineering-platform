package codexapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	runtimeprovider "github.com/jiying2007/engineering-platform/internal/runtime"
)

const (
	QualifiedCodexVersion       = "0.155.0"
	QualifiedCodexReleaseTag    = "rust-v0.155.0"
	QualifiedCodexReleaseCommit = "f0a1b8f"
)

type QualificationReceipt struct {
	SchemaVersion                int    `json:"schema_version"`
	CLI                          string `json:"cli"`
	Version                      string `json:"version"`
	ReleaseTag                   string `json:"release_tag"`
	ReleaseCommit                string `json:"release_commit"`
	BinaryDigest                 string `json:"binary_digest"`
	StableSchemaDigest           string `json:"stable_schema_digest"`
	ExperimentalSchemaDigest     string `json:"experimental_schema_digest"`
	Transport                    string `json:"transport"`
	FreshProcess                 bool   `json:"fresh_process"`
	ManagedDaemon                bool   `json:"managed_daemon"`
	PerThreadConfigOverride      bool   `json:"per_thread_config_override"`
	InitializePassed             bool   `json:"initialize_passed"`
	ThreadStartPassed            bool   `json:"thread_start_passed"`
	ThreadStartModel             string `json:"thread_start_model"`
	StableSchemaContractChecked  bool   `json:"stable_schema_contract_checked"`
	ExperimentalSurfaceChecked   bool   `json:"experimental_surface_checked"`
	CredentialSafeConfigDigest   string `json:"credential_safe_config_digest"`
	CredentialSafeProfileChecked bool   `json:"credential_safe_profile_checked"`
}

// Qualify exercises a real Codex binary without making a model turn. It verifies
// exact CLI version, hashes executable bytes, generates both stable/experimental
// app-server schemas, checks the fields our adapter depends on, then starts a
// fresh stdio app-server and completes initialize + ephemeral thread/start.
// No daemon, proxy, per-thread config override, credential, turn or tool runs.
func Qualify(ctx context.Context, executable, expectedVersion, model string) (QualificationReceipt, error) {
	var receipt QualificationReceipt
	if expectedVersion != QualifiedCodexVersion || strings.TrimSpace(model) == "" || len(model) > 128 {
		return receipt, fmt.Errorf("qualification requires the repository-pinned Codex version and model")
	}
	executable, err := canonicalPath(executable, false)
	if err != nil {
		return receipt, err
	}
	info, err := os.Stat(executable)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || info.Mode().Perm()&0o111 == 0 {
		return receipt, fmt.Errorf("trusted executable required")
	}
	digest, err := executableDigest(executable)
	if err != nil {
		return receipt, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return receipt, err
	}
	root, err := os.MkdirTemp(cwd, ".engineering-platform-codex-qualification-")
	if err != nil {
		return receipt, err
	}
	defer os.RemoveAll(root)

	versionHome := filepath.Join(root, "version-home")
	if err := os.Mkdir(versionHome, 0o700); err != nil {
		return receipt, err
	}
	out, diagnostics, err := codexVersion(ctx, executable, versionHome)
	if err != nil {
		return receipt, fmt.Errorf("codex version: %w; stderr=%s", err, strings.TrimSpace(diagnostics))
	}
	if got, want := strings.TrimSpace(string(out)), "codex-cli "+expectedVersion; got != want {
		return receipt, fmt.Errorf("unexpected Codex version %q want %q; stderr=%s", got, want, strings.TrimSpace(diagnostics))
	}

	stable := filepath.Join(root, "stable")
	experimental := filepath.Join(root, "experimental")
	for _, dir := range []string{stable, experimental} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			return receipt, err
		}
	}
	if err := generateSchema(ctx, executable, filepath.Join(root, "stable-home"), stable, false); err != nil {
		return receipt, err
	}
	if err := generateSchema(ctx, executable, filepath.Join(root, "experimental-home"), experimental, true); err != nil {
		return receipt, err
	}
	stableDigest, err := validateSchemaTree(stable, false)
	if err != nil {
		return receipt, fmt.Errorf("stable schema: %w", err)
	}
	experimentalDigest, err := validateSchemaTree(experimental, true)
	if err != nil {
		return receipt, fmt.Errorf("experimental schema: %w", err)
	}

	if err := qualifyCredentialSafeProfile(ctx, executable, filepath.Join(root, "credential-safe-home")); err != nil {
		return receipt, err
	}

	probeRoot := filepath.Join(root, "probe")
	work := filepath.Join(probeRoot, "work")
	home := filepath.Join(probeRoot, "home")
	for _, dir := range []string{work, home} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return receipt, err
		}
	}
	provider, err := NewPinnedProvider(executable, digest)
	if err != nil {
		return receipt, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	cmd, err := provider.Command(probeCtx, runtimeprovider.LaunchSpec{Dir: work, Env: []string{"HOME=" + home}})
	if err != nil {
		return receipt, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return receipt, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return receipt, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &boundedWriter{writer: &stderr, remaining: 64 << 10}
	if err := cmd.Start(); err != nil {
		return receipt, err
	}
	client := NewClient(stdout, stdin)
	defer client.Close()
	adapter, err := NewAdapter(client, work)
	if err != nil {
		cancel()
		_ = cmd.Wait()
		return receipt, err
	}
	if err := adapter.Initialize(probeCtx, "engineering-platform-codex-qualification-v1"); err != nil {
		cancel()
		_ = cmd.Wait()
		return receipt, fmt.Errorf("real app-server initialize failed: %w; stderr=%s", err, strings.TrimSpace(stderr.String()))
	}
	if _, err := adapter.StartThread(probeCtx, model); err != nil {
		cancel()
		_ = cmd.Wait()
		return receipt, fmt.Errorf("real app-server thread/start failed: %w; stderr=%s", err, strings.TrimSpace(stderr.String()))
	}
	_ = client.Close()
	cancel()
	_ = cmd.Wait()

	receipt = QualificationReceipt{
		SchemaVersion:                1,
		CLI:                          "codex-cli",
		Version:                      expectedVersion,
		ReleaseTag:                   QualifiedCodexReleaseTag,
		ReleaseCommit:                QualifiedCodexReleaseCommit,
		BinaryDigest:                 digest,
		StableSchemaDigest:           stableDigest,
		ExperimentalSchemaDigest:     experimentalDigest,
		Transport:                    "stdio",
		FreshProcess:                 true,
		ManagedDaemon:                false,
		PerThreadConfigOverride:      false,
		InitializePassed:             true,
		ThreadStartPassed:            true,
		ThreadStartModel:             model,
		StableSchemaContractChecked:  true,
		ExperimentalSurfaceChecked:   true,
		CredentialSafeConfigDigest:   canonical.BytesDigest([]byte(credentialSafeConfig)),
		CredentialSafeProfileChecked: true,
	}
	return receipt, nil
}

func qualifyCredentialSafeProfile(ctx context.Context, executable, home string) error {
	for _, dir := range []string{home, filepath.Join(home, ".codex"), filepath.Join(home, ".config"), filepath.Join(home, ".cache")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	configPath := filepath.Join(home, ".codex", "config.toml")
	if err := os.WriteFile(configPath, []byte(credentialSafeConfig), 0o600); err != nil {
		return err
	}
	output, err := codexCommand(ctx, executable, home, "features", "list")
	if err != nil {
		return fmt.Errorf("credential-safe feature probe: %w: %s", err, strings.TrimSpace(string(output)))
	}
	required := map[string]bool{"shell_tool": false, "view_image": false}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if _, ok := required[fields[0]]; ok && fields[1] == "stable" && fields[2] == "false" {
			required[fields[0]] = true
		}
	}
	for feature, disabled := range required {
		if !disabled {
			return fmt.Errorf("credential-safe Codex feature %s was not proven disabled", feature)
		}
	}
	info, err := os.Stat(configPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		return fmt.Errorf("credential-safe Codex config is not owner-private")
	}
	return nil
}

func codexVersion(ctx context.Context, executable, home string) ([]byte, string, error) {
	for _, dir := range []string{home, filepath.Join(home, ".codex"), filepath.Join(home, ".config"), filepath.Join(home, ".cache")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, "", err
		}
	}
	cmd := exec.CommandContext(ctx, executable, "--version")
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"LANG=C.UTF-8",
		"TZ=UTC",
		"HOME=" + home,
		"CODEX_HOME=" + filepath.Join(home, ".codex"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
	}
	var stderr bytes.Buffer
	cmd.Stderr = &boundedWriter{writer: &stderr, remaining: 64 << 10}
	stdout, err := cmd.Output()
	return stdout, stderr.String(), err
}

func generateSchema(ctx context.Context, executable, home, out string, experimental bool) error {
	if err := os.Mkdir(home, 0o700); err != nil {
		return err
	}
	args := []string{"app-server", "generate-json-schema", "--out", out}
	if experimental {
		args = append(args, "--experimental")
	}
	output, err := codexCommand(ctx, executable, home, args...)
	if err != nil {
		return fmt.Errorf("generate app-server schema: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func codexCommand(ctx context.Context, executable, home string, args ...string) ([]byte, error) {
	for _, dir := range []string{home, filepath.Join(home, ".codex"), filepath.Join(home, ".config"), filepath.Join(home, ".cache")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"LANG=C.UTF-8",
		"TZ=UTC",
		"HOME=" + home,
		"CODEX_HOME=" + filepath.Join(home, ".codex"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
	}
	return cmd.CombinedOutput()
}

func validateSchemaTree(root string, experimental bool) (string, error) {
	files := []string{}
	var threadStart []byte
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if filepath.Ext(path) != ".json" {
			return fmt.Errorf("unexpected non-JSON schema file %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, rel)
		if filepath.Base(path) == "ThreadStartParams.json" {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			threadStart = data
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	if len(files) == 0 || len(threadStart) == 0 {
		return "", fmt.Errorf("generated schema is missing ThreadStartParams.json")
	}
	if !bytes.Contains(threadStart, []byte(`"approvalPolicy"`)) || !bytes.Contains(threadStart, []byte(`"sandbox"`)) {
		return "", fmt.Errorf("thread/start stable fields missing")
	}
	hasPermissions := bytes.Contains(threadStart, []byte(`"permissions"`))
	if experimental != hasPermissions {
		return "", fmt.Errorf("experimental permissions surface mismatch")
	}

	h := sha256.New()
	required := map[string]bool{
		`"read-only"`:          false,
		`"workspace-write"`:    false,
		`"danger-full-access"`: false,
		`"never"`:              false,
		`"on-request"`:         false,
	}
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return "", err
		}
		for token := range required {
			if bytes.Contains(data, []byte(token)) {
				required[token] = true
			}
		}
		_, _ = io.WriteString(h, rel)
		_, _ = h.Write([]byte{0})
		var length [8]byte
		n := uint64(len(data))
		for i := 7; i >= 0; i-- {
			length[i] = byte(n)
			n >>= 8
		}
		_, _ = h.Write(length[:])
		_, _ = h.Write(data)
	}
	for token, found := range required {
		if !found {
			return "", fmt.Errorf("stable protocol token %s missing", token)
		}
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

type boundedWriter struct {
	writer    io.Writer
	remaining int
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	if len(p) > w.remaining {
		p = p[:w.remaining]
	}
	if len(p) == 0 {
		return 0, fmt.Errorf("diagnostic output limit exceeded")
	}
	n, err := w.writer.Write(p)
	w.remaining -= n
	return n, err
}

// MarshalQualification is intentionally deterministic so CI can retain an exact
// machine-readable receipt without timestamps or host paths.
func MarshalQualification(receipt QualificationReceipt) ([]byte, error) {
	return json.MarshalIndent(receipt, "", "  ")
}
