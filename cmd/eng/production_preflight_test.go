package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/production"
)

func TestProductionPreflightReportsInternalPublisherBlocker(t *testing.T) {
	root := t.TempDir()
	exec := func(name string) string {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		return path
	}
	env := func(name, content string) string {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	backup := filepath.Join(root, "backup")
	if err := os.Mkdir(backup, 0o700); err != nil {
		t.Fatal(err)
	}
	control := "LISTEN_HOST=127.0.0.1\nPORT=18443\nDATABASE_URL=postgres://production\nAUTO_MIGRATE=0\nCONTROL_TLS_CERT_FILE=/etc/engineering-platform/tls/control.crt\nCONTROL_TLS_KEY_FILE=/etc/engineering-platform/tls/control.key\nCONTROL_CLIENT_CA_FILE=/etc/engineering-platform/tls/ca.crt\nCONTROL_AUTH_POLICY_FILE=/etc/engineering-platform/access-policy.json\n"
	worker := "CONTROL_ENDPOINT=https://127.0.0.1:18443\nCONTROL_CLIENT_CERT_FILE=/etc/engineering-platform/tls/worker.crt\nCONTROL_CLIENT_KEY_FILE=/etc/engineering-platform/tls/worker.key\nCONTROL_SERVER_CA_FILE=/etc/engineering-platform/tls/ca.crt\n"
	config := production.Config{
		Version: production.ConfigVersion, DeploymentMode: production.DeploymentMode,
		ControlBinary: exec("control-plane"), WorkerBinary: exec("worker"), EngBinary: exec("eng"),
		ControlEnvFile:         env("control.env", control),
		AdmissionEnvFile:       env("admission.env", worker),
		PreparationEnvFile:     env("preparation.env", worker+"WORKER_PREPARATION_CONFIG=/etc/engineering-platform/preparation.json\n"),
		BackupDirectory:        backup,
		ControlServiceUser:     "engineering-control",
		AdmissionServiceUser:   "engineering-admission",
		PreparationServiceUser: "engineering-preparation",
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "production.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := productionPreflight([]string{"--config", path}); err != nil {
		t.Fatal(err)
	}
}
