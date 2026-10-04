//go:build linux

package preparation_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/sandbox"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"github.com/jiying2007/engineering-platform/internal/workeragent"
)

func readbackFixture(t *testing.T, mode string) (workeragent.PostTurnReadbackRequest, *completedTransport, codexexec.Receipt) {
	t.Helper()
	testsupport.RequireProcessNamespaces(t)
	p, _, c, request, runtime := completedSetup(t, mode)
	receipt, err := workeragent.ExecuteCodex(context.Background(), c, p, request, runtime)
	if mode == "success" {
		mustCheckpoint(t, err)
	} else {
		var failure *workeragent.PostTurnFailureError
		if !errors.As(err, &failure) {
			t.Fatal(err)
		}
	}
	dir := filepath.Dir(c.work)
	data, err := os.ReadFile(filepath.Join(dir, "codex-"+c.permit.Token.ID+".json"))
	mustCheckpoint(t, err)
	q := workeragent.PostTurnReadbackRequest{Records: dir, RunID: c.permit.Token.RunID, ExecutionID: c.permit.Token.ID, PermitDigest: canonical.BytesDigest(data)}
	return q, c, receipt
}
func TestActualPostTurnReadbackPreservesObservationsAndBytes(t *testing.T) {
	for _, mode := range []string{"success", "ignored", "report-lost", "checkpoint-lost", "final-renew"} {
		t.Run(mode, func(t *testing.T) {
			q, c, receipt := readbackFixture(t, mode)
			snapshot := func() string {
				t.Helper()
				found, _ := filepath.Glob(filepath.Join(q.Records, "codex-*.json"))
				data := map[string]string{}
				for _, name := range found {
					b, e := os.ReadFile(name)
					mustCheckpoint(t, e)
					data[name] = canonical.BytesDigest(b)
				}
				d, _ := canonical.Digest(data)
				return d
			}
			before := snapshot()
			calls, _ := canonical.Digest(c.calls)
			r, err := workeragent.InspectPostTurn(context.Background(), q)
			mustCheckpoint(t, err)
			if r.CoreObservation != "NOT_OBSERVED" || r.SourceBytes != "NOT_READ" || r.BundleBytes != "NOT_READ" || r.ExecutionAuthorized || r.ReplayAuthorized || r.ProductionQualified {
				t.Fatal("invented authority", r)
			}
			if mode == "success" {
				if r.LocalObservation != "PHASE_ENTRIES_ONLY" || len(r.EnteredPhases) != 6 || r.CheckpointDigest != "" {
					t.Fatal(r)
				}
			} else {
				if r.LocalObservation != "FAILED_UNCONFIRMED" || r.CheckpointDigest == "" || r.LocalRegistrationClaim != (mode != "checkpoint-lost") {
					t.Fatal(r)
				}
				q.Archive = filepath.Join(c.root, "artifacts", q.ExecutionID+".source-checkpoint.tar")
			}
			if mode != "ignored" {
				q.Bundle = filepath.Join(c.root, "artifacts", q.ExecutionID+".bundle")
			}
			verified, err := workeragent.InspectPostTurn(context.Background(), q)
			mustCheckpoint(t, err)
			if q.Archive != "" && verified.SourceBytes != "BYTES_VERIFIED" {
				t.Fatal("source not read back")
			}
			if q.Bundle != "" && verified.BundleBytes != "BYTES_VERIFIED_NOT_GIT_REPLAYED" {
				t.Fatal("bundle not read back")
			}
			afterCalls, _ := canonical.Digest(c.calls)
			if snapshot() != before || afterCalls != calls {
				t.Fatal("readback mutated records or called transport")
			}
			raw, err := json.Marshal(verified)
			mustCheckpoint(t, err)
			for _, secret := range []string{c.work, c.root, "TEST-ONLY-PRIVATE", "completed model source preserved", "fixture_only", "archive_path", "acceptance_criteria"} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("private content in report", secret)
				}
			}
			status := codexexec.Status{Token: c.permit.Token, LeaseUntil: c.permit.LeaseUntil, State: codexexec.Unknown}
			td, err := c.transcript.Digest()
			mustCheckpoint(t, err)
			status.Runtime = &codexexec.ControlRuntime{State: "SEALED", Binding: c.transcript.Close.Binding, Transcript: &c.transcript, Digest: td}
			if mode != "success" {
				status.SourceCheckpoint = &c.checkpoint
			}
			if mode == "success" {
				status.State = codexexec.Finished
				status.Receipt = &receipt
			}
			if mode == "report-lost" || mode == "checkpoint-lost" {
				// Test-only response of a Core that committed before its first reply was lost.
				b, e := os.ReadFile(filepath.Join(q.Records, "codex-"+sandbox.Hash([]byte(q.ExecutionID + ":result"))[7:]+".json"))
				mustCheckpoint(t, e)
				var local struct {
					Result     codexexec.Result `json:"result"`
					BundlePath string           `json:"bundle_path"`
				}
				mustCheckpoint(t, json.Unmarshal(b, &local))
				d, e := canonical.Digest(local.Result)
				mustCheckpoint(t, e)
				r := codexexec.Receipt{Kind: codexexec.Kind, Token: c.permit.Token, Worker: subject, PreparationDigest: c.permit.Preparation.FactsDigest, Result: local.Result, ResultDigest: d, ReceivedAt: time.Now().UTC()}
				status.State = codexexec.Finished
				status.Receipt = &r
			}
			body, err := json.Marshal(status)
			mustCheckpoint(t, err)
			pki := testsupport.NewPKI(t)
			var count atomic.Int32
			var statusCode atomic.Int32
			var response atomic.Value
			response.Store(body)
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				count.Add(1)
				if req.Method != http.MethodGet || req.URL.Path != "/api/v1/runs/"+q.RunID+"/codex" {
					t.Error("write/unbound path", req.Method, req.URL.Path)
				}
				if code := statusCode.Load(); code != 0 {
					w.WriteHeader(int(code))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write(response.Load().([]byte))
			}))
			server.TLS = pki.ServerTLS()
			server.StartTLS()
			defer server.Close()
			client, err := controlclient.New(server.URL, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pki.Roots, Certificates: []tls.Certificate{pki.ClientCertificate(t, "urn:engineering-platform:readback-observer")}})
			mustCheckpoint(t, err)
			defer client.Close()
			live, err := workeragent.ObservePostTurnCore(context.Background(), client, verified)
			mustCheckpoint(t, err)
			if count.Load() != 1 || live.CoreObservation != status.State || live.LocalObservation != verified.LocalObservation || live.ExecutionAuthorized || live.ReplayAuthorized {
				t.Fatal("Core/local outcome conflated", live)
			}
			if mode == "checkpoint-lost" && (live.LocalRegistrationClaim || live.CoreCheckpoint != "MATCHES_LOCAL_DESCRIPTOR") {
				t.Fatal("lost registration reply conflated", live)
			}
			for _, mutation := range []func(map[string]any){
				func(v map[string]any) { v["token"].(map[string]any)["execution_id"] = strings.Repeat("a", 64) },
				func(v map[string]any) {
					v["runtime"].(map[string]any)["transcript_digest"] = "sha256:" + strings.Repeat("0", 64)
				},
				func(v map[string]any) { v["state"] = "READY" },
				func(v map[string]any) { v["execution_authorized"] = true },
			} {
				var v map[string]any
				mustCheckpoint(t, json.Unmarshal(body, &v))
				mutation(v)
				changed, marshalErr := json.Marshal(v)
				mustCheckpoint(t, marshalErr)
				response.Store(changed)
				n := count.Load()
				if _, err = workeragent.ObservePostTurnCore(context.Background(), client, verified); err == nil || count.Load() != n+1 {
					t.Fatal("bad Core response accepted or retried")
				}
			}
			response.Store(append([]byte(`{"state":"FINISHED",`), body[1:]...))
			if _, err = workeragent.ObservePostTurnCore(context.Background(), client, verified); err == nil {
				t.Fatal("duplicate Core field accepted")
			}
			statusCode.Store(http.StatusServiceUnavailable)
			n := count.Load()
			if _, err = workeragent.ObservePostTurnCore(context.Background(), client, verified); err == nil || count.Load() != n+1 {
				t.Fatal("failed GET accepted or automatically retried")
			}

		})
	}
}
func TestActualPostTurnReadbackRejectsTamperingAndNeverFollowsPaths(t *testing.T) {
	original, c, _ := readbackFixture(t, "report-lost")
	files, _ := filepath.Glob(filepath.Join(original.Records, "codex-*.json"))
	bytesByName := map[string][]byte{}
	for _, path := range files {
		b, e := os.ReadFile(path)
		mustCheckpoint(t, e)
		bytesByName[filepath.Base(path)] = b
	}
	copyRecords := func(t *testing.T) workeragent.PostTurnReadbackRequest {
		t.Helper()
		q := original
		q.Records = t.TempDir()
		mustCheckpoint(t, os.Chmod(q.Records, 0700))
		for name, b := range bytesByName {
			mustCheckpoint(t, os.WriteFile(filepath.Join(q.Records, name), b, 0600))
		}
		return q
	}
	name := func(s string) string { return "codex-" + sandbox.Hash([]byte(original.ExecutionID + s))[7:] + ".json" }
	tests := map[string]func(*testing.T, *workeragent.PostTurnReadbackRequest){
		"wrong-permit": func(t *testing.T, q *workeragent.PostTurnReadbackRequest) {
			q.PermitDigest = "sha256:" + strings.Repeat("0", 64)
		},
		"wrong-run": func(t *testing.T, q *workeragent.PostTurnReadbackRequest) { q.RunID = "other-run" },
		"path-alias": func(t *testing.T, q *workeragent.PostTurnReadbackRequest) {
			alias := q.Records + "-alias"
			mustCheckpoint(t, os.Symlink(q.Records, alias))
			t.Cleanup(func() { os.Remove(alias) })
			q.Records = alias
		},
		"public-dir": func(t *testing.T, q *workeragent.PostTurnReadbackRequest) {
			mustCheckpoint(t, os.Chmod(q.Records, 0755))
		},
		"phase-hole": func(t *testing.T, q *workeragent.PostTurnReadbackRequest) {
			mustCheckpoint(t, os.Remove(filepath.Join(q.Records, name(":post-turn:FINALIZE"))))
		},
		"missing-turn": func(t *testing.T, q *workeragent.PostTurnReadbackRequest) {
			mustCheckpoint(t, os.Remove(filepath.Join(q.Records, name(":turn"))))
		},
		"missing-result": func(t *testing.T, q *workeragent.PostTurnReadbackRequest) {
			mustCheckpoint(t, os.Remove(filepath.Join(q.Records, name(":result"))))
		},
		"truncated-phase": func(t *testing.T, q *workeragent.PostTurnReadbackRequest) {
			mustCheckpoint(t, os.WriteFile(filepath.Join(q.Records, name(":post-turn:FINALIZE")), []byte(`{"version":`), 0600))
		},
		"oversized-phase": func(t *testing.T, q *workeragent.PostTurnReadbackRequest) {
			mustCheckpoint(t, os.WriteFile(filepath.Join(q.Records, name(":post-turn:FINALIZE")), []byte(strings.Repeat(" ", 1<<20+1)), 0600))
		},
		"phase-symlink": func(t *testing.T, q *workeragent.PostTurnReadbackRequest) {
			path := filepath.Join(q.Records, name(":post-turn:FINALIZE"))
			mustCheckpoint(t, os.Remove(path))
			mustCheckpoint(t, os.Symlink(filepath.Join(original.Records, name(":post-turn:FINALIZE")), path))
		},
		"phase-fifo": func(t *testing.T, q *workeragent.PostTurnReadbackRequest) {
			path := filepath.Join(q.Records, name(":post-turn:FINALIZE"))
			mustCheckpoint(t, os.Remove(path))
			mustCheckpoint(t, syscall.Mkfifo(path, 0600))
		},
		"public-record": func(t *testing.T, q *workeragent.PostTurnReadbackRequest) {
			mustCheckpoint(t, os.Chmod(filepath.Join(q.Records, name(":post-turn:FINALIZE")), 0644))
		},
		"wrong-bundle": func(t *testing.T, q *workeragent.PostTurnReadbackRequest) {
			q.Bundle = filepath.Join(q.Records, "bad.bundle")
			mustCheckpoint(t, os.WriteFile(q.Bundle, []byte("bad"), 0600))
		},
		"wrong-archive": func(t *testing.T, q *workeragent.PostTurnReadbackRequest) {
			q.Archive = filepath.Join(q.Records, "bad.tar")
			mustCheckpoint(t, os.WriteFile(q.Archive, []byte("bad"), 0600))
		},
	}
	for label, mutation := range tests {
		t.Run(label, func(t *testing.T) {
			q := copyRecords(t)
			mutation(t, &q)
			if _, err := workeragent.InspectPostTurn(context.Background(), q); err == nil {
				t.Fatal("invalid local input accepted")
			}
		})
	}
	for label, mutation := range map[string]func(map[string]any){
		"authority":        func(v map[string]any) { v["execution_authorized"] = true },
		"delivery":         func(v map[string]any) { v["delivery_confirmed_by_worker"] = true },
		"mixed-input":      func(v map[string]any) { v["run_input_manifest_digest"] = "sha256:" + strings.Repeat("0", 64) },
		"mixed-transcript": func(v map[string]any) { v["control_transcript_digest"] = "sha256:" + strings.Repeat("0", 64) },
		"wrong-phase":      func(v map[string]any) { v["phase"] = "TURN_RECEIPT" },
		"checkpoint-drift": func(v map[string]any) {
			v["source_checkpoint"].(map[string]any)["snapshot_digest"] = "sha256:" + strings.Repeat("0", 64)
		},
	} {
		t.Run(label, func(t *testing.T) {
			q := copyRecords(t)
			path := filepath.Join(q.Records, name(":post-turn-failure"))
			var v map[string]any
			mustCheckpoint(t, json.Unmarshal(bytesByName[filepath.Base(path)], &v))
			mutation(v)
			b, e := json.Marshal(v)
			mustCheckpoint(t, e)
			mustCheckpoint(t, os.WriteFile(path, b, 0600))
			if _, err := workeragent.InspectPostTurn(context.Background(), q); err == nil {
				t.Fatal("false observation accepted")
			}
		})
	}
	t.Run("metadata-paths-not-followed", func(t *testing.T) {
		q := copyRecords(t)
		path := filepath.Join(q.Records, name(":source-checkpoint"))
		var v map[string]any
		mustCheckpoint(t, json.Unmarshal(bytesByName[filepath.Base(path)], &v))
		v["archive_path"] = "/never/read/private-credential"
		b, e := json.Marshal(v)
		mustCheckpoint(t, e)
		mustCheckpoint(t, os.WriteFile(path, b, 0600))
		r, e := workeragent.InspectPostTurn(context.Background(), q)
		mustCheckpoint(t, e)
		if r.SourceBytes != "NOT_READ" {
			t.Fatal("followed unrequested path")
		}
		q.Archive = filepath.Join(c.root, "artifacts", q.ExecutionID+".source-checkpoint.tar")
		_, e = workeragent.InspectPostTurn(context.Background(), q)
		mustCheckpoint(t, e)
	})
	t.Run("only-entered-is-not-crash-or-success", func(t *testing.T) {
		q := copyRecords(t)
		for file := range bytesByName {
			if file != "codex-"+q.ExecutionID+".json" && file != name(":post-turn:TURN_RECEIPT") {
				mustCheckpoint(t, os.Remove(filepath.Join(q.Records, file)))
			}
		}
		r, e := workeragent.InspectPostTurn(context.Background(), q)
		mustCheckpoint(t, e)
		if r.LocalObservation != "PHASE_ENTRIES_ONLY" || r.CoreObservation != "NOT_OBSERVED" || r.CheckpointDigest != "" {
			t.Fatal(r)
		}
	})
	t.Run("only-permit-is-not-progress", func(t *testing.T) {
		q := copyRecords(t)
		for file := range bytesByName {
			if file != "codex-"+q.ExecutionID+".json" {
				mustCheckpoint(t, os.Remove(filepath.Join(q.Records, file)))
			}
		}
		r, e := workeragent.InspectPostTurn(context.Background(), q)
		mustCheckpoint(t, e)
		if r.LocalObservation != "NO_POST_TURN_RECORDS" || len(r.EnteredPhases) != 0 || r.CoreObservation != "NOT_OBSERVED" {
			t.Fatal(r)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, e = workeragent.InspectPostTurn(ctx, q); !errors.Is(e, context.Canceled) {
			t.Fatal("cancelled readback proceeded", e)
		}
	})

}
