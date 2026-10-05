package main

import (
	"strings"
	"testing"
)

func TestCaptureExecutionCLIHasNoAuthorityOrImplicitPaths(t *testing.T) {
	good := []string{"--records", "/private/records", "--run", "run", "--execution", strings.Repeat("a", 64), "--permit-digest", "sha256:" + strings.Repeat("b", 64), "--context", "/private/context", "--out", "/private/retained/set.tar"}
	if _, err := parseExecutionCapture(good); err != nil {
		t.Fatal(err)
	}
	for _, extra := range [][]string{{"--core"}, {"--execute"}, {"--restore"}, {"--identity", "owner"}, {"--context", "/other"}, {"--run=other"}, {"--out=/other"}, {"extra"}} {
		args := append(append([]string{}, good...), extra...)
		if _, err := parseExecutionCapture(args); err == nil {
			t.Fatalf("accepted %q", args)
		}
	}
	for i := 0; i < len(good); i += 2 {
		args := append([]string{}, good[:i]...)
		args = append(args, good[i+2:]...)
		if _, err := parseExecutionCapture(args); err == nil {
			t.Fatalf("missing required %s accepted", good[i])
		}
	}
	for _, p := range []string{"relative", "/private/../context", "/private/context/"} {
		args := append([]string{}, good...)
		args[9] = p
		if _, err := parseExecutionCapture(args); err == nil {
			t.Fatal("noncanonical path accepted", p)
		}
	}
}
