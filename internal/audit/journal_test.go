package audit

import (
	"testing"
	"time"
)

func TestAuditChainDetectsTampering(t *testing.T) {
	var j Journal
	_, _ = j.Append("run.created", "sha256:a", time.Unix(1, 0))
	_, _ = j.Append("run.started", "sha256:b", time.Unix(2, 0))
	events := j.Events()
	if err := Verify(events); err != nil {
		t.Fatalf("expected valid audit chain: %v", err)
	}
	events[0].PayloadDigest = "sha256:tampered"
	if err := Verify(events); err == nil {
		t.Fatal("expected tampering to be detected")
	}
}

func TestCheckpointRequiresValidEvents(t *testing.T) {
	var j Journal
	_, _ = j.Append("work.created", "sha256:a", time.Unix(1, 0))
	events := j.Events()
	cp, err := NewCheckpoint(events, time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if cp.EventCount != 1 || cp.RootDigest == "" {
		t.Fatalf("unexpected checkpoint: %#v", cp)
	}
}
