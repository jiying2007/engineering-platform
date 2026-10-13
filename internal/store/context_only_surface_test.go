package store_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/store"
	"github.com/jiying2007/engineering-platform/internal/store/postgres"
)

// Check concrete method sets as well as the required service interfaces. A
// context-free convenience on a concrete type is still an accidental escape
// hatch even if no current production caller uses it. Tests must call the same
// Context methods directly, not retain an unscoped fixture compatibility layer.
func TestConcreteAuthoritySurfacesRequireCallerContext(t *testing.T) {
	coreNames := strings.Fields("BeginRecovery CompleteRecovery CreateCheckpoint CreateClosureAndUpdateWork CreateDelivery CreateEvidence CreateExecutionAndUpdateWork CreateReviewAndUpdateWork CreateTaskAndUpdateWork CreateVerification CreateWork GetAttempt GetCheckpoint GetClosure GetDelivery GetEvidence GetExecution GetRecovery GetReview GetRunInputByDigest GetSteering GetTask GetTaskByDigest GetTaskRevision GetVerification GetVerificationPlanByDigest GetWork RecordSteering UpdateExecution UpdateExecutionAndWork UpdateWork")
	actionNames := strings.Fields("Create Get GetByIdempotencyKey Update")
	coreAndAction := append(append([]string{}, coreNames...), actionNames...)
	cases := []struct {
		name   string
		typeOf reflect.Type
		names  []string
	}{
		{"Memory", reflect.TypeOf((*store.Memory)(nil)), coreAndAction},
		{"PostgreSQL", reflect.TypeOf((*postgres.Store)(nil)), coreAndAction},
		{"ActionMemory", reflect.TypeOf((*action.MemoryRepository)(nil)), actionNames},
		{"ActionService", reflect.TypeOf((*action.Service)(nil)), []string{"Get"}},
	}
	contextType := reflect.TypeOf((*context.Context)(nil)).Elem()
	checked := 0
	for _, test := range cases {
		for _, name := range test.names {
			checked++
			t.Run(test.name+"/"+name, func(t *testing.T) {
				if _, ok := test.typeOf.MethodByName(name); ok {
					t.Errorf("context-free authority method restored: %s.%s", test.name, name)
				}
				method, ok := test.typeOf.MethodByName(name + "Context")
				if !ok || method.Type.NumIn() < 2 || method.Type.In(1) != contextType {
					t.Errorf("mandatory caller Context method missing: %s.%sContext", test.name, name)
				}
			})
		}
	}
	if checked != 75 {
		t.Fatalf("concrete authority surface inventory changed without review: %d", checked)
	}
}
