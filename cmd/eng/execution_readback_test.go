package main

import (
	"strings"
	"testing"
)

func TestExecutionReadbackCLIRejectsEffectAndAmbiguousFlags(t *testing.T) {
	good := []string{"--records", "/private/records", "--run", "run", "--execution", strings.Repeat("a", 64), "--permit-digest", "sha256:" + strings.Repeat("b", 64)}
	if _, err := parseExecutionReadback(good); err != nil {
		t.Fatal(err)
	}
	for _, extra := range [][]string{{"--core"}, {"--archive", "/private/source.tar"}, {"--bundle", "/private/result.bundle"}} {
		if _, err := parseExecutionReadback(append(append([]string{}, good...), extra...)); err != nil {
			t.Fatal(err)
		}
	}
	bad := [][]string{nil, {"--run", "run"}, append(append([]string{}, good...), "--actor", "spoof"), append(append([]string{}, good...), "--apply"), append(append([]string{}, good...), "--resume"), append(append([]string{}, good...), "--run=other"), append(append([]string{}, good...), "--archive", "../x"), append(append([]string{}, good...), "--core", "--core=false"), append(append([]string{}, good...), "positional")}
	for _, args := range bad {
		if _, err := parseExecutionReadback(args); err == nil {
			t.Fatal("ambiguous/effect flags accepted", args)
		}
	}
	for i, value := range map[int]string{1: "relative", 3: "../run", 5: "bad", 7: "invalid"} {
		v := append([]string{}, good...)
		v[i] = value
		if _, err := parseExecutionReadback(v); err == nil {
			t.Fatal("invalid binding accepted", i)
		}
	}
}
