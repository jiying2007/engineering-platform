package main

import "testing"

func TestInstallationReadbackCLIRequiresExactExternalIdentity(t *testing.T) {
	good := []string{"--dir", "/release", "--source-commit", "0123456789012345678901234567890123456789", "--manifest-digest", "sha256:0123456789012345678901234567890123456789012345678901234567890123"}
	if _, err := parseInstallationReadback(good); err != nil {
		t.Fatal(err)
	}
	for _, extra := range [][]string{{"extra"}, {"--execute"}, {"--dir=/other"}, {"--manifest-digest=sha256:other"}} {
		args := append(append([]string{}, good...), extra...)
		if _, err := parseInstallationReadback(args); err == nil {
			t.Fatal("ambiguous readback accepted", extra)
		}
	}
	for i := 0; i < len(good); i += 2 {
		args := append([]string{}, good[:i]...)
		args = append(args, good[i+2:]...)
		if _, err := parseInstallationReadback(args); err == nil {
			t.Fatal("missing readback identity accepted", good[i])
		}
	}
}
