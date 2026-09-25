package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/core"
	"github.com/jiying2007/engineering-platform/internal/store"
	pgstore "github.com/jiying2007/engineering-platform/internal/store/postgres"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
)

const engineerSubject = "urn:engineering-platform:engineer:alice"
const ciSubject = "urn:engineering-platform:service:ci"
const verifierSubject = "urn:engineering-platform:service:verifier"
const reviewerSubject = "urn:engineering-platform:service:reviewer"
const recoverySubject = "urn:engineering-platform:operator:recovery"
const reconcilerSubject = "urn:engineering-platform:operator:reconciler"

func testAccessPolicy(t *testing.T) *access.Policy {
	t.Helper()
	p, err := access.New(access.Document{Version: 1, Principals: []access.PrincipalSpec{
		{Subject: engineerSubject, Scope: "platform", Capabilities: []string{access.Read, access.WorkCreate, access.TaskCreate, access.RunStart, access.RunControl, access.RunComplete, access.DeliveryCreate, access.ClosureCreate, access.ActionExecute}, Actions: []access.ActionGrant{{Action: "ci.dispatch", RiskClass: "CONTROLLED_MUTATION", Capability: "ci"}}},
		{Subject: ciSubject, Scope: "platform", Capabilities: []string{access.EvidenceRegister}, EvidenceIssuer: "test-ci", EvidenceProcedures: []string{"ci.test"}},
		{Subject: verifierSubject, Scope: "platform", Capabilities: []string{access.VerificationCreate}},
		{Subject: reviewerSubject, Scope: "platform", Capabilities: []string{access.ReviewCreate}},
		{Subject: recoverySubject, Scope: "platform", Capabilities: []string{access.RecoveryBegin, access.RecoveryComplete}},
		{Subject: reconcilerSubject, Scope: "platform", Capabilities: []string{access.Read, access.RecoveryReconcile}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func securedTestServer(t *testing.T, backend store.Store, actions ActionGateway, options AuthenticatedOptions) (*httptest.Server, *testsupport.PKI) {
	t.Helper()
	h, err := NewAuthenticatedHandler(backend, actions, testAccessPolicy(t), options)
	if err != nil {
		t.Fatal(err)
	}
	pki := testsupport.NewPKI(t)
	server := httptest.NewUnstartedServer(h)
	server.TLS = pki.ServerTLS()
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	t.Cleanup(server.Close)
	return server, pki
}

func secureCall(t *testing.T, client *http.Client, url string, body any, want int) []byte {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return secureRaw(t, client, http.MethodPost, url, data, want)
}

func secureRaw(t *testing.T, client *http.Client, method, url string, data []byte, want int) []byte {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	result, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != want {
		t.Fatalf("%s status=%d want=%d err=%v body=%s", url, response.StatusCode, want, err, result)
	}
	return result
}

func TestAuthenticatedCoreLifecycle(t *testing.T) { authenticatedLifecycle(t, store.NewMemory()) }

func TestAuthenticatedPostgresCoreLifecycle(t *testing.T) {
	databaseURL := os.Getenv("POSTGRES_TEST_URL")
	if databaseURL == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	name := "ep_http_" + hex.EncodeToString(nonce[:])
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = name
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	backend := pgstore.New(pool)
	t.Cleanup(backend.Close)
	if err := backend.ApplyCoreMigration(ctx); err != nil {
		t.Fatal(err)
	}
	authenticatedLifecycle(t, backend)
}

func authenticatedLifecycle(t *testing.T, backend store.Store) {
	t.Helper()
	server, pki := securedTestServer(t, backend, nil, AuthenticatedOptions{})
	engineer, ci, verifier, reviewer := pki.Client(t, engineerSubject), pki.Client(t, ciSubject), pki.Client(t, verifierSubject), pki.Client(t, reviewerSubject)
	base := server.URL + "/api/v1"
	secureCall(t, engineer, base+"/work-items", map[string]any{"work_item_id": "work-auth", "title": "authenticated API fixture", "human_owner": engineerSubject}, http.StatusCreated)
	taskBody := secureCall(t, engineer, base+"/task-contracts", map[string]any{
		"contract":          map[string]any{"task_contract_id": "task-auth", "work_item_id": "work-auth", "task_type": "FEATURE"},
		"material":          map[string]any{"repository": "repo", "base_commit": strings.Repeat("a", 40), "acceptance_criteria": []string{"tests pass"}},
		"subsystem":         "driver",
		"verification_plan": map[string]any{"verification_plan_id": "plan-auth", "criteria": []any{map[string]any{"criterion_id": "test", "statement": "tests pass", "evidence_requirements": []any{map[string]any{"requirement_id": "ci", "procedure": "ci.test", "issuer": "test-ci"}}}}},
	}, http.StatusCreated)
	var task struct {
		Digest string `json:"digest"`
	}
	mustJSON(t, taskBody, &task)
	secureCall(t, engineer, base+"/runs", map[string]any{"run_id": "run-auth", "task_contract_digest": task.Digest, "attempt_id": "attempt-auth", "run_input": map[string]any{"runtime_profile": "test", "tool_profile": "test", "worker_profile": "test", "policy_profile": "test"}}, http.StatusCreated)
	steer := map[string]any{"steering_command_id": "steer-auth", "execution_epoch": 1, "sequence": 1, "actor": ciSubject, "content_digest": "sha256:test"}
	secureCall(t, engineer, base+"/runs/run-auth/steer", steer, http.StatusForbidden)
	steer["actor"] = engineerSubject
	secureCall(t, engineer, base+"/runs/run-auth/steer", steer, http.StatusAccepted)
	secureCall(t, engineer, base+"/runs/run-auth/complete", map[string]any{"execution_epoch": 1}, http.StatusOK)
	secureCall(t, engineer, base+"/deliveries", map[string]any{"delivery_receipt_id": "delivery-auth", "run_id": "run-auth", "result_commit": strings.Repeat("b", 40)}, http.StatusCreated)
	evidence := map[string]any{"evidence_id": "evidence-auth", "requirement_id": "ci", "issuer": "test-ci", "procedure": "ci.test", "result": "PASS", "applicable": true}
	payload := map[string]any{"delivery_receipt_id": "delivery-auth", "evidence": evidence}
	secureCall(t, engineer, base+"/evidence", payload, http.StatusForbidden)
	evidence["issuer"] = "forged-ci"
	secureCall(t, ci, base+"/evidence", payload, http.StatusForbidden)
	evidence["issuer"], evidence["procedure"] = "test-ci", "release.sign"
	secureCall(t, ci, base+"/evidence", payload, http.StatusForbidden)
	if _, err := backend.GetEvidence("evidence-auth"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("denied evidence persisted: %v", err)
	}
	evidence["procedure"] = "ci.test"
	secureCall(t, ci, base+"/evidence", payload, http.StatusCreated)
	verification := map[string]any{"verification_report_id": "verification-auth", "delivery_receipt_id": "delivery-auth", "verifier": engineerSubject, "evidence_ids": []string{"evidence-auth"}}
	secureCall(t, verifier, base+"/verifications", verification, http.StatusForbidden)
	verification["verifier"] = verifierSubject
	secureCall(t, verifier, base+"/verifications", verification, http.StatusCreated)

	review := map[string]any{
		"review_report_id":       "review-auth",
		"delivery_receipt_id":    "delivery-auth",
		"verification_report_id": "verification-auth",
		"reviewer":               engineerSubject,
		"result":                 "PASS",
	}
	secureCall(t, reviewer, base+"/reviews", review, http.StatusForbidden)
	review["reviewer"] = reviewerSubject
	secureCall(t, engineer, base+"/reviews", review, http.StatusForbidden)
	secureCall(t, verifier, base+"/reviews", review, http.StatusForbidden)
	secureCall(t, reviewer, base+"/reviews", review, http.StatusCreated)
	persistedReview, err := backend.GetReview("review-auth")
	if err != nil || persistedReview.Reviewer != reviewerSubject || persistedReview.Result != "PASS" {
		t.Fatalf("authenticated review not persisted exactly: %#v %v", persistedReview, err)
	}

	secureCall(t, engineer, base+"/closures", map[string]any{
		"closure_receipt_id":     "closure-auth",
		"delivery_receipt_id":    "delivery-auth",
		"verification_report_id": "verification-auth",
		"review_report_id":       "review-auth",
	}, http.StatusCreated)
	final, err := backend.GetWork("work-auth")
	if err != nil || final.State != core.WorkClosed {
		t.Fatalf("authenticated lifecycle did not close: %v %v", final, err)
	}
}

func TestTLSAndWireAuthorityCannotBeSpoofed(t *testing.T) {
	backend := store.NewMemory()
	server, pki := securedTestServer(t, backend, nil, AuthenticatedOptions{})
	noCert := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pki.Roots}}
	defer noCert.CloseIdleConnections()
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/recovery", nil)
	req.Header.Set("X-Forwarded-Client-Cert", engineerSubject)
	if response, err := (&http.Client{Transport: noCert, Timeout: 5 * time.Second}).Do(req); err == nil {
		response.Body.Close()
		t.Fatal("missing TLS certificate accepted")
	}
	// A certificate from a different CA is not accepted even with a known URI.
	wrong := testsupport.NewPKI(t)
	foreign := wrong.ClientCertificate(t, engineerSubject)
	badTransport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pki.Roots, Certificates: []tls.Certificate{foreign}}}
	defer badTransport.CloseIdleConnections()
	if response, err := (&http.Client{Transport: badTransport, Timeout: 5 * time.Second}).Do(req); err == nil {
		response.Body.Close()
		t.Fatal("foreign CA accepted")
	}
	for _, subjects := range [][]string{{"urn:engineering-platform:unknown"}, {}, {engineerSubject, ciSubject}} {
		secureRaw(t, pki.Client(t, subjects...), http.MethodGet, server.URL+"/api/v1/recovery", nil, http.StatusUnauthorized)
	}
	engineer := pki.Client(t, engineerSubject)
	secureCall(t, engineer, server.URL+"/api/v1/task-contracts", map[string]any{
		"contract": map[string]any{"task_contract_id": "denied-degradation", "work_item_id": "missing", "task_type": "DEBUG"},
		"material": map[string]any{"degradation_approved_by": engineerSubject, "degradation_reason": "caller claims approval"},
	}, http.StatusForbidden)
	secureRaw(t, engineer, http.MethodGet, server.URL+"/api/v1/recovery", nil, http.StatusOK)
	for _, body := range []string{
		`{"work_item_id":"spoof","title":"test","human_owner":"` + engineerSubject + `","HUMAN_OWNER":"fake"}`,
		`{"work_item_id":"spoof","title":"test","human_owner":"` + engineerSubject + `","human_owner":"fake"}`,
		`{"work_item_id":"spoof","title":"test","human_owner":"` + engineerSubject + `"} {}`,
	} {
		secureRaw(t, engineer, http.MethodPost, server.URL+"/api/v1/work-items", []byte(body), http.StatusBadRequest)
	}
	secureCall(t, engineer, server.URL+"/api/v1/work-items", map[string]any{"work_item_id": "spoof", "title": "test", "human_owner": "fake"}, http.StatusForbidden)
	if _, err := backend.GetWork("spoof"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("spoofed work persisted")
	}
	secureCall(t, engineer, server.URL+"/api/v1/recovery/begin", map[string]any{"expected_recovery_epoch": 0}, http.StatusForbidden)
	operator := pki.Client(t, recoverySubject)
	secureCall(t, operator, server.URL+"/api/v1/recovery/begin", map[string]any{"expected_recovery_epoch": 0}, http.StatusOK)
	secureCall(t, operator, server.URL+"/api/v1/recovery/complete", map[string]any{"recovery_epoch": 1}, http.StatusServiceUnavailable)
	state, err := backend.GetRecovery()
	if err != nil || string(state.Mode) != "RECOVERY_RECONCILIATION" {
		t.Fatal("completion bypassed reconciliation authority")
	}
	// A future route has no implicit grant, even when its path looks like an API.
	secureCall(t, engineer, server.URL+"/api/v1/admin/new", map[string]any{}, http.StatusForbidden)
}

type rejectingRecoveryGate struct{ calls atomic.Int64 }

func (g *rejectingRecoveryGate) AuthorizeCompletion(_ context.Context, subject string, epoch uint64) error {
	if subject == recoverySubject && epoch == 1 {
		g.calls.Add(1)
	}
	return errors.New("no retained reconciliation evidence")
}
func TestRecoveryRequiresIndependentEpochGate(t *testing.T) {
	backend := store.NewMemory()
	if _, err := backend.BeginRecovery(0); err != nil {
		t.Fatal(err)
	}
	gate := &rejectingRecoveryGate{}
	server, pki := securedTestServer(t, backend, nil, AuthenticatedOptions{RecoveryCompletion: gate})
	secureCall(t, pki.Client(t, recoverySubject), server.URL+"/api/v1/recovery/complete", map[string]any{"recovery_epoch": 1, "reconciled": true}, http.StatusForbidden)
	if gate.calls.Load() != 1 {
		t.Fatal("gate did not receive authenticated subject/exact epoch")
	}
}

type recordingGateway struct{ calls atomic.Int64 }

func (g *recordingGateway) Execute(ctx context.Context, req action.Request) (action.Receipt, error) {
	id, ok := AuthenticatedIdentity(ctx)
	if !ok || id.Subject() != req.RequestedBy {
		return action.Receipt{}, errors.New("missing principal")
	}
	g.calls.Add(1)
	return action.Receipt{ID: "test-receipt"}, nil
}
func (*recordingGateway) Get(string) (action.Operation, error) {
	return action.Operation{}, errors.New("not used")
}
func (*recordingGateway) Reconcile(context.Context, string) (action.Receipt, error) {
	return action.Receipt{}, errors.New("not used")
}

func TestActionRequiresExactPrincipalRiskAndCapabilityGrant(t *testing.T) {
	gateway := &recordingGateway{}
	server, pki := securedTestServer(t, store.NewMemory(), gateway, AuthenticatedOptions{})
	client := pki.Client(t, engineerSubject)
	body := map[string]any{"action_request_id": "action-auth", "execution_epoch": 1, "recovery_epoch": 0, "action": "ci.dispatch", "risk_class": "OBSERVE", "capability": "ci", "parameters_digest": "sha256:test", "idempotency_key": "action-auth", "requested_by": engineerSubject}
	url := server.URL + "/api/v1/runs/run-auth/actions"
	secureCall(t, client, url, body, http.StatusForbidden)
	body["risk_class"], body["requested_by"] = "CONTROLLED_MUTATION", ciSubject
	secureCall(t, client, url, body, http.StatusForbidden)
	body["requested_by"], body["capability"] = engineerSubject, "release"
	secureCall(t, client, url, body, http.StatusForbidden)
	if gateway.calls.Load() != 0 {
		t.Fatal("denied request reached gateway")
	}
	body["capability"] = "ci"
	secureCall(t, client, url, body, http.StatusAccepted)
	if gateway.calls.Load() != 1 {
		t.Fatal("valid request did not receive principal")
	}
}

func TestAllCoreRoutesHaveExplicitAccessPolicy(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "server.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "HandleFunc" {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok {
			t.Error("route registration must be literal for review")
			return true
		}
		pattern, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Error(err)
			return true
		}
		if pattern == "GET /healthz" {
			return true
		}
		seen[pattern] = true
		if routeCapabilities[pattern] == "" {
			t.Errorf("route missing explicit capability: %s", pattern)
		}
		return true
	})
	if len(seen) != len(routeCapabilities) {
		t.Fatal("stale access policy route mapping")
	}
}
