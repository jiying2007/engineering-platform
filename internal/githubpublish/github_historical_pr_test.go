package githubpublish

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func historicalPullProbePlan(t *testing.T) (Plan, Configuration) {
	t.Helper()
	state, config := publisherFixture(t)
	provider, err := New(config, state, &publisherRemote{})
	if err != nil {
		t.Fatal(err)
	}
	plan, _, err := provider.derive(context.Background(), state.run.ID, state.run.CurrentEpoch, "", false)
	if err != nil {
		t.Fatal(err)
	}
	return plan, config
}

func serveHistoricalPullFixture(t *testing.T, plan Plan, branchExists, historical bool, failHistory bool, posts *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing scoped publication credential in test request")
		}
		if r.Method != http.MethodGet {
			posts.Add(1)
			w.WriteHeader(http.StatusConflict)
			return
		}
		prefix := "/repos/" + plan.Repository
		switch r.URL.Path {
		case prefix + "/git/ref/heads/" + plan.BaseRef:
			_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]string{"sha": plan.BaseCommit}})
		case prefix + "/git/ref/heads/" + plan.Branch:
			if !branchExists {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]string{"sha": plan.ResultCommit}})
		case prefix + "/pulls":
			q := r.URL.Query()
			wantHead := strings.SplitN(plan.Repository, "/", 2)[0] + ":" + plan.Branch
			if q.Get("head") != wantHead ||
				(q.Get("state") == "all" && q.Get("base") != "") ||
				(q.Get("state") == "open" && q.Get("base") != plan.BaseRef) {
				t.Errorf("GitHub PR query used incorrect head/state/base filter: %v", q)
			}
			if failHistory && q.Get("state") == "all" {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			if q.Get("state") == "all" {
				if q.Get("per_page") != "100" {
					t.Errorf("historical pull query not sufficiently bounded: %v", q)
				}
				if historical {
					_ = json.NewEncoder(w).Encode([]map[string]any{{
						"number": 123, "state": "closed",
						"head": map[string]string{"ref": plan.Branch, "sha": plan.ResultCommit},
						"base": map[string]string{"ref": plan.BaseRef, "sha": plan.BaseCommit},
					}})
				} else {
					_ = json.NewEncoder(w).Encode([]any{})
				}
				return
			}
			if q.Get("state") == "open" {
				_ = json.NewEncoder(w).Encode([]any{})
				return
			}
			t.Errorf("unknown PR-list state: %v", q)
			w.WriteHeader(http.StatusBadRequest)
		default:
			t.Errorf("unexpected GitHub endpoint %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestGitHubObserveDeletedBranchWithClosedPRNeverGrantsAbsent(t *testing.T) {
	for _, tc := range []struct {
		name       string
		historical bool
		fail       bool
		want       Observation
	}{
		{"no-ever-published-pr", false, false, ObservedAbsent},
		{"closed-pr-with-deleted-ref", true, false, ObservedPartial},
		{"unknown-upstream-history", false, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, config := historicalPullProbePlan(t)
			var writes atomic.Int32
			server := serveHistoricalPullFixture(t, plan, false, tc.historical, tc.fail, &writes)
			defer server.Close()
			g, err := newGitHubRemote(config.GitExecutable, config.TokenFile)
			if err != nil {
				t.Fatal(err)
			}
			g.apiBase = server.URL
			observed, err := g.Observe(context.Background(), plan)
			if tc.fail {
				if err == nil || observed.Outcome == ObservedAbsent {
					t.Fatalf("GitHub history failure was accepted as safe absence: %#v %v", observed, err)
				}
			} else if err != nil || observed.Outcome != tc.want {
				t.Fatalf("incorrect historical remote observation: got=%#v want=%s err=%v", observed, tc.want, err)
			}
			if writes.Load() != 0 {
				t.Fatal("reconciliation performed GitHub mutation")
			}
		})
	}
}

func TestGitHubPublishNeverReopensClosedDeterministicBranch(t *testing.T) {
	for _, branchExists := range []bool{false, true} {
		t.Run(fmt.Sprint("branch-exists=", branchExists), func(t *testing.T) {
			plan, config := historicalPullProbePlan(t)
			var writes atomic.Int32
			server := serveHistoricalPullFixture(t, plan, branchExists, true, false, &writes)
			defer server.Close()
			g, err := newGitHubRemote(config.GitExecutable, config.TokenFile)
			if err != nil {
				t.Fatal(err)
			}
			g.apiBase = server.URL
			// Emulate local Git bundle validation, but fail on push. The
			// no-replay guard must reject historic PRs before that mutation.
			gitPath := filepath.Join(t.TempDir(), "git")
			script := "#!/bin/sh\ncase \"$*\" in\n" +
				" *\"rev-parse --verify refs/ep/base^{commit}\"*) echo " + plan.BaseCommit + " ;;\n" +
				" *\"rev-parse --verify refs/ep/result^{commit}\"*) echo " + plan.ResultCommit + " ;;\n" +
				" *\"push --porcelain\"*) exit 91 ;;\n" +
				"esac\nexit 0\n"
			if err := os.WriteFile(gitPath, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			g.git = gitPath
			bundle := filepath.Join(config.ArtifactRoot, plan.ExecutionID+".bundle")
			_, err = g.Publish(context.Background(), plan, bundle)
			if err == nil || !strings.Contains(err.Error(), "prior pull request history") {
				t.Fatalf("historically closed PR did not block fresh publication: %v", err)
			}
			if writes.Load() != 0 {
				t.Fatal("historical PR guard allowed GitHub mutation")
			}
		})
	}
}

func TestGitHubHistoricalPullNullResponseFailsClosed(t *testing.T) {
	plan, config := historicalPullProbePlan(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/"+plan.Repository+"/pulls" ||
			r.URL.Query().Get("state") != "all" {
			t.Errorf("unexpected history query: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("null"))
	}))
	defer server.Close()
	g, err := newGitHubRemote(config.GitExecutable, config.TokenFile)
	if err != nil {
		t.Fatal(err)
	}
	g.apiBase = server.URL
	prior, err := g.hasHistoricalPull(context.Background(), "test-token", plan)
	if err == nil || prior {
		t.Fatalf("missing PR history array was treated as empty verified history: %v %v", prior, err)
	}
}
