package cievidence

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestGitHubClientFetchLiveFacts(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/" + TrustedRepository + "/actions/runs/42":
			fmt.Fprint(w, `{"id":42,"name":"CI","event":"push","status":"completed","conclusion":"success","run_attempt":1,"head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","head_branch":"main","path":".github/workflows/ci.yml","repository":{"full_name":"jiying2007/engineering-platform"}}`)
		case "/repos/" + TrustedRepository + "/actions/runs/42/jobs":
			fmt.Fprint(w, `{"total_count":4,"jobs":[{"id":1,"name":"go","status":"completed","conclusion":"success","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},{"id":2,"name":"offline-container-integration","status":"completed","conclusion":"success","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},{"id":3,"name":"codex-app-server-0.155.0-qualification","status":"completed","conclusion":"success","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},{"id":4,"name":"postgres-authority-restore-drill","status":"completed","conclusion":"success","head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}`)
		case "/repos/" + TrustedRepository + "/actions/runs/42/artifacts":
			fmt.Fprint(w, `{"total_count":3,"artifacts":[{"id":4,"name":"engineering-binaries-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size_in_bytes":10,"digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","expired":false,"workflow_run":{"id":42,"head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},{"id":5,"name":"codex-0.155.0-qualification-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size_in_bytes":11,"digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222","expired":false,"workflow_run":{"id":42,"head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},{"id":6,"name":"trusted-ci-evidence-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size_in_bytes":12,"digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333","expired":false,"workflow_run":{"id":42,"head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	client := &GitHubClient{base: base, http: server.Client()}
	facts, err := client.FetchLiveFacts(context.Background(), TrustedRepository, 42)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Run.ID != 42 || facts.Run.Conclusion != "success" || len(facts.Jobs) != 4 || len(facts.Artifacts) != 3 {
		t.Fatalf("unexpected live facts: %#v", facts)
	}
}

func TestGitHubClientFetchesPRIdentityAndUnchangedWorkflowBlobs(t *testing.T) {
	const head = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const baseSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const workflowBlob = "cccccccccccccccccccccccccccccccccccccccc"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/" + TrustedRepository + "/actions/runs/42":
			fmt.Fprintf(w, `{"id":42,"name":"CI","event":"pull_request","status":"completed","conclusion":"success","run_attempt":1,"head_sha":"%s","head_branch":"engineering-platform/aaaaaaaaaaaaaaaaaaaaaaaa","path":".github/workflows/ci.yml","repository":{"id":1383377268,"full_name":"jiying2007/engineering-platform"},"pull_requests":[{"number":7,"head":{"ref":"engineering-platform/aaaaaaaaaaaaaaaaaaaaaaaa","sha":"%s","repo":{"id":1383377268}},"base":{"ref":"main","sha":"%s","repo":{"id":1383377268}}}]}`, head, head, baseSHA)
		case "/repos/" + TrustedRepository + "/actions/runs/42/jobs":
			fmt.Fprintf(w, `{"total_count":4,"jobs":[{"id":1,"name":"go","status":"completed","conclusion":"success","head_sha":"%s"},{"id":2,"name":"offline-container-integration","status":"completed","conclusion":"success","head_sha":"%s"},{"id":3,"name":"codex-app-server-0.155.0-qualification","status":"completed","conclusion":"success","head_sha":"%s"},{"id":4,"name":"postgres-authority-restore-drill","status":"completed","conclusion":"success","head_sha":"%s"}]}`, head, head, head, head)
		case "/repos/" + TrustedRepository + "/actions/runs/42/artifacts":
			fmt.Fprintf(w, `{"total_count":3,"artifacts":[{"id":4,"name":"engineering-binaries-%s","size_in_bytes":10,"digest":"sha256:1111111111111111111111111111111111111111111111111111111111111111","expired":false,"workflow_run":{"id":42,"head_sha":"%s"}},{"id":5,"name":"codex-0.155.0-qualification-%s","size_in_bytes":11,"digest":"sha256:2222222222222222222222222222222222222222222222222222222222222222","expired":false,"workflow_run":{"id":42,"head_sha":"%s"}},{"id":6,"name":"trusted-ci-evidence-%s","size_in_bytes":12,"digest":"sha256:3333333333333333333333333333333333333333333333333333333333333333","expired":false,"workflow_run":{"id":42,"head_sha":"%s"}}]}`, head, head, head, head, head, head)
		case "/repos/" + TrustedRepository + "/contents/.github/workflows/ci.yml":
			ref := r.URL.Query().Get("ref")
			if ref != head && ref != baseSHA {
				http.Error(w, "unexpected ref", http.StatusBadRequest)
				return
			}
			fmt.Fprintf(w, `{"path":".github/workflows/ci.yml","type":"file","sha":"%s","size":1234}`, workflowBlob)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	client := &GitHubClient{base: base, http: server.Client()}
	facts, err := client.FetchLiveFacts(context.Background(), TrustedRepository, 42)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Run.PullRequestNumber != 7 || facts.Run.PullHeadSHA != head || facts.Run.PullBaseSHA != baseSHA ||
		facts.Run.PullHeadRepositoryID != facts.Run.RepositoryID ||
		facts.Run.PullBaseRepositoryID != facts.Run.RepositoryID ||
		facts.Run.WorkflowHeadBlobSHA != workflowBlob || facts.Run.WorkflowBaseBlobSHA != workflowBlob {
		t.Fatalf("unexpected PR live facts: %#v", facts.Run)
	}
}

func TestGitHubClientRejectsIncompletePagination(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/repos/"+TrustedRepository+"/actions/runs/42":
			fmt.Fprint(w, `{"id":42,"name":"CI","event":"push","status":"completed","conclusion":"success","run_attempt":1,"head_sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","repository":{"full_name":"jiying2007/engineering-platform"}}`)
		case r.URL.Path == "/repos/"+TrustedRepository+"/actions/runs/42/jobs":
			fmt.Fprint(w, `{"total_count":101,"jobs":[]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	base, _ := url.Parse(server.URL)
	client := &GitHubClient{base: base, http: server.Client()}
	if _, err := client.FetchLiveFacts(context.Background(), TrustedRepository, 42); err == nil {
		t.Fatal("incomplete GitHub pagination accepted")
	}
}

func TestGitHubClientRejectsWrongRepositoryBeforeNetwork(t *testing.T) {
	client, err := NewGitHubClient("")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.FetchLiveFacts(context.Background(), "other/repo", 42); err == nil {
		t.Fatal("untrusted repository accepted")
	}
}
