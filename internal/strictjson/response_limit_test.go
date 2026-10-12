package strictjson

import (
	"strings"
	"testing"
)

func TestExplicitResponseBudgetDoesNotRelaxRequestBudget(t *testing.T) {
	data := []byte(`{"body":"` + strings.Repeat("x", MaxBytes) + `"}`)
	if ValidateForeignObject(data) == nil || ValidateObject(data) == nil {
		t.Fatal("request byte budget was relaxed")
	}
	if err := ValidateForeignObjectLimit(data, 4*MaxBytes); err != nil {
		t.Fatal("explicit response budget rejected", err)
	}
	for _, limit := range []int{-1, 0, len(data) - 1} {
		if ValidateForeignObjectLimit(data, limit) == nil {
			t.Fatal("invalid response limit accepted", limit)
		}
	}
	if ValidateForeignObjectLimit([]byte(`{"a":1,"a":2}`), 4*MaxBytes) == nil {
		t.Fatal("response byte budget bypassed duplicate guard")
	}
}
