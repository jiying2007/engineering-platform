package api

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/recovery"
	"github.com/jiying2007/engineering-platform/internal/store"
)

const secretDiagnostic = "private SQL credential SELECT sensitive_token"

func TestAuthorityErrorWritersNeverEchoSQLOrProviderDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name   string
		writer func(http.ResponseWriter, error)
		err    error
		status int
	}{
		{"store-not-found", writeStoreError, fmt.Errorf("%s: %w", secretDiagnostic, store.ErrNotFound), http.StatusNotFound},
		{"store-failed", writeStoreError, errors.New(secretDiagnostic), http.StatusInternalServerError},
		{"mutation-conflict", writeMutationError, fmt.Errorf("%s: %w", secretDiagnostic, store.ErrConflict), http.StatusConflict},
		{"mutation-failed", writeMutationError, errors.New(secretDiagnostic), http.StatusUnprocessableEntity},
		{"action-denied", writeActionError, fmt.Errorf("%s: %w", secretDiagnostic, action.ErrDenied), http.StatusForbidden},
		{"action-conflict", writeActionError, fmt.Errorf("%s: %w", secretDiagnostic, action.ErrIdempotencyConflict), http.StatusConflict},
		{"action-missing", writeActionError, fmt.Errorf("%s: %w", secretDiagnostic, action.ErrOperationAbsent), http.StatusNotFound},
		{"action-provider-failed", writeActionError, errors.New(secretDiagnostic), http.StatusUnprocessableEntity},
		{"commit-unknown", writeActionError, fmt.Errorf("%s: %w", secretDiagnostic, store.ErrCommitOutcomeUnknown), http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.writer(rec, tc.err)
			body := rec.Body.String()
			if rec.Code != tc.status || strings.Contains(body, secretDiagnostic) ||
				strings.Contains(body, "sensitive_token") || !strings.Contains(body, "\"error\"") {
				t.Fatalf("authority error leaked internal details or changed status: %d %q", rec.Code, body)
			}
			if tc.name == "commit-unknown" &&
				(!strings.Contains(body, "commit_outcome_unknown") || !strings.Contains(body, "read back")) {
				t.Fatalf("unknown COMMIT lost mandatory readback semantics: %q", body)
			}
		})
	}
}

type sensitiveRecoveryErrorStore struct {
	*store.Memory
	err error
}

func (s *sensitiveRecoveryErrorStore) GetRecoveryContext(context.Context) (recovery.Manager, error) {
	return recovery.Manager{}, s.err
}
func (s *sensitiveRecoveryErrorStore) BeginRecoveryContext(context.Context, uint64) (recovery.Manager, error) {
	return recovery.Manager{}, s.err
}
func (s *sensitiveRecoveryErrorStore) CompleteRecoveryContext(context.Context, uint64, bool) (recovery.Manager, error) {
	return recovery.Manager{}, s.err
}
func (s *sensitiveRecoveryErrorStore) CreateRecoveryProof(context.Context, uint64, string) (recovery.Proof, error) {
	return recovery.Proof{}, s.err
}
func (s *sensitiveRecoveryErrorStore) GetRecoveryProof(context.Context, uint64) (recovery.Proof, error) {
	return recovery.Proof{}, s.err
}

func TestRecoveryHTTPHandlersDoNotLeakDriverDiagnostics(t *testing.T) {
	h := NewServer(&sensitiveRecoveryErrorStore{
		Memory: store.NewMemory(), err: fmt.Errorf("upstream %s: %w", secretDiagnostic, errors.New("connection lost")),
	}).Handler()
	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   any
		status int
	}{
		{"get", http.MethodGet, "/api/v1/recovery", nil, http.StatusServiceUnavailable},
		{"begin", http.MethodPost, "/api/v1/recovery/begin", map[string]any{"expected_recovery_epoch": 0}, http.StatusInternalServerError},
		{"complete", http.MethodPost, "/api/v1/recovery/complete", map[string]any{"recovery_epoch": 1}, http.StatusInternalServerError},
		{"proof", http.MethodPost, "/api/v1/recovery/proofs", map[string]any{"recovery_epoch": 1, "reconciler": "reviewer"}, http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := string(mustRequest(t, h, tc.method, tc.path, tc.body, tc.status))
			if strings.Contains(body, secretDiagnostic) ||
				strings.Contains(body, "sensitive_token") ||
				!strings.Contains(body, "\"error\"") {
				t.Fatalf("Recovery route leaked SQL or provider secret: %q", body)
			}
		})
	}
}

// All API handlers must map untrusted provider/Store errors to fixed codes,
// not expose the raw err.Error() inside writeError, including concatenations.
func TestAPIErrorWritersDoNotReflectRawErrorStrings(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil || len(paths) == 0 {
		t.Fatalf("cannot list API source files: %v", err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		set := token.NewFileSet()
		file, err := parser.ParseFile(set, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			target, ok := call.Fun.(*ast.Ident)
			if !ok || target.Name != "writeError" {
				return true
			}
			for _, arg := range call.Args {
				ast.Inspect(arg, func(child ast.Node) bool {
					inner, ok := child.(*ast.CallExpr)
					if !ok {
						return true
					}
					member, ok := inner.Fun.(*ast.SelectorExpr)
					if ok && member.Sel.Name == "Error" {
						t.Errorf("%s: raw Error() passed to public API error writer", set.Position(inner.Pos()))
					}
					return true
				})
			}
			return true
		})
	}
}
