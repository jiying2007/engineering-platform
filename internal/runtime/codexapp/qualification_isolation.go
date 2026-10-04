package codexapp

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/processscope"
	runtimeprovider "github.com/jiying2007/engineering-platform/internal/runtime"
)

// This is a stable description of the tested host class, NOT a portable host
// attestation. Every actual engineering CLI execution re-runs Qualify and checks
// the entire receipt. PIDs, inode numbers and timestamps are deliberately absent:
// they describe a process lifetime, not a reproducible qualification identity.
func qualificationEnvironmentDigest() (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("engineering qualification requires Linux process namespaces")
	}
	file, err := os.Open("/proc/sys/kernel/osrelease")
	if err != nil {
		return "", err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 4097))
	kernel := strings.TrimSpace(string(raw))
	if err != nil || kernel == "" || len(raw) > 4096 || strings.ContainsAny(kernel, "\x00\r\n") {
		return "", fmt.Errorf("bounded kernel identity required")
	}
	return canonical.Digest(struct {
		Version      int    `json:"version"`
		OS           string `json:"os"`
		Architecture string `json:"architecture"`
		Kernel       string `json:"kernel"`
		EffectiveUID int    `json:"effective_uid"`
		EffectiveGID int    `json:"effective_gid"`
		Mechanism    string `json:"mechanism"`
	}{1, runtime.GOOS, runtime.GOARCH, kernel, os.Geteuid(), os.Getegid(), processscope.Mechanism})
}

// Only initialize/thread-start are sent. No credential warmup, model turn, tool
// invocation, resume, user input or approval is issued by this no-account probe.
func qualifyIsolatedEngineeringStartup(ctx context.Context, executable, digest, model, work, home string) (err error) {
	provider, err := NewPinnedProvider(executable, digest)
	if err != nil {
		return err
	}
	provider.engineeringMode = true
	provider.qualificationOnly = true
	probeCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	cmd, err := provider.Command(probeCtx, runtimeprovider.LaunchSpec{Dir: work, Env: []string{"HOME=" + home}})
	if err != nil {
		return err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	// Scope owns Wait. Use a separate pipe so Wait cannot close StdoutPipe while
	// Client is reading a final response. Parent ends are explicitly owned here.
	output, writer, err := os.Pipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	defer output.Close()
	defer writer.Close()
	cmd.Stdout = writer
	var diagnostics diagnosticBuffer
	cmd.Stderr = &boundedWriter{writer: &diagnostics, remaining: 64 << 10}
	scope, err := processscope.Start(cmd)
	if err != nil {
		_ = stdin.Close()
		return err
	}
	_ = writer.Close()
	client := NewClient(output, stdin)
	defer client.Close()
	defer func() {
		_ = client.Close()
		stopCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		proof, stopErr := scope.Stop(stopCtx)
		if stopErr != nil || !proof.Quiescent() {
			err = fmt.Errorf("isolated qualification stop unconfirmed (prior error: %v): %v", err, stopErr)
		}
	}()
	adapter, err := NewAdapter(client, work)
	if err != nil {
		return err
	}
	if err = adapter.Initialize(probeCtx, "engineering-platform-isolated-qualification-v2"); err != nil {
		return fmt.Errorf("isolated app-server initialize: %w; stderr=%s", err, strings.TrimSpace(diagnostics.String()))
	}
	if _, err = adapter.StartEngineeringThread(probeCtx, model); err != nil {
		return fmt.Errorf("isolated workspace-write thread/start: %w; stderr=%s", err, strings.TrimSpace(diagnostics.String()))
	}
	if probeCtx.Err() != nil {
		return probeCtx.Err()
	}
	return nil
}
