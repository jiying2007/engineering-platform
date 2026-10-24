package githubpublish

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/processscope"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func TestPublisherGitStartFailurePreservesUnknownScope(t *testing.T) {
	for _, tc := range []struct {
		name      string
		started   bool
		uncertain bool
		priorReap bool
	}{
		{name: "never-started"},
		{name: "started-without-scope", started: true},
		{name: "prior-reap-does-not-cover-new-child", started: true, priorReap: true},
		{name: "prior-uncertainty-survives-prelaunch-failure", uncertain: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &gitProofLog{uncertain: tc.uncertain}
			if tc.priorReap {
				state.proofs = []processscope.Proof{testsupport.ProcessScopeFixture()}
			}
			ctx := context.WithValue(context.Background(), gitProofKey{}, state)
			cmd := &exec.Cmd{}
			if tc.started {
				// A synthetic handle tests classification only. No process operation
				// or production quiescence proof is created by this fixture.
				cmd.Process = &os.Process{}
			}
			cause := errors.New("synthetic namespace startup failure")
			err := publisherGitStartError(ctx, cmd, cause)
			if !errors.Is(err, cause) || errors.Is(err, ErrPublisherGitQuiescenceUnknown) != tc.started {
				t.Fatalf("start failure classification changed: %v", err)
			}
			wantUnknown := tc.started || tc.uncertain
			if state.uncertain != wantUnknown || publisherGitQuiescent(ctx) == wantUnknown {
				t.Fatalf("cleanup ignored unresolved original process: %+v", state)
			}
			wantProofs := 0
			if tc.priorReap {
				wantProofs = 1
			}
			if len(state.proofs) != wantProofs {
				t.Fatal("startup failure fabricated process evidence")
			}
		})
	}
}

func TestPublisherGitStartFailureKeepsVerifiedSource(t *testing.T) {
	s, plan := publisherServiceFixture(t)
	remote := s.remote.(*serviceRemote)
	var staged string
	remote.onPublish = func(path string) error {
		staged = path
		// The handle is synthetic; this exercises the original service cleanup
		// and witness boundary, not a real kernel or external publication.
		return publisherGitStartError(context.Background(), &exec.Cmd{Process: &os.Process{}}, errors.New("synthetic namespace identity failure"))
	}
	id := testDispatchIdentity(plan)
	response := evidenceCall(t, s, plan, id, "/v1/publish")
	if response.Code != http.StatusBadGateway || remote.publishCalls != 1 || staged == "" {
		t.Fatal(response.Code, response.Body.String(), remote.publishCalls)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(staged)) })
	if _, err := os.Stat(staged); err != nil {
		t.Fatal("failed scope startup deleted its verified source", err)
	}
	if _, _, err := s.journal.completed(context.Background(), id, plan); err == nil {
		t.Fatal("failed scope startup gained completion evidence")
	}
	if response := evidenceCall(t, s, plan, id, "/v1/publish"); response.Code != http.StatusConflict || remote.publishCalls != 1 {
		t.Fatal("failed scope startup authorized another publication", response.Code, remote.publishCalls)
	}
}
