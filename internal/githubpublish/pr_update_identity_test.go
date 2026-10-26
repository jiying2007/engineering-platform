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

	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func TestPublisherPRUpdateRejectsChangedPRNumber(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	plan, config := historicalPullProbePlan(t)
	expected := pullRepositoryResponse(plan)
	changed := pullRepositoryResponse(plan)
	changed["number"] = 18
	changed["html_url"] = "https://github.com/" + plan.Repository + "/pull/18"
	var patches atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		prefix := "/repos/" + plan.Repository
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("test publisher credential missing")
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == prefix+"/git/ref/heads/"+plan.BaseRef:
			_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]string{"sha": plan.BaseCommit}})
		case r.Method == http.MethodGet && r.URL.Path == prefix+"/git/ref/heads/"+plan.Branch:
			_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]string{"sha": plan.ResultCommit}})
		case r.Method == http.MethodGet && r.URL.Path == prefix+"/pulls":
			if r.URL.Query().Get("state") != "open" {
				t.Errorf("unexpected PR query: %s", r.URL)
			}
			_ = json.NewEncoder(w).Encode([]any{expected})
		case r.Method == http.MethodPatch && r.URL.Path == prefix+"/pulls/17":
			patches.Add(1)
			_ = json.NewEncoder(w).Encode(changed)
		default:
			t.Errorf("unexpected publication request %s %s", r.Method, r.URL)
			http.Error(w, "unexpected request", http.StatusConflict)
		}
	}))
	defer server.Close()
	g, err := newGitHubRemote(config.GitExecutable, config.TokenFile)
	if err != nil {
		t.Fatal(err)
	}
	g.apiBase = server.URL
	gitPath := filepath.Join(t.TempDir(), "git")
	script := fmt.Sprintf("#!/bin/sh\ncase \"$*\" in\n *\"rev-parse --verify refs/ep/base^{commit}\"*) echo %s ;;\n *\"rev-parse --verify refs/ep/result^{commit}\"*) echo %s ;;\n *\"push --porcelain\"*) exit 91 ;;\nesac\nexit 0\n", plan.BaseCommit, plan.ResultCommit)
	if err := os.WriteFile(gitPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	g.git = gitPath
	receipt, err := g.Publish(context.Background(), plan, filepath.Join(config.ArtifactRoot, plan.ExecutionID+".bundle"), testDispatchIdentity(plan))
	if err == nil || receipt != (PublicationReceipt{}) || patches.Load() != 1 {
		t.Fatalf("PR identity drift was accepted: receipt=%+v err=%v PATCH=%d", receipt, err, patches.Load())
	}
	if strings.Contains(err.Error(), "Bearer") {
		t.Fatal("credential leaked in PR identity failure")
	}
}
