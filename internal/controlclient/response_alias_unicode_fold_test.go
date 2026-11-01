package controlclient

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode"
)

// The transport's typed case-alias rejection must agree with actual
// encoding/json field matching, including Unicode simple folds. These are
// deterministic samples across Go's Unicode letter/digit tables, not a claim
// that every Unicode character or custom UnmarshalJSON implementation is covered.
func TestTypedResponseUnicodeFoldDifferential(t *testing.T) {
	type pair struct{ tag, key string }
	var cases []pair
	for r := rune(0x80); r < 0x110000; r++ {
		if (!unicode.IsLetter(r) && !unicode.IsDigit(r)) || r%67 != 0 {
			continue
		}
		tag := "a" + string(r) + "z"
		variants := []string{
			"A" + string(r) + "z",
			"a" + string(unicode.ToUpper(r)) + "Z",
			"a" + string(unicode.ToLower(r)) + "z",
			"a" + string(unicode.ToTitle(r)) + "Z",
		}
		for _, key := range variants {
			if key != tag {
				cases = append(cases, pair{tag, key})
			}
		}
	}
	if len(cases) < 6000 {
		t.Fatalf("insufficient Unicode differential coverage: %d", len(cases))
	}
	for _, tc := range cases {
		kind := reflect.StructOf([]reflect.StructField{{
			Name: "Field", Type: reflect.TypeOf(""),
			Tag: reflect.StructTag(fmt.Sprintf("json:%q", tc.tag)),
		}})
		wire, err := json.Marshal(map[string]string{tc.key: "MARKER"})
		if err != nil {
			t.Fatal(err)
		}
		value := reflect.New(kind)
		if err := json.Unmarshal(wire, value.Interface()); err != nil {
			t.Fatal(err)
		}
		assigned := value.Elem().Field(0).String() == "MARKER"
		guardErr := rejectTypedResponseAliases(wire, kind)
		if assigned && guardErr == nil {
			t.Fatalf("accepted Unicode field alias: tag=%q key=%q", tc.tag, tc.key)
		}
		if !assigned && guardErr != nil {
			t.Fatalf("rejected a nonmatching Unicode key: tag=%q key=%q: %v", tc.tag, tc.key, guardErr)
		}
		if assigned && !strings.EqualFold(tc.tag, tc.key) {
			t.Fatalf("stdlib folds but Go EqualFold does not: tag=%q key=%q", tc.tag, tc.key)
		}
	}
}
