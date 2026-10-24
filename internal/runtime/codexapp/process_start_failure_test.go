package codexapp

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestScopeStartFailurePreservesUnknownProcess(t *testing.T) {
	for _, lane := range []struct {
		name        string
		unconfirmed error
	}{
		{"probe", ErrProbeStopUnconfirmed},
		{"engineering", ErrEngineeringStopUnconfirmed},
	} {
		for _, started := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/started=%t", lane.name, started), func(t *testing.T) {
				cmd := &exec.Cmd{}
				if started {
					// Classification fixture only; no Wait, Kill, or proof fabrication.
					cmd.Process = &os.Process{}
				}
				cause := errors.New("synthetic namespace startup failure")
				err := scopeStartError(cmd, cause, lane.unconfirmed)
				if !errors.Is(err, cause) || errors.Is(err, lane.unconfirmed) != started {
					t.Fatalf("lost startup failure or process uncertainty: %v", err)
				}
				if !started && err != cause {
					t.Fatal("ordinary prelaunch failure changed classification")
				}
			})
		}
	}
}

func TestProbeDirectoryCleanupKeepsUnconfirmedProcessInputs(t *testing.T) {
	cause := errors.New("synthetic operation failure")
	for _, tc := range []struct {
		name     string
		probeErr error
		retain   bool
	}{
		{name: "completed-probe"},
		{name: "never-started", probeErr: scopeStartError(&exec.Cmd{}, cause, ErrProbeStopUnconfirmed)},
		{name: "confirmed-failed-exit", probeErr: cause},
		{name: "started-without-scope", probeErr: scopeStartError(&exec.Cmd{Process: &os.Process{}}, cause, ErrProbeStopUnconfirmed), retain: true},
		{name: "wrapped-stop-unknown", probeErr: fmt.Errorf("qualification: %w", errors.Join(cause, ErrProbeStopUnconfirmed)), retain: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			owned := filepath.Join(root, "probe-inputs")
			if err := os.Mkdir(owned, 0700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(owned, "input")
			if err := os.WriteFile(marker, []byte("original private input"), 0600); err != nil {
				t.Fatal(err)
			}
			cleanupProbeDirectory(owned, tc.probeErr)
			data, err := os.ReadFile(marker)
			if tc.retain {
				if err != nil || string(data) != "original private input" {
					t.Fatal("unconfirmed probe lost its private inputs", err)
				}
			} else if !os.IsNotExist(err) {
				t.Fatal("settled or unstarted probe directory was not cleaned", err)
			}
		})
	}
}
