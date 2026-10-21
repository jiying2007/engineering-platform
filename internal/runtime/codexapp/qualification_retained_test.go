package codexapp

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// This digest freezes the schema-v3 field order at a8557646. It must not be
// regenerated from the current QualificationReceipt struct when v4 changes.
const retainedV3GoldenDigest = "sha256:06b7ec9f14df3ea06d1ba60619cba96d2a1ffccd7c90fc1fd47e4605b4c7783c"

func TestRetainedQualificationV3KeepsOriginalDigestWithoutAdmission(t *testing.T) {
	raw, err := os.ReadFile("testdata/qualification_v3.json")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := VerifyRetainedQualification(raw, retainedV3GoldenDigest)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.SchemaVersion != 3 || receipt.FilesystemReadBoundary != "" || receipt.FilesystemReadIsolationQualified {
		t.Fatalf("historical evidence acquired new semantics: %+v", receipt)
	}
	if receipt.Validate() == nil {
		t.Fatal("historical evidence admitted as a new qualification")
	}
	if _, err := receipt.Digest(); err == nil {
		t.Fatal("historical evidence acquired a current qualification digest")
	}
	if _, err := MarshalQualification(receipt); err == nil {
		t.Fatal("historical evidence reissued as current qualification")
	}
	if VerifyExecutableQualification(context.Background(), "/must-not-start", receipt.ThreadStartModel, receipt) == nil {
		t.Fatal("historical evidence admitted to executable verification")
	}

	// A newly issued v4 receipt has its own digest, and cannot be substituted for
	// the original receipt merely because its binary/profile identity matches.
	receipt.SchemaVersion = QualificationSchemaVersion
	receipt.FilesystemReadBoundary = FilesystemReadBoundaryHostUID
	current, err := MarshalQualification(receipt)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := receipt.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyRetainedQualification(current, digest); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyRetainedQualification(current, retainedV3GoldenDigest); err == nil {
		t.Fatal("v4 qualification replaced frozen v3 evidence")
	}
}

func TestRetainedQualificationRejectsUnboundOrInventedV3Evidence(t *testing.T) {
	raw, err := os.ReadFile("testdata/qualification_v3.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"", "sha256:" + strings.Repeat("0", 64)} {
		if _, err := VerifyRetainedQualification(raw, expected); err == nil {
			t.Fatal("unbound or mismatched historical qualification accepted")
		}
	}
	for name, mutate := range map[string]func(map[string]any){
		"new-boundary":       func(v map[string]any) { v["filesystem_read_boundary"] = FilesystemReadBoundaryHostUID },
		"new-false-claim":    func(v map[string]any) { v["filesystem_read_isolation_qualified"] = false },
		"new-true-claim":     func(v map[string]any) { v["filesystem_read_isolation_qualified"] = true },
		"unknown":            func(v map[string]any) { v["unexpected_field"] = false },
		"missing-zero-value": func(v map[string]any) { delete(v, "credential_used") },
		"null-zero-value":    func(v map[string]any) { v["credential_used"] = nil },
		"other-schema":       func(v map[string]any) { v["schema_version"] = 2 },
		"changed-identity":   func(v map[string]any) { v["version"] = "0.157.2" },
	} {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal(raw, &value); err != nil {
				t.Fatal(err)
			}
			mutate(value)
			changed, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyRetainedQualification(changed, retainedV3GoldenDigest); err == nil {
				t.Fatal("altered historical evidence accepted")
			}
		})
	}
}
