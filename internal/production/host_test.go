package production

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
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

func TestHostFileRejectsHardLinkAlias(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	alias := filepath.Join(root, "alias.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if err := hostFile(path, false, os.Getuid(), false); err == nil {
		t.Fatal("hard-linked configuration accepted")
	}
}

func TestServicePathAccessChecksTargetAndEveryDirectory(t *testing.T) {
	root := t.TempDir()
	private := filepath.Join(root, "private")
	if err := os.Mkdir(private, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(private, 0o700) })
	path := filepath.Join(private, "config")
	if err := os.WriteFile(path, []byte("x"), 0o400); err != nil {
		t.Fatal(err)
	}
	gids := map[uint32]bool{uint32(os.Getgid()): true}
	for _, gid := range mustGroups(t) {
		gids[uint32(gid)] = true
	}
	if err := servicePathAccess(path, os.Getuid(), gids, 4); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o200); err != nil {
		t.Fatal(err)
	}
	if err := servicePathAccess(path, os.Getuid(), gids, 4); err == nil {
		t.Fatal("unreadable file accepted")
	}
	if err := os.Chmod(path, 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(private, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := servicePathAccess(path, os.Getuid(), gids, 4); err == nil {
		t.Fatal("untraversable parent accepted")
	}
}

func TestUnixModeAccessUsesOwnerGroupAndOtherBits(t *testing.T) {
	info := fakeUnixFileInfo{mode: 0o640, stat: syscall.Stat_t{Uid: 1001, Gid: 2001}}
	if !unixModeAllows(info, 1001, map[uint32]bool{}, 4) {
		t.Fatal("owner read was rejected")
	}
	if !unixModeAllows(info, 1002, map[uint32]bool{2001: true}, 4) {
		t.Fatal("group read was rejected")
	}
	if unixModeAllows(info, 1002, map[uint32]bool{2002: true}, 4) {
		t.Fatal("other read was invented")
	}
}

func mustGroups(t *testing.T) []int {
	t.Helper()
	groups, err := os.Getgroups()
	if err != nil {
		t.Fatal(err)
	}
	return groups
}

type fakeUnixFileInfo struct {
	mode os.FileMode
	stat syscall.Stat_t
}

func (f fakeUnixFileInfo) Name() string       { return "fake" }
func (f fakeUnixFileInfo) Size() int64        { return 1 }
func (f fakeUnixFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeUnixFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeUnixFileInfo) IsDir() bool        { return false }
func (f fakeUnixFileInfo) Sys() any           { return &f.stat }
