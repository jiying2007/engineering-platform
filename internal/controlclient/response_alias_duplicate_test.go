package controlclient

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

type duplicateNestedItem struct {
	ID string `json:"id"`
}
type duplicateCustomJSON struct{ Last string }

// Custom unmarshallers own their own wire semantics; the transport must not
// reinterpret their interior fields as another typed response schema.
func (c *duplicateCustomJSON) UnmarshalJSON(data []byte) error {
	var fields map[string]string
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	c.Last = fields["id"]
	return nil
}

type duplicateResponseReceipt struct {
	ID     string `json:"id"`
	Status struct {
		OK bool `json:"ok"`
	} `json:"status"`
	Items  []duplicateNestedItem          `json:"items,omitempty"`
	Index  map[string]duplicateNestedItem `json:"index,omitempty"`
	Meta   map[string]string              `json:"meta,omitempty"`
	Custom duplicateCustomJSON            `json:"custom,omitempty"`
}

func TestDuplicateTypedResponseMemberRejected(t *testing.T) {
	cases := []struct{ name, wire string }{
		{"root", `{"id":"first","id":"overwritten","status":{"ok":true}}`},
		{"nested", `{"id":"safe","status":{"ok":false,"ok":true}}`},
		{"slice_element", `{"id":"safe","status":{"ok":true},"items":[{"id":"first","id":"last"}]}`},
		{"map_struct_element", `{"id":"safe","status":{"ok":true},"index":{"item":{"id":"first","id":"last"}}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var oracle duplicateResponseReceipt
			if err := json.Unmarshal([]byte(tc.wire), &oracle); err != nil {
				t.Fatal("bad standard-library fixture", err)
			}
			original := duplicateResponseReceipt{ID: "unmodified"}
			original.Status.OK = false
			out := original
			if err := decodeResponse(context.Background(), []byte(tc.wire), &out); err == nil {
				t.Fatalf("duplicate typed response name accepted: %#v", out)
			}
			if !reflect.DeepEqual(out, original) {
				t.Fatalf("failed decode mutated caller: before=%#v after=%#v", original, out)
			}
		})
	}
}

func TestDuplicateResponseMemberIndependentObjectAndMapSemantics(t *testing.T) {
	allowed := []struct{ name, wire string }{
		{"separate_slice_objects", `{"id":"ok","status":{"ok":true},"items":[{"id":"one"},{"id":"two"}]}`},
		{"case_sensitive_map", `{"id":"ok","status":{"ok":true},"meta":{"ID":"one","id":"two"}}`},
		{"map_duplicate_key_existing_contract", `{"id":"ok","status":{"ok":true},"meta":{"id":"one","id":"two"}}`},
		{"custom_unmarshal_boundary", `{"id":"ok","status":{"ok":true},"custom":{"id":"one","id":"two"}}`},
	}
	for _, tc := range allowed {
		t.Run(tc.name, func(t *testing.T) {
			var want, got duplicateResponseReceipt
			if err := json.Unmarshal([]byte(tc.wire), &want); err != nil {
				t.Fatal(err)
			}
			if err := decodeResponse(context.Background(), []byte(tc.wire), &got); err != nil {
				t.Fatalf("valid response rejected: %v", err)
			}
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("decoder semantics drift: want=%#v got=%#v", want, got)
			}
		})
	}
}

func TestDuplicateTypedResponseRootArrayMap(t *testing.T) {
	cases := []struct {
		name, wire string
		out        any
	}{
		{"root_array", `[{"id":"one","id":"two"}]`, new([]duplicateNestedItem)},
		{"root_map_struct", `{"x":{"id":"one","id":"two"}}`, new(map[string]duplicateNestedItem)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := decodeResponse(context.Background(), []byte(tc.wire), tc.out); err == nil {
				t.Fatal("root container duplicate typed key accepted")
			}
		})
	}
}
