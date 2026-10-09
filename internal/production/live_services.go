package production

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// LiveServiceObservation is a bounded local host fact, not a heartbeat,
// provider qualification, capacity measurement, or production READY grant.
type LiveServiceObservation struct {
	Unit        string    `json:"unit"`
	ServiceUser string    `json:"service_user"`
	PID         int       `json:"pid"`
	Binary      string    `json:"binary"`
	ObservedAt  time.Time `json:"observed_at"`
}

type liveServiceSpec struct {
	unit, user, binary string
	argv               []string
}

func liveServiceSpecs(c Config) []liveServiceSpec {
	return []liveServiceSpec{
		{"engineering-control-plane.service", c.ControlServiceUser, c.ControlBinary, []string{"--production"}},
		{"engineering-worker-admission.service", c.AdmissionServiceUser, c.WorkerBinary, []string{"--profile=worker/admission-production", "--admission-only"}},
		{"engineering-worker-preparation.service", c.PreparationServiceUser, c.WorkerBinary, []string{"--profile=worker/codex-production", "--prepare-only"}},
		{"engineering-publisher.service", c.PublisherServiceUser, c.PublisherBinary, []string{}},
	}
}

// LiveControlLinks derives the expected Control listener and Publisher mTLS
// config from the same owner-private deployment files validated by preflight.
// Callers must complete Check(c) before probing running services.
func LiveControlLinks(c Config) (string, string, error) {
	if c.Version != ConfigVersion || c.DeploymentMode != DeploymentMode {
		return "", "", fmt.Errorf("production deployment configuration required")
	}
	control, err := readEnvFile(c.ControlEnvFile)
	if err != nil {
		return "", "", err
	}
	if err := validateControlEnv(control); err != nil {
		return "", "", err
	}
	return "https://" + net.JoinHostPort(control["LISTEN_HOST"], control["PORT"]), control["GITHUB_PUBLISHER_REMOTE_FILE"], nil
}

func parseSystemdUnit(raw []byte, unit string) (int, error) {
	if len(raw) == 0 || len(raw) > 4096 || !strings.HasSuffix(string(raw), "\n") {
		return 0, fmt.Errorf("bounded systemd unit fact required")
	}
	allowed := map[string]bool{"Id": true, "LoadState": true, "ActiveState": true, "SubState": true, "MainPID": true}
	observed := map[string]string{}
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok || !allowed[k] || v == "" || strings.TrimSpace(v) != v || strings.ContainsAny(v, "\r\x00") {
			return 0, fmt.Errorf("invalid systemd unit fact")
		}
		if _, duplicate := observed[k]; duplicate {
			return 0, fmt.Errorf("duplicate systemd unit fact")
		}
		observed[k] = v
	}
	if len(observed) != len(allowed) || observed["Id"] != unit ||
		observed["LoadState"] != "loaded" || observed["ActiveState"] != "active" ||
		observed["SubState"] != "running" {
		return 0, fmt.Errorf("service unit is missing, ambiguous or not running")
	}
	pid, err := strconv.Atoi(observed["MainPID"])
	if err != nil || pid <= 1 || pid > 1<<24 || strconv.Itoa(pid) != observed["MainPID"] {
		return 0, fmt.Errorf("invalid systemd main PID")
	}
	return pid, nil
}

func readProcessBounded(name string, max int64) ([]byte, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil || len(data) == 0 || int64(len(data)) > max {
		return nil, fmt.Errorf("process identity source unreadable or oversized")
	}
	return data, nil
}

func verifyLiveProcess(pid int, spec liveServiceSpec) error {
	account, err := user.Lookup(spec.user)
	if err != nil || account == nil {
		return fmt.Errorf("configured live service account unavailable")
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil || uid <= 0 {
		return fmt.Errorf("non-root live service UID required")
	}
	root := filepath.Join("/proc", strconv.Itoa(pid))
	want, err := os.Stat(spec.binary)
	if err != nil || !want.Mode().IsRegular() {
		return fmt.Errorf("expected deployed executable unavailable")
	}
	got, err := os.Stat(filepath.Join(root, "exe"))
	if err != nil || !os.SameFile(got, want) {
		return fmt.Errorf("live executable does not match source-verified distribution")
	}
	status, err := readProcessBounded(filepath.Join(root, "status"), 16<<10)
	if err != nil {
		return err
	}
	foundUID := false
	for _, line := range strings.Split(string(status), "\n") {
		if !strings.HasPrefix(line, "Uid:") {
			continue
		}
		if foundUID {
			return fmt.Errorf("duplicate live process UID")
		}
		fields := strings.Fields(strings.TrimPrefix(line, "Uid:"))
		if len(fields) != 4 {
			return fmt.Errorf("invalid live process UID")
		}
		for _, raw := range fields {
			id, e := strconv.Atoi(raw)
			if e != nil || id != uid {
				return fmt.Errorf("running process UID differs from configured service identity")
			}
		}
		foundUID = true
	}
	if !foundUID {
		return fmt.Errorf("live process UID not observable")
	}
	cmdline, err := readProcessBounded(filepath.Join(root, "cmdline"), 4096)
	if err != nil || cmdline[len(cmdline)-1] != 0 {
		return fmt.Errorf("live process command line not observable")
	}
	argv := strings.Split(string(cmdline[:len(cmdline)-1]), "\x00")
	if len(argv) != 1+len(spec.argv) || argv[0] != spec.binary {
		return fmt.Errorf("running service argv differs from canonical role")
	}
	for i, wantArg := range spec.argv {
		if argv[i+1] != wantArg {
			return fmt.Errorf("running service mode does not match canonical role")
		}
	}
	return nil
}

// ObserveLiveServices checks four fixed production systemd units against the
// actual process executable bytes, UIDs and role-specific argv. It performs
// no restart, publication, model invocation or capacity inference.
func ObserveLiveServices(ctx context.Context, c Config) ([]LiveServiceObservation, error) {
	if runtime.GOOS != "linux" || ctx == nil {
		return nil, fmt.Errorf("Linux host and bounded context required")
	}
	seenPIDs := map[int]bool{}
	result := make([]LiveServiceObservation, 0, 4)
	for _, spec := range liveServiceSpecs(c) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cmd := exec.CommandContext(ctx, "/usr/bin/systemctl", "show", "--no-pager",
			"--property=Id,LoadState,ActiveState,SubState,MainPID", spec.unit)
		facts, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("systemd unit query failed for %s: %w", spec.unit, err)
		}
		pid, err := parseSystemdUnit(facts, spec.unit)
		if err != nil || seenPIDs[pid] {
			return nil, fmt.Errorf("service unit %s is not independently running: %v", spec.unit, err)
		}
		if err := verifyLiveProcess(pid, spec); err != nil {
			return nil, fmt.Errorf("service unit %s: %w", spec.unit, err)
		}
		seenPIDs[pid] = true
		result = append(result, LiveServiceObservation{
			Unit: spec.unit, ServiceUser: spec.user, PID: pid,
			Binary: filepath.Base(spec.binary), ObservedAt: time.Now().UTC(),
		})
	}
	return result, nil
}
