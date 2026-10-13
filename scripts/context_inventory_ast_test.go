//go:build ignore

package main

import (
	"strings"
	"testing"
)

func TestASTContextOriginBinding(t *testing.T) {
	cases := []struct {
		name, text string
		want       []string
		reject     bool
	}{
		{"normal", `package p;import "context";func root(){_ = context.Background()}`, []string{"Background:direct_call"}, false},
		{"import-alias", `package p;import c "context";func root(){_ = c.TODO()}`, []string{"TODO:direct_call"}, false},
		{"function-value", `package p;import c "context";var makeRoot=c.Background;func root(){_ = makeRoot()}`, []string{"Background:function_value"}, false},
		{"method-value-alias", `package p;import c "context";func root(){newRoot:=c.WithoutCancel;_=newRoot(nil)}`, []string{"WithoutCancel:function_value"}, false},
		{"parenthesized", `package p;import c "context";func root(){_=(c.Background)()}`, []string{"Background:direct_call"}, false},
		{"multiline", "package p\nimport c \"context\"\nfunc f(){_=c.\nBackground(\n)}", []string{"Background:direct_call"}, false},
		{"comments-and-strings", "package p\nimport \"context\"\n// context.TODO()\n/*context.Background()*/\nvar s=\"context.WithoutCancel(x)\"", nil, false},
		{"unrelated-import", `package p;import context "example/context";func root(){_=context.Background()}`, nil, false},
		{"local-shadow", `package p;import c "context";func root(){c:=struct{Background func() int}{};_=c.Background()}`, nil, false},
		{"parameter-shadow", `package p;import c "context";func root(c interface{TODO() int}){_=c.TODO()}`, nil, false},
		{"nested-shadow", `package p;import c "context";func root(){_=c.Background();{c:=struct{TODO func() int}{};_=c.TODO()};_=c.WithoutCancel(nil)}`, []string{"Background:direct_call", "WithoutCancel:direct_call"}, false},
		{"shadow-later", `package p;import c "context";func root(){_=c.Background();c:=struct{TODO func() int}{};_=c.TODO()}`, []string{"Background:direct_call"}, false},
		{"literal-capture", `package p;import c "context";func root(){_ = func(){_=c.Background}}`, []string{"Background:function_value"}, false},
		{"package-origin", `package p;import c "context";var root=c.Background()`, []string{"Background:direct_call"}, false},
		{"cross-file-alias-origin", `package p;import c "context";var newRoot=c.Background`, []string{"Background:function_value"}, false},
		{"unused-blank", `package p;import _ "context";func root(){}`, nil, false},
		{"dot-import", `package p;import . "context";var c=Background()`, nil, true},
		{"bad-syntax", `package p;func {`, nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scan([]source{{"internal/a.go", tc.text}})
			if tc.reject {
				if err == nil {
					t.Fatal("unclassifiable source accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, f := range got {
				if f.Kind == "context_constructor" {
					names = append(names, f.Name+":"+f.ReferenceForm)
				}
			}
			if strings.Join(names, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("got %v want %v", names, tc.want)
			}
		})
	}
}
func TestASTPhysicalPositionsAndLegacyReferences(t *testing.T) {
	got, err := scan([]source{{"internal/z.go", "package p\nimport c \"context\"\n//line fabricated.go:1\nvar f = c.Background\nvar g = s.GetWork\n"}})
	if err != nil || len(got) != 2 || got[0].Path != "internal/z.go" || got[0].Line != 4 || got[1].Line != 5 || got[1].ReferenceForm != "function_value" {
		t.Fatalf("invalid physical evidence: %+v %v", got, err)
	}
}
func TestASTSourceBudgetsAndDeterminism(t *testing.T) {
	for _, files := range [][]source{make([]source, maxFiles+1), {{"a.go", "package p"}, {"a.go", "package p"}}, {{"a.go", strings.Repeat("x", (2<<20)+1)}}} {
		if _, err := scan(files); err == nil {
			t.Fatal("unbounded or duplicate source accepted")
		}
	}
	files := []source{{"internal/z.go", `package p;import "context";var c=context.TODO()`}, {"cmd/a.go", `package p;import "context";var c=context.Background()`}}
	got, err := scan(files)
	if err != nil || len(got) != 2 || got[0].Path != "cmd/a.go" {
		t.Fatalf("unstable ordering %+v %v", got, err)
	}
}
