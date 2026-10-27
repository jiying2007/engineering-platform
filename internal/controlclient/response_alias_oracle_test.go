package controlclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// Keep exact-case wire tags aligned with the Go JSON decoder across nested
// typed responses. This uses independent stdlib decoding as the oracle: when a
// noncanonical spelling actually assigns a typed field, the transport guard
// must reject it. A canonical spelling must remain admissible.
func TestTypedResponseAliasStdlibOracle(t *testing.T) {
	tags := []string{
		"id", "ID", "Id", "user_id", "HTTPURL", "foo", "Foo", "Key",
		"Kelvin", "k", "K", "İd", "Σ", "σ", "ς", "🐈", "a.b", "a b", "X-Y", "", "-", "x,invalid",
	}
	wrappers := []func(reflect.Type) reflect.Type{
		func(t reflect.Type) reflect.Type { return t },
		func(t reflect.Type) reflect.Type { return reflect.MapOf(reflect.TypeOf(""), t) },
		func(t reflect.Type) reflect.Type { return reflect.SliceOf(t) },
		func(t reflect.Type) reflect.Type {
			return reflect.MapOf(reflect.TypeOf(""), reflect.SliceOf(reflect.PointerTo(t)))
		},
	}
	count := 0
	for _, tag := range tags {
		base := reflect.StructOf([]reflect.StructField{{
			Name: "Field", Type: reflect.TypeOf(""), Tag: reflect.StructTag(fmt.Sprintf("json:%q", tag)),
		}})
		canonical := responseTypedFields(base)
		for shape, wrap := range wrappers {
			typ := wrap(base)
			keys := []string{"Field", "field", "FIELD", "FiElD", tag, strings.ToLower(tag), strings.ToUpper(tag), strings.ToTitle(tag)}
			for _, key := range keys {
				if key == "" || key == "-" {
					continue
				}
				leaf, err := json.Marshal(map[string]any{key: "MARKER"})
				if err != nil {
					t.Fatal(err)
				}
				wire := leaf
				switch shape {
				case 1:
					wire = append(append([]byte(`{"outer":`), leaf...), '}')
				case 2:
					wire = append(append([]byte{'['}, leaf...), ']')
				case 3:
					wire = append(append([]byte(`{"outer":[`), leaf...), ']', '}')
				}
				value := reflect.New(typ)
				if err := json.Unmarshal(wire, value.Interface()); err != nil {
					continue
				}
				decoded, err := json.Marshal(value.Interface())
				if err != nil {
					t.Fatal(err)
				}
				assigned := bytes.Contains(decoded, []byte("MARKER"))
				_, exact := canonical[key]
				guardErr := rejectTypedResponseAliases(wire, typ)
				count++
				if assigned && !exact && guardErr == nil {
					t.Errorf("accepted folded alias: tag=%q key=%q shape=%d wire=%s", tag, key, shape, wire)
				}
				if assigned && exact && guardErr != nil {
					t.Errorf("rejected canonical spelling: tag=%q key=%q shape=%d: %v", tag, key, shape, guardErr)
				}
			}
		}
	}
	if count < 600 {
		t.Fatalf("unexpectedly weak coverage: %d", count)
	}
}

// Two exact JSON tags are legal even when they case-fold to each other;
// noncanonical third spellings must not be silently assigned to either.
func TestTypedResponseAliasExactTagCollisionOracle(t *testing.T) {
	pairs := [][2]string{
		{"id", "ID"}, {"foo", "Foo"}, {"K", "k"}, {"K", "K"},
		{"σ", "ς"}, {"İd", "id"}, {"a b", "A B"}, {"a.b", "A.B"},
	}
	for _, pair := range pairs {
		typ := reflect.StructOf([]reflect.StructField{
			{Name: "First", Type: reflect.TypeOf(""), Tag: reflect.StructTag(fmt.Sprintf("json:%q", pair[0]))},
			{Name: "Second", Type: reflect.TypeOf(""), Tag: reflect.StructTag(fmt.Sprintf("json:%q", pair[1]))},
		})
		fields := responseTypedFields(typ)
		keys := []string{pair[0], pair[1], strings.ToUpper(pair[0]), strings.ToLower(pair[0]),
			strings.ToTitle(pair[0]), strings.ToUpper(pair[1]), strings.ToLower(pair[1]), strings.ToTitle(pair[1])}
		for _, key := range keys {
			wire, err := json.Marshal(map[string]any{key: "MARKER"})
			if err != nil {
				t.Fatal(err)
			}
			value := reflect.New(typ)
			if err := json.Unmarshal(wire, value.Interface()); err != nil {
				continue
			}
			decoded, err := json.Marshal(value.Interface())
			if err != nil {
				t.Fatal(err)
			}
			assigned := bytes.Contains(decoded, []byte("MARKER"))
			_, exact := fields[key]
			guardErr := rejectTypedResponseAliases(wire, typ)
			if assigned && !exact && guardErr == nil {
				t.Errorf("accepted alias: tags=%v key=%q", pair, key)
			}
			if assigned && exact && guardErr != nil {
				t.Errorf("rejected exact tag: tags=%v key=%q: %v", pair, key, guardErr)
			}
		}
	}
}
