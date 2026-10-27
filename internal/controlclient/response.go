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
	canonical := responseTypedFields(typ)
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
				if _, exact := canonical[key]; !exact {
					return fmt.Errorf("response member case alias rejected")
				}
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

var responseJSONUnmarshaler = reflect.TypeOf((*json.Unmarshaler)(nil)).Elem()

func responseUsesJSONUnmarshaler(t reflect.Type) bool {
	if t.Implements(responseJSONUnmarshaler) {
		return true
	}
	return t.Kind() != reflect.Pointer && reflect.PointerTo(t).Implements(responseJSONUnmarshaler)
}

// responseTypedFields tracks exact JSON spellings, with direct fields taking
// priority over anonymously embedded fields. We only need the canonical field
// names for rejecting case-fold aliases; the decoder retains final field
// selection/ambiguity authority.
func responseTypedFields(t reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type)
	visiting := make(map[reflect.Type]bool)
	var collect func(reflect.Type)
	collect = func(current reflect.Type) {
		for current.Kind() == reflect.Pointer {
			current = current.Elem()
		}
		if current.Kind() != reflect.Struct || visiting[current] {
			return
		}
		visiting[current] = true
		defer delete(visiting, current)
		var promoted []reflect.Type
		for i := 0; i < current.NumField(); i++ {
			field := current.Field(i)
			tag := field.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name := strings.Split(tag, ",")[0]
			if field.Anonymous && name == "" && !responseUsesJSONUnmarshaler(field.Type) {
				embedded := field.Type
				for embedded.Kind() == reflect.Pointer {
					embedded = embedded.Elem()
				}
				if embedded.Kind() == reflect.Struct {
					promoted = append(promoted, embedded)
					continue
				}
			}
			if !field.IsExported() {
				continue
			}
			if name == "" {
				name = field.Name
			}
			if _, present := fields[name]; !present {
				fields[name] = field.Type
			}
		}
		for _, embedded := range promoted {
			collect(embedded)
		}
	}
	collect(t)
	return fields
}

// Case-folded keys may overwrite typed JSON fields. Walk the already-bounded
// JSON only once, preserving case-sensitive map keys and custom decoder types.
func rejectTypedResponseAliases(data []byte, typ reflect.Type) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	cache := make(map[reflect.Type]map[string]reflect.Type)
	if err := walkTypedResponseAliases(decoder, typ, cache, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("trailing typed response JSON")
	}
	return nil
}

func walkTypedResponseAliases(d *json.Decoder, typ reflect.Type, cache map[reflect.Type]map[string]reflect.Type, depth int) error {
	if depth > 64 {
		return fmt.Errorf("typed response nesting limit exceeded")
	}
	for typ != nil {
		if responseUsesJSONUnmarshaler(typ) {
			typ = nil // caller-owned custom JSON representation, not typed fields
			break
		}
		if typ.Kind() != reflect.Pointer {
			break
		}
		typ = typ.Elem()
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		var fields map[string]reflect.Type
		var element reflect.Type
		if typ != nil {
			switch typ.Kind() {
			case reflect.Struct:
				fields = cache[typ]
				if fields == nil {
					fields = responseTypedFields(typ)
					cache[typ] = fields
				}
			case reflect.Map:
				element = typ.Elem()
			}
		}
		for d.More() {
			keyToken, err := d.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("typed response object key required")
			}
			child := element
			if fields != nil {
				child = fields[key]
				if child == nil {
					for canonical := range fields {
						if strings.EqualFold(key, canonical) {
							return fmt.Errorf("typed response member case alias rejected")
						}
					}
				}
			}
			if err := walkTypedResponseAliases(d, child, cache, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return fmt.Errorf("invalid typed response object")
		}
	case '[':
		var element reflect.Type
		if typ != nil && (typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array) {
			element = typ.Elem()
		}
		for d.More() {
			if err := walkTypedResponseAliases(d, element, cache, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return fmt.Errorf("invalid typed response array")
		}
	default:
		return fmt.Errorf("unexpected typed response delimiter")
	}
	return nil
}
