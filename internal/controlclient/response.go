package controlclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
)

func responseDestination(out any) (reflect.Value, error) {
	v := reflect.ValueOf(out)
	if !v.IsValid() || v.Kind() != reflect.Pointer || v.IsNil() || !v.Elem().CanSet() {
		return reflect.Value{}, fmt.Errorf("non-nil writable response pointer required")
	}
	return v.Elem(), nil
}

// Decode into a new value, not a shallow copy: a map/slice/pointer in the
// caller's previous receipt must not be mutated by a failed decode, nor may
// omitted members silently retain data from an earlier operation.
func decodeResponse(data []byte, out any) error {
	dst, err := responseDestination(out)
	if err != nil {
		return err
	}
	temporary := reflect.New(dst.Type())
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(temporary.Interface()); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing control JSON")
	}
	if err := requireResponseMembers(data, dst.Type()); err != nil {
		return err
	}
	dst.Set(temporary.Elem())
	return nil
}

// Explicit non-omitempty top-level wire fields are required to be present.
// A nullable member such as {"assignment":null} remains valid, but {} is not
// an empty claim receipt. Domain identity/digest checks remain with consumers;
// transport validation never grants execution authority.
func requireResponseMembers(data []byte, typ reflect.Type) error {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return nil
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(data, &members); err != nil || members == nil {
		return fmt.Errorf("response object required")
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() || field.Anonymous {
			continue
		}
		tag, tagged := field.Tag.Lookup("json")
		if !tagged || tag == "-" {
			continue
		}
		parts := strings.Split(tag, ",")
		name := parts[0]
		if name == "" {
			name = field.Name
		}
		optional := false
		for _, option := range parts[1:] {
			optional = optional || option == "omitempty" || option == "omitzero"
		}
		raw, present := members[name]
		if !present && !optional {
			return fmt.Errorf("required response member missing")
		}
		for key := range members {
			if key != name && strings.EqualFold(key, name) {
				return fmt.Errorf("response member case alias rejected")
			}
		}
		if present && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			switch field.Type.Kind() {
			case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface:
			default:
				return fmt.Errorf("non-null response member required")
			}
		}
	}
	return nil
}
