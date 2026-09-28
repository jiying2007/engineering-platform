package strictjson

import (
	"strings"
	"testing"
)

func TestForeignObjectAllowsExternalKeyNamesButRejectsAmbiguity(t *testing.T) {
	for _, data := range []string{
		`{"auth_mode":"chatgpt","OPENAI_API_KEY":null,"tokens":{"id_token":"id","access_token":"access","refresh_token":"refresh","account_id":"acct"},"last_refresh":"2026-09-28T00:00:00Z"}`,
		`{"camelCase":true,"UPPER_CASE":"ok"}`,
	} {
		if err := ValidateForeignObject([]byte(data)); err != nil {
			t.Fatalf("foreign object rejected: %v", err)
		}
	}

	for _, data := range []string{
		`{"OPENAI_API_KEY":"a","OPENAI_API_KEY":"b"}`,
		`{"OPENAI_API_KEY":"a","OPENAI_API_K\u0045Y":"b"}`,
		`[]`,
		`{"x":1} trailing`,
	} {
		if err := ValidateForeignObject([]byte(data)); err == nil {
			t.Fatalf("ambiguous foreign object accepted: %s", data)
		}
	}

	if err := ValidateObject([]byte(`{"OPENAI_API_KEY":null}`)); err == nil {
		t.Fatal("internal wire validator accepted external uppercase key")
	}
}

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
