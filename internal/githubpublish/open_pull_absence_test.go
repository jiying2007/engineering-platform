package githubpublish

import (
	"context"
	"encoding/json"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestGitHubNullOpenPullReadbackNeverAuthorizesCreate(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	plan, config := historicalPullProbePlan(t)
	var mutations, historyQueries atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			mutations.Add(1)
			w.WriteHeader(http.StatusConflict)
			return
		}
		prefix := "/repos/" + plan.Repository
		switch r.URL.Path {
		case prefix + "/git/ref/heads/" + plan.BaseRef:
			_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]string{"sha": plan.BaseCommit}})
		case prefix + "/git/ref/heads/" + plan.Branch:
			_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]string{"sha": plan.ResultCommit}})
		case prefix + "/pulls":
			if r.URL.Query().Get("state") == "open" {
				_, _ = w.Write([]byte("null"))
				return
			}
			historyQueries.Add(1)
			_, _ = w.Write([]byte("[]"))
		default:
			t.Errorf("unexpected read: %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	g, err := newGitHubRemote(config.GitExecutable, config.TokenFile)
	if err != nil {
		t.Fatal(err)
	}
	g.apiBase = server.URL
	// Only local stage validation is simulated. All REST decisions traverse the
	// real client/server. No real Git push or provider credential is used.
	gitPath := filepath.Join(t.TempDir(), "git")
	script := "#!/bin/sh\ncase \"$*\" in\n" +
		" *\"rev-parse --verify refs/ep/base^{commit}\"*) echo " + plan.BaseCommit + " ;;\n" +
		" *\"rev-parse --verify refs/ep/result^{commit}\"*) echo " + plan.ResultCommit + " ;;\n" +
		" *\"push --porcelain\"*) exit 91 ;;\n" +
		"esac\nexit 0\n"
	if err := os.WriteFile(gitPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	g.git = gitPath
	if _, err := g.Publish(context.Background(), plan, filepath.Join(config.ArtifactRoot, plan.ExecutionID+".bundle"), testDispatchIdentity(plan)); err == nil {
		t.Fatal("unobserved open-PR query was accepted")
	}
	if mutations.Load() != 0 || historyQueries.Load() != 0 {
		t.Fatalf("unusable query became publication authority: mutations=%d later-history=%d", mutations.Load(), historyQueries.Load())
	}
	result, err := g.Observe(context.Background(), plan)
	if err == nil || result.Outcome != "" {
		t.Fatalf("null open-PR query became an observed fact: %+v %v", result, err)
	}
}

func TestGitHubOpenPullAbsenceRequiresAnExplicitArray(t *testing.T) {
	for _, body := range []string{"null", "{}", "\"null\"", "[", "[]"} {
		t.Run(body, func(t *testing.T) {
			plan, config := historicalPullProbePlan(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Error("readback made a mutation")
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			g, err := newGitHubRemote(config.GitExecutable, config.TokenFile)
			if err != nil {
				t.Fatal(err)
			}
			g.apiBase = server.URL
			_, found, err := g.findOpenPull(context.Background(), "test-token", plan)
			if body == "[]" {
				if err != nil || found {
					t.Fatalf("explicit empty list rejected: %v", err)
				}
			} else if err == nil || found {
				t.Fatal("unusable response was treated as empty history")
			}
		})
	}
}
