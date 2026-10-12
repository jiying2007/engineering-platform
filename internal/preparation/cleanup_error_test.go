package preparation_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/workeragent"
	"github.com/jiying2007/engineering-platform/internal/workspace"
)

// Inject a lease failure after actual preparation. Broken owner bytes force a
// cleanup error without permission assumptions (also reproducible under root).
type cleanupFailureTransport struct {
	transport
	root     string
	failure  error
	injected bool
}

func (c *cleanupFailureTransport) Call(ctx context.Context, method, path string, in, out any) error {
	if path == "/api/v1/worker/renew" {
		slots, err := filepath.Glob(filepath.Join(c.root, "workspaces", "*", "prepared.json"))
		if err != nil {
			return err
		}
		if len(slots) == 1 {
			if err := os.WriteFile(filepath.Join(filepath.Dir(slots[0]), "owner.json"), []byte("{}"), 0600); err != nil {
				return err
			}
			c.injected = true
			return c.failure
		}
	}
	return c.transport.Call(ctx, method, path, in, out)
}
func TestPrepareAgentPreservesLeaseAndCleanupFailures(t *testing.T) {
	a, c := fixture(t)
	p, err := preparation.New(c)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	primary := errors.New("injected lease loss")
	client := &cleanupFailureTransport{transport: transport{a: a}, root: c.Root, failure: primary}
	receipt, err := workeragent.PrepareOnce(context.Background(), client, a.Token.Profile, p)
	if receipt != nil || !client.injected || !errors.Is(err, primary) || !errors.Is(err, workspace.ErrCleanupIncomplete) || !errors.Is(err, workspace.ErrUnmanagedWorkspace) {
		t.Fatalf("primary/cleanup failure hidden: receipt=%+v injected=%v err=%v", receipt, client.injected, err)
	}
	if client.claims != 1 || client.calls != 0 {
		t.Fatal("failure repeated work or reported success")
	}
}
