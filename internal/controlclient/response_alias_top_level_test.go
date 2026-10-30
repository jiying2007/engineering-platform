package controlclient

import (
	"context"
	"reflect"
	"testing"
)

// Top-level object responses may be decoded directly to typed maps instead
// of a wrapper struct. Nested struct member spellings must still be checked.
func TestTopLevelTypedMapResponseAliases(t *testing.T) {
	type node struct {
		ID string `json:"id"`
	}
	type nested struct {
		Items []*node `json:"items"`
	}
	tests := []struct {
		name string
		wire string
		bad  bool
	}{
		{"single_noncanonical", `{"a":{"ID":"bad"}}`, true},
		{"single_canonical", `{"a":{"id":"okay"}}`, false},
		{"slice_descendant_noncanonical", `{"a":{"items":[{"Id":"bad"}]}}`, true},
		{"slice_descendant_canonical", `{"a":{"items":[{"id":"okay"}]}}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prior := map[string]nested{"keep": {Items: []*node{{ID: "previous"}}}}
			if tc.name == "single_noncanonical" || tc.name == "single_canonical" {
				old := map[string]node{"keep": {ID: "previous"}}
				err := decodeResponse(context.Background(), []byte(tc.wire), &old)
				if (err != nil) != tc.bad {
					t.Fatalf("expected reject=%t, error=%v", tc.bad, err)
				}
				if tc.bad && !reflect.DeepEqual(old, map[string]node{"keep": {ID: "previous"}}) {
					t.Fatalf("failed decode mutated output: %#v", old)
				}
				if !tc.bad && old["a"].ID != "okay" {
					t.Fatalf("canonical typed map not decoded: %#v", old)
				}
				return
			}
			err := decodeResponse(context.Background(), []byte(tc.wire), &prior)
			if (err != nil) != tc.bad {
				t.Fatalf("expected reject=%t, error=%v", tc.bad, err)
			}
			if tc.bad && !reflect.DeepEqual(prior, map[string]nested{"keep": {Items: []*node{{ID: "previous"}}}}) {
				t.Fatalf("failed decode mutated output: %#v", prior)
			}
			if !tc.bad && (len(prior["a"].Items) != 1 || prior["a"].Items[0].ID != "okay") {
				t.Fatalf("canonical array descendant not decoded: %#v", prior)
			}
		})
	}

	// An opaque map keeps JSON's case-sensitive key semantics.
	var opaque map[string]any
	if err := decodeResponse(context.Background(), []byte(`{"Foo":1,"foo":2}`), &opaque); err != nil || len(opaque) != 2 {
		t.Fatalf("untyped map keys incorrectly rejected: %#v err=%v", opaque, err)
	}
}
