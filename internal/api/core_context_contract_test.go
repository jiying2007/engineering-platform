package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// The caller-scoped Core Store interface is mandatory for all supported
// Work/Task/Run/Checkpoint/Delivery/Evidence/Verification/Review/Closure
// HTTP operations. Prevent another optional legacy fallback from entering
// these seven adapters, even when a DEV/test Store type changes.
func TestCoreHTTPContextsNeverFallBackToUnscopedStore(t *testing.T) {
	paths := []string{
		"intake_context.go",
		"run_context.go",
		"run_control_context.go",
		"checkpoint_delivery_context.go",
		"evidence_verification_context.go",
		"review_closure_context.go",
		"steering_context.go",
	}
	for _, path := range paths {
		set := token.NewFileSet()
		file, err := parser.ParseFile(set, path, nil, 0)
		if err != nil {
			t.Fatalf("cannot parse %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch v := node.(type) {
			case *ast.TypeAssertExpr:
				if sel, ok := v.X.(*ast.SelectorExpr); ok && sel.Sel.Name == "store" {
					if receiver, ok := sel.X.(*ast.Ident); ok && receiver.Name == "s" {
						t.Errorf("%s: optional HTTP Store type assertion can bypass caller Context", set.Position(v.Pos()))
					}
				}
			case *ast.SelectorExpr:
				owner, ok := v.X.(*ast.SelectorExpr)
				if !ok || owner.Sel.Name != "store" {
					return true
				}
				receiver, ok := owner.X.(*ast.Ident)
				if !ok || receiver.Name != "s" {
					return true
				}
				if len(v.Sel.Name) < len("Context") || v.Sel.Name[len(v.Sel.Name)-len("Context"):] != "Context" {
					t.Errorf("%s: HTTP Core Store operation bypassed mandatory scoped method: %s", set.Position(v.Pos()), v.Sel.Name)
				}
			}
			return true
		})
	}
}
