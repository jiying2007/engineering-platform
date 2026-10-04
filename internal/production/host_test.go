package production

import (
	"fmt"
	"os"
	"os/user"
	"strings"
	"testing"
)

func TestAuditInvalidHostFailsClosed(t *testing.T) {
	result, err := Check(validConfig(t))
	if err == nil || result.HostValidated || result.OperationallyReady {
		t.Fatalf("unprovisioned host accepted: %#v %v", result, err)
	}
}
func TestServiceUIDsMustExistAndBeDistinct(t *testing.T) {
	for _, kind := range []string{"missing", "root", "alias"} {
		t.Run(kind, func(t *testing.T) {
			lookup := func(name string) (*user.User, error) {
				if kind == "missing" {
					return nil, fmt.Errorf("absent")
				}
				uid := "1001"
				if kind == "root" {
					uid = "0"
				}
				return &user.User{Uid: uid, Username: name}, nil
			}
			if _, err := resolveUsers([]string{"service-a", "service-b"}, lookup); err == nil {
				t.Fatal("invalid UIDs accepted")
			}
		})
	}
}
func TestMalformedDeploymentValuesRejectedBeforeHostAdmission(t *testing.T) {
	for _, pair := range [][2]string{{"PORT=18443", "PORT=invalid-port"}, {"PORT=18443", "PORT=0"}, {"DATABASE_URL=postgres://operator@127.0.0.1/production?sslmode=require", "DATABASE_URL=not-a-postgres-url"}, {"DEPLOYMENT_MODE=production", "DEPLOYMENT_MODE=pilot"}} {
		config := validConfig(t)
		data, err := os.ReadFile(config.ControlEnvFile)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(config.ControlEnvFile, []byte(strings.Replace(string(data), pair[0], pair[1], 1)), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := CheckConfiguration(config); err == nil {
			t.Fatalf("invalid config %s accepted", pair[1])
		}
	}
	for _, raw := range []string{"not-a-url", "http://localhost", "https://user:pass@localhost", "https://localhost:0", "https://localhost/path"} {
		if validateEndpoint(raw) == nil {
			t.Fatalf("invalid endpoint %s accepted", raw)
		}
	}
}
