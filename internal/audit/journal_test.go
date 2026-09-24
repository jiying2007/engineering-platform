package audit

import (
	"testing"
	"time"
)

func TestAuditChainDetectsTampering(t *testing.T) {
	var j Journal
	_, _ = j.AppendInput(Input{
		Type:          "run.created",
		AggregateType: "Run",
		AggregateID:   "run-1",
		PayloadDigest: "sha256:a",
		CorrelationID: "corr-1",
	}, time.Unix(1, 0))
	_, _ = j.AppendInput(Input{
		Type:          "run.started",
		AggregateType: "Run",
		AggregateID:   "run-1",
		PayloadDigest: "sha256:b",
		CorrelationID: "corr-1",
		CausationID:   "cmd-1",
	}, time.Unix(2, 0))
	events := j.Events()
	if err := Verify(events); err != nil {
		t.Fatalf("expected valid audit chain: %v", err)
	}

	events[0].PayloadDigest = "sha256:tampered"
	if err := Verify(events); err == nil {
		t.Fatal("expected payload tampering to be detected")
	}
}

func TestAuditDigestCoversAggregateAndCausality(t *testing.T) {
	now := time.Unix(1, 0)
	base, err := Build(1, "", Input{
		Type:          "run.created",
		AggregateType: "Run",
		AggregateID:   "run-1",
		PayloadDigest: "sha256:a",
		CorrelationID: "corr-1",
		CausationID:   "cmd-1",
	}, now)
	if err != nil {
		t.Fatal(err)
	}

	cases := []Input{
		{Type: "run.created", AggregateType: "Task", AggregateID: "run-1", PayloadDigest: "sha256:a", CorrelationID: "corr-1", CausationID: "cmd-1"},
		{Type: "run.created", AggregateType: "Run", AggregateID: "run-2", PayloadDigest: "sha256:a", CorrelationID: "corr-1", CausationID: "cmd-1"},
		{Type: "run.created", AggregateType: "Run", AggregateID: "run-1", PayloadDigest: "sha256:a", CorrelationID: "corr-2", CausationID: "cmd-1"},
		{Type: "run.created", AggregateType: "Run", AggregateID: "run-1", PayloadDigest: "sha256:a", CorrelationID: "corr-1", CausationID: "cmd-2"},
	}
	for _, input := range cases {
		other, err := Build(1, "", input, now)
		if err != nil {
			t.Fatal(err)
		}
		if other.Digest == base.Digest {
			t.Fatalf("audit digest did not change for identity/causality mutation: %#v", input)
		}
	}
}

func TestAuditDigestExcludesCreatedAt(t *testing.T) {
	input := Input{
		Type:          "work.created",
		AggregateType: "Work",
		AggregateID:   "work-1",
		PayloadDigest: "sha256:a",
	}
	first, err := Build(1, "", input, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(1, "", input, time.Unix(999, 0))
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest {
		t.Fatalf("created_at must not affect audit content digest: %s != %s", first.Digest, second.Digest)
	}
}

func TestAuditSequenceAndPreviousDigestAreChained(t *testing.T) {
	first, err := Build(1, "", Input{Type: "a", PayloadDigest: "sha256:a"}, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(2, first.Digest, Input{Type: "b", PayloadDigest: "sha256:b"}, time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify([]Event{first, second}); err != nil {
		t.Fatalf("expected valid explicit chain: %v", err)
	}

	second.Sequence = 3
	if err := Verify([]Event{first, second}); err == nil {
		t.Fatal("expected sequence gap to be rejected")
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
