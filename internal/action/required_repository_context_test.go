package action

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// Compile-time admission: an unscoped-only Repository can no longer be
// passed to Action Service. The same interface is used by PostgreSQL/Core.
var _ Repository = (*MemoryRepository)(nil)

func TestRequiredMemoryActionContextRejectsPreCancelledAdmission(t *testing.T) {
	repo := NewMemoryRepository()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	op := Operation{ID: "scoped-only", RunID: "run-1", State: Planned, IdempotencyKey: "scoped-only"}
	if err := repo.CreateContext(ctx, op); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Action Create admitted: %v", err)
	}
	if _, err := repo.GetContext(ctx, op.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Action GET bypassed Context: %v", err)
	}
	if _, err := repo.GetByIdempotencyKeyContext(ctx, op.IdempotencyKey); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled idempotency lookup bypassed Context: %v", err)
	}
	if err := repo.UpdateContext(ctx, op); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Action update bypassed Context: %v", err)
	}
	if _, err := repo.Get(op.ID); !errors.Is(err, ErrOperationAbsent) {
		t.Fatalf("cancelled Action Create left a reservation: %v", err)
	}
	if err := repo.CreateContext(context.Background(), op); err != nil {
		t.Fatalf("independently scoped valid admission failed: %v", err)
	}
	if got, err := repo.GetContext(context.Background(), op.ID); err != nil || got.ID != op.ID {
		t.Fatalf("independent scoped readback lost durable identity: %+v %v", got, err)
	}
}

// A regression adding an optional legacy Repository type assertion or direct
// context-free call to Action Service is not a harmless compatibility detail:
// it can bypass the initiating mTLS caller's cancellation before dispatch.
func TestActionServiceDoesNotSelectLegacyRepositoryMethods(t *testing.T) {
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, "service.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]bool{
		"Create": true, "Get": true, "GetByIdempotencyKey": true, "Update": true,
	}
	ast.Inspect(file, func(node ast.Node) bool {
		sel, ok := node.(*ast.SelectorExpr)
		if !ok || !forbidden[sel.Sel.Name] {
			return true
		}
		owner, ok := sel.X.(*ast.SelectorExpr)
		if !ok || owner.Sel.Name != "repository" {
			return true
		}
		if receiver, ok := owner.X.(*ast.Ident); ok && receiver.Name == "s" {
			t.Errorf("%s: Service directly invoked context-free Action Repository operation", set.Position(sel.Pos()))
		}
		return true
	})
}
