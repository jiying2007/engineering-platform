package postgres

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/api"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/provideridentity"
	"github.com/jiying2007/engineering-platform/internal/runtime/codexapp"
	"github.com/jiying2007/engineering-platform/internal/sourcecheckpoint"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"github.com/jiying2007/engineering-platform/internal/workeragent"
)

func liveControlFixture(t *testing.T, s *Store) (codexexec.Permit, codexexec.ControlBinding, codexexec.ControlInput) {
	t.Helper()
	ctx := context.Background()
	request := codexFixture(t, s)
	p, err := s.StartCodex(ctx, codexTestWorker, request)
	workerOK(t, err)
	b := codexexec.ControlBinding{Token: p.Token, ExecutionEpoch: 1, ThreadID: "thread", TurnID: "turn"}
	workerOK(t, s.BindCodexControl(ctx, codexTestWorker, b))
	i := codexexec.ControlInput{ID: "live-control", ExecutionEpoch: 1, Sequence: 1, Actor: "urn:engineering-platform:engineer:alice", ThreadID: "thread", TurnID: "turn", Text: "先修复故障，不要扩展范围。"}
	return p, b, i
}
func TestCodexControlsOneShotAndExactTranscript(t *testing.T) {
	s := integrationStore(t)
	p, b, i := liveControlFixture(t, s)
	ctx := context.Background()
	d, err := s.QueueCodexControl(ctx, p.Token.RunID, codexexec.ControlSteer, i)
	workerOK(t, err)
	if d.State != codexexec.ControlQueued {
		t.Fatal(d)
	}
	again, err := s.QueueCodexControl(ctx, p.Token.RunID, codexexec.ControlSteer, i)
	workerOK(t, err)
	a, _ := json.Marshal(d)
	a2, _ := json.Marshal(again)
	if string(a) != string(a2) {
		t.Fatal("retry changed original command")
	}
	altered := i
	altered.Text = "different"
	if _, err = s.QueueCodexControl(ctx, p.Token.RunID, codexexec.ControlSteer, altered); err == nil {
		t.Fatal("changed ID retry accepted")
	}
	next, err := s.ClaimCodexControl(ctx, codexTestWorker, b)
	workerOK(t, err)
	if next == nil || next.State != codexexec.ControlDispatching {
		t.Fatal("missing dispatch")
	}
	if _, err = s.ClaimCodexControl(ctx, codexTestWorker, b); err == nil {
		t.Fatal("unacknowledged control was redelivered")
	}
	r := codexexec.ControlSettlement{Binding: b, ID: i.ID, Outcome: codexexec.ControlAccepted}
	_, err = s.SettleCodexControl(ctx, codexTestWorker, r)
	workerOK(t, err)
	_, err = s.SettleCodexControl(ctx, codexTestWorker, r)
	workerOK(t, err)
	r.Outcome = codexexec.ControlUnknown
	if _, err = s.SettleCodexControl(ctx, codexTestWorker, r); err == nil {
		t.Fatal("terminal receipt overwritten")
	}
	transcript, err := s.CloseCodexControl(ctx, codexTestWorker, codexexec.ControlClose{Binding: b, TurnStatus: "completed", ProcessScope: testsupport.ProcessScopeFixture()})
	workerOK(t, err)
	if !transcript.AllowsDelivery() {
		t.Fatal("accepted input did not permit delivery")
	}
	result := codexResult(t, p)
	result.ControlTranscriptDigest, err = transcript.Digest()
	workerOK(t, err)
	wrong := result
	wrong.ControlTranscriptDigest = canonical.BytesDigest([]byte("other"))
	if _, err = s.FinishCodex(ctx, codexTestWorker, codexexec.Report{Token: p.Token, Result: wrong}); err == nil {
		t.Fatal("wrong transcript permitted result")
	}
	_, err = s.FinishCodex(ctx, codexTestWorker, codexexec.Report{Token: p.Token, Result: result})
	workerOK(t, err)
	workerOK(t, s.ApplyCoreMigration(ctx))
	read, err := s.GetCodexControl(ctx, i.ID)
	workerOK(t, err)
	if read.State != codexexec.ControlAccepted || read.Payload.Text != i.Text {
		t.Fatal("migration lost exact steering")
	}
	status, err := s.GetCodex(ctx, p.Token.RunID)
	workerOK(t, err)
	if status.Runtime == nil || status.Runtime.Digest != result.ControlTranscriptDigest {
		t.Fatal("readback omitted transcript")
	}
	assertCount(t, s, "SELECT count(*) FROM audit_events WHERE event_type='steering.observed'", 1)
}
func TestCodexControlsUnknownNotReplayedAndLateQueueRejected(t *testing.T) {
	for _, claim := range []bool{false, true} {
		t.Run(fmt.Sprint(claim), func(t *testing.T) {
			s := integrationStore(t)
			p, b, i := liveControlFixture(t, s)
			ctx := context.Background()
			_, err := s.QueueCodexControl(ctx, p.Token.RunID, codexexec.ControlSteer, i)
			workerOK(t, err)
			if claim {
				_, err = s.ClaimCodexControl(ctx, codexTestWorker, b)
				workerOK(t, err)
			}
			c := codexexec.ControlClose{Binding: b, TurnStatus: "completed", ProcessScope: testsupport.ProcessScopeFixture()}
			transcript, err := s.CloseCodexControl(ctx, codexTestWorker, c)
			workerOK(t, err)
			want := codexexec.ControlNotApplied
			if claim {
				want = codexexec.ControlUnknown
			}
			if transcript.AllowsDelivery() || transcript.Deliveries[0].State != want {
				t.Fatal("unresolved input hidden by turn completion")
			}
			again, err := s.CloseCodexControl(ctx, codexTestWorker, c)
			workerOK(t, err)
			d1, _ := transcript.Digest()
			d2, _ := again.Digest()
			if d1 != d2 {
				t.Fatal("close retry drift")
			}
			c.TurnStatus = "interrupted"
			if _, err = s.CloseCodexControl(ctx, codexTestWorker, c); err == nil {
				t.Fatal("close outcome rewritten")
			}
			i.ID = "late"
			i.Sequence = 2
			if _, err = s.QueueCodexControl(ctx, p.Token.RunID, codexexec.ControlSteer, i); err == nil {
				t.Fatal("late input accepted")
			}
			if _, err = s.ClaimCodexControl(ctx, codexTestWorker, b); err == nil {
				t.Fatal("sealed delivery replay")
			}
			workerOK(t, s.FailCodex(ctx, codexTestWorker, p.Token))
			state, err := s.BeginRecovery(p.Token.RecoveryEpoch)
			workerOK(t, err)
			if _, err = s.CreateRecoveryProof(ctx, state.Epoch, "reconciler"); err == nil {
				t.Fatal("unknown model execution disappeared from recovery")
			}
		})
	}
}
func TestCodexControlsAuthorityFencesAndInterruptEvidence(t *testing.T) {
	s := integrationStore(t)
	p, b, i := liveControlFixture(t, s)
	ctx := context.Background()
	for _, change := range []func(*codexexec.ControlInput){func(v *codexexec.ControlInput) { v.ExecutionEpoch++ }, func(v *codexexec.ControlInput) { v.TurnID = "other" }, func(v *codexexec.ControlInput) { v.ThreadID = "other" }, func(v *codexexec.ControlInput) { v.Text = "" }} {
		bad := i
		change(&bad)
		if _, err := s.QueueCodexControl(ctx, p.Token.RunID, codexexec.ControlSteer, bad); err == nil {
			t.Fatal("foreign or invalid input accepted")
		}
	}
	if err := s.BindCodexControl(ctx, "urn:engineering-platform:worker:other", b); err == nil {
		t.Fatal("foreign Worker bound turn")
	}
	i.Text = ""
	_, err := s.QueueCodexControl(ctx, p.Token.RunID, codexexec.ControlInterrupt, i)
	workerOK(t, err)
	_, err = s.ClaimCodexControl(ctx, codexTestWorker, b)
	workerOK(t, err)
	report := codexexec.ControlSettlement{Binding: b, ID: i.ID, Outcome: codexexec.ControlInterrupted}
	if _, err = s.SettleCodexControl(ctx, codexTestWorker, report); err == nil {
		t.Fatal("ACK API manufactured turn completion")
	}
	report.Outcome = codexexec.ControlInterruptACK
	_, err = s.SettleCodexControl(ctx, codexTestWorker, report)
	workerOK(t, err)
	d, err := s.GetCodexControl(ctx, i.ID)
	workerOK(t, err)
	if d.State != codexexec.ControlInterruptACK {
		t.Fatal("ACK claimed stop")
	}
	i.ID = "after-interrupt"
	i.Sequence = 2
	i.Text = "continue"
	if _, err = s.QueueCodexControl(ctx, p.Token.RunID, codexexec.ControlSteer, i); err == nil {
		t.Fatal("implicit resume accepted")
	}
	transcript, err := s.CloseCodexControl(ctx, codexTestWorker, codexexec.ControlClose{Binding: b, TurnStatus: "interrupted", ProcessScope: testsupport.ProcessScopeFixture()})
	workerOK(t, err)
	if transcript.Deliveries[0].State != codexexec.ControlInterrupted || transcript.AllowsDelivery() {
		t.Fatal("interruption became successful delivery")
	}
	result := codexResult(t, p)
	result.ControlTranscriptDigest, _ = transcript.Digest()
	if _, err = s.FinishCodex(ctx, codexTestWorker, codexexec.Report{Token: p.Token, Result: result}); err == nil {
		t.Fatal("interrupted result published")
	}
}

// The model protocol below is a deliberately local fixture, not live provider
// evidence. The rest is the production mTLS API, PG store, Worker turn segment,
// real process pipes and JSON-RPC adapter. No fixture credential leaves the host.

// Keep fixture authorization construction independently runnable without PG.
// Preparation still requires the existing poll/report/profile grants; tests must
// satisfy that contract rather than relax production policy to fit the fixture.
func controlWirePolicy(operator, viewer, workerProfile, profileDigest string) access.Document {
	return access.Document{Version: 1, Principals: []access.PrincipalSpec{
		{Subject: operator, Scope: "platform", Capabilities: []string{access.Read, access.RunControl}},
		{Subject: viewer, Scope: "platform", Capabilities: []string{access.Read}},
		{Subject: codexTestWorker, Scope: "platform", Capabilities: []string{access.Read, access.WorkerPoll, access.WorkerReport, access.WorkerPrepare, access.ActionExecute}, WorkerProfiles: []string{workerProfile}, Actions: []access.ActionGrant{{Action: codexexec.Action, RiskClass: "CONTROLLED_MUTATION", Capability: profileDigest}}},
	}}
}

func TestCodexControlsWirePolicyWithoutDatabase(t *testing.T) {
	build := func() access.Document {
		return controlWirePolicy("urn:engineering-platform:engineer:control-test", "urn:engineering-platform:viewer:test", "worker/codex", canonical.BytesDigest([]byte("fixture-profile")))
	}
	if _, err := access.New(build()); err != nil {
		t.Fatal(err)
	}
	for _, missing := range []string{access.WorkerPoll, access.WorkerReport} {
		doc := build()
		worker := &doc.Principals[2]
		var kept []string
		for _, capability := range worker.Capabilities {
			if capability != missing {
				kept = append(kept, capability)
			}
		}
		worker.Capabilities = kept
		if _, err := access.New(doc); err == nil {
			t.Fatalf("fixture accepted without %s", missing)
		}
	}
	doc := build()
	doc.Principals[2].WorkerProfiles = nil
	if _, err := access.New(doc); err == nil {
		t.Fatal("fixture accepted without exact worker profile")
	}
}

func TestCodexControlsMTLSToLiveProcess(t *testing.T) {
	testsupport.RequireProcessNamespaces(t)
	for _, mode := range []string{"steer", "interrupt", "lost-reply", "recovery"} {
		t.Run(mode, func(t *testing.T) {
			s := integrationStore(t)
			python, err := exec.LookPath("python3")
			workerOK(t, err)
			base := t.TempDir()
			work := filepath.Join(base, "work")
			home := filepath.Join(base, "home")
			auth := filepath.Join(base, "auth")
			for _, p := range []string{work, home, auth} {
				workerOK(t, os.Mkdir(p, 0700))
			}
			executable := filepath.Join(base, "fixture-codex")
			program := []byte("#!" + python + " -S\n" + testsupport.ControlledCodexProtocol)
			workerOK(t, os.WriteFile(executable, program, 0700))
			login := filepath.Join(auth, "auth.json")
			workerOK(t, os.WriteFile(login, []byte(`{"fixture_only":"not-a-real-credential"}`), 0600))
			profile := codexexec.Profile{Version: 3, Provider: provideridentity.OpenAIChatGPTTrustedSelfHosted(), CodexVersion: "0.157.1", BinaryDigest: canonical.BytesDigest(program), QualificationDigest: canonical.BytesDigest([]byte("fixture-not-a-live-qualification")), EngineeringConfigDigest: codexapp.EngineeringConfigDigest(), Model: "fixture-only", Sandbox: "workspace-write", ApprovalPolicy: "never"}
			req := codexFixture(t, s, profile)
			pd, err := profile.Digest()
			workerOK(t, err)
			operator := "urn:engineering-platform:engineer:control-test"
			viewer := "urn:engineering-platform:viewer:test"
			policy, err := access.New(controlWirePolicy(operator, viewer, req.WorkerProfile, pd))
			workerOK(t, err)
			handler, err := api.NewAuthenticatedHandler(s, nil, policy, api.AuthenticatedOptions{})
			workerOK(t, err)
			pki := testsupport.NewPKI(t)
			server := httptest.NewUnstartedServer(handler)
			server.TLS = pki.ServerTLS()
			server.StartTLS()
			defer server.Close()
			client := func(subject string) *controlclient.Client {
				c, e := controlclient.New(server.URL, &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pki.Roots, Certificates: []tls.Certificate{pki.ClientCertificate(t, subject)}})
				workerOK(t, e)
				t.Cleanup(c.Close)
				return c
			}
			worker := client(codexTestWorker)
			engineer := client(operator)
			reader := client(viewer)
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			var permit codexexec.Permit
			workerOK(t, worker.Call(ctx, http.MethodPost, "/api/v1/worker/codex/start", req, &permit))
			if mode == "lost-reply" {
				workerOK(t, os.WriteFile(filepath.Join(work, "drop-reply"), []byte("test"), 0600))
			}
			type turnResult struct {
				receipt    codexapp.EngineeringReceipt
				transcript codexexec.ControlTranscript
				err        error
			}
			done := make(chan turnResult, 1)
			go func() {
				receipt, transcript, e := workeragent.RunCodexTurn(ctx, worker, permit, workeragent.CodexRuntime{Executable: executable, SavedLoginFile: login}, work, home, "fixture input")
				done <- turnResult{receipt, transcript, e}
			}()
			waitFor(t, func() bool {
				runtime, e := s.GetCodexControlRuntime(ctx, req.RunID)
				return e == nil && runtime != nil && runtime.State == "ACTIVE"
			})
			for _, op := range []string{"pause", "resume", "takeover"} {
				expectHTTP(t, engineer.Call(ctx, http.MethodPost, "/api/v1/runs/"+req.RunID+"/"+op, map[string]uint64{"execution_epoch": 1}, nil), http.StatusConflict)
			}
			if mode == "recovery" {
				_, err = s.BeginRecovery(permit.Token.RecoveryEpoch)
				workerOK(t, err)
			} else {
				input := codexexec.ControlInput{ID: "wire-control", ExecutionEpoch: 1, Sequence: 1, Actor: operator, ThreadID: "thread", TurnID: "turn", Text: "只修复失败测试，不要修改权限。"}
				path := "/api/v1/runs/" + req.RunID + "/steer"
				if mode == "interrupt" {
					input.Text = ""
					path = "/api/v1/runs/" + req.RunID + "/interrupt"
				}
				expectHTTP(t, reader.Call(ctx, http.MethodPost, path, input, nil), http.StatusForbidden)
				spoof := input
				spoof.Actor = viewer
				expectHTTP(t, engineer.Call(ctx, http.MethodPost, path, spoof, nil), http.StatusForbidden)
				var queued codexexec.ControlDelivery
				workerOK(t, engineer.Call(ctx, http.MethodPost, path, input, &queued))
				if queued.State != codexexec.ControlQueued {
					t.Fatal("queue response claimed effect")
				}
				if mode == "lost-reply" {
					waitFor(t, func() bool { _, e := os.Stat(filepath.Join(work, "requests.jsonl")); return e == nil })
					cancel()
				} else {
					expected := codexexec.ControlAccepted
					if mode == "interrupt" {
						expected = codexexec.ControlInterruptACK
					}
					waitFor(t, func() bool { d, e := s.GetCodexControl(ctx, input.ID); return e == nil && d.State == expected })
					runtime, e := s.GetCodexControlRuntime(ctx, req.RunID)
					workerOK(t, e)
					if runtime.State != "ACTIVE" || runtime.Transcript != nil {
						t.Fatal("ACK manufactured terminal proof")
					}
					workerOK(t, os.WriteFile(filepath.Join(work, "allow-finish"), []byte("fixture release"), 0600))
				}
			}
			var result turnResult
			select {
			case result = <-done:
			case <-time.After(8 * time.Second):
				t.Fatal("controlled process did not terminate")
			}
			checkCtx := context.Background()
			if mode == "steer" {
				workerOK(t, result.err)
				if !result.transcript.AllowsDelivery() {
					t.Fatal("accepted steering absent from transcript")
				}
				final := codexResult(t, permit)
				final.Codex = result.receipt
				final.ControlTranscriptDigest, err = result.transcript.Digest()
				workerOK(t, err)
				var receipt codexexec.Receipt
				workerOK(t, worker.Call(checkCtx, http.MethodPost, "/api/v1/worker/codex/report", codexexec.Report{Token: permit.Token, Result: final}, &receipt))
				if receipt.Result.ControlTranscriptDigest != final.ControlTranscriptDigest {
					t.Fatal("result lost control lineage")
				}
			} else {
				if result.err == nil {
					t.Fatal("interrupt/ambiguous/revoked turn returned success")
				}
				workerOK(t, s.FailCodex(checkCtx, codexTestWorker, permit.Token))
				status, e := s.GetCodex(checkCtx, req.RunID)
				workerOK(t, e)
				if status.State != codexexec.Unknown || status.Receipt != nil {
					t.Fatal("non-deliverable turn promoted")
				}
				if mode == "interrupt" && (status.Runtime.Transcript.Deliveries[0].State != codexexec.ControlInterrupted || !status.Runtime.Transcript.Close.ProcessScope.Quiescent()) {
					t.Fatal("missing actual interrupt/exit observations")
				}
				if mode == "lost-reply" && status.Runtime.Transcript.Deliveries[0].State != codexexec.ControlUnknown {
					t.Fatal("lost control reply hidden")
				}

				// Real stopped-subprocess source -> immutable local bytes ->
				// actual mTLS/PG artifact observation -> fresh restore/readback.
				artifactRoot := filepath.Join(base, "checkpoint-artifacts")
				workerOK(t, os.Mkdir(artifactRoot, 0700))
				gitBundlePath := filepath.Join(base, "checkpoint-git-base.bundle")
				gitBundleBytes := []byte("TEST-ONLY-SOURCECHECKPOINT-GIT-BASE")
				workerOK(t, os.WriteFile(gitBundlePath, gitBundleBytes, 0600))
				gitBundle := sourcecheckpoint.GitBundle{Path: gitBundlePath, Digest: canonical.BytesDigest(gitBundleBytes), Size: int64(len(gitBundleBytes)), Head: permit.Preparation.Facts.BaseCommit}
				artifact, e := sourcecheckpoint.Capture(checkCtx, work, artifactRoot, permit, result.transcript, gitBundle)
				workerOK(t, e)
				var checkpoint codexexec.SourceCheckpoint
				expectHTTP(t, reader.Call(checkCtx, http.MethodPost, "/api/v1/worker/codex/source-checkpoint", artifact.Facts, nil), http.StatusForbidden)
				workerOK(t, worker.Call(checkCtx, http.MethodPost, "/api/v1/worker/codex/source-checkpoint", artifact.Facts, &checkpoint))
				if checkpoint != artifact.Facts {
					t.Fatal("checkpoint observation changed")
				}
				workerOK(t, worker.Call(checkCtx, http.MethodPost, "/api/v1/worker/codex/source-checkpoint", artifact.Facts, &checkpoint))
				changed := artifact.Facts
				changed.ArchiveDigest = canonical.BytesDigest([]byte("substituted artifact"))
				expectHTTP(t, worker.Call(checkCtx, http.MethodPost, "/api/v1/worker/codex/source-checkpoint", changed, nil), http.StatusConflict)
				workerOK(t, os.Chmod(base, 0700))
				restored, e := sourcecheckpoint.Restore(checkCtx, artifact.Path, checkpoint.ArchiveDigest, req.RunID, filepath.Join(base, "recovery-copy"))
				workerOK(t, e)
				if restored != artifact.Facts {
					t.Fatal("recovery source drift")
				}
				restoredGit, e := os.ReadFile(filepath.Join(base, "recovery-copy", "git-base.bundle"))
				workerOK(t, e)
				if !bytes.Equal(restoredGit, gitBundleBytes) {
					t.Fatal("recovery Git-base bytes drift")
				}
				status, e = s.GetCodex(checkCtx, req.RunID)
				workerOK(t, e)
				if status.State != codexexec.Unknown || status.Receipt != nil || status.SourceCheckpoint == nil || *status.SourceCheckpoint != artifact.Facts {
					t.Fatal("checkpoint granted execution or lost readback")
				}
				assertCount(t, s, "SELECT count(*) FROM audit_events WHERE event_type='worker.codex.source-checkpoint-retained'", 1)
			}
			if mode != "recovery" {
				raw, e := os.ReadFile(filepath.Join(work, "requests.jsonl"))
				workerOK(t, e)
				lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
				if len(lines) != 1 {
					t.Fatal("control request replayed", len(lines))
				}
				if mode == "steer" && !strings.Contains(string(raw), "\\u53ea") {
					t.Fatal("steering text never reached subprocess")
				}
			}
		})
	}
}
func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("fixture condition not reached")
}
func expectHTTP(t *testing.T, err error, status int) {
	t.Helper()
	var e *controlclient.HTTPError
	if !errors.As(err, &e) || e.Status != status {
		t.Fatalf("HTTP error %v want %d", err, status)
	}
}

func TestCodexControlsReserveInterruptCapacityAndRejectExpiredLease(t *testing.T) {
	s := integrationStore(t)
	p, b, i := liveControlFixture(t, s)
	ctx := context.Background()
	for n := 1; n < codexexec.MaxControls; n++ {
		i.ID = fmt.Sprintf("control-%d", n)
		i.Sequence = uint64(n)
		_, err := s.QueueCodexControl(ctx, p.Token.RunID, codexexec.ControlSteer, i)
		workerOK(t, err)
	}
	i.ID = "over-capacity"
	i.Sequence = codexexec.MaxControls
	if _, err := s.QueueCodexControl(ctx, p.Token.RunID, codexexec.ControlSteer, i); err == nil {
		t.Fatal("steering consumed cancellation slot")
	}
	i.ID = "reserved-interrupt"
	i.Text = ""
	_, err := s.QueueCodexControl(ctx, p.Token.RunID, codexexec.ControlInterrupt, i)
	workerOK(t, err)
	_, err = s.pool.Exec(ctx, `UPDATE worker_codex_executions SET lease_until=clock_timestamp()-interval '1 second' WHERE execution_id=$1`, p.Token.ID)
	workerOK(t, err)
	if _, err = s.ClaimCodexControl(ctx, codexTestWorker, b); err == nil {
		t.Fatal("expired lease dispatched a control")
	}
	tr, err := s.CloseCodexControl(ctx, codexTestWorker, codexexec.ControlClose{Binding: b, TurnStatus: "unknown", ProcessScope: testsupport.ProcessScopeFixture()})
	workerOK(t, err)
	if len(tr.Deliveries) != codexexec.MaxControls || tr.AllowsDelivery() {
		t.Fatal("expired execution lost its control ledger")
	}
}
