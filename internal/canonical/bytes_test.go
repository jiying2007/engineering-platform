package canonical

import (
	"strings"
	"testing"
)

func TestBytesDigestIsNotJSONDigest(t *testing.T) {
	want := "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := BytesDigest([]byte("abc")); got != want {
		t.Fatalf("raw bytes digest: %s", got)
	}
	jsonDigest, err := Digest([]byte("abc"))
	if err != nil || jsonDigest == want {
		t.Fatalf("raw and JSON/base64 encodings must differ: %s %v", jsonDigest, err)
	}
	if !ValidDigest(want) {
		t.Fatal("valid lowercase SHA-256 rejected")
	}
	for _, bad := range []string{"", "abc", strings.ToUpper(want), want + "\n", want[:len(want)-1]} {
		if ValidDigest(bad) {
			t.Fatalf("invalid digest accepted: %q", bad)
		}
	}
}
