package main

import "testing"

func TestInstallationSwitchCLIRejectsImplicitEffectsAndAmbiguity(t *testing.T) {
	good := []string{
		"--active", "/releases/active", "--candidate", "/releases/candidate", "--previous", "/releases/previous",
		"--active-source", "0123456789012345678901234567890123456789",
		"--active-manifest-digest", "sha256:0123456789012345678901234567890123456789012345678901234567890123",
		"--candidate-source", "abcdefabcdefabcdefabcdefabcdefabcdefabcd",
		"--candidate-manifest-digest", "sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd",
	}
	if _, err := parseInstallationSwitch(good); err != nil {
		t.Fatal(err)
	}
	for _, extra := range [][]string{{"extra"}, {"--start"}, {"--migrate"}, {"--rollback-db"}, {"--active=/other"}} {
		args := append(append([]string{}, good...), extra...)
		if _, err := parseInstallationSwitch(args); err == nil {
			t.Fatal("implicit or ambiguous effect accepted", extra)
		}
	}
	for i := 0; i < len(good); i += 2 {
		args := append([]string{}, good[:i]...)
		args = append(args, good[i+2:]...)
		if _, err := parseInstallationSwitch(args); err == nil {
			t.Fatal("missing switch identity accepted", good[i])
		}
	}
}
