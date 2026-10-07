package main

import "testing"

func TestArtifactRetentionCLIRequiresExplicitAnchoredReplication(t *testing.T) {
	good := []string{"replicate", "--config", "/private/retention.json", "--config-digest", "sha256:0123456789012345678901234567890123456789012345678901234567890123"}
	if _, err := parseArtifactRetention(good); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		nil,
		{"verify"},
		{"replicate", "--config", "/private/retention.json"},
		{"replicate", "--config", "/private/retention.json", "--config-digest", good[4], "--delete"},
		{"replicate", "--config", "/private/retention.json", "--config-digest", good[4], "--config=/other"},
		{"replicate", "--config", "/private/retention.json", "--config-digest", good[4], "extra"},
	} {
		if _, err := parseArtifactRetention(args); err == nil {
			t.Fatal("ambiguous or mutating retention CLI accepted", args)
		}
	}
}
