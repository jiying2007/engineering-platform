package codexapp

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/processscope"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func TestProbeGateWriterRemainsOwnedAfterProcessWait(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", probeStartGate, "codex-probe", "/bin/true")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8"}
	childInput, input, err := probeStdinPipe(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer childInput.Close()
	defer input.Close()
	scope, err := processscope.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		stopping, stop := context.WithTimeout(context.WithoutCancel(ctx), probeStopTimeout)
		defer stop()
		proof, err := scope.Stop(stopping)
		if err != nil || !proof.Quiescent() {
			t.Errorf("startup gate scope did not stop: %+v %v", proof, err)
		}
	}()
	if _, err := input.Write([]byte("\n")); err != nil {
		t.Fatal(err)
	}
	// Force Wait to finish before closing the caller's writer. StdinPipe gives
	// Wait ownership of this descriptor, making this ordering fail every time.
	proof, err := scope.Wait(ctx)
	if err != nil || !proof.Quiescent() {
		t.Fatalf("startup gate command did not finish: %+v %v", proof, err)
	}
	if err := input.Close(); err != nil {
		t.Fatalf("process Wait closed the caller-owned startup gate writer: %v", err)
	}
}

func TestProbeGateCloseDeliversEOF(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	root := t.TempDir()
	executable := probeFixture(t, root, "#!/bin/sh\nif IFS= read -r unexpected; then exit 1; fi\nprintf 'stdin EOF\\n'\n")
	output, diagnostics, err := runCodexProbe(t.Context(), executable, probeFixtureDigest(t, executable), filepath.Join(root, "home"), probeBudget{
		timeout: 2 * time.Second, stdoutBytes: 1024,
	})
	if err != nil || string(output) != "stdin EOF\n" || len(diagnostics) != 0 {
		t.Fatalf("startup gate did not close stdin after release: stdout=%q stderr=%q err=%v", output, diagnostics, err)
	}
}
