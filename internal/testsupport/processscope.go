package testsupport

import (
	"context"
	"errors"
	"github.com/jiying2007/engineering-platform/internal/processscope"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// ProcessScopeFixture is fabricated UNIT TEST data. It is never runtime proof,
// model evidence, admission, or a retained production observation.
func ProcessScopeFixture() processscope.Proof {
	now := time.Unix(1700000000, 0).UTC()
	return processscope.Proof{Version: 1, Mechanism: processscope.Mechanism, HostPID: 123, NamespaceID: 456, ParentNamespaceID: 789, StartedAt: now, ReapedAt: now.Add(time.Second), InitReaped: true}
}

// RequireProcessNamespaces only skips on developer hosts without the required
// kernel feature. Canonical CI requires this feature explicitly and must fail.
func RequireProcessNamespaces(t *testing.T) {
	t.Helper()
	cmd := exec.Command("/bin/cat")
	in, e := cmd.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	scope, e := processscope.Start(cmd)
	if e != nil {
		_ = in.Close()
		if os.Getenv("EP_REQUIRE_PID_NAMESPACE_TESTS") != "1" && (errors.Is(e, syscall.EPERM) || errors.Is(e, syscall.EINVAL) || errors.Is(e, syscall.ENOSYS)) {
			t.Skip("kernel namespaces unavailable; no uncontained fallback")
		}
		t.Fatal(e)
	}
	_ = in.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	proof, e := scope.Wait(ctx)
	if e != nil || !proof.Quiescent() {
		t.Fatal(proof, e)
	}
}
