package main

import (
	"strings"
	"testing"
)

func TestCaptureExecutionCLIHasNoAuthorityOrImplicitPaths(t *testing.T) {
	good := []string{"--records", "/private/records", "--run", "run", "--execution", strings.Repeat("a", 64), "--permit-digest", "sha256:" + strings.Repeat("b", 64), "--context", "/private/context", "--runtime-binary", "/private/runtime/codex", "--qualification-receipt", "/private/runtime/qualification.json", "--out", "/private/retained/set.tar"}
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
	for _, item := range []struct{ flag, value string }{
		{"--context", "relative"}, {"--runtime-binary", "/private/../codex"},
		{"--qualification-receipt", "/private/runtime/qualification.json/"}, {"--out", "relative"},
	} {
		args := append([]string{}, good...)
		for i := 0; i < len(args)-1; i += 2 {
			if args[i] == item.flag {
				args[i+1] = item.value
			}
		}
		if _, err := parseExecutionCapture(args); err == nil {
			t.Fatal("noncanonical path accepted", item.flag, item.value)
		}
	}
}
