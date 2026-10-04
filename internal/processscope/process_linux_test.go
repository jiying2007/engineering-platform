//go:build linux

package processscope

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func requireNamespace(t *testing.T) {
	t.Helper()
	cmd := exec.Command("/bin/cat")
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	s, err := Start(cmd)
	if err != nil {
		_ = in.Close()
		if os.Getenv("EP_REQUIRE_PID_NAMESPACE_TESTS") == "1" {
			t.Fatal(err)
		}
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOSYS) {
			t.Skip("host denies required namespaces; no plain-process fallback")
		}
		t.Fatal(err)
	}
	_ = in.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	p, err := s.Wait(ctx)
	if err != nil || !p.Quiescent() {
		t.Fatal(p, err)
	}
}

func TestNamespaceKillsDetachedDescendantsBeforeCapture(t *testing.T) {
	requireNamespace(t)
	for _, mode := range []string{"natural", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			ready := filepath.Join(root, "ready")
			heartbeat := filepath.Join(root, "heartbeat")
			script := `import os,sys,time
pid=os.fork()
if pid==0:
 os.setsid()
 pid=os.fork()
 if pid: os._exit(0)
 for fd in (0,1,2):
  try: os.close(fd)
  except OSError: pass
 with open(sys.argv[2],"ab",buffering=0) as f:
  open(sys.argv[1],"w").write(str(os.getpid()))
  while True:
   f.write(b"x");time.sleep(.005)
else:
 sys.stdin.readline()
`
			python, err := exec.LookPath("python3")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd := exec.CommandContext(ctx, python, "-S", "-c", script, ready, heartbeat)
			in, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			s, err := Start(cmd)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				c, end := context.WithTimeout(context.Background(), time.Second)
				defer end()
				_, _ = s.Stop(c)
			}()
			until := time.Now().Add(2 * time.Second)
			for {
				if b, e := os.ReadFile(ready); e == nil && strings.TrimSpace(string(b)) != "" {
					break
				}
				if time.Now().After(until) {
					t.Fatal("grandchild not started")
				}
				time.Sleep(5 * time.Millisecond)
			}
			c, end := context.WithTimeout(context.Background(), 2*time.Second)
			defer end()
			p, e := s.Wait(func() context.Context { x, y := context.WithCancel(c); y(); return x }())
			if e == nil || p.Quiescent() {
				t.Fatal("live process attested stopped")
			}
			if mode == "cancel" {
				cancel()
			} else {
				_ = in.Close()
			}
			proof, waitErr := s.Wait(c)
			if !proof.Quiescent() || (mode == "natural" && waitErr != nil) {
				t.Fatal(proof, waitErr)
			}
			before, e := os.ReadFile(heartbeat)
			if e != nil || len(before) == 0 {
				t.Fatal("missing descendant writes", e)
			}
			time.Sleep(50 * time.Millisecond)
			after, e := os.ReadFile(heartbeat)
			if e != nil || string(before) != string(after) {
				t.Fatal("detached descendant wrote after namespace init reap")
			}
		})
	}
}
func TestScopeRefusesLaunchOverrides(t *testing.T) {
	for _, cmd := range []*exec.Cmd{nil, {SysProcAttr: &syscall.SysProcAttr{}}, {ExtraFiles: []*os.File{os.Stdin}}} {
		if _, e := Start(cmd); e == nil {
			t.Fatal("unsafe launch override accepted")
		}
	}
}

func TestScopeParentDeathFixture(t *testing.T) {
	root := os.Getenv("EP_TEST_SCOPE_PARENT")
	if root == "" {
		return
	}
	python, e := exec.LookPath("python3")
	if e != nil {
		t.Fatal(e)
	}
	script := `import os,sys,time
pid=os.fork()
if pid==0:
 os.setsid()
 pid=os.fork()
 if pid: os._exit(0)
 with open(sys.argv[1],"ab",buffering=0) as f:
  while True: f.write(b"x");time.sleep(.005)
else:
 sys.stdin.readline()
`
	cmd := exec.Command(python, "-S", "-c", script, filepath.Join(root, "writes"))
	in, e := cmd.StdinPipe()
	if e != nil {
		t.Fatal(e)
	}
	defer in.Close()
	scope, e := Start(cmd)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(root, "ready"), []byte("scope-started"), 0600); e != nil {
		t.Fatal(e)
	}
	_, _ = scope.Wait(context.Background())
}

func TestNamespaceDiesWithWorkerParent(t *testing.T) {
	requireNamespace(t)
	root := t.TempDir()
	parent := exec.Command(os.Args[0], "-test.run=^TestScopeParentDeathFixture$")
	parent.Env = append(os.Environ(), "EP_TEST_SCOPE_PARENT="+root)
	if e := parent.Start(); e != nil {
		t.Fatal(e)
	}
	defer parent.Process.Kill()
	deadline := time.Now().Add(3 * time.Second)
	for {
		b, e := os.ReadFile(filepath.Join(root, "writes"))
		if e == nil && len(b) > 0 {
			break
		}
		if time.Now().After(deadline) {
			_ = parent.Process.Kill()
			_ = parent.Wait()
			t.Fatal("nested workload failed to start")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if e := parent.Process.Kill(); e != nil {
		t.Fatal(e)
	}
	_ = parent.Wait()
	time.Sleep(50 * time.Millisecond)
	before, e := os.ReadFile(filepath.Join(root, "writes"))
	if e != nil {
		t.Fatal(e)
	}
	time.Sleep(100 * time.Millisecond)
	after, e := os.ReadFile(filepath.Join(root, "writes"))
	if e != nil || string(before) != string(after) {
		t.Fatal("descendant survived Worker death")
	}
}
