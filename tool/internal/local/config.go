package local

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const maxConfigBytes = 1 << 20

var configName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var gitBase = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,254}$`)

// LoadProject accepts exactly one YAML document. Unknown fields are errors so
// a misspelled timeout or runtime cannot silently change execution behavior.
func LoadProject(path, name string) (Project, error) {
	return LoadProjectOverrides(path, name, "", "", "")
}

// LoadProjectOverrides applies explicit registration options before validation.
// A shared file can therefore define checks while the local CLI supplies the
// machine-specific root and runtime. Unknown YAML keys still fail immediately.
func LoadProjectOverrides(path, name, root, base, runtimeName string) (Project, error) {
	var project Project
	if err := decodeConfig(path, &project); err != nil {
		return project, err
	}
	project.Name = name
	if root != "" {
		project.Root = root
	} else if project.Root == "" {
		project.Root = filepath.Dir(path)
	} else if !filepath.IsAbs(project.Root) {
		project.Root = filepath.Join(filepath.Dir(path), project.Root)
	}
	if base != "" {
		project.Base = base
	}
	if runtimeName != "" {
		project.Runtime = runtimeName
	}
	project, err := normalizeProject(project)
	if err != nil {
		return Project{}, err
	}
	return project, project.Validate()
}

func LoadRuntime(path, name string) (Runtime, error) {
	// A pointer here distinguishes an explicit fixture declaration from a
	// missing model field. The exported, frozen representation remains a bool.
	var raw struct {
		Argv    []string `yaml:"argv"`
		Stdin   string   `yaml:"stdin"`
		Timeout string   `yaml:"timeout"`
		Model   *bool    `yaml:"model"`
	}
	if err := decodeConfig(path, &raw); err != nil {
		return Runtime{}, err
	}
	if raw.Model == nil {
		return Runtime{}, fmt.Errorf("runtime must explicitly declare model: true or model: false")
	}
	runtime := normalizeRuntime(Runtime{Name: name, Argv: raw.Argv, Stdin: raw.Stdin, Timeout: raw.Timeout, Model: *raw.Model})
	return runtime, runtime.Validate()
}

func decodeConfig(path string, target any) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect configuration: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxConfigBytes {
		return fmt.Errorf("configuration must be a regular file no larger than %d bytes", maxConfigBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open configuration: %w", err)
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return fmt.Errorf("inspect configuration: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxConfigBytes {
		return fmt.Errorf("configuration must be a regular file no larger than %d bytes", maxConfigBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxConfigBytes+1))
	if err != nil {
		return fmt.Errorf("read configuration: %w", err)
	}
	if len(data) > maxConfigBytes {
		return fmt.Errorf("configuration exceeds size limit")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("configuration must contain exactly one YAML document")
	}
	return nil
}

func normalizeProject(project Project) (Project, error) {
	if project.Root == "" {
		return Project{}, fmt.Errorf("project root is required")
	}
	root, err := filepath.Abs(project.Root)
	if err != nil {
		return Project{}, fmt.Errorf("resolve project root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Project{}, fmt.Errorf("resolve project root: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return Project{}, fmt.Errorf("project root must be an existing directory")
	}
	project.Root = root
	if project.Base == "" {
		project.Base = "main"
	}
	// Copy slices before normalizing, so freezing cannot mutate caller config.
	project.Checks = append([]Check(nil), project.Checks...)
	for i := range project.Checks {
		project.Checks[i].Argv = append([]string(nil), project.Checks[i].Argv...)
		if project.Checks[i].Timeout == "" {
			project.Checks[i].Timeout = "10m"
		}
	}
	project.Artifacts = append([]string(nil), project.Artifacts...)
	return project, nil
}

func normalizeRuntime(runtime Runtime) Runtime {
	runtime.Argv = append([]string(nil), runtime.Argv...)
	if runtime.Stdin == "" {
		runtime.Stdin = "task"
	}
	if runtime.Timeout == "" {
		runtime.Timeout = "30m"
	}
	return runtime
}

func (project Project) Validate() error {
	if !configName.MatchString(project.Name) {
		return fmt.Errorf("invalid project registration name")
	}
	if !filepath.IsAbs(project.Root) || strings.ContainsRune(project.Root, '\x00') {
		return fmt.Errorf("project root must be an absolute local directory")
	}
	if !gitBase.MatchString(project.Base) || strings.Contains(project.Base, "..") || strings.Contains(project.Base, "//") || strings.HasSuffix(project.Base, ".lock") || strings.HasSuffix(project.Base, "/") || strings.HasSuffix(project.Base, ".") {
		return fmt.Errorf("base must be a branch, ref, or exact commit name without revision operators")
	}
	if !configName.MatchString(project.Runtime) {
		return fmt.Errorf("project must name an explicitly configured runtime")
	}
	if len(project.Checks) > 32 || len(project.Artifacts) > 64 {
		return fmt.Errorf("project has too many checks or artifact patterns")
	}
	names := make(map[string]bool)
	for _, check := range project.Checks {
		if !configName.MatchString(check.Name) || names[check.Name] {
			return fmt.Errorf("check names must be nonempty and unique")
		}
		names[check.Name] = true
		if err := validateArgv(check.Argv); err != nil {
			return fmt.Errorf("check %s: %w", check.Name, err)
		}
		if err := validateDuration(check.Timeout); err != nil {
			return fmt.Errorf("check %s: %w", check.Name, err)
		}
	}
	for _, pattern := range project.Artifacts {
		if pattern == "" || len(pattern) > 4096 || filepath.IsAbs(pattern) || strings.ContainsAny(pattern, "\\\x00") {
			return fmt.Errorf("artifact patterns must be relative paths within the task worktree")
		}
		for _, part := range strings.Split(pattern, "/") {
			if part == ".." || part == ".git" {
				return fmt.Errorf("artifact patterns cannot include parent paths or Git metadata")
			}
		}
		if _, err := filepath.Match(pattern, "test"); err != nil {
			return fmt.Errorf("invalid artifact pattern")
		}
	}
	return nil
}

func (runtime Runtime) Validate() error {
	if !configName.MatchString(runtime.Name) {
		return fmt.Errorf("invalid runtime registration name")
	}
	if err := validateArgv(runtime.Argv); err != nil {
		return fmt.Errorf("runtime: %w", err)
	}
	if runtime.Stdin != "task" && runtime.Stdin != "none" {
		return fmt.Errorf("runtime stdin must be task or none")
	}
	return validateDuration(runtime.Timeout)
}

func validateDuration(value string) error {
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 || duration > 24*time.Hour {
		return fmt.Errorf("timeout must be a positive duration no longer than 24h")
	}
	return nil
}

func validateArgv(argv []string) error {
	if len(argv) == 0 || len(argv) > 128 || strings.TrimSpace(argv[0]) == "" {
		return fmt.Errorf("argv must contain an explicit executable and at most 128 arguments")
	}
	bytes := 0
	for _, arg := range argv {
		bytes += len(arg)
		if strings.ContainsRune(arg, '\x00') {
			return fmt.Errorf("argv cannot contain NUL bytes")
		}
		// Configuration is rendered by status/doctor. Known secret-bearing flags
		// must use the runtime's existing private credential store instead.
		lower := strings.ToLower(arg)
		for _, flag := range []string{"--api-key", "--access-token", "--token", "--password", "--secret", "authorization:"} {
			if lower == flag || strings.HasPrefix(lower, flag+"=") || strings.HasPrefix(lower, "authorization:") {
				return fmt.Errorf("credentials must not be placed in recorded argv")
			}
		}
		if parsed, err := url.Parse(arg); err == nil && parsed.Scheme != "" && parsed.User != nil {
			return fmt.Errorf("credential-bearing URLs must not be placed in recorded argv")
		}
	}
	if bytes > 64<<10 {
		return fmt.Errorf("argv exceeds 64 KiB")
	}
	return nil
}

// Freeze deep-copies and validates exactly the settings used by this task.
// It does not execute commands, inspect login credentials, or call a model.
func Freeze(project Project, runtime Runtime) (Snapshot, string, error) {
	project, err := normalizeProject(project)
	if err != nil {
		return Snapshot{}, "", err
	}
	runtime = normalizeRuntime(runtime)
	if err := project.Validate(); err != nil {
		return Snapshot{}, "", err
	}
	if err := runtime.Validate(); err != nil {
		return Snapshot{}, "", err
	}
	if project.Runtime != runtime.Name {
		return Snapshot{}, "", fmt.Errorf("project runtime does not match the selected local runtime")
	}
	snapshot := Snapshot{Project: project, Runtime: runtime}
	digest, err := snapshotDigest(snapshot)
	return snapshot, digest, err
}

func snapshotDigest(snapshot Snapshot) (string, error) {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
