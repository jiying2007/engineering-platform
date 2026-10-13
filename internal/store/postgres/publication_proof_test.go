package postgres

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/action"
	"github.com/jiying2007/engineering-platform/internal/canonical"
)

func TestPublicationRecoveryDatabaseRequiresOriginalGitProof(t *testing.T) {
	for _, mode := range []string{"legacy", "missing", "unreaped", "digest", "start-time"} {
		t.Run(mode, func(t *testing.T) {
			s, req, e := publicationDatabaseFixture(t, action.Manual)
			switch mode {
			case "legacy":
				e.Completion.Version = 1
				e.Completion.GitQuiescence = nil
			case "missing":
				e.Completion.GitQuiescence = nil
			case "unreaped":
				e.Completion.GitQuiescence.Proofs[0].InitReaped = false
				e.Completion.GitQuiescence.Digest, _ = canonical.Digest(e.Completion.GitQuiescence.Proofs)
			case "digest":
				e.Completion.GitQuiescence.Digest = "invalid"
			case "start-time":
				e.Completion.GitQuiescence.Proofs[0].StartedAt = e.Start.StartedAt.Add(-1)
				e.Completion.GitQuiescence.Digest, _ = canonical.Digest(e.Completion.GitQuiescence.Proofs)
			}
			if _, err := s.ConfirmRecoveredPublication(context.Background(), "reviewer", req, e); err == nil {
				t.Fatal("unqualified evidence settled original Action")
			}
			var state string
			var count int
			if err := s.pool.QueryRow(context.Background(), `SELECT state,(SELECT count(*) FROM audit_events WHERE event_type='recovery.github.publication.confirmed') FROM external_operations WHERE operation_id=$1`, req.Identity.OperationID).Scan(&state, &count); err != nil {
				t.Fatal(err)
			}
			if state != string(action.Manual) || count != 0 {
				t.Fatal("denial mutated ledger/audit", state, count)
			}
		})
	}
}
func TestPublicationRecoveryDatabaseHistoricalReceiptReadback(t *testing.T) {
	s, req, e := publicationDatabaseFixture(t, action.Manual)
	r, err := s.ConfirmRecoveredPublication(context.Background(), "reviewer", req, e)
	if err != nil {
		t.Fatal(err)
	}
	// Represent bytes retained by the PREVIOUS release, not a new recovery grant.
	// The mutation is confined to this isolated database fixture.
	r.Evidence.Completion.Version = 1
	r.Evidence.Completion.GitQuiescence = nil
	r.EvidenceDigest, _ = r.Evidence.Digest()
	raw, _ := json.Marshal(r)
	digest, _ := canonical.Digest(r)
	if _, err = s.pool.Exec(context.Background(), "UPDATE external_operations SET receipt_json=$2::jsonb WHERE operation_id=$1", req.Identity.OperationID, string(raw)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(context.Background(), "UPDATE audit_events SET payload_digest=$2 WHERE aggregate_id=$1 AND event_type='recovery.github.publication.confirmed'", req.Identity.OperationID, digest); err != nil {
		t.Fatal(err)
	}
	prior, err := s.GetRecoveredPublication(context.Background(), req.Identity.OperationID)
	if err != nil || prior.Validate() != nil || prior.Evidence.ValidateForRecovery() == nil {
		t.Fatal("historical readback became current admission", err)
	}
	if _, err = s.ConfirmRecoveredPublication(context.Background(), "reviewer", req, prior.Evidence); err != nil {
		t.Fatal("exact historic repeat no longer reads", err)
	}
}
