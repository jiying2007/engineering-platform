package main

import "testing"

func TestInstallationCLIRejectsImplicitEffectsAndAmbiguity(t *testing.T) {
	good := []string{"--from", "/source", "--into", "/new", "--source-commit", "0123456789012345678901234567890123456789"}
	if _, err := parseInstall(good, false); err != nil {
		t.Fatal(err)
	}
	for _, extra := range [][]string{{"--execute"}, {"--enable"}, {"--migrate"}, {"--identity", "root"}, {"--source-commit=other"}, {"--into=/other"}, {"extra"}} {
		args := append(append([]string{}, good...), extra...)
		if _, err := parseInstall(args, false); err == nil {
			t.Fatal("accepted", extra)
		}
	}
	for i := 0; i < len(good); i += 2 {
		args := append([]string{}, good[:i]...)
		args = append(args, good[i+2:]...)
		if _, err := parseInstall(args, false); err == nil {
			t.Fatal("missing", good[i])
		}
	}
	if _, err := parseInstall(good, true); err == nil {
		t.Fatal("verify accepted write flags")
	}
}
