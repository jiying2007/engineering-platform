package postgres

import (
	"context"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/codexexec"
	"github.com/jiying2007/engineering-platform/internal/testsupport"
	"testing"
)

func TestCodexCheckpointRequiresSealedProofAndFrozenPreparation(t *testing.T) {
	s := integrationStore(t)
	p, b, _ := liveControlFixture(t, s)
	ctx := context.Background()
	transcript := codexexec.ControlTranscript{Version: 2, Close: codexexec.ControlClose{Binding: b, TurnStatus: "interrupted", ProcessScope: testsupport.ProcessScopeFixture()}, Deliveries: []codexexec.ControlDelivery{}}
	td, err := transcript.Digest()
	workerOK(t, err)
	c := codexexec.SourceCheckpoint{Version: 1, Binding: b, TaskDigest: p.Assignment.Intent.TaskDigest, InputDigest: p.Assignment.Intent.InputDigest, BaseCommit: p.Preparation.Facts.BaseCommit, BaselineDigest: p.Preparation.Facts.SourceDigest, TranscriptDigest: td, SnapshotDigest: canonical.BytesDigest([]byte("TEST-ONLY")), ArchiveDigest: canonical.BytesDigest([]byte("TEST-ONLY")), ArchiveSize: 1}
	if _, err := s.SaveCodexCheckpoint(ctx, codexTestWorker, c); err == nil {
		t.Fatal("active runtime accepted snapshot")
	}
	transcript, err = s.CloseCodexControl(ctx, codexTestWorker, transcript.Close)
	workerOK(t, err)
	c.TranscriptDigest, err = transcript.Digest()
	workerOK(t, err)
	for _, change := range []func(*codexexec.SourceCheckpoint){func(x *codexexec.SourceCheckpoint) { x.TaskDigest = canonical.BytesDigest([]byte("bad")) }, func(x *codexexec.SourceCheckpoint) { x.InputDigest = canonical.BytesDigest([]byte("bad")) }, func(x *codexexec.SourceCheckpoint) { x.BaseCommit = "0000000000000000000000000000000000000000" }, func(x *codexexec.SourceCheckpoint) { x.BaselineDigest = canonical.BytesDigest([]byte("bad")) }, func(x *codexexec.SourceCheckpoint) { x.TranscriptDigest = canonical.BytesDigest([]byte("bad")) }, func(x *codexexec.SourceCheckpoint) { x.Binding.ExecutionEpoch++ }} {
		bad := c
		change(&bad)
		if _, err := s.SaveCodexCheckpoint(ctx, codexTestWorker, bad); err == nil {
			t.Fatal("foreign snapshot identity accepted")
		}
	}
	if _, err := s.SaveCodexCheckpoint(ctx, "urn:engineering-platform:worker:other", c); err == nil {
		t.Fatal("foreign worker accepted")
	}
	_, err = s.SaveCodexCheckpoint(ctx, codexTestWorker, c)
	workerOK(t, err)
	workerOK(t, s.ApplyCoreMigration(ctx))
	status, err := s.GetCodex(ctx, p.Token.RunID)
	workerOK(t, err)
	if status.SourceCheckpoint == nil || *status.SourceCheckpoint != c {
		t.Fatal("migration/readback lost snapshot")
	}
	if _, err := s.FinishCodex(ctx, codexTestWorker, codexexec.Report{Token: p.Token, Result: codexResult(t, p)}); err == nil {
		t.Fatal("source checkpoint granted successful delivery")
	}
}
