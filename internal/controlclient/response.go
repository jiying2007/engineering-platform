package controlclient

import (
	"bytes"
	"context"
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
func decodeResponse(ctx context.Context, data []byte, out any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dst, err := responseDestination(out)
	if err != nil {
		return err
	}
	temporary := reflect.New(dst.Type())
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(temporary.Interface()); err != nil {
		return errors.Join(ctx.Err(), err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.Join(ctx.Err(), fmt.Errorf("trailing control JSON"))
	}
	if err := requireResponseMembers(data, dst.Type()); err != nil {
		return errors.Join(ctx.Err(), err)
	}
	if err := ctx.Err(); err != nil {
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
	return rejectTypedResponseAliases(data, typ)
}

// rejectTypedResponseAliases rejects JSON spellings which encoding/json would
// case-fold onto a typed struct member. A differently cased nested identity may
// otherwise overwrite an earlier value even after strict duplicate-key checks.
// Opaque map keys and types with their own UnmarshalJSON contract remain intact;
// nested field presence and domain authority are validated by their consumers.
var responseJSONUnmarshaler = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()

func rejectTypedResponseAliases(data []byte, typ reflect.Type) error {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Implements(responseJSONUnmarshaler) || reflect.PointerTo(typ).Implements(responseJSONUnmarshaler) {
		return nil
	}
	switch typ.Kind() {
	case reflect.Slice, reflect.Array:
		element := typ.Elem()
		for element.Kind() == reflect.Pointer || element.Kind() == reflect.Slice || element.Kind() == reflect.Array {
			element = element.Elem()
		}
		if element.Kind() != reflect.Struct {
			return nil
		}
		trimmed := bytes.TrimSpace(data)
		if len(trimmed) == 0 || trimmed[0] != '[' {
			return nil
		}
		var items []json.RawMessage
		if err := json.Unmarshal(data, &items); err != nil {
			return err
		}
		for _, item := range items {
			if err := rejectTypedResponseAliases(item, typ.Elem()); err != nil {
				return err
			}
		}
	case reflect.Struct:
		trimmed := bytes.TrimSpace(data)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			return nil
		}
		var members map[string]json.RawMessage
		if err := json.Unmarshal(data, &members); err != nil {
			return err
		}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if !field.IsExported() {
				continue
			}
			tag := field.Tag.Get("json")
			name := strings.Split(tag, ",")[0]
			if name == "-" || field.Anonymous && name == "" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			for key := range members {
				if key != name && strings.EqualFold(key, name) {
					return fmt.Errorf("typed response member case alias rejected")
				}
			}
			if value, present := members[name]; present {
				if err := rejectTypedResponseAliases(value, field.Type); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
