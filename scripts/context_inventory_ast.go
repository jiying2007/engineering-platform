//go:build ignore

// Read-only, module-independent AST frontend for authority_context_inventory.py.
// Parse precisely the same bytes that Python hashes, including inactive build
// variants. This identifies origin references, not run-time points-to or lifetime.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

const maxInput = 96 << 20
const maxFiles = 10000
const maxFindings = 20000

type source struct {
	Path string `json:"path"`
	Text string `json:"text"`
}
type finding struct {
	Path          string `json:"path"`
	Line          int    `json:"line"`
	Column        int    `json:"column"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	ReferenceForm string `json:"reference_form"`
}

var origins = map[string]bool{"Background": true, "TODO": true, "WithoutCancel": true}
var legacy = func() map[string]bool {
	m := map[string]bool{}
	for _, n := range strings.Fields("CreateWork GetWork UpdateWork GetRecovery BeginRecovery CompleteRecovery GetTask GetTaskByDigest GetExecution GetAttempt GetTaskRevision GetRunInputByDigest CreateExecutionAndUpdateWork UpdateExecution RecordSteering GetSteering UpdateExecutionAndWork CreateCheckpoint GetCheckpoint CreateDelivery GetDelivery CreateEvidence GetEvidence CreateVerification GetVerification CreateReviewAndUpdateWork GetReview CreateClosureAndUpdateWork GetClosure") {
		m[n] = true
	}
	return m
}()

func scan(files []source) ([]finding, error) {
	if len(files) > maxFiles {
		return nil, errors.New("too many source files")
	}
	result := []finding{}
	seen := map[string]bool{}
	for _, src := range files {
		if len(src.Text) > 2<<20 || src.Path == "" || len(src.Path) > 4096 || seen[src.Path] {
			return nil, errors.New("invalid source inventory budget or identity")
		}
		seen[src.Path] = true
		set := token.NewFileSet()
		// Deliberately retain the Go 1.25 parser's lexical object resolution. An
		// imported alias and a local variable that shadows it are not equivalent.
		// No SkipObjectResolution: using that flag would erase this distinction.
		f, err := parser.ParseFile(set, src.Path, src.Text, 0)
		if err != nil {
			return nil, fmt.Errorf("invalid Go syntax in %s", src.Path)
		}
		aliases := map[string]bool{}
		for _, imp := range f.Imports {
			name, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return nil, errors.New("invalid Go import")
			}
			if name != "context" {
				continue
			}
			alias := "context"
			if imp.Name != nil {
				alias = imp.Name.Name
			}
			// Dot imports are not silently classified with incomplete package binding.
			// Production has none; reject the style rather than guess across files.
			if alias == "." {
				return nil, fmt.Errorf("dot-imported context requires explicit import in %s", src.Path)
			}
			if alias != "_" {
				aliases[alias] = true
			}
		}
		direct := map[*ast.SelectorExpr]bool{}
		ast.Inspect(f, func(node ast.Node) bool {
			if c, ok := node.(*ast.CallExpr); ok {
				expr := c.Fun
				for {
					p, ok := expr.(*ast.ParenExpr)
					if !ok {
						break
					}
					expr = p.X
				}
				if s, ok := expr.(*ast.SelectorExpr); ok {
					direct[s] = true
				}
			}
			return true
		})
		ast.Inspect(f, func(node ast.Node) bool {
			s, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			kind := ""
			if id, ok := s.X.(*ast.Ident); ok && aliases[id.Name] && (id.Obj == nil || id.Obj.Kind == ast.Pkg) && origins[s.Sel.Name] {
				kind = "context_constructor"
			}
			if legacy[s.Sel.Name] {
				kind = "unscoped_name_candidate"
			}
			if kind != "" {
				pos := set.PositionFor(s.Pos(), false) // Ignore //line remapping of evidence locations.
				form := "function_value"
				if direct[s] {
					form = "direct_call"
				}
				result = append(result, finding{src.Path, pos.Line, pos.Column, kind, s.Sel.Name, form})
			}
			return true
		})
		if len(result) > maxFindings {
			return nil, errors.New("too many source references")
		}
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		return a.Name < b.Name
	})
	return result, nil
}
func main() {
	data, err := io.ReadAll(io.LimitReader(os.Stdin, maxInput+1))
	if err == nil && len(data) > maxInput {
		err = errors.New("source request exceeds budget")
	}
	var files []source
	if err == nil {
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.DisallowUnknownFields()
		err = dec.Decode(&files)
		if err == nil {
			var tail any
			if dec.Decode(&tail) != io.EOF {
				err = errors.New("trailing source request")
			}
		}
	}
	var result []finding
	if err == nil {
		result, err = scan(files)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err = json.NewEncoder(os.Stdout).Encode(result); err != nil {
		os.Exit(1)
	}
}
