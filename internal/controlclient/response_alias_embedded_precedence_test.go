package controlclient

import (
	"context"
	"encoding/json"
	"testing"
)

type deepPrivateLeaf struct {
	Data string `json:"data"`
}
type deepPrivateWrapper struct{ deepPrivateLeaf }
type closePrivateData struct {
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}
type layeredPrivateEnvelope struct {
	deepPrivateWrapper
	closePrivateData
}
type untaggedFirstLeaf struct{ Data string }
type taggedSecondLeaf struct {
	Data struct {
		ID string `json:"id"`
	} `json:"Data"`
}
type taggedPriorityEnvelope struct {
	untaggedFirstLeaf
	taggedSecondLeaf
}

func TestTypedResponseEmbeddedFieldPriority(t *testing.T) {
	cases := []struct {
		name, wire string
		out        any
		canonical  bool
	}{
		{"shallow_over_deep_alias", `{"data":{"ID":"bad"}}`, &layeredPrivateEnvelope{}, false},
		{"shallow_over_deep_exact", `{"data":{"id":"good"}}`, &layeredPrivateEnvelope{}, true},
		{"tagged_over_untagged_alias", `{"Data":{"ID":"bad"}}`, &taggedPriorityEnvelope{}, false},
		{"tagged_over_untagged_exact", `{"Data":{"id":"good"}}`, &taggedPriorityEnvelope{}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tc.wire), tc.out); err != nil {
				t.Fatal("bad fixture", err)
			}
			switch v := tc.out.(type) {
			case *layeredPrivateEnvelope:
				if v.closePrivateData.Data.ID == "" {
					t.Fatal("expected decoded shallow value")
				}
			case *taggedPriorityEnvelope:
				if v.taggedSecondLeaf.Data.ID == "" {
					t.Fatal("expected decoded tagged value")
				}
			}
			switch v := tc.out.(type) {
			case *layeredPrivateEnvelope:
				*v = layeredPrivateEnvelope{}
			case *taggedPriorityEnvelope:
				*v = taggedPriorityEnvelope{}
			}
			err := decodeResponse(context.Background(), []byte(tc.wire), tc.out)
			if tc.canonical && err != nil {
				t.Fatal("canonical rejected", err)
			}
			if !tc.canonical && err == nil {
				t.Fatal("folded nested alias accepted")
			}
		})
	}
}
