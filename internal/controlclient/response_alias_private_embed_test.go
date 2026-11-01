package controlclient

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

type privateTaggedIdentity struct {
	ID string `json:"id"`
}
type exportedOuterWithPrivateEmbedded struct {
	privateTaggedIdentity `json:"node"`
}
type exportedPlainPrivateEmbedded struct{ privateTaggedIdentity }

func TestTaggedUnexportedEmbeddedStructAliases(t *testing.T) {
	cases := []struct {
		name, wire string
		reject     bool
		expected   string
	}{
		{"alias", `{"node":{"ID":"overwritten"}}`, true, ""},
		{"duplicate_fold", `{"node":{"id":"first","Id":"overwritten"}}`, true, ""},
		{"canonical", `{"node":{"id":"correct"}}`, false, "correct"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var oracle exportedOuterWithPrivateEmbedded
			if err := json.Unmarshal([]byte(tc.wire), &oracle); err != nil {
				t.Fatal(err)
			}
			if oracle.ID == "" {
				t.Fatal("fixture does not reproduce standard library decoded identity")
			}
			out := exportedOuterWithPrivateEmbedded{privateTaggedIdentity: privateTaggedIdentity{ID: "retained"}}
			err := decodeResponse(context.Background(), []byte(tc.wire), &out)
			if (err != nil) != tc.reject {
				t.Fatalf("wantReject=%v err=%v", tc.reject, err)
			}
			if tc.reject && out.ID != "retained" {
				t.Fatalf("rejected response modified output: %+v", out)
			}
			if !tc.reject && out.ID != tc.expected {
				t.Fatalf("good response: %+v", out)
			}
		})
	}
	var nested map[string]exportedOuterWithPrivateEmbedded
	wire := []byte(`{"a":{"node":{"ID":"bad"}}}`)
	oracle := map[string]exportedOuterWithPrivateEmbedded{}
	if err := json.Unmarshal(wire, &oracle); err != nil || oracle["a"].ID != "bad" {
		t.Fatal("bad fixture", err, oracle)
	}
	if err := decodeResponse(context.Background(), wire, &nested); err == nil {
		t.Fatal("nested typed map alias accepted")
	}
	// Keep untagged anonymous promotion and exact-case naming unchanged.
	var plain exportedPlainPrivateEmbedded
	if err := decodeResponse(context.Background(), []byte(`{"id":"good"}`), &plain); err != nil || plain.ID != "good" {
		t.Fatal("untagged promotion changed", err, plain)
	}
	if reflect.TypeOf(exportedOuterWithPrivateEmbedded{}).Field(0).IsExported() {
		t.Fatal("fixture unexpectedly has exported embedded field")
	}
}
