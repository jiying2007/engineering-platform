package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestWorkerRoutesHaveSeparateExplicitAccessPolicy(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "worker.go", nil, 0)
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
			t.Error("worker route must be literal")
			return true
		}
		pattern, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Error(err)
			return true
		}
		if seen[pattern] || workerRouteCapabilities[pattern] == "" || routeCapabilities[pattern] != "" {
			t.Errorf("duplicate, unmapped or overlapping worker route: %s", pattern)
		}
		seen[pattern] = true
		return true
	})
	if len(seen) != len(workerRouteCapabilities) {
		t.Fatal("stale worker access policy")
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/worker/claim", strings.NewReader(`{"worker_profile":"worker/ubuntu"}`))
	response := httptest.NewRecorder()
	NewServer(nil).Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatal("unauthenticated development API exposed a worker route")
	}
}
