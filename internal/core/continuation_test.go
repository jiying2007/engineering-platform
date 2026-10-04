package core

import (
	"encoding/json"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"strings"
	"testing"
)

func TestContinuationRefBindsRunInput(t *testing.T) {
	m := RunInputManifest{RunID: "next", TaskContractDigest: canonical.BytesDigest([]byte("task"))}
	original, _ := m.Digest()
	ref := ContinuationRef{SourceRunID: "old", CheckpointDigest: canonical.BytesDigest([]byte("descriptor")), ArchiveDigest: canonical.BytesDigest([]byte("bytes")), ArchiveSize: 10, SnapshotDigest: canonical.BytesDigest([]byte("snapshot"))}
	m.Continuation = &ref
	bound, err := m.Digest()
	if err != nil || original == bound {
		t.Fatal("unbound continuation")
	}
	data, _ := json.Marshal(m)
	var read RunInputManifest
	if json.Unmarshal(data, &read) != nil || *read.Continuation != ref {
		t.Fatal("roundtrip")
	}
	for _, change := range []func(*ContinuationRef){func(c *ContinuationRef) { c.SourceRunID = "next" }, func(c *ContinuationRef) { c.ArchiveSize = 0 }, func(c *ContinuationRef) { c.ArchiveSize = 321 << 20 }, func(c *ContinuationRef) { c.SourceRunID = "old\n" }, func(c *ContinuationRef) { c.CheckpointDigest = strings.Repeat("0", 64) }} {
		bad := ref
		change(&bad)
		copy := m
		copy.Continuation = &bad
		if _, e := copy.Digest(); e == nil {
			t.Fatal("bad reference accepted")
		}
	}
}
