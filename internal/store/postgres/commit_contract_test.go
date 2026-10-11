package postgres

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// Every PostgreSQL authority transaction that can mutate a durable Run,
// Action, Recovery, Worker or Codex fact must classify a failed COMMIT ACK as
// unknown. These are the only deliberately direct COMMIT implementations:
// one centralized classifier, one Outbox-specific classifier and one
// non-effectful schema-readiness transaction.
//
// Keep this allowlist explicit: changing an existing transaction's semantic
// category requires reviewing its recovery/readback contract, not merely
// adding another direct tx.Commit(...) call.
func TestProductionCommitSitesHaveExplicitUnknownOutcomeAuthority(t *testing.T) {
	allowed := map[string]int{
		"commit_outcome.go":   1, // commitAuthority classifies typed Core/Worker unknown.
		"outbox_store.go":     1, // Outbox has dedicated ErrOutcomeUnknown.
		"schema_readiness.go": 1, // Read-only transaction has no durable side effect.
	}
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) == 0 {
		t.Fatal("no PostgreSQL source files found; commit guard did not run")
	}
	seen := make(map[string]int)
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Commit" {
				return true
			}
			seen[path]++
			if max, permitted := allowed[path]; !permitted || seen[path] > max {
				t.Errorf("%s: unclassified direct PostgreSQL Commit; use commitAuthority or document and test a separate outcome contract",
					fset.Position(call.Pos()))
			}
			return true
		})
	}
	for path, want := range allowed {
		if seen[path] != want {
			t.Errorf("%s: direct COMMIT boundary changed: got %d, expected %d; re-audit the outcome/readback contract",
				path, seen[path], want)
		}
	}
}
