package controlclient

import (
	"context"
	"encoding/json"
	"testing"
)

type responseInvalidTagInner struct {
	ID string `json:"🐈"`
}
type responseInvalidTagOuter struct {
	Node responseInvalidTagInner `json:"node"`
}

type responseInvalidTagEmbedded struct{ responseInvalidTagInner }

func TestResponseInvalidTagNameFallsBackToExactField(t *testing.T) {
	cases := []struct {
		name, body string
		out        any
		wantFail   bool
	}{
		{"nested alias", `{"node":{"iD":"bad"}}`, &responseInvalidTagOuter{}, true},
		{"nested exact", `{"node":{"ID":"good"}}`, &responseInvalidTagOuter{}, false},
		{"embedded alias", `{"iD":"bad"}`, &responseInvalidTagEmbedded{}, true},
		{"embedded exact", `{"ID":"good"}`, &responseInvalidTagEmbedded{}, false},
		{"top-level alias", `{"iD":"bad"}`, &responseInvalidTagInner{}, true},
		{"top-level exact", `{"ID":"good"}`, &responseInvalidTagInner{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw any
			if err := json.Unmarshal([]byte(tc.body), &raw); err != nil {
				t.Fatal(err)
			}
			err := decodeResponse(context.Background(), []byte(tc.body), tc.out)
			if (err != nil) != tc.wantFail {
				t.Fatalf("wanted rejection=%v, got err=%v", tc.wantFail, err)
			}
		})
	}
}
