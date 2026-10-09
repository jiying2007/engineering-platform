package production

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseSystemdUnitRejectsUnqualifiedOrAmbiguousServices(t *testing.T) {
	unit := "engineering-control-plane.service"
	good := "Id=" + unit + "\nLoadState=loaded\nActiveState=active\nSubState=running\nMainPID=1234\n"
	pid, err := parseSystemdUnit([]byte(good), unit)
	if err != nil || pid != 1234 {
		t.Fatalf("canonical service observation not admitted: pid=%d err=%v", pid, err)
	}
	bad := []string{
		"", "Id=" + unit + "\n", strings.TrimSuffix(good, "\n"),
		strings.Replace(good, "Id="+unit, "Id=other.service", 1),
		strings.Replace(good, "LoadState=loaded", "LoadState=masked", 1),
		strings.Replace(good, "ActiveState=active", "ActiveState=failed", 1),
		strings.Replace(good, "SubState=running", "SubState=dead", 1),
		strings.Replace(good, "MainPID=1234", "MainPID=0", 1),
		strings.Replace(good, "MainPID=1234", "MainPID=-1", 1),
		strings.Replace(good, "MainPID=1234", "MainPID=01234", 1),
		good + "Id=" + unit + "\n", good + "ManagerClaims=ready\n",
		strings.Repeat("x", 4097),
	}
	for i, raw := range bad {
		if _, err := parseSystemdUnit([]byte(raw), unit); err == nil {
			t.Fatalf("invalid or misleading unit %d was accepted: %q", i, raw)
		}
	}
}

func TestLiveControlLinksBindActualConfiguredListener(t *testing.T) {
	config := validConfig(t)
	endpoint, remote, err := LiveControlLinks(config)
	if err != nil || endpoint != "https://127.0.0.1:18443" ||
		remote != "/etc/engineering-platform/publisher-remote.json" {
		t.Fatalf("unexpected live control identity: %q %q %v", endpoint, remote, err)
	}
	config.DeploymentMode = "pilot"
	if _, _, err := LiveControlLinks(config); err == nil {
		t.Fatal("pilot configuration produced production live links")
	}
}

func TestLiveServiceSpecsKeepFourDistinctRoleIdentities(t *testing.T) {
	config := validConfig(t)
	specs := liveServiceSpecs(config)
	if len(specs) != 4 ||
		specs[0].unit != "engineering-control-plane.service" ||
		specs[1].unit != "engineering-worker-admission.service" ||
		specs[2].unit != "engineering-worker-preparation.service" ||
		specs[3].unit != "engineering-publisher.service" ||
		specs[0].binary != config.ControlBinary ||
		specs[1].binary != config.WorkerBinary ||
		specs[2].binary != config.WorkerBinary ||
		specs[3].binary != config.PublisherBinary ||
		len(specs[0].argv) != 1 || specs[0].argv[0] != "--production" ||
		len(specs[1].argv) != 2 || specs[1].argv[1] != "--admission-only" ||
		len(specs[2].argv) != 2 || specs[2].argv[1] != "--prepare-only" ||
		len(specs[3].argv) != 0 {
		t.Fatalf("canonical unit roles drifted: %#v", specs)
	}
}

func TestLiveProcessProbeChild(t *testing.T) {
	if os.Getenv("EP_LIVE_PROBE_CHILD") != "1" {
		return
	}
	fmt.Println("CHILD_STARTED")
	time.Sleep(10 * time.Second)
}

func TestVerifyLiveProcessChecksActualUIDBinaryAndRoleFlags(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getuid() == 0 {
		t.Skip("requires unprivileged Linux /proc identity")
	}
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	arg := "-test.run=^TestLiveProcessProbeChild$"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, arg)
	cmd.Env = append(os.Environ(), "EP_LIVE_PROBE_CHILD=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "CHILD_STARTED" {
		t.Fatalf("test service process did not start: %q %v", line, err)
	}
	spec := liveServiceSpec{unit: "test.service", user: u.Username, binary: binary, argv: []string{arg}}
	if err := verifyLiveProcess(cmd.Process.Pid, spec); err != nil {
		t.Fatalf("correct real process identity rejected: %v", err)
	}
	bad := spec
	bad.argv = []string{"--admission-only"}
	if verifyLiveProcess(cmd.Process.Pid, bad) == nil {
		t.Fatal("different service argv admitted")
	}
	bad = spec
	bad.binary = "/bin/sh"
	if verifyLiveProcess(cmd.Process.Pid, bad) == nil {
		t.Fatal("wrong executable admitted")
	}
	bad = spec
	bad.user = "nonexistent-ep-service-for-test"
	if verifyLiveProcess(cmd.Process.Pid, bad) == nil {
		t.Fatal("wrong service account admitted")
	}
	if _, err := os.Stat(filepath.Join("/proc", fmt.Sprint(cmd.Process.Pid), "exe")); err != nil {
		t.Fatal(err)
	}
}

func TestProcessStartTicksBindsLinuxPIDGeneration(t *testing.T) {
	fields := make([]string, 24)
	for i := range fields {
		fields[i] = "0"
	}
	fields[0] = "S"
	fields[19] = "123456"
	good := []byte("7321 (worker ) unusual name) " + strings.Join(fields, " ") + "\n")
	ticks, err := parseProcessStartTicks(good, 7321)
	if err != nil || ticks != 123456 {
		t.Fatalf("valid start tick identity rejected: %d %v", ticks, err)
	}
	for _, tc := range []struct {
		name string
		raw  []byte
		pid  int
	}{
		{"wrong-pid", good, 7322},
		{"missing-close", []byte("7321 (worker " + strings.Join(fields, " ") + "\n"), 7321},
		{"short-record", []byte("7321 (worker) S 1 2 3\n"), 7321},
		{"zero-generation", []byte(strings.Replace(string(good), "123456", "0", 1)), 7321},
		{"negative-generation", []byte(strings.Replace(string(good), "123456", "-1", 1)), 7321},
		{"noncanonical-generation", []byte(strings.Replace(string(good), "123456", "00123456", 1)), 7321},
		{"overflow-generation", []byte(strings.Replace(string(good), "123456", "18446744073709551616", 1)), 7321},
		{"injected-null", append(append([]byte{}, good...), 0), 7321},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := parseProcessStartTicks(tc.raw, tc.pid); err == nil {
				t.Fatalf("malformed generation accepted: ticks=%d", got)
			}
		})
	}
	if runtime.GOOS != "linux" {
		return
	}
	pid := os.Getpid()
	live, err := processStartTicks(pid)
	if err != nil || live == 0 {
		t.Fatalf("real proc start ticks unavailable: pid=%d ticks=%d err=%v", pid, live, err)
	}
	again, err := processStartTicks(pid)
	if err != nil || again != live {
		t.Fatalf("same running process generation changed: before=%d after=%d err=%v", live, again, err)
	}
}
