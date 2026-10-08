package githubpublish

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGitHubAuthenticatedAPINeverFollowsRedirect(t *testing.T) {
	dir := t.TempDir()
	gitPath := filepath.Join(dir, "git")
	if err := os.WriteFile(gitPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte("test-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	g, err := newGitHubRemote(gitPath, tokenPath)
	if err != nil {
		t.Fatal(err)
	}

	var redirectTargetCalls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectTargetCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/start" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("publisher request identity/header changed before redirection")
		}
		w.Header().Set("Location", target.URL+"/sink")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	g.apiBase = origin.URL

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPatch} {
		t.Run(method, func(t *testing.T) {
			var input any
			if method != http.MethodGet {
				input = map[string]string{"value": "publisher-request"}
			}
			status, err := g.request(context.Background(), "test-token", method, "/start", nil, input, nil)
			if status != http.StatusTemporaryRedirect || err == nil {
				t.Fatalf("credentialed %s followed HTTP redirect: status=%d err=%v", method, status, err)
			}
		})
	}
	if got := redirectTargetCalls.Load(); got != 0 {
		t.Fatalf("redirected target received %d credentialed publisher requests", got)
	}
}

func TestPublisherGitCredentialNeverFollowsRemoteRedirect(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git executable required for direct Git transport policy test")
	}
	dir := t.TempDir()
	g := &githubRemote{git: gitPath}
	got, err := g.gitOutput(context.Background(), "test-token", dir, dir, "config", "--get", "http.followRedirects")
	if err != nil || strings.TrimSpace(got) != "false" {
		t.Fatalf("Git transport redirects not disabled: value=%q err=%v", got, err)
	}
}
