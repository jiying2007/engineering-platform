package production

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	ConfigVersion   = 1
	DeploymentMode  = "ubuntu-systemd-v1"
	StateReady      = "READY"
	StateBlocked    = "BLOCKED"
	StatePending    = "PENDING"
	OverallInternal = "INTERNAL_BLOCKED"
	OverallProvider = "PROVIDER_PENDING"
)

var serviceUserPattern = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
var envKeyPattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

type Config struct {
	Version                int    `json:"version"`
	DeploymentMode         string `json:"deployment_mode"`
	ControlBinary          string `json:"control_binary"`
	WorkerBinary           string `json:"worker_binary"`
	EngBinary              string `json:"eng_binary"`
	PublisherBinary        string `json:"publisher_binary"`
	ControlEnvFile         string `json:"control_env_file"`
	AdmissionEnvFile       string `json:"admission_env_file"`
	PreparationEnvFile     string `json:"preparation_env_file"`
	PublisherEnvFile       string `json:"publisher_env_file"`
	BackupDirectory        string `json:"backup_directory"`
	ControlServiceUser     string `json:"control_service_user"`
	AdmissionServiceUser   string `json:"admission_service_user"`
	PreparationServiceUser string `json:"preparation_service_user"`
	PublisherServiceUser   string `json:"publisher_service_user"`
}

type Result struct {
	Version           int      `json:"version"`
	DeploymentMode    string   `json:"deployment_mode"`
	ControlPlane      string   `json:"control_plane"`
	WorkerAdmission   string   `json:"worker_admission"`
	WorkerPreparation string   `json:"worker_preparation"`
	Publisher         string   `json:"publisher"`
	Provider          string   `json:"provider"`
	Internal          string   `json:"internal"`
	Overall           string   `json:"overall"`
	Blockers          []string `json:"blockers,omitempty"`
}

func Check(config Config) (Result, error) {
	result := Result{
		Version: ConfigVersion, DeploymentMode: DeploymentMode,
		ControlPlane: StateBlocked, WorkerAdmission: StateBlocked, WorkerPreparation: StateBlocked,
		Publisher: StateBlocked, Provider: StatePending, Internal: StateBlocked, Overall: OverallInternal,
	}
	if config.Version != ConfigVersion || config.DeploymentMode != DeploymentMode {
		return result, fmt.Errorf("production preflight requires config version 1 and ubuntu-systemd-v1")
	}
	if err := validateServiceUsers(config); err != nil {
		return result, err
	}
	for name, path := range map[string]string{
		"control_binary":   config.ControlBinary,
		"worker_binary":    config.WorkerBinary,
		"eng_binary":       config.EngBinary,
		"publisher_binary": config.PublisherBinary,
	} {
		if err := safeExecutable(path); err != nil {
			return result, fmt.Errorf("%s: %w", name, err)
		}
	}
	if err := safeDirectory(config.BackupDirectory); err != nil {
		return result, fmt.Errorf("backup_directory: %w", err)
	}

	control, err := readEnvFile(config.ControlEnvFile)
	if err != nil {
		return result, fmt.Errorf("control_env_file: %w", err)
	}
	if err := validateControlEnv(control); err != nil {
		return result, err
	}
	result.ControlPlane = StateReady

	admission, err := readEnvFile(config.AdmissionEnvFile)
	if err != nil {
		return result, fmt.Errorf("admission_env_file: %w", err)
	}
	if err := validateWorkerEnv(admission, false); err != nil {
		return result, fmt.Errorf("admission worker environment: %w", err)
	}
	result.WorkerAdmission = StateReady

	preparation, err := readEnvFile(config.PreparationEnvFile)
	if err != nil {
		return result, fmt.Errorf("preparation_env_file: %w", err)
	}
	if err := validateWorkerEnv(preparation, true); err != nil {
		return result, fmt.Errorf("preparation worker environment: %w", err)
	}
	result.WorkerPreparation = StateReady

	publisher, err := readEnvFile(config.PublisherEnvFile)
	if err != nil {
		return result, fmt.Errorf("publisher_env_file: %w", err)
	}
	if err := validatePublisherEnv(publisher); err != nil {
		return result, fmt.Errorf("publisher environment: %w", err)
	}
	result.Publisher = StateReady
	result.Internal = StateReady
	result.Overall = OverallProvider
	result.Blockers = append(result.Blockers, "unattended_provider_live_qualification")
	return result, nil
}

func validateServiceUsers(c Config) error {
	users := []string{c.ControlServiceUser, c.AdmissionServiceUser, c.PreparationServiceUser, c.PublisherServiceUser}
	seen := map[string]bool{}
	for _, user := range users {
		if !serviceUserPattern.MatchString(user) || user == "root" || seen[user] {
			return fmt.Errorf("control/admission/preparation/publisher require distinct non-root service users")
		}
		seen[user] = true
	}
	return nil
}

func safeExecutable(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("absolute normalized executable required")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return fmt.Errorf("executable symlink alias is not allowed")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("safe non-group/world-writable executable required")
	}
	return nil
}

func safeDirectory(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("absolute normalized directory required")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return fmt.Errorf("directory symlink alias is not allowed")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("safe non-group/world-writable directory required")
	}
	return nil
}

func readEnvFile(path string) (map[string]string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fmt.Errorf("absolute normalized env file required")
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm()&0o077 != 0 || before.Size() <= 0 || before.Size() > 64<<10 {
		return nil, fmt.Errorf("owner-private bounded regular env file required")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, fmt.Errorf("env file changed before read")
	}
	values := map[string]string{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 4096), 64<<10)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || !envKeyPattern.MatchString(key) || value == "" || strings.TrimSpace(key) != key ||
			strings.ContainsAny(value, "\r\n\x00") {
			return nil, fmt.Errorf("invalid EnvironmentFile entry")
		}
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("duplicate EnvironmentFile key")
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(before, after) || after.Size() != before.Size() {
		return nil, fmt.Errorf("env file changed during read")
	}
	return values, nil
}

func validateControlEnv(values map[string]string) error {
	required := []string{
		"LISTEN_HOST", "PORT", "DATABASE_URL", "AUTO_MIGRATE",
		"CONTROL_TLS_CERT_FILE", "CONTROL_TLS_KEY_FILE", "CONTROL_CLIENT_CA_FILE", "CONTROL_AUTH_POLICY_FILE",
		"GITHUB_PUBLISHER_PLAN_FILE", "GITHUB_PUBLISHER_REMOTE_FILE",
	}
	for _, key := range required {
		if values[key] == "" {
			return fmt.Errorf("control environment missing %s", key)
		}
	}
	if values["AUTO_MIGRATE"] != "0" {
		return fmt.Errorf("production control plane requires AUTO_MIGRATE=0")
	}
	if values["INSECURE_DEV"] != "" || values["GITHUB_PUBLISHER_CONFIG_FILE"] != "" {
		return fmt.Errorf("production control plane must not carry development mode or publisher configuration")
	}
	allowed := map[string]bool{}
	for _, key := range required {
		allowed[key] = true
	}
	for key := range values {
		if !allowed[key] {
			return fmt.Errorf("unexpected control environment key %s", key)
		}
	}
	return nil
}

func validatePublisherEnv(values map[string]string) error {
	required := []string{
		"LISTEN_HOST", "PORT", "PUBLISHER_CONFIG_FILE",
		"PUBLISHER_TLS_CERT_FILE", "PUBLISHER_TLS_KEY_FILE",
		"PUBLISHER_CLIENT_CA_FILE", "PUBLISHER_CONTROL_SUBJECT",
	}
	allowed := map[string]bool{}
	for _, key := range required {
		allowed[key] = true
		if values[key] == "" {
			return fmt.Errorf("missing %s", key)
		}
	}
	if values["LISTEN_HOST"] != "127.0.0.1" && values["LISTEN_HOST"] != "::1" {
		return fmt.Errorf("publisher v1 requires literal loopback LISTEN_HOST")
	}
	for key := range values {
		if !allowed[key] {
			return fmt.Errorf("unexpected publisher environment key %s", key)
		}
	}
	return nil
}

func validateWorkerEnv(values map[string]string, preparation bool) error {
	required := []string{"CONTROL_ENDPOINT", "CONTROL_CLIENT_CERT_FILE", "CONTROL_CLIENT_KEY_FILE", "CONTROL_SERVER_CA_FILE"}
	if preparation {
		required = append(required, "WORKER_PREPARATION_CONFIG")
	}
	allowed := map[string]bool{}
	for _, key := range required {
		allowed[key] = true
		if values[key] == "" {
			return fmt.Errorf("missing %s", key)
		}
	}
	for key := range values {
		if !allowed[key] {
			return fmt.Errorf("unexpected worker environment key %s", key)
		}
	}
	return nil
}
