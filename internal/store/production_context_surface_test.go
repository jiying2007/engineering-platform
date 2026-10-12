package store

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// Current production source has no legitimate use of the concrete legacy Core
// convenience methods. Reject direct calls AND method-value references across
// cmd/internal, rather than checking only seven HTTP adapters. This is a source
// guard, not a proof of all interprocedural cancellation/OS completion behavior.
func TestProductionSourceNeverUsesUnscopedCoreConvenienceMethods(t *testing.T) {
	names := strings.Fields("CreateWork GetWork UpdateWork GetRecovery BeginRecovery CompleteRecovery CreateTaskAndUpdateWork GetTask GetTaskRevision GetTaskByDigest GetVerificationPlanByDigest CreateExecutionAndUpdateWork GetExecution GetAttempt GetRunInputByDigest UpdateExecution RecordSteering GetSteering UpdateExecutionAndWork CreateCheckpoint GetCheckpoint CreateDelivery GetDelivery CreateEvidence GetEvidence CreateVerification GetVerification CreateReviewAndUpdateWork GetReview CreateClosureAndUpdateWork GetClosure")
	forbidden := map[string]bool{}
	for _, name := range names {
		forbidden[name] = true
	}
	for _, folder := range []string{"../../cmd", "../../internal"} {
		err := filepath.WalkDir(folder, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			set := token.NewFileSet()
			source, err := parser.ParseFile(set, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(source, func(node ast.Node) bool {
				if selected, ok := node.(*ast.SelectorExpr); ok && forbidden[selected.Sel.Name] {
					t.Errorf("%s: production source referenced context-free Core %s", set.Position(selected.Pos()), selected.Sel.Name)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
