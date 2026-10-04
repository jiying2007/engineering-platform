package main

import "testing"

func TestSourceCheckpointCLIRejectsUnsafeOrImplicitActions(t *testing.T) {
	for _, args := range [][]string{nil, {"pause"}, {"resume"}, {"takeover"}, {"verify"}, {"restore", "--archive", "x", "--digest", "bad", "--run", "r"}, {"verify", "--archive", "/missing", "--digest", "bad", "--run", "r"}, {"verify", "--archive", "x", "--digest", "bad", "--run", "r", "--destination", "live"}, {"verify", "--actor", "someone"}} {
		if err := sourceCheckpoint(args); err == nil {
			t.Fatalf("invalid command accepted: %v", args)
		}
	}
}
