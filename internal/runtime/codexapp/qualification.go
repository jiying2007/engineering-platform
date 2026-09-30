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
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	runtimeprovider "github.com/jiying2007/engineering-platform/internal/runtime"
)

const CompatibilityContractVersion = 1

var codexVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)

type QualificationReceipt struct {
	SchemaVersion                int    `json:"schema_version"`
	CompatibilityContractVersion int    `json:"compatibility_contract_version"`
	CLI                          string `json:"cli"`
	Version                      string `json:"version"`
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
	EngineeringConfigDigest      string `json:"engineering_config_digest"`
	EngineeringProfileChecked    bool   `json:"engineering_profile_checked"`
}

// Qualify exercises the actual Codex binary without making a model turn.
// Admission is compatibility-based rather than version-pinned: the binary must
// expose the protocol/schema/profile semantics required by this contract. The
// observed version and exact binary digest are retained for provenance.
func Qualify(ctx context.Context, executable, model string) (QualificationReceipt, error) {
	var receipt QualificationReceipt
	if strings.TrimSpace(model) == "" || len(model) > 128 {
		return receipt, fmt.Errorf("qualification requires a bounded model")
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
	version, err := ParseCodexVersionOutput(string(out))
	if err != nil {
		return receipt, fmt.Errorf("unsupported Codex version output %q: %w; stderr=%s", strings.TrimSpace(string(out)), err, strings.TrimSpace(diagnostics))
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
	if err := qualifyEngineeringProfile(ctx, executable, filepath.Join(root, "engineering-home")); err != nil {
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
		SchemaVersion:                2,
		CompatibilityContractVersion: CompatibilityContractVersion,
		CLI:                          "codex-cli",
		Version:                      version,
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
		EngineeringConfigDigest:      EngineeringConfigDigest(),
		EngineeringProfileChecked:    true,
	}
	if err := receipt.Validate(); err != nil {
		return QualificationReceipt{}, err
	}
	return receipt, nil
}

func ValidCodexVersion(version string) bool {
	return len(version) <= 64 && codexVersionPattern.MatchString(version)
}

func ParseCodexVersionOutput(output string) (string, error) {
	const prefix = "codex-cli "
	value := strings.TrimSpace(output)
	if !strings.HasPrefix(value, prefix) {
		return "", fmt.Errorf("expected codex-cli version output")
	}
	version := strings.TrimSpace(strings.TrimPrefix(value, prefix))
	if !ValidCodexVersion(version) {
		return "", fmt.Errorf("invalid Codex semantic version")
	}
	return version, nil
}

func (r QualificationReceipt) Validate() error {
	if r.SchemaVersion != 2 || r.CompatibilityContractVersion != CompatibilityContractVersion ||
		r.CLI != "codex-cli" || !ValidCodexVersion(r.Version) ||
		!canonical.ValidDigest(r.BinaryDigest) || !canonical.ValidDigest(r.StableSchemaDigest) ||
		!canonical.ValidDigest(r.ExperimentalSchemaDigest) || r.Transport != "stdio" ||
		!r.FreshProcess || r.ManagedDaemon || r.PerThreadConfigOverride ||
		!r.InitializePassed || !r.ThreadStartPassed ||
		strings.TrimSpace(r.ThreadStartModel) == "" || len(r.ThreadStartModel) > 128 ||
		!r.StableSchemaContractChecked || !r.ExperimentalSurfaceChecked ||
		r.CredentialSafeConfigDigest != CredentialSafeConfigDigest() ||
		!r.CredentialSafeProfileChecked ||
		r.EngineeringConfigDigest != EngineeringConfigDigest() ||
		!r.EngineeringProfileChecked {
		return fmt.Errorf("invalid Codex compatibility qualification receipt")
	}
	return nil
}

func (r QualificationReceipt) Digest() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	return canonical.Digest(r)
}

// VerifyExecutableQualification re-binds a retained compatibility receipt to
// the exact native Codex executable without starting a model turn. It checks the
// executable bytes, semantic version and exact model identity frozen by the
// qualification receipt.
func VerifyExecutableQualification(ctx context.Context, executable, model string, receipt QualificationReceipt) error {
	if err := receipt.Validate(); err != nil {
		return err
	}
	if !validTextModel(model) || receipt.ThreadStartModel != model {
		return fmt.Errorf("exact qualified model required")
	}
	resolved, err := canonicalPath(executable, false)
	if err != nil {
		return err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("trusted executable required")
	}
	digest, err := executableDigest(resolved)
	if err != nil {
		return err
	}
	if digest != receipt.BinaryDigest {
		return fmt.Errorf("Codex binary digest does not match qualification")
	}
	home, err := os.MkdirTemp("", "engineering-platform-codex-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(home)
	out, diagnostics, err := codexVersion(ctx, resolved, home)
	if err != nil {
		return fmt.Errorf("codex version: %w; stderr=%s", err, strings.TrimSpace(diagnostics))
	}
	version, err := ParseCodexVersionOutput(string(out))
	if err != nil {
		return err
	}
	if version != receipt.Version {
		return fmt.Errorf("Codex version does not match qualification")
	}
	return nil
}

func validTextModel(model string) bool {
	return model != "" && len(model) <= 128 && strings.TrimSpace(model) == model && !strings.ContainsAny(model, "\r\n\x00")
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

func qualifyEngineeringProfile(ctx context.Context, executable, home string) error {
	for _, dir := range []string{home, filepath.Join(home, ".codex"), filepath.Join(home, ".config"), filepath.Join(home, ".cache")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	configPath := filepath.Join(home, ".codex", "config.toml")
	if err := os.WriteFile(configPath, []byte(engineeringConfig), 0o600); err != nil {
		return err
	}
	output, err := codexCommand(ctx, executable, home, "features", "list")
	if err != nil {
		return fmt.Errorf("engineering feature probe: %w: %s", err, strings.TrimSpace(string(output)))
	}
	required := map[string]string{"shell_tool": "true", "view_image": "false"}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if want, ok := required[fields[0]]; ok && fields[1] == "stable" && fields[2] == want {
			delete(required, fields[0])
		}
	}
	if len(required) != 0 {
		return fmt.Errorf("engineering Codex features were not proven: %v", required)
	}
	info, err := os.Stat(configPath)
	if err != nil || info.Mode().Perm() != 0o600 {
		return fmt.Errorf("engineering Codex config is not owner-private")
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
	if err := receipt.Validate(); err != nil {
		return nil, err
	}
	return json.MarshalIndent(receipt, "", "  ")
}
