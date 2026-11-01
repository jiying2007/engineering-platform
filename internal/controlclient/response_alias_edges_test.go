package controlclient

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

type responseAliasNode struct {
	ID string `json:"id"`
}

type responseAliasEmbed struct{ responseAliasNode }

type responseAliasDual struct {
	Lower string `json:"id"`
	Upper string `json:"ID"`
}

func TestResponseTypedAliasEdges(t *testing.T) {
	cases := []struct {
		name, body string
		target     any
		reject     bool
	}{
		{"map_value_case_alias", `{"records":{"a":{"ID":"bad"}}}`, &struct {
			Records map[string]responseAliasNode `json:"records"`
		}{}, true},
		{"map_value_correct", `{"records":{"a":{"id":"okay"}}}`, &struct {
			Records map[string]responseAliasNode `json:"records"`
		}{}, false},
		{"embedded_case_alias", `{"node":{"ID":"bad"}}`, &struct {
			Node responseAliasEmbed `json:"node"`
		}{}, true},
		{"embedded_correct", `{"node":{"id":"okay"}}`, &struct {
			Node responseAliasEmbed `json:"node"`
		}{}, false},
		{"nested_slice_case_alias", `{"items":[[{"ID":"bad"}]]}`, &struct {
			Items [][]*responseAliasNode `json:"items"`
		}{}, true},
		{"nested_slice_correct", `{"items":[[{"id":"okay"}]]}`, &struct {
			Items [][]*responseAliasNode `json:"items"`
		}{}, false},
		{"two_canonical_nested_fields", `{"node":{"id":"lower","ID":"upper"}}`, &struct {
			Node responseAliasDual `json:"node"`
		}{}, false},
		{"folded_noncanonical_nested_field", `{"node":{"id":"lower","Id":"bad"}}`, &struct {
			Node responseAliasDual `json:"node"`
		}{}, true},
		{"two_canonical_top_fields", `{"id":"lower","ID":"upper"}`, &responseAliasDual{}, false},
		{"folded_noncanonical_top_field", `{"id":"lower","Id":"bad"}`, &responseAliasDual{}, true},
		{"opaque_map_case_distinct_keys", `{"opaque":{"Foo":"one","foo":"two"}}`, &struct {
			Opaque map[string]string `json:"opaque"`
		}{}, false},
		{"raw_message_case_distinct_keys", `{"raw":{"Foo":"one","foo":"two"}}`, &struct {
			Raw json.RawMessage `json:"raw"`
		}{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := reflect.ValueOf(tc.target).Elem().Interface()
			err := decodeResponse(context.Background(), []byte(tc.body), tc.target)
			if (err != nil) != tc.reject {
				t.Fatalf("unexpected result: rejected=%v, err=%v", tc.reject, err)
			}
			if tc.reject && !reflect.DeepEqual(reflect.ValueOf(tc.target).Elem().Interface(), before) {
				t.Fatal("failed receipt contaminated output")
			}
		})
	}
}
