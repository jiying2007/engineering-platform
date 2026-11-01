package controlclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"
)

// These are transport-level negative probes, not proof of a remote mutation.
// encoding/json ordinarily accepts nested "ID" as "id", including when both
// spellings occur; the typed Control client must not accept either alias.
func TestControlRejectsNestedTypedResponseAliases(t *testing.T) {
	p := newPKI(t)
	var calls atomic.Int32
	var reply string
	s := testServer(t, p, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, reply)
	}))
	c, err := New(s.URL, p.clientTLS())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	type nested struct {
		ID string `json:"id"`
	}
	type receipt struct {
		Details nested            `json:"details"`
		Entries []nested          `json:"entries,omitempty"`
		Opaque  map[string]string `json:"opaque,omitempty"`
	}
	for _, tc := range []struct {
		name string
		body string
	}{
		{"nested duplicate alias", `{"details":{"id":"original","ID":"overwritten"}}`},
		{"nested sole alias", `{"details":{"ID":"overwritten"}}`},
		{"array duplicate alias", `{"details":{"id":"correct"},"entries":[{"id":"original","Id":"overwritten"}]}`},
		{"array sole alias", `{"details":{"id":"correct"},"entries":[{"Id":"overwritten"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reply = tc.body
			out := receipt{
				Details: nested{ID: "retained"},
				Entries: []nested{{ID: "retained"}},
				Opaque:  map[string]string{"keep": "original"},
			}
			before := calls.Load()
			err := c.Call(context.Background(), http.MethodPost, "/api/v1/work-items", struct{}{}, &out)
			if !errors.Is(err, ErrPostOutcomeUnobserved) || calls.Load() != before+1 {
				t.Fatalf("accepted/retried nested identity alias: %v", err)
			}
			if out.Details.ID != "retained" || len(out.Entries) != 1 ||
				out.Entries[0].ID != "retained" ||
				!reflect.DeepEqual(out.Opaque, map[string]string{"keep": "original"}) {
				t.Fatalf("failed decode changed original receipt: %+v", out)
			}
		})
	}

	// A user-owned opaque map is not a typed authority struct. Its distinct
	// case-sensitive keys remain legal; canonical nested fields still decode.
	reply = `{"details":{"id":"correct"},"entries":[{"id":"correct"}],"opaque":{"Foo":"one","foo":"two"}}`
	var out receipt
	if err := c.Call(context.Background(), http.MethodPost, "/api/v1/work-items", struct{}{}, &out); err != nil ||
		out.Details.ID != "correct" || len(out.Entries) != 1 || out.Entries[0].ID != "correct" ||
		!reflect.DeepEqual(out.Opaque, map[string]string{"Foo": "one", "foo": "two"}) {
		t.Fatalf("canonical/opaque response rejected: %+v %v", out, err)
	}
}
