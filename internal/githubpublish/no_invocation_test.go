//go:build linux

package githubpublish

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func TestPublisherNoInvocationFenceRejectsActiveAndReplays(t *testing.T) {
	s, p := publisherServiceFixture(t)
	id := testDispatchIdentity(p)
	if w := evidenceCall(t, s, p, id, "/v1/dispatch-no-invocation"); w.Code != http.StatusConflict {
		t.Fatal("absent history was sealed", w.Code)
	}
	lease, start, err := s.journal.begin(context.Background(), id, p)
	if err != nil {
		t.Fatal(err)
	}
	if start.Version != 2 {
		t.Fatal("new starts lack barrier contract")
	}
	// A second object in the SAME process cannot borrow the active description.
	reopened, err := newDispatchJournal(s.journal.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reopened.sealUninvoked(context.Background(), id, p); err == nil {
		t.Fatal("live caller was sealed")
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	w := evidenceCall(t, s, p, id, "/v1/dispatch-no-invocation")
	if w.Code != http.StatusOK {
		t.Fatal(w.Code, w.Body.String())
	}
	var e NonInvocationEvidence
	if json.Unmarshal(w.Body.Bytes(), &e) != nil || e.Validate() != nil {
		t.Fatal("invalid proof")
	}
	again := evidenceCall(t, s, p, id, "/v1/dispatch-no-invocation")
	if again.Code != http.StatusOK || again.Body.String() != w.Body.String() {
		t.Fatal("repeat changed immutable evidence")
	}
	if w = evidenceCall(t, s, p, id, "/v1/publish"); w.Code != http.StatusConflict {
		t.Fatal("sealed caller replayed")
	}
	id.OperationID = "different"
	id.IdempotencyKey = "different"
	if w = evidenceCall(t, s, p, id, "/v1/publish"); w.Code != http.StatusConflict {
		t.Fatal("new identity bypassed occupied slot")
	}
	if w = evidenceCall(t, s, p, id, "/v1/dispatch-no-invocation"); w.Code != http.StatusConflict {
		t.Fatal("wrong original identity accepted")
	}
	r := s.remote.(*serviceRemote)
	if r.publishCalls != 0 || r.observeCalls != 0 {
		t.Fatal("non-invocation recovery touched remote")
	}
}
func TestPublisherNoInvocationFenceRejectsHistoricalAndArmed(t *testing.T) {
	for _, mode := range []string{"v1", "intent", "partial-intent", "symlink-intent", "missing-start", "corrupt-seal", "foreign-seal", "completed"} {
		t.Run(mode, func(t *testing.T) {
			s, p := publisherServiceFixture(t)
			id := testDispatchIdentity(p)
			lease, start, err := s.journal.begin(context.Background(), id, p)
			if err != nil {
				t.Fatal(err)
			}
			slot := filepath.Join(s.journal.path, dispatchSlot(p))
			switch mode {
			case "v1":
				start.Version = 1
				start.Incarnation = ""
				raw, _ := json.Marshal(start)
				err = os.WriteFile(filepath.Join(slot, "started.json"), raw, 0600)
			case "intent":
				err = lease.arm(context.Background(), start)
			case "partial-intent":
				err = os.WriteFile(filepath.Join(slot, "invoked.json"), []byte("{"), 0600)
			case "symlink-intent":
				err = os.Symlink("started.json", filepath.Join(slot, "invoked.json"))
			case "missing-start":
				err = os.Remove(filepath.Join(slot, "started.json"))
			case "corrupt-seal":
				err = os.WriteFile(filepath.Join(slot, "not-invoked.json"), []byte("null"), 0600)
			case "foreign-seal":
				err = writeWitness(context.Background(), lease.Root, "not-invoked.json", DispatchNonInvocation{Kind: "PUBLISH_NOT_INVOKED_V1", StartDigest: canonical.BytesDigest([]byte("wrong")), SealedAt: time.Now().UTC()})
			case "completed":
				err = os.WriteFile(filepath.Join(slot, "completed.json"), []byte("{}"), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = lease.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err = s.journal.sealUninvoked(context.Background(), id, p); err == nil {
				t.Fatal("ambiguous/historical slot promoted")
			}
			if w := evidenceCall(t, s, p, id, "/v1/publish"); w.Code != http.StatusConflict {
				t.Fatal("rejected slot became replayable")
			}
			r := s.remote.(*serviceRemote)
			if r.publishCalls != 0 || r.observeCalls != 0 {
				t.Fatal("negative rejection touched remote")
			}
		})
	}
}
func TestPublisherInvocationBarrierPrecedesRemote(t *testing.T) {
	s, p := publisherServiceFixture(t)
	id := testDispatchIdentity(p)
	r := s.remote.(*serviceRemote)
	r.onPublish = func(string) error {
		root, err := os.OpenRoot(filepath.Join(s.journal.path, dispatchSlot(p)))
		if err != nil {
			return err
		}
		defer root.Close()
		var st DispatchStart
		var i DispatchInvocation
		if err = readWitness(context.Background(), root, "started.json", &st); err != nil {
			return err
		}
		if err = readWitness(context.Background(), root, "invoked.json", &i); err != nil {
			return err
		}
		if err = i.Validate(st); err != nil {
			return err
		}
		if _, err = s.journal.sealUninvoked(context.Background(), id, p); err == nil {
			return fmt.Errorf("running publish was fenced")
		}
		return errors.New("original remote result unknown")
	}
	if w := evidenceCall(t, s, p, id, "/v1/publish"); w.Code != http.StatusBadGateway {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := s.journal.sealUninvoked(context.Background(), id, p); err == nil {
		t.Fatal("failed original call treated as never invoked")
	}
	if r.publishCalls != 1 || r.observeCalls != 0 {
		t.Fatal("unexpected external call count")
	}
}
func TestPublisherUninvokedLeaseRejectsCancelledArming(t *testing.T) {
	s, p := publisherServiceFixture(t)
	id := testDispatchIdentity(p)
	lease, st, err := s.journal.begin(context.Background(), id, p)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(lease.arm(ctx, st), context.Canceled) {
		t.Fatal("cancelled lease armed")
	}
	if !witnessAbsent(lease.Root, "invoked.json") {
		t.Fatal("cancelled before arm wrote intent")
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.journal.sealUninvoked(ctx, id, p); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled fence changed slot")
	}
	if _, err = s.journal.sealUninvoked(context.Background(), id, p); err != nil {
		t.Fatal(err)
	}
	// A closed/released lease must not authorize any later invocation.
	if lease.arm(context.Background(), st) == nil {
		t.Fatal("released lease armed")
	}
}

func TestPublisherWitnessLockChild(t *testing.T) {
	path := os.Getenv("EP_TEST_WITNESS_CHILD")
	if path == "" {
		return
	}
	var request publishRequest
	if json.Unmarshal([]byte(os.Getenv("EP_TEST_WITNESS_REQUEST")), &request) != nil {
		t.Fatal("bad child request")
	}
	j, err := newDispatchJournal(path)
	if err != nil {
		t.Fatal(err)
	}
	lease, start, err := j.begin(context.Background(), request.Identity, request.Plan)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	switch os.Getenv("EP_TEST_WITNESS_MODE") {
	case "armed-no-certificate":
		if err := lease.arm(context.Background(), start); err != nil {
			t.Fatal(err)
		}
	case "armed-cancelled":
		parent, cancel := context.WithCancel(context.Background())
		defer cancel()
		ctx := cancelAfterIntentContext{parent, cancel, filepath.Join(path, dispatchSlot(request.Plan), "invoked.json")}
		if !errors.Is(lease.prepareInvocation(ctx, start), context.Canceled) {
			t.Fatal("original child did not retain cancellation")
		}
	}
	fmt.Println("STARTED_LOCK_HELD")
	// Parent owns this pipe; finite parent deadline guarantees cleanup on failure.
	var one [1]byte
	_, _ = os.Stdin.Read(one[:])
}
func TestPublisherNoInvocationFenceAfterRealProcessExit(t *testing.T) {
	for _, mode := range []string{"before-arm", "armed-no-certificate", "armed-cancelled"} {
		t.Run(mode, func(t *testing.T) { testPublisherFenceAfterRealProcessExit(t, mode) })
	}
}
func testPublisherFenceAfterRealProcessExit(t *testing.T, mode string) {
	s, p := publisherServiceFixture(t)
	id := testDispatchIdentity(p)
	raw, _ := json.Marshal(publishRequest{Plan: p, Identity: id})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPublisherWitnessLockChild$")
	cmd.Env = append(os.Environ(), "EP_TEST_WITNESS_CHILD="+s.journal.path, "EP_TEST_WITNESS_REQUEST="+string(raw), "EP_TEST_WITNESS_MODE="+mode)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	ready, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || strings.TrimSpace(ready) != "STARTED_LOCK_HELD" {
		t.Fatal("child not ready", ready, err)
	}
	if _, err = s.journal.sealUninvoked(context.Background(), id, p); err == nil {
		t.Fatal("live other process was sealed")
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err = cmd.Wait(); err == nil {
		t.Fatal("SIGKILL child unexpectedly succeeded")
	}
	waited = true
	e, err := s.journal.sealUninvoked(context.Background(), id, p)
	if mode == "armed-no-certificate" {
		if err == nil {
			t.Fatal("dead process manufactured missing original cancellation certificate")
		}
	} else {
		version := 1
		if mode == "armed-cancelled" {
			version = 2
		}
		if err != nil || e.Validate() != nil || e.Version != version {
			t.Fatal("original pre-entry evidence lost", e.Version, err)
		}
	}
	if w := evidenceCall(t, s, p, id, "/v1/publish"); w.Code != http.StatusConflict {
		t.Fatal("killed original execution replayed")
	}
}

func TestPublisherNoInvocationRejectsRestoredStartSnapshot(t *testing.T) {
	s, p := publisherServiceFixture(t)
	id := testDispatchIdentity(p)
	lease, _, err := s.journal.begin(context.Background(), id, p)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	original := filepath.Join(s.journal.path, dispatchSlot(p))
	copyRoot := t.TempDir()
	if err = os.Chmod(copyRoot, 0700); err != nil {
		t.Fatal(err)
	}
	copySlot := filepath.Join(copyRoot, dispatchSlot(p))
	if err = os.Mkdir(copySlot, 0700); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(original, "started.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(copySlot, "started.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	restored, err := newDispatchJournal(copyRoot)
	if err != nil {
		t.Fatal(err)
	}
	// The backup has no intent, while the original can still go on to publish.
	// Its different inode must be rejected regardless of whether the old lock is held.
	if _, err = restored.sealUninvoked(context.Background(), id, p); err == nil {
		t.Fatal("old snapshot synthesized no-effect proof")
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = restored.sealUninvoked(context.Background(), id, p); err == nil {
		t.Fatal("old snapshot accepted after original stopped")
	}
	if _, err = s.journal.sealUninvoked(context.Background(), id, p); err != nil {
		t.Fatal("original no-invocation not sealable", err)
	}
}
func TestPublisherNoInvocationRejectsOtherBootIdentity(t *testing.T) {
	s, p := publisherServiceFixture(t)
	id := testDispatchIdentity(p)
	lease, start, err := s.journal.begin(context.Background(), id, p)
	if err != nil {
		t.Fatal(err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	start.Incarnation = canonical.BytesDigest([]byte("a different boot/device/inode"))
	raw, _ := json.Marshal(start)
	if err = os.WriteFile(filepath.Join(s.journal.path, dispatchSlot(p), "started.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.journal.sealUninvoked(context.Background(), id, p); err == nil {
		t.Fatal("foreign boot/slot accepted")
	}
}

func TestPublisherNoInvocationEvidenceRealMTLS(t *testing.T) {
	s, p := publisherServiceFixture(t)
	id := testDispatchIdentity(p)
	lease, _, err := s.journal.begin(context.Background(), id, p)
	if err != nil {
		t.Fatal(err)
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	pki := testsupport.NewPKI(t)
	server := httptest.NewUnstartedServer(s.Handler())
	server.TLS = pki.ServerTLS()
	server.StartTLS()
	defer server.Close()
	client, err := newRemoteClient(healthRemoteConfig(t, pki, server.URL, s.controlSubject))
	if err != nil {
		t.Fatal(err)
	}
	defer client.client.CloseIdleConnections()
	wrong, err := newRemoteClient(healthRemoteConfig(t, pki, server.URL, "urn:engineering-platform:worker:wrong"))
	if err != nil {
		t.Fatal(err)
	}
	defer wrong.client.CloseIdleConnections()
	if _, err = wrong.FenceDispatch(context.Background(), id, p); err == nil {
		t.Fatal("wrong peer sealed original slot")
	}
	evidence, err := client.FenceDispatch(context.Background(), id, p)
	if err != nil || evidence.Validate() != nil {
		t.Fatal(evidence, err)
	}
	repeated, err := client.FenceDispatch(context.Background(), id, p)
	a, _ := canonical.Digest(evidence)
	b, _ := canonical.Digest(repeated)
	if err != nil || a != b {
		t.Fatal("mTLS repeat changed proof", err)
	}
	if _, err = client.Publish(context.Background(), p, "publisher resolves its own bundle", id); err == nil {
		t.Fatal("mTLS fence allowed publication")
	}
	r := s.remote.(*serviceRemote)
	if r.publishCalls != 0 || r.observeCalls != 0 {
		t.Fatal("no-invocation evidence called a remote")
	}
}
