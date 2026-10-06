package servicenotify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalEndpointNotificationWiring(t *testing.T) {
	root := filepath.Join("..", "..")
	read := func(path string) string {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	for _, role := range []string{"control-plane", "publisher"} {
		raw := read("examples/production/systemd/engineering-" + role + ".service")
		for _, part := range []string{"Type=notify\n", "NotifyAccess=main\n", "KillMode=control-group\n", "StartLimitBurst=3\n", "RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6\n"} {
			if !strings.Contains(raw, part) {
				t.Fatal(role, "missing", part)
			}
		}
	}
	for _, role := range []string{"admission", "preparation"} {
		raw := read("examples/production/systemd/engineering-worker-" + role + ".service")
		for _, part := range []string{"Type=simple\n", "Requires=engineering-control-plane.service\n", "After=network-online.target engineering-control-plane.service\n"} {
			if !strings.Contains(raw, part) {
				t.Fatal(role, "missing", part)
			}
		}
		if strings.Contains(raw, "NotifyAccess=") {
			t.Fatal("worker granted endpoint notification")
		}
	}
	for _, role := range []string{"control-plane", "publisher-service"} {
		raw := read("cmd/" + role + "/main.go")
		if !strings.Contains(raw, "servicenotify.FromEnvironment()") || !strings.Contains(raw, "notifier.Listener(ctx, listener)") {
			t.Fatal("not wired to actual entrypoint", role)
		}
	}
}
