package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProductionObserverCheckRequiresExplicitIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "observer.jsonl")
	base := []string{"--journal", path, "--session", monitorTestSession, "--endpoint-digest", monitorTestEndpoint}
	with := func(extra ...string) []string { return append(append([]string{}, base...), extra...) }
	replace := func(index int, value string) []string {
		args := append([]string{}, base...)
		args[index] = value
		return args
	}
	for _, gate := range []bool{false, true} {
		args := base
		if gate {
			args = with("--require-current-clear")
		}
		o, err := parseProductionObserverCheck(args)
		if err != nil || o.journal != path || o.session != monitorTestSession || o.endpoint != monitorTestEndpoint || o.requireCurrentClear != gate {
			t.Fatalf("explicit observer pins changed: %+v, %v", o, err)
		}
	}
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"missing-all", nil},
		{"missing-journal", append([]string{}, base[2:]...)},
		{"missing-session", append(append([]string{}, base[:2]...), base[4:]...)},
		{"missing-endpoint", append([]string{}, base[:4]...)},
		{"relative-journal", replace(1, "observer.jsonl")},
		{"noncanonical-journal", replace(1, path+string(os.PathSeparator)+"..")},
		{"nul-journal", replace(1, path+"\x00")},
		{"empty-session", replace(3, "")},
		{"short-session", replace(3, monitorTestSession[:31])},
		{"uppercase-session", replace(3, strings.ToUpper(monitorTestSession))},
		{"nonhex-session", replace(3, strings.Repeat("g", 32))},
		{"raw-endpoint", replace(5, "https://127.0.0.1:8443")},
		{"short-endpoint-digest", replace(5, monitorTestEndpoint[:70])},
		{"uppercase-endpoint-digest", replace(5, strings.ToUpper(monitorTestEndpoint))},
		{"nonhex-endpoint-digest", replace(5, "sha256:"+strings.Repeat("g", 64))},
		{"duplicate-session", with("--session=" + monitorTestSession)},
		{"duplicate-endpoint", with("--endpoint-digest=" + monitorTestEndpoint)},
		{"duplicate-gate", with("--require-current-clear", "--require-current-clear=false")},
		{"invalid-gate", with("--require-current-clear=maybe")},
		{"unexpected-position", with("extra")},
		{"readiness-flag", with("--require-ready")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseProductionObserverCheck(tc.args); err == nil {
				t.Fatal("accepted incomplete or ambiguous observer identity", tc.args)
			}
		})
	}
}

func TestProductionMonitorExplicitSessionIsScoped(t *testing.T) {
	window := []string{"--observe-for", "2s", "--interval", "1s", "--max-pending-age", "1m"}
	monitor := append(append([]string{}, window...), "--monitor-for", "30s", "--stale-after", "5s")
	for _, session := range []string{"", monitorTestSession} {
		args := append([]string{}, monitor...)
		if session != "" {
			args = append(args, "--session", session)
		}
		o, err := parseProductionStatus(args)
		if err != nil || !o.monitor || o.session != session {
			t.Fatal("monitor session selection changed", o, err)
		}
	}
	for _, args := range [][]string{
		{"--session", monitorTestSession},
		append(append([]string{}, window...), "--session", monitorTestSession),
		append(append([]string{}, monitor...), "--session", ""),
		append(append([]string{}, monitor...), "--session", strings.ToUpper(monitorTestSession)),
		append(append([]string{}, monitor...), "--session", strings.Repeat("g", 32)),
		append(append([]string{}, monitor...), "--session", monitorTestSession, "--session="+monitorTestSession),
	} {
		if _, err := parseProductionStatus(args); err == nil {
			t.Fatal("accepted invalid or out-of-scope monitor session", args)
		}
	}
}

func TestProductionObserverCheckCancellationPreventsRead(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		var ctx context.Context
		var cancel context.CancelFunc
		want := error(context.Canceled)
		if deadline {
			ctx, cancel = context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			want = context.DeadlineExceeded
		} else {
			ctx, cancel = context.WithCancel(t.Context())
			cancel()
		}
		var output bytes.Buffer
		o := productionObserverCheckOptions{journal: filepath.Join(t.TempDir(), "absent.jsonl"), session: monitorTestSession, endpoint: monitorTestEndpoint, requireCurrentClear: true}
		err := checkProductionObserverJournal(ctx, o, &output)
		cancel()
		if !errors.Is(err, want) || output.Len() != 0 {
			t.Fatal("cancelled check read an absent file, emitted output, or lost cancellation", err, output.String())
		}
	}
}
