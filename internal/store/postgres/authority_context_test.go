package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPostgresAuthorityBudgetPreservesCaller(t *testing.T) {
	type key struct{}
	root := context.WithValue(context.Background(), key{}, "caller-owned")
	started := time.Now()
	ctx, cancel := authorityContext(root)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok || deadline.Before(started.Add(maxStatementTimeout)) || deadline.After(time.Now().Add(maxStatementTimeout)) || ctx.Value(key{}) != "caller-owned" {
		t.Fatalf("unbounded caller did not acquire the declared fallback: %s %v", deadline, ok)
	}
	// A nested method must not start another full wait budget or own the outer
	// cancel function. The first deadline follows the transaction to settlement.
	nested, stop := authorityContext(ctx)
	stop()
	if nested != ctx || ctx.Err() != nil {
		t.Fatal("nested method changed the deadline or cancelled its caller")
	}
	cancel()
	if !errors.Is(ctx.Err(), context.Canceled) || root.Err() != nil {
		t.Fatal("child cancellation was lost or cancelled the caller")
	}
	for _, duration := range []time.Duration{-time.Second, time.Second, time.Hour} {
		parent, end := context.WithTimeout(root, duration)
		child, finish := authorityContext(parent)
		finish()
		want, _ := parent.Deadline()
		got, bounded := child.Deadline()
		if child != parent || !bounded || got != want || child.Err() != parent.Err() {
			t.Fatalf("explicit caller budget %s was changed", duration)
		}
		end()
		if !errors.Is(child.Err(), context.Canceled) && !errors.Is(child.Err(), context.DeadlineExceeded) {
			t.Fatal("explicit caller cancellation was lost")
		}
	}
	parent, end := context.WithCancel(root)
	end()
	child, finish := authorityContext(parent)
	defer finish()
	if !errors.Is(child.Err(), context.Canceled) {
		t.Fatal("pre-cancelled no-deadline caller acquired a live child")
	}
}

// A bounded direct operation must assign its original Context and defer its
// cancel in the enclosing function, not hide a timer in an unused closure.
// This is a source contract for the PostgreSQL entry boundary, not a general
// interprocedural theorem. Existing caller-derived 5s/30s policies stay valid.
func boundedAuthorityEntry(fn *ast.FuncDecl, contextAlias string) bool {
	if fn.Body == nil || fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
		return false
	}
	p := fn.Type.Params.List[0]
	if len(p.Names) != 1 {
		return false
	}
	ctx := p.Names[0].Name
	for i, statement := range fn.Body.List {
		assign, ok := statement.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 2 || len(assign.Rhs) != 1 {
			continue
		}
		left, ok := assign.Lhs[0].(*ast.Ident)
		stop, stopOK := assign.Lhs[1].(*ast.Ident)
		call, callOK := assign.Rhs[0].(*ast.CallExpr)
		if !ok || left.Name != ctx || !stopOK || stop.Name == "_" || !callOK || len(call.Args) == 0 {
			continue
		}
		parent, ok := call.Args[0].(*ast.Ident)
		if !ok || parent.Name != ctx {
			continue
		}
		valid := false
		switch target := call.Fun.(type) {
		case *ast.Ident:
			valid = target.Name == "authorityContext" && len(call.Args) == 1
		case *ast.SelectorExpr:
			pkg, ok := target.X.(*ast.Ident)
			valid = ok && pkg.Name == contextAlias && (target.Sel.Name == "WithTimeout" || target.Sel.Name == "WithDeadline") && len(call.Args) == 2
		}
		if !valid || i+1 >= len(fn.Body.List) {
			continue
		}
		deferred, ok := fn.Body.List[i+1].(*ast.DeferStmt)
		if !ok || len(deferred.Call.Args) != 0 {
			continue
		}
		called, ok := deferred.Call.Fun.(*ast.Ident)
		if ok && called.Name == stop.Name {
			// A timer after a SQL/helper call does not bound its pool wait.
			for _, earlier := range fn.Body.List[:i] {
				unboundedCall := false
				ast.Inspect(earlier, func(n ast.Node) bool {
					if _, deferredBody := n.(*ast.FuncLit); deferredBody {
						return false
					}
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					for _, arg := range call.Args {
						if id, ok := arg.(*ast.Ident); ok && id.Name == ctx {
							unboundedCall = true
						}
					}
					return true
				})
				if unboundedCall {
					return false
				}
			}
			return true
		}
	}
	return false
}

const relayBudgetBodyDigest = "c44901be2c7ff21ce28c4734afb982594eb80486f86bac74d45a71d16c363487"

func TestPostgresAuthorityBudgetEveryPublicEntry(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		alias := ""
		for _, imp := range f.Imports {
			name, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			if name == "context" {
				alias = "context"
				if imp.Name != nil {
					alias = imp.Name.Name
				}
			}
		}
		for _, declaration := range f.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || !fn.Name.IsExported() || fn.Body == nil || fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
				continue
			}
			if fn.Recv == nil && fn.Name.Name != "Open" {
				continue
			}
			if fn.Recv != nil {
				ptr, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
				if !ok {
					continue
				}
				recv, ok := ptr.X.(*ast.Ident)
				if !ok || recv.Name != "Store" {
					continue
				}
			}
			parameter, ok := fn.Type.Params.List[0].Type.(*ast.SelectorExpr)
			if !ok || parameter.Sel.Name != "Context" {
				continue
			}
			pkg, ok := parameter.X.(*ast.Ident)
			if !ok || pkg.Name != alias {
				continue
			}
			seen++
			if fn.Name.Name == "RelayRunStarts" {
				// Existing finite batch: at most 64 original 5s steps, each
				// immediately cancelled. Do not replace this with a new short
				// whole-batch timer. Exact normalized implementation is reviewed.
				var body bytes.Buffer
				if err := printer.Fprint(&body, fset, fn); err != nil {
					t.Fatal(err)
				}
				if got := fmt.Sprintf("%x", sha256.Sum256(body.Bytes())); got != relayBudgetBodyDigest {
					t.Errorf("bounded relay policy changed: %s", got)
				}
				continue
			}
			if !boundedAuthorityEntry(fn, alias) {
				t.Errorf("%s: PostgreSQL entry %s has no caller-derived whole-operation budget", fset.Position(fn.Pos()), fn.Name.Name)
			}
		}
	}
	if seen < 84 {
		t.Fatalf("authority entry enumeration lost coverage: %d", seen)
	}
	t.Logf("Audited %d public PostgreSQL Context entries (including startup Open/migration; existing explicit budgets retained)", seen)
}

func TestPostgresAuthorityBudgetGuardRejectsDetachedOrUnreleasedTimers(t *testing.T) {
	cases := []struct {
		body  string
		valid bool
	}{
		{"ctx,cancel:=authorityContext(ctx);defer cancel();_ = ctx", true},
		{"ctx,cancel:=c.WithTimeout(ctx,d);defer cancel();_ = ctx", true},
		{"ctx,cancel:=c.WithDeadline(ctx,d);defer cancel();_ = ctx", true},
		{"_ = ctx", false},
		{"s.pool.Ping(ctx);ctx,cancel:=authorityContext(ctx);defer cancel();_ = ctx", false},
		{"unbounded(ctx);ctx,cancel:=authorityContext(ctx);defer cancel();_ = ctx", false},
		{"ctx,cancel:=authorityContext(ctx);_ = cancel;_ = ctx", false},
		{"ctx,cancel:=authorityContext(c.Background());defer cancel();_ = ctx", false},
		{"_ = func(){ctx,cancel:=authorityContext(ctx);defer cancel();_ = ctx}", false},
		{"other,cancel:=authorityContext(ctx);defer cancel();_ = other", false},
		{"ctx,cancel:=authorityContext(ctx);defer wrong();_ = cancel;_ = ctx", false},
	}
	for _, tc := range cases {
		f, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package p; import c \"context\";func (s *Store) Read(ctx c.Context) {"+tc.body+"}", 0)
		if err != nil {
			t.Fatal(err)
		}
		fn := f.Decls[1].(*ast.FuncDecl)
		if got := boundedAuthorityEntry(fn, "c"); got != tc.valid {
			t.Errorf("guard=%v want=%v for %q", got, tc.valid, tc.body)
		}
	}
}

// Context provenance at SQL sinks. Standard AST object resolution binds an
// identifier to its actual local declaration, not merely to the text "ctx".
// In particular, a similarly named struct field or an unreviewed provider of
// Context is not accepted. Method-value aliases of SQL sinks are rejected so
// they cannot evade this current-source inventory. The Go compiler supplies
// type checking separately; this does not claim arbitrary dynamic points-to proof.
func callerSQLContext(expr ast.Expr, alias string, rollback bool, budget int) bool {
	if budget <= 0 {
		return false
	}
	switch x := expr.(type) {
	case *ast.ParenExpr:
		return callerSQLContext(x.X, alias, rollback, budget-1)
	case *ast.Ident:
		if x.Obj == nil {
			return false
		}
		switch decl := x.Obj.Decl.(type) {
		case *ast.Field:
			typ, ok := decl.Type.(*ast.SelectorExpr)
			if !ok || typ.Sel.Name != "Context" {
				return false
			}
			pkg, ok := typ.X.(*ast.Ident)
			return ok && pkg.Name == alias
		case *ast.AssignStmt:
			for i, target := range decl.Lhs {
				id, ok := target.(*ast.Ident)
				if !ok || id.Obj != x.Obj {
					continue
				}
				if len(decl.Rhs) == len(decl.Lhs) {
					return callerSQLContext(decl.Rhs[i], alias, rollback, budget-1)
				}
				if i == 0 && len(decl.Rhs) == 1 {
					return callerSQLContext(decl.Rhs[0], alias, rollback, budget-1)
				}
			}
		}
	case *ast.CallExpr:
		if target, ok := x.Fun.(*ast.Ident); ok && target.Name == "authorityContext" && len(x.Args) == 1 {
			return callerSQLContext(x.Args[0], alias, rollback, budget-1)
		}
		selector, ok := x.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		pkg, ok := selector.X.(*ast.Ident)
		if !ok || pkg.Name != alias {
			return false
		}
		if (selector.Sel.Name == "WithTimeout" || selector.Sel.Name == "WithDeadline" || selector.Sel.Name == "WithCancel") && len(x.Args) > 0 {
			return callerSQLContext(x.Args[0], alias, rollback, budget-1)
		}
		// The existing direct Rollback is the sole deliberately independent
		// SQL Context origin. Its 5s child and direct commit/rollback ownership
		// are separately guarded by the retained transaction contract test.
		return rollback && selector.Sel.Name == "Background" && len(x.Args) == 0
	}
	return false
}

// A parameter declaration alone is not sufficient after reassignment. Refuse
// replacing a known Context local with an unreviewed source. This intentionally
// permits only the actual caller or the same reviewed child constructors above.
func sqlContextAssignmentsSafe(body *ast.BlockStmt, alias string, rollback bool) bool {
	safe := true
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, left := range assign.Lhs {
			id, ok := left.(*ast.Ident)
			if !ok || !callerSQLContext(id, alias, rollback, 32) {
				continue
			}
			var right ast.Expr
			if len(assign.Lhs) == len(assign.Rhs) {
				right = assign.Rhs[i]
			} else if i == 0 && len(assign.Rhs) == 1 {
				right = assign.Rhs[0]
			}
			if right == nil || !callerSQLContext(right, alias, rollback, 32) {
				safe = false
			}
		}
		return true
	})
	return safe
}

func TestPostgresAuthorityBudgetSQLContextProvenance(t *testing.T) {
	sinks := map[string]bool{"Acquire": true, "Begin": true, "BeginTx": true, "Exec": true, "Query": true, "QueryRow": true, "SendBatch": true, "CopyFrom": true, "Ping": true, "Commit": true, "Rollback": true}
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		alias := ""
		for _, imp := range f.Imports {
			name, _ := strconv.Unquote(imp.Path.Value)
			if name == "context" {
				alias = "context"
				if imp.Name != nil {
					alias = imp.Name.Name
				}
			}
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if !sqlContextAssignmentsSafe(fn.Body, alias, path == "outbox_store.go" && fn.Name.Name == "rollbackOutbox") {
				t.Errorf("%s: a caller Context was replaced by an unreviewed origin", fset.Position(fn.Pos()))
			}
			var stack []ast.Node
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if n == nil {
					stack = stack[:len(stack)-1]
					return false
				}
				var parent ast.Node
				if len(stack) > 0 {
					parent = stack[len(stack)-1]
				}
				stack = append(stack, n)
				selector, ok := n.(*ast.SelectorExpr)
				if !ok || !sinks[selector.Sel.Name] {
					return true
				}
				call, ok := parent.(*ast.CallExpr)
				if !ok || call.Fun != selector {
					t.Errorf("%s: indirect SQL method value needs explicit Context review", fset.Position(n.Pos()))
					return true
				}
				seen++
				rollback := path == "outbox_store.go" && fn.Name.Name == "rollbackOutbox" && selector.Sel.Name == "Rollback"
				if len(call.Args) == 0 || !callerSQLContext(call.Args[0], alias, rollback, 32) {
					t.Errorf("%s: SQL %s is not bound to its owning caller or reviewed rollback", fset.Position(n.Pos()), selector.Sel.Name)
				}
				return true
			})
		}
	}
	if seen < 200 {
		t.Fatalf("SQL Context inventory unexpectedly small: %d", seen)
	}
	t.Logf("Verified %d direct SQL calls against actual Context declarations; independent rollback remains the only exception", seen)
	// Exercising declaration identity catches name-based false positives.
	fixtures := []struct {
		source string
		want   bool
	}{
		{`func f(ctx c.Context){pool.Exec(ctx,"q")}`, true},
		{`func f(ctx c.Context){child,stop:=c.WithTimeout(ctx,d);defer stop();pool.Exec(child,"q")}`, true},
		{`func f(ctx c.Context){alias:=ctx;pool.Exec(alias,"q")}`, true},
		{`func f(ctx c.Context){pool.Exec(s.ctx,"q")}`, false},
		{`func f(ctx c.Context){other:=unknownContext();pool.Exec(other,"q")}`, false},
		{`func f(ctx c.Context){other:=c.Background();pool.Exec(other,"q")}`, false},
		{`func f(ctx c.Context){ctx=unknownContext();pool.Exec(ctx,"q")}`, false},
		{`func f(ctx c.Context){ctx=c.Background();pool.Exec(ctx,"q")}`, false},
	}
	for _, tc := range fixtures {
		f, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package p;import c \"context\";"+tc.source, 0)
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Exec" {
				return true
			}
			calls++
			if got := callerSQLContext(call.Args[0], "c", false, 32) && sqlContextAssignmentsSafe(f.Decls[1].(*ast.FuncDecl).Body, "c", false); got != tc.want {
				t.Errorf("SQL provenance=%v want=%v: %s", got, tc.want, tc.source)
			}
			return true
		})
		if calls != 1 {
			t.Fatal("fixture did not exercise one SQL call")
		}
	}
}
