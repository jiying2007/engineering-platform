package githubpublish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

type responseProbeTransport func(*http.Request) (*http.Response, error)

func (f responseProbeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type responseProbeBody struct {
	reader   io.Reader
	readErr  error
	closeErr error
	onRead   func()
	onClose  func()
	closed   int
}

func (b *responseProbeBody) Read(p []byte) (int, error) {
	if b.onRead != nil {
		b.onRead()
		b.onRead = nil
	}
	if b.readErr != nil {
		return 0, b.readErr
	}
	return b.reader.Read(p)
}

func (b *responseProbeBody) Close() error {
	b.closed++
	if b.onClose != nil {
		b.onClose()
	}
	return b.closeErr
}

func responseProbeRemote(body io.ReadCloser, status int, calls *int) *githubRemote {
	return &githubRemote{apiBase: "https://api.invalid", client: &http.Client{
		Transport: responseProbeTransport(func(*http.Request) (*http.Response, error) {
			*calls++
			return &http.Response{StatusCode: status, Body: body, Header: make(http.Header)}, nil
		}),
	}}
}

func TestPublisherGitHubResponseIncomplete404(t *testing.T) {
	for _, kind := range []string{"body-read", "body-close", "body-limit", "cancel-read", "cancel-close"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cause := errors.New("private response diagnostic must not leak")
			body := &responseProbeBody{reader: strings.NewReader("not found")}
			switch kind {
			case "body-read":
				body.readErr = cause
			case "body-close":
				body.closeErr = cause
			case "body-limit":
				body.reader = strings.NewReader(strings.Repeat("x", (2<<20)+1))
			case "cancel-read":
				body.onRead = cancel
				cause = context.Canceled
			case "cancel-close":
				body.onClose = cancel
				cause = context.Canceled
			}
			calls := 0
			g := responseProbeRemote(body, http.StatusNotFound, &calls)
			sha, found, err := g.getRef(ctx, "local-test-only", "owner/repo", "heads/main")
			if err == nil || found || sha != "" {
				t.Fatalf("incomplete 404 became absence: sha=%q found=%v err=%v", sha, found, err)
			}
			if kind != "body-limit" && !errors.Is(err, cause) {
				t.Fatalf("original read/close/cancellation cause lost: %v", err)
			}
			if strings.Contains(err.Error(), "private response diagnostic") {
				t.Fatal("private cause leaked into response error text")
			}
			if calls != 1 || body.closed != 1 {
				t.Fatalf("request repeated or body not closed exactly once: %d/%d", calls, body.closed)
			}
		})
	}
}

func TestPublisherGitHubResponseCompleteStatus(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusForbidden, http.StatusConflict, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			body := &responseProbeBody{reader: strings.NewReader("")}
			calls := 0
			g := responseProbeRemote(body, status, &calls)
			sha, found, err := g.getRef(context.Background(), "local-test-only", "owner/repo", "heads/main")
			if sha != "" || found || (err == nil) != (status == http.StatusNotFound) {
				t.Fatalf("complete HTTP status misclassified: %s %v %v", sha, found, err)
			}
			if calls != 1 || body.closed != 1 {
				t.Fatal("complete status was retried or not closed")
			}
		})
	}
}

func TestPublisherGitHubResponseAtomicDecode(t *testing.T) {
	type record struct {
		SHA   string `json:"sha"`
		Count int    `json:"count"`
	}
	for _, kind := range []string{"type-error", "close-error", "cancel-close", "status-error", "valid"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			body := &responseProbeBody{reader: strings.NewReader(`{"sha":"new","count":2}`)}
			status := http.StatusOK
			switch kind {
			case "type-error":
				body.reader = strings.NewReader(`{"sha":"new","count":"wrong"}`)
			case "close-error":
				body.closeErr = errors.New("private close diagnostic")
			case "cancel-close":
				body.onClose = cancel
			case "status-error":
				status = http.StatusServiceUnavailable
			}
			calls := 0
			g := responseProbeRemote(body, status, &calls)
			before := record{SHA: "retained", Count: 1}
			out := before
			gotStatus, err := g.request(ctx, "local-test-only", http.MethodGet, "/probe", nil, nil, &out)
			if kind == "valid" {
				if err != nil || out != (record{SHA: "new", Count: 2}) {
					t.Fatalf("complete valid response rejected: %+v %v", out, err)
				}
			} else if err == nil || !reflect.DeepEqual(out, before) {
				t.Fatalf("failed response mutated accepted output: %+v %v", out, err)
			}
			if gotStatus != status || body.closed != 1 || calls != 1 {
				t.Fatal("response status, close or exchange count lost")
			}
		})
	}
}

type responseCancelInput struct{ cancel context.CancelFunc }

func (v responseCancelInput) MarshalJSON() ([]byte, error) {
	v.cancel()
	return []byte(`{}`), nil
}

func TestPublisherGitHubResponseCancelledBeforeExchange(t *testing.T) {
	for _, duringMarshal := range []bool{false, true} {
		t.Run(fmt.Sprint(duringMarshal), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var input any
			if duringMarshal {
				input = responseCancelInput{cancel: cancel}
			} else {
				cancel()
			}
			calls := 0
			body := &responseProbeBody{reader: strings.NewReader(`{}`)}
			g := responseProbeRemote(body, http.StatusCreated, &calls)
			_, err := g.request(ctx, "local-test-only", http.MethodPost, "/probe", nil, input, nil)
			if !errors.Is(err, context.Canceled) || calls != 0 {
				t.Fatalf("cancelled caller initiated exchange: calls=%d err=%v", calls, err)
			}
		})
	}
}

func truncatedRefServer(t *testing.T, plan Plan, history, mutations *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			// Real HTTP framing truncation: the transport observes the 404
			// headers followed by EOF before the declared body has arrived.
			w.Header().Set("Content-Length", "64")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("{"))
		case prefix + "/pulls":
			history.Add(1)
			_, _ = w.Write([]byte("[]"))
		default:
			t.Errorf("unexpected HTTP read %s", r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
}

func TestPublisherGitHubResponseTruncated404Observation(t *testing.T) {
	plan, config := historicalPullProbePlan(t)
	var history, mutations atomic.Int32
	server := truncatedRefServer(t, plan, &history, &mutations)
	defer server.Close()
	g, err := newGitHubRemote(config.GitExecutable, config.TokenFile)
	if err != nil {
		t.Fatal(err)
	}
	g.apiBase = server.URL
	result, err := g.Observe(context.Background(), plan)
	if err == nil || result.Outcome != "" || history.Load() != 0 || mutations.Load() != 0 {
		t.Fatalf("truncated 404 created an observation: result=%+v err=%v history=%d writes=%d", result, err, history.Load(), mutations.Load())
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("real transport EOF cause lost: %v", err)
	}
}

func TestPublisherGitHubResponseTruncated404Publication(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	plan, config := historicalPullProbePlan(t)
	var history, mutations atomic.Int32
	server := truncatedRefServer(t, plan, &history, &mutations)
	defer server.Close()
	g, err := newGitHubRemote(config.GitExecutable, config.TokenFile)
	if err != nil {
		t.Fatal(err)
	}
	g.apiBase = server.URL
	dir := t.TempDir()
	gitPath := filepath.Join(dir, "git")
	marker := filepath.Join(dir, "push-attempt")
	// A local executable stands in for Git staging and records push attempts.
	// Only loopback HTTP is real; no GitHub credential or external write exists.
	script := "#!/bin/sh\ncase \"$*\" in\n" +
		" *\"rev-parse --verify refs/ep/base^{commit}\"*) echo " + plan.BaseCommit + " ;;\n" +
		" *\"rev-parse --verify refs/ep/result^{commit}\"*) echo " + plan.ResultCommit + " ;;\n" +
		" *\"push --porcelain\"*) echo attempted > '" + marker + "' ;;\n" +
		"esac\nexit 0\n"
	if err := os.WriteFile(gitPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	g.git = gitPath
	_, err = g.Publish(context.Background(), plan, filepath.Join(config.ArtifactRoot, plan.ExecutionID+".bundle"), testDispatchIdentity(plan))
	_, markerErr := os.Stat(marker)
	if err == nil || !errors.Is(markerErr, os.ErrNotExist) || history.Load() != 0 || mutations.Load() != 0 {
		t.Fatalf("incomplete ref query reached publication: err=%v push-marker=%v history=%d writes=%d", err, markerErr, history.Load(), mutations.Load())
	}
}
