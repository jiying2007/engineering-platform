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

type pullRepositoryCase struct {
	name   string
	mutate func(map[string]any)
}

func pullRepositoryCases() []pullRepositoryCase {
	cases := []pullRepositoryCase{{name: "same-repository"}}
	for _, side := range []string{"head", "base"} {
		cases = append(cases,
			pullRepositoryCase{side + "-missing", func(pr map[string]any) {
				delete(pr[side].(map[string]any), "repo")
			}},
			pullRepositoryCase{side + "-null", func(pr map[string]any) {
				pr[side].(map[string]any)["repo"] = nil
			}},
			pullRepositoryCase{side + "-wrong-type", func(pr map[string]any) {
				pr[side].(map[string]any)["repo"] = "not-a-repository"
			}},
			pullRepositoryCase{side + "-foreign-repository", func(pr map[string]any) {
				repo := pr[side].(map[string]any)["repo"].(map[string]any)
				repo["full_name"] = strings.SplitN(repo["full_name"].(string), "/", 2)[0] + "/another-fork"
				repo["id"] = 456
			}},
			pullRepositoryCase{side + "-same-id-wrong-name", func(pr map[string]any) {
				repo := pr[side].(map[string]any)["repo"].(map[string]any)
				repo["full_name"] = strings.SplitN(repo["full_name"].(string), "/", 2)[0] + "/another-fork"
			}},
			pullRepositoryCase{side + "-missing-id", func(pr map[string]any) {
				delete(pr[side].(map[string]any)["repo"].(map[string]any), "id")
			}},
			pullRepositoryCase{side + "-wrong-id-type", func(pr map[string]any) {
				pr[side].(map[string]any)["repo"].(map[string]any)["id"] = "123"
			}},
		)
	}
	return append(cases, pullRepositoryCase{"inconsistent-repository-ids", func(pr map[string]any) {
		pr["head"].(map[string]any)["repo"].(map[string]any)["id"] = 456
	}})
}

func pullRepositoryResponse(plan Plan) map[string]any {
	return map[string]any{
		"number": 17, "html_url": "https://github.com/" + plan.Repository + "/pull/17",
		"state": "open", "title": "existing title", "body": "existing body",
		"head": map[string]any{"ref": plan.Branch, "sha": plan.ResultCommit,
			"repo": map[string]any{"id": 123, "full_name": plan.Repository}},
		"base": map[string]any{"ref": plan.BaseRef, "sha": plan.BaseCommit,
			"repo": map[string]any{"id": 123, "full_name": plan.Repository}},
	}
}

func servePullRepositoryResponse(t *testing.T, plan Plan, response map[string]any, mutations *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Error("test publication credential missing")
		}
		prefix := "/repos/" + plan.Repository
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			mutations.Add(1)
			if r.Method != http.MethodPatch || r.URL.Path != prefix+"/pulls/17" {
				t.Errorf("unexpected mutation %s %s", r.Method, r.URL)
				w.WriteHeader(http.StatusConflict)
				return
			}
			// An invalid head/base must be rejected before this PATCH is sent.
			// Echoing the same PR makes the old implementation's false success
			// visible through the complete HTTP response path.
			_ = json.NewEncoder(w).Encode(response)
			return
		}
		switch r.URL.Path {
		case prefix + "/git/ref/heads/" + plan.BaseRef:
			_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]string{"sha": plan.BaseCommit}})
		case prefix + "/git/ref/heads/" + plan.Branch:
			_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]string{"sha": plan.ResultCommit}})
		case prefix + "/pulls":
			query := r.URL.Query()
			if query.Get("state") != "open" || query.Get("base") != plan.BaseRef ||
				query.Get("head") != strings.SplitN(plan.Repository, "/", 2)[0]+":"+plan.Branch {
				t.Errorf("wrong publication query: %s", r.URL)
			}
			_ = json.NewEncoder(w).Encode([]any{response})
		default:
			t.Errorf("unexpected publication read: %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestGitHubObserveRequiresExactPullRepositoryIdentity(t *testing.T) {
	for _, tc := range pullRepositoryCases() {
		t.Run(tc.name, func(t *testing.T) {
			plan, config := historicalPullProbePlan(t)
			response := pullRepositoryResponse(plan)
			if tc.mutate != nil {
				tc.mutate(response)
			}
			var mutations atomic.Int32
			server := servePullRepositoryResponse(t, plan, response, &mutations)
			defer server.Close()
			g, err := newGitHubRemote(config.GitExecutable, config.TokenFile)
			if err != nil {
				t.Fatal(err)
			}
			g.apiBase = server.URL
			observed, err := g.Observe(context.Background(), plan)
			if tc.mutate == nil {
				if err != nil || observed.Outcome != ObservedConfirmed || observed.Receipt.validateForObservation(plan) != nil {
					t.Fatalf("same-repository observation rejected: %+v %v", observed, err)
				}
			} else if observed.Outcome == ObservedConfirmed || observed.Outcome == ObservedAbsent || observed.Receipt != (PublicationReceipt{}) {
				t.Fatalf("unbound PR repository became an authoritative observation: %+v %v", observed, err)
			}
			if mutations.Load() != 0 {
				t.Fatal("observation issued a mutation")
			}
		})
	}
}

func TestGitHubPublishRequiresExactPullRepositoryBeforeUpdate(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	for _, tc := range pullRepositoryCases() {
		t.Run(tc.name, func(t *testing.T) {
			plan, config := historicalPullProbePlan(t)
			response := pullRepositoryResponse(plan)
			if tc.mutate != nil {
				tc.mutate(response)
			}
			var mutations atomic.Int32
			server := servePullRepositoryResponse(t, plan, response, &mutations)
			defer server.Close()
			g, err := newGitHubRemote(config.GitExecutable, config.TokenFile)
			if err != nil {
				t.Fatal(err)
			}
			g.apiBase = server.URL
			// Only local Git bundle checks are simulated. Each command runs in
			// the real namespace scope; HTTP reads/PATCH use the actual transport.
			// This fixture has no external credential and never performs a push.
			gitPath := filepath.Join(t.TempDir(), "git")
			script := fmt.Sprintf("#!/bin/sh\ncase \"$*\" in\n *\"rev-parse --verify refs/ep/base^{commit}\"*) echo %s ;;\n *\"rev-parse --verify refs/ep/result^{commit}\"*) echo %s ;;\n *\"push --porcelain\"*) exit 91 ;;\nesac\nexit 0\n", plan.BaseCommit, plan.ResultCommit)
			if err := os.WriteFile(gitPath, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			g.git = gitPath
			receipt, err := g.Publish(context.Background(), plan, filepath.Join(config.ArtifactRoot, plan.ExecutionID+".bundle"), testDispatchIdentity(plan))
			if tc.mutate == nil {
				if err != nil || receipt.validateForPublish(plan) != nil || receipt.PublicationOutcome != "UPDATED" || mutations.Load() != 1 {
					t.Fatalf("same-repository update rejected: %+v %v mutations=%d", receipt, err, mutations.Load())
				}
			} else if err == nil || receipt != (PublicationReceipt{}) || mutations.Load() != 0 {
				t.Fatalf("unbound existing PR reached publication update: %+v %v mutations=%d", receipt, err, mutations.Load())
			}
		})
	}
}

func TestGitHubPublishRequiresExactPullRepositoryAfterMutation(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	for _, operation := range []string{"create", "update"} {
		for _, tc := range []pullRepositoryCase{
			{name: "same-repository"},
			{name: "foreign-head", mutate: func(pr map[string]any) {
				repo := pr["head"].(map[string]any)["repo"].(map[string]any)
				repo["id"], repo["full_name"] = 456, strings.SplitN(repo["full_name"].(string), "/", 2)[0]+"/another-fork"
			}},
			{name: "null-head", mutate: func(pr map[string]any) { pr["head"].(map[string]any)["repo"] = nil }},
			{name: "foreign-base", mutate: func(pr map[string]any) {
				repo := pr["base"].(map[string]any)["repo"].(map[string]any)
				repo["id"], repo["full_name"] = 456, strings.SplitN(repo["full_name"].(string), "/", 2)[0]+"/another-fork"
			}},
			{name: "null-base", mutate: func(pr map[string]any) { pr["base"].(map[string]any)["repo"] = nil }},
		} {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				plan, config := historicalPullProbePlan(t)
				var mutations, historyQueries atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					prefix := "/repos/" + plan.Repository
					w.Header().Set("Content-Type", "application/json")
					if r.Header.Get("Authorization") != "Bearer test-token" {
						t.Error("test publication credential missing")
					}
					if r.Method == http.MethodGet {
						switch r.URL.Path {
						case prefix + "/git/ref/heads/" + plan.BaseRef:
							_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]string{"sha": plan.BaseCommit}})
						case prefix + "/git/ref/heads/" + plan.Branch:
							_ = json.NewEncoder(w).Encode(map[string]any{"object": map[string]string{"sha": plan.ResultCommit}})
						case prefix + "/pulls":
							query := r.URL.Query()
							if query.Get("head") != strings.SplitN(plan.Repository, "/", 2)[0]+":"+plan.Branch {
								t.Error("publication head query drift")
							}
							if query.Get("state") == "open" {
								if operation == "update" {
									// Only the later PATCH response changes identity.
									_ = json.NewEncoder(w).Encode([]any{pullRepositoryResponse(plan)})
								} else {
									_, _ = w.Write([]byte("[]"))
								}
								return
							}
							if operation != "create" || query.Get("state") != "all" {
								t.Error("unexpected PR history lookup")
							}
							historyQueries.Add(1)
							_, _ = w.Write([]byte("[]"))
						default:
							t.Errorf("unexpected publication read: %s", r.URL)
							w.WriteHeader(http.StatusNotFound)
						}
						return
					}
					mutations.Add(1)
					method, endpoint := http.MethodPost, prefix+"/pulls"
					if operation == "update" {
						method, endpoint = http.MethodPatch, prefix+"/pulls/17"
					}
					if r.Method != method || r.URL.Path != endpoint {
						t.Errorf("unexpected mutation: %s %s", r.Method, r.URL)
						w.WriteHeader(http.StatusConflict)
						return
					}
					response := pullRepositoryResponse(plan)
					response["title"], response["body"] = publicationText(plan)
					if tc.mutate != nil {
						tc.mutate(response)
					}
					_ = json.NewEncoder(w).Encode(response)
				}))
				defer server.Close()
				g, err := newGitHubRemote(config.GitExecutable, config.TokenFile)
				if err != nil {
					t.Fatal(err)
				}
				g.apiBase = server.URL
				// The exact branch already exists, so this local namespace fixture
				// must never push. Only the REST mutation/response boundary is tested.
				gitPath := filepath.Join(t.TempDir(), "git")
				script := fmt.Sprintf("#!/bin/sh\ncase \"$*\" in\n *\"rev-parse --verify refs/ep/base^{commit}\"*) echo %s ;;\n *\"rev-parse --verify refs/ep/result^{commit}\"*) echo %s ;;\n *\"push --porcelain\"*) exit 91 ;;\nesac\nexit 0\n", plan.BaseCommit, plan.ResultCommit)
				if err := os.WriteFile(gitPath, []byte(script), 0700); err != nil {
					t.Fatal(err)
				}
				g.git = gitPath
				receipt, err := g.Publish(context.Background(), plan, filepath.Join(config.ArtifactRoot, plan.ExecutionID+".bundle"), testDispatchIdentity(plan))
				if tc.mutate == nil {
					outcome := "CREATED"
					if operation == "update" {
						outcome = "UPDATED"
					}
					if err != nil || receipt.validateForPublish(plan) != nil || receipt.PublicationOutcome != outcome {
						t.Fatalf("valid mutation response rejected: %+v %v", receipt, err)
					}
				} else if err == nil || receipt != (PublicationReceipt{}) {
					t.Fatalf("unbound mutation response became a publication receipt: %+v %v", receipt, err)
				}
				wantHistory := int32(0)
				if operation == "create" {
					wantHistory = 1
				}
				if mutations.Load() != 1 || historyQueries.Load() != wantHistory {
					t.Fatalf("mutation replayed or absence checks changed: mutations=%d history=%d", mutations.Load(), historyQueries.Load())
				}
			})
		}
	}
}
