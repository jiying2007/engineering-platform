package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/run"
	"github.com/jiying2007/engineering-platform/internal/session"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

func continueCLIFixture(t *testing.T, actor string) codexexec.ContinuationReceipt {
	t.Helper()
	q := codexexec.ContinueRequest{SourceRunID: "source", RunID: "next", AttemptID: "attempt", CheckpointDigest: canonical.BytesDigest([]byte("checkpoint")), ExpectedRunVersion: 1, ExpectedWorkVersion: 3, ExecutionEpoch: 1, Reason: "explicit source continuation"}
	input := core.RunInputManifest{RunID: q.RunID, TaskContractDigest: canonical.BytesDigest([]byte("task")), RuntimeProfile: "codex/runtime", WorkerProfile: "worker/codex", ToolProfile: "codex/" + canonical.BytesDigest([]byte("profile")), PolicyProfile: "policy", Continuation: &core.ContinuationRef{SourceRunID: q.SourceRunID, CheckpointDigest: q.CheckpointDigest, ArchiveDigest: canonical.BytesDigest([]byte("archive")), ArchiveSize: 1, SnapshotDigest: canonical.BytesDigest([]byte("snapshot"))}}
	d, e := input.Digest()
	if e != nil {
		t.Fatal(e)
	}
	value := run.New(q.RunID, input.TaskContractDigest, d)
	now := time.Now().UTC()
	attempt, e := value.StartAttempt(q.AttemptID, now)
	if e != nil {
		t.Fatal(e)
	}
	r := codexexec.ContinuationReceipt{Version: 1, Actor: actor, Request: q, Run: *value, Attempt: attempt, Session: *session.New(q.RunID, 1), Input: input, SourceDisposition: codexexec.Stopped, CreatedAt: now}
	if r.Validate() != nil {
		t.Fatal("invalid CLI fixture")
	}
	return r
}
func TestRunContinueStrictPrivateInput(t *testing.T) {
	q := continueCLIFixture(t, "urn:engineering-platform:engineer:test").Request
	file := filepath.Join(t.TempDir(), "request.json")
	raw, _ := json.Marshal(q)
	if e := os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	valid := []string{"authorize", "--request", file}
	o, e := parseContinue(valid)
	if e != nil || o.q != q {
		t.Fatal(o, e)
	}
	for _, args := range [][]string{nil, {"resume"}, {"takeover"}, {"status", "--run", "../other"}, {"authorize", "--run", "source"}, {"status", "--request", file}, {"authorize", "--request", file, "--actor", "spoof"}, {"authorize", "--request", file, "trailing"}, {"authorize", "--request", file, "--run", "source"}} {
		if _, e := parseContinue(args); e == nil {
			t.Fatal("unsafe command admitted", args)
		}
	}
	link := file + "-link"
	if e := os.Symlink(file, link); e != nil {
		t.Fatal(e)
	}
	if _, e := parseContinue([]string{"authorize", "--request", link}); e == nil {
		t.Fatal("alias admitted")
	}
	for _, bad := range [][]byte{[]byte(`{"actor":"spoof"}`), append(raw, []byte(` {}`)...), []byte(strings.Repeat("x", 16<<10+1)), []byte(`{"source_run_id":"source","source_run_id":"other"}`)} {
		if e := os.WriteFile(file, bad, 0600); e != nil {
			t.Fatal(e)
		}
		if _, e := parseContinue(valid); e == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
	if e := os.WriteFile(file, raw, 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Chmod(file, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := parseContinue(valid); e == nil {
		t.Fatal("nonprivate decision accepted")
	}
}
func TestRunContinueMTLSReceiptBindingAndNoReplay(t *testing.T) {
	for _, mode := range []string{"authorize", "status", "lost", "changed-actor", "changed-decision", "changed-source", "changed-input"} {
		t.Run(mode, func(t *testing.T) {
			actor := "urn:engineering-platform:engineer:cli-test"
			good := continueCLIFixture(t, actor)
			pki := testsupport.NewPKI(t)
			var calls atomic.Int32
			h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.TLS.PeerCertificates[0].URIs[0].String() != actor {
					t.Error("TLS identity drift")
				}
				method, path := http.MethodPost, "/api/v1/runs/source/continue"
				if mode == "status" {
					method, path = http.MethodGet, "/api/v1/runs/source/continuation"
				} else {
					var q codexexec.ContinueRequest
					if json.NewDecoder(r.Body).Decode(&q) != nil || q != good.Request {
						t.Error("decision drift")
					}
				}
				if r.Method != method || r.URL.Path != path {
					t.Error("wrong route")
				}
				if mode == "lost" {
					w.WriteHeader(503)
					return
				}
				reply := good
				switch mode {
				case "changed-actor":
					reply.Actor = "urn:engineering-platform:engineer:other"
				case "changed-decision":
					reply.Request.Reason = "other"
				case "changed-source":
					reply.Request.SourceRunID = "foreign"
				case "changed-input":
					reply.Input.PolicyProfile = "different"
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(reply)
			})
			s := httptest.NewUnstartedServer(h)
			s.TLS = pki.ServerTLS()
			s.StartTLS()
			defer s.Close()
			c, e := controlclient.New(s.URL, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pki.Roots, Certificates: []tls.Certificate{pki.ClientCertificate(t, actor)}})
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			o := continueOptions{action: "authorize", run: "source", q: good.Request}
			if mode == "status" {
				o.action = "status"
			}
			reply, e := executeContinue(context.Background(), c, o)
			if mode == "authorize" || mode == "status" {
				if e != nil || reply.Actor != actor {
					t.Fatal(reply, e)
				}
			} else if e == nil {
				t.Fatal("bad response accepted", mode)
			}
			if calls.Load() != 1 {
				t.Fatal("request replay", calls.Load())
			}
		})
	}
}
