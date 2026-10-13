package githubpublish

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func TestPublisherGitNamespaceReapsFastCommands(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		proofs := &gitProofLog{}
		ctx = context.WithValue(ctx, gitProofKey{}, proofs)
		var out, diagnostic boundedOutput
		err := runPublisherGit(ctx, git, []string{"--version"}, []string{"PATH=/usr/bin:/bin", "LANG=C"}, &out, &diagnostic)
		cancel()
		if err != nil || len(proofs.proofs) != 1 || !proofs.proofs[0].Quiescent() {
			t.Fatalf("fast command has no kernel witness: %v %+v", err, proofs)
		}
	}
}
func TestPublisherGitNamespaceKillsDetachedHelper(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	root := t.TempDir()
	ready := filepath.Join(root, "ready")
	heartbeat := filepath.Join(root, "heartbeat")
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	script := `import os,sys,time
pid=os.fork()
if pid==0:
 os.setsid()
 for fd in (0,1,2):
  try:os.close(fd)
  except OSError:pass
 with open(sys.argv[2],'ab',buffering=0) as f:
  open(sys.argv[1],'w').write('ready')
  deadline=time.monotonic()+2
  while time.monotonic()<deadline:f.write(b'x');time.sleep(.002)
else:
 deadline=time.time()+2
 while not os.path.exists(sys.argv[1]):
  if time.time()>deadline:sys.exit(4)
  time.sleep(.002)
`
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	proofs := &gitProofLog{}
	ctx = context.WithValue(ctx, gitProofKey{}, proofs)
	var out, diagnostic boundedOutput
	if err := runPublisherGit(ctx, python, []string{"-S", "-c", script, ready, heartbeat}, []string{"PATH=/usr/bin:/bin", "LANG=C"}, &out, &diagnostic); err != nil {
		t.Fatal(err)
	}
	if len(proofs.proofs) != 1 || !proofs.proofs[0].Quiescent() {
		t.Fatal("helper kill has no kernel proof")
	}
	before, err := os.Stat(heartbeat)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	after, err := os.Stat(heartbeat)
	if err != nil || before.Size() != after.Size() {
		t.Fatal("detached helper survived namespace reap")
	}
}

func TestPublisherGitNamespaceCancellationReapsTree(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	ctx, cancel := context.WithCancel(context.Background())
	proofs := &gitProofLog{}
	ctx = context.WithValue(ctx, gitProofKey{}, proofs)
	ready := filepath.Join(t.TempDir(), "ready")
	var out, diagnostic boundedOutput
	done := make(chan error, 1)
	go func() {
		done <- runPublisherGit(ctx, "/bin/sh", []string{"-c", `printf ready > "$1"; exec sleep 30`, "fixture", ready}, []string{"PATH=/usr/bin:/bin"}, &out, &diagnostic)
	}()
	deadline := time.After(3 * time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	defer cancel()
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("command exited before cancellation: %v", err)
		case <-deadline:
			cancel()
			t.Fatal("command did not enter original scope")
		case <-tick.C:
		}
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled command reported success")
		}
	case <-time.After(6 * time.Second):
		t.Fatal("cancelled namespace not reaped")
	}
	if len(proofs.proofs) != 1 || !proofs.proofs[0].Quiescent() || proofs.uncertain {
		t.Fatalf("missing original cancellation proof: %+v", proofs)
	}
}
func TestPublisherGitNamespacePrecancelledStartsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	proofs := &gitProofLog{}
	ctx = context.WithValue(ctx, gitProofKey{}, proofs)
	var out, diagnostic boundedOutput
	if err := runPublisherGit(ctx, "/not/executed", nil, nil, &out, &diagnostic); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if len(proofs.proofs) != 0 || proofs.uncertain {
		t.Fatal("fabricated process observation")
	}
}
func TestPublisherGitUnconfirmedReapQuarantinesVerifiedSource(t *testing.T) {
	s, p := publisherServiceFixture(t)
	remote := s.remote.(*serviceRemote)
	var staged string
	remote.onPublish = func(path string) error { staged = path; return ErrPublisherGitQuiescenceUnknown }
	id := testDispatchIdentity(p)
	w := evidenceCall(t, s, p, id, "/v1/publish")
	if w.Code != http.StatusBadGateway || remote.publishCalls != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	if staged == "" {
		t.Fatal("original private snapshot missing")
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(staged)) })
	if _, err := os.Stat(staged); err != nil {
		t.Fatal("unconfirmed child lost its quarantined input", err)
	}
	if _, _, err := s.journal.completed(context.Background(), id, p); err == nil {
		t.Fatal("unconfirmed reap gained a completion witness")
	}
	if w := evidenceCall(t, s, p, id, "/v1/publish"); w.Code != http.StatusConflict {
		t.Fatal("quarantine re-admitted publication")
	}
}

// The exact former github.go implementation fails this regression: Cmd.Run
// returns after the direct parent while its detached, closed-stdio child lives.
// The fixture self-expires even on that expected failing baseline.
func TestPublisherGitOutputReapsDetachedDescendant(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	root := t.TempDir()
	ready, heartbeat := filepath.Join(root, "ready"), filepath.Join(root, "heartbeat")
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "helper.py")
	body := `import os,sys,time
pid=os.fork()
if pid==0:
 os.setsid()
 for fd in (0,1,2):
  try:os.close(fd)
  except OSError:pass
 with open(sys.argv[2],'ab',buffering=0) as f:
  open(sys.argv[1],'w').write('ready')
  deadline=time.monotonic()+2
  while time.monotonic()<deadline:f.write(b'x');time.sleep(.002)
else:
 deadline=time.monotonic()+2
 while not os.path.exists(sys.argv[1]):
  if time.monotonic()>deadline:sys.exit(4)
  time.sleep(.002)
`
	if err := os.WriteFile(script, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(root, "fixture-git")
	shell := "#!/bin/sh\nexec " + strconv.Quote(python) + " -S " + strconv.Quote(script) + " " + strconv.Quote(ready) + " " + strconv.Quote(heartbeat) + "\n"
	if err := os.WriteFile(launcher, []byte(shell), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	remote := &githubRemote{git: launcher}
	if _, err := remote.gitOutput(ctx, "fixture-not-a-credential", root, root, "ignored"); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(heartbeat)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	after, err := os.Stat(heartbeat)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("detached helper heartbeat after Git return: before=%d after=%d", before.Size(), after.Size())
	if before.Size() != after.Size() {
		t.Fatal("detached Git descendant survived original parent return")
	}
}
