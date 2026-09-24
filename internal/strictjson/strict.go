// Package strictjson rejects ambiguous wire names before authorization and decoding.
package strictjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const MaxBytes = 1 << 20

// ValidateObject requires one bounded JSON object, unique lower_snake_case keys
// at every depth and valid UTF-8. This prevents encoding/json's case-insensitive
// struct matching or duplicate-key merging from changing authorization meaning.
func ValidateObject(data []byte) error {
	if len(data) == 0 || len(data) > MaxBytes || !utf8.Valid(data) {
		return fmt.Errorf("invalid JSON size or encoding")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	first, err := d.Token()
	if err != nil || first != json.Delim('{') {
		return fmt.Errorf("JSON object required")
	}
	if err := object(d, 1); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}

func object(d *json.Decoder, depth int) error {
	seen := map[string]bool{}
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok || !validKey(key) || seen[key] {
			return fmt.Errorf("invalid or duplicate JSON member")
		}
		seen[key] = true
		if err := value(d, depth+1); err != nil {
			return err
		}
	}
	end, err := d.Token()
	if err != nil || end != json.Delim('}') {
		return fmt.Errorf("invalid JSON object")
	}
	return nil
}

func value(d *json.Decoder, depth int) error {
	if depth > 64 {
		return fmt.Errorf("JSON nesting limit exceeded")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		return object(d, depth)
	case '[':
		for d.More() {
			if err := value(d, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return fmt.Errorf("invalid JSON array")
		}
		return nil
	default:
		return fmt.Errorf("unexpected JSON delimiter")
	}
}

func validKey(s string) bool {
	if len(s) == 0 || len(s) > 128 || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for _, ch := range s {
		if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '_') {
			return false
		}
	}
	return true
}

func Decode(data []byte, dst any) error {
	if err := ValidateObject(data); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	return d.Decode(dst)
}

// ReadObject bounds the read before allocating a request/configuration body.
func ReadObject(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if err := ValidateObject(data); err != nil {
		return nil, err
	}
	return data, nil
}

// NonBlank avoids treating whitespace-only human attestation as an explanation.
func NonBlank(s string) bool { return strings.TrimSpace(s) != "" }
