package main

import "testing"

func TestProductionPublisherHealthCLIRejectsImplicitEffects(t *testing.T) {
	good := []string{"--config", "/etc/engineering-platform/publisher-remote.json"}
	if got, err := parseProductionPublisherHealth(good); err != nil || got != good[1] {
		t.Fatal(got, err)
	}
	for _, extra := range [][]string{{"extra"}, {"--execute"}, {"--publish"}, {"--repair"}, {"--config=/other"}} {
		args := append(append([]string{}, good...), extra...)
		if _, err := parseProductionPublisherHealth(args); err == nil {
			t.Fatal("state-changing or ambiguous health option accepted", extra)
		}
	}
	if _, err := parseProductionPublisherHealth(nil); err == nil {
		t.Fatal("missing publisher health configuration accepted")
	}
}
