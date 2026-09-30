package production

import (
	"os"
	"strings"
	"path/filepath"
	"testing"
)

func writeExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeEnv(t *testing.T, dir, name, value string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func validConfig(t *testing.T) Config {
	t.Helper()
	root := t.TempDir()
	backup := filepath.Join(root, "backup")
	if err := os.Mkdir(backup, 0o700); err != nil {
		t.Fatal(err)
	}
	control := "LISTEN_HOST=127.0.0.1\nPORT=18443\nDATABASE_URL=postgres://production\nAUTO_MIGRATE=0\nCONTROL_TLS_CERT_FILE=/etc/engineering-platform/tls/control.crt\nCONTROL_TLS_KEY_FILE=/etc/engineering-platform/tls/control.key\nCONTROL_CLIENT_CA_FILE=/etc/engineering-platform/tls/ca.crt\nCONTROL_AUTH_POLICY_FILE=/etc/engineering-platform/access-policy.json\n"
	worker := "CONTROL_ENDPOINT=https://127.0.0.1:18443\nCONTROL_CLIENT_CERT_FILE=/etc/engineering-platform/tls/worker.crt\nCONTROL_CLIENT_KEY_FILE=/etc/engineering-platform/tls/worker.key\nCONTROL_SERVER_CA_FILE=/etc/engineering-platform/tls/ca.crt\n"
	return Config{
		Version: ConfigVersion, DeploymentMode: DeploymentMode,
		ControlBinary:          writeExecutable(t, root, "control-plane"),
		WorkerBinary:           writeExecutable(t, root, "worker"),
		EngBinary:              writeExecutable(t, root, "eng"),
		ControlEnvFile:         writeEnv(t, root, "control.env", control),
		AdmissionEnvFile:       writeEnv(t, root, "admission.env", worker),
		PreparationEnvFile:     writeEnv(t, root, "preparation.env", worker+"WORKER_PREPARATION_CONFIG=/etc/engineering-platform/preparation.json\n"),
		BackupDirectory:        backup,
		ControlServiceUser:     "engineering-control",
		AdmissionServiceUser:   "engineering-admission",
		PreparationServiceUser: "engineering-preparation",
	}
}

func TestCheckProductionBaselineKeepsExplicitRemainingBlockers(t *testing.T) {
	result, err := Check(validConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	if result.ControlPlane != StateReady || result.WorkerAdmission != StateReady ||
		result.WorkerPreparation != StateReady || result.Publisher != StateBlocked ||
		result.Provider != StatePending || result.Internal != StateBlocked ||
		result.Overall != OverallInternal || len(result.Blockers) != 2 {
		t.Fatalf("unexpected production preflight: %#v", result)
	}
}

func TestCheckProductionBaselineRejectsPublisherCredentialInControlPlane(t *testing.T) {
	config := validConfig(t)
	data, err := os.ReadFile(config.ControlEnvFile)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("GITHUB_PUBLISHER_CONFIG_FILE=/etc/engineering-platform/publisher.json\n")...)
	if err := os.WriteFile(config.ControlEnvFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(config); err == nil {
		t.Fatal("control plane accepted publisher configuration")
	}
}

func TestCheckProductionBaselineRejectsSharedServiceIdentityAndAutoMigrate(t *testing.T) {
	config := validConfig(t)
	config.AdmissionServiceUser = config.ControlServiceUser
	if _, err := Check(config); err == nil {
		t.Fatal("shared service identity accepted")
	}
	config = validConfig(t)
	data, _ := os.ReadFile(config.ControlEnvFile)
	data = []byte(strings.ReplaceAll(string(data), "AUTO_MIGRATE=0", "AUTO_MIGRATE=1"))
	if err := os.WriteFile(config.ControlEnvFile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(config); err == nil {
		t.Fatal("AUTO_MIGRATE=1 accepted for production")
	}
}
