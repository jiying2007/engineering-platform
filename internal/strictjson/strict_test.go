package strictjson

import (
	"strings"
	"testing"
)

func TestWireAmbiguityRejected(t *testing.T) {
	for _, data := range []string{
		`null`, `[]`, `{} {}`, `{"actor":"a","actor":"b"}`,
		`{"actor":"a","ACTOR":"b"}`, `{"nested":{"issuer":"a","iss\u0075er":"b"}}`,
		`{"nested":[{"IsSuEr":"a"}]}`, `{"nested":{"bad-key":true}}`,
		string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}),
		`{"x":` + strings.Repeat(`[`, 65) + `0` + strings.Repeat(`]`, 65) + `}`,
		`{"x":"` + strings.Repeat("x", MaxBytes) + `"}`,
	} {
		if err := ValidateObject([]byte(data)); err == nil {
			t.Errorf("ambiguous input accepted (len=%d)", len(data))
		}
	}
	for _, data := range []string{`{}`, `{"actor":"urn:engineering-platform:engineer","nested":[{"n":1},true,null,"日志"]}`} {
		if err := ValidateObject([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	var target struct {
		Actor string `json:"actor"`
	}
	if err := Decode([]byte(`{"actor":"ok","unknown":1}`), &target); err == nil {
		t.Fatal("unknown schema member accepted")
	}
	if _, err := ReadObject(strings.NewReader(`{"actor":"ok"}`)); err != nil {
		t.Fatal(err)
	}
}
