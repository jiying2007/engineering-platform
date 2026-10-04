package processscope

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// Probe exercises the actual launch/reap policy as the calling Unix identity.
// It reads no account configuration and invokes no model or network endpoint.
func Probe(ctx context.Context) (Proof, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/cat")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C"}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		return Proof{}, err
	}
	defer in.Close()
	scope, err := Start(cmd)
	if err != nil {
		return Proof{}, err
	}
	_ = in.Close()
	proof, err := scope.Wait(ctx)
	if err != nil || !proof.Quiescent() {
		clean, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_, _ = scope.Stop(clean)
		return Proof{}, fmt.Errorf("process-scope probe failed: %v", err)
	}
	return proof, nil
}
