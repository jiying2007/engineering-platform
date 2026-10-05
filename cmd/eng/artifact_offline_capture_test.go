package main

import (
	"strings"
	"testing"
)

func TestOfflineCaptureCLIRequiresAnchorsAndNoAuthority(t *testing.T) {
	good := []string{"--records", "/private/records", "--run", "run", "--execution", strings.Repeat("a", 64), "--permit-digest", "sha256:" + strings.Repeat("b", 64), "--report-digest", "sha256:" + strings.Repeat("c", 64), "--out", "/private/store/build.tar"}
	if _, err := parseOfflineCapture(good); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(good); i += 2 {
		args := append([]string{}, good[:i]...)
		args = append(args, good[i+2:]...)
		if _, err := parseOfflineCapture(args); err == nil {
			t.Fatal("missing", good[i])
		}
	}
	for _, extra := range [][]string{{"--core"}, {"--execute"}, {"--identity", "owner"}, {"--restore"}, {"--run=other"}, {"--report-digest=x"}, {"extra"}} {
		args := append(append([]string{}, good...), extra...)
		if _, err := parseOfflineCapture(args); err == nil {
			t.Fatal("accepted", extra)
		}
	}
	for i := 1; i < len(good); i += 2 {
		args := append([]string{}, good...)
		args[i] = "../other"
		if _, err := parseOfflineCapture(args); err == nil {
			t.Fatal("invalid value", good[i-1])
		}
	}
	args := append([]string{}, good...)
	args[len(args)-1] = "/private/records/output.tar"
	if _, err := parseOfflineCapture(args); err == nil {
		t.Fatal("overlap accepted")
	}
}
