package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jiying2007/engineering-platform/internal/audit"
	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/recovery"
)

func TestPostgresRecoveryProofRejectsInconsistentRetainedJSON(t *testing.T) {
	for _, fault := range []struct {
		name string
		sql  string
	}{
		{"reconciler", `UPDATE recovery_reconciliation_proofs SET proof_json=jsonb_set(proof_json,'{reconciler}','"replacement-reconciler"') WHERE recovery_epoch=1`},
		{"epoch", `UPDATE recovery_reconciliation_proofs SET proof_json=jsonb_set(proof_json,'{recovery_epoch}','2') WHERE recovery_epoch=1`},
		{"null_facts", `UPDATE recovery_reconciliation_proofs SET proof_json=jsonb_set(proof_json,'{facts}','null') WHERE recovery_epoch=1`},
		{"missing_facts", `UPDATE recovery_reconciliation_proofs SET proof_json=proof_json-'facts' WHERE recovery_epoch=1`},
		{"missing_clear_counter", `UPDATE recovery_reconciliation_proofs SET proof_json=proof_json#-'{facts,worker_leases}' WHERE recovery_epoch=1`},
		{"null_clear_counter", `UPDATE recovery_reconciliation_proofs SET proof_json=jsonb_set(proof_json,'{facts,worker_leases}','null') WHERE recovery_epoch=1`},
		{"unknown_member", `UPDATE recovery_reconciliation_proofs SET proof_json=proof_json||'{"unexpected":true}' WHERE recovery_epoch=1`},
	} {
		t.Run(fault.name, func(t *testing.T) {
			s, proof := recoveryProofIntegrityFixture(t)
			ctx := context.Background()
			if err := s.AuthorizeCompletion(ctx, proof.Reconciler, 1); err == nil {
				t.Fatal("original reconciler was authorized before corruption")
			}
			if err := s.AuthorizeCompletion(ctx, "independent-completer", 1); err != nil {
				t.Fatal(err)
			}
			// Corrupt only the JSON retained in an isolated fixture. The original
			// SQL identity/facts/head/time projection and all audit rows survive.
			const originalQuery = `SELECT to_jsonb(p)-'proof_json',
 (SELECT jsonb_agg(to_jsonb(a) ORDER BY sequence) FROM audit_events a)
 FROM recovery_reconciliation_proofs p WHERE recovery_epoch=1`
			var rowBefore, auditBefore, rowAfter, auditAfter []byte
			if err := s.pool.QueryRow(ctx, originalQuery).Scan(&rowBefore, &auditBefore); err != nil {
				t.Fatal(err)
			}
			if _, err := s.pool.Exec(ctx, fault.sql); err != nil {
				t.Fatal(err)
			}
			if err := s.pool.QueryRow(ctx, originalQuery).Scan(&rowAfter, &auditAfter); err != nil {
				t.Fatal(err)
			}
			if string(rowAfter) != string(rowBefore) || string(auditAfter) != string(auditBefore) {
				t.Fatal("fault changed original row projection or audit")
			}
			assertRecoveryProofIntegrityBlocked(t, s, 1, proof.Reconciler)
		})
	}
}

func TestPostgresRecoveryProofRequiresOriginalProjectionAndAudit(t *testing.T) {
	for _, fault := range []struct {
		name string
		sql  string
	}{
		{"row_reconciler", `UPDATE recovery_reconciliation_proofs SET reconciler='replacement-reconciler' WHERE recovery_epoch=1`},
		{"row_facts_digest", `UPDATE recovery_reconciliation_proofs SET facts_digest='sha256:'||repeat('0',64) WHERE recovery_epoch=1`},
		{"row_audit_sequence", `UPDATE recovery_reconciliation_proofs SET audit_sequence=audit_sequence+1 WHERE recovery_epoch=1`},
		{"row_audit_digest", `UPDATE recovery_reconciliation_proofs SET audit_digest='sha256:'||repeat('0',64) WHERE recovery_epoch=1`},
		{"row_created_at", `UPDATE recovery_reconciliation_proofs SET created_at=created_at+interval '1 second' WHERE recovery_epoch=1`},
		{"coherent_json_and_row_but_original_audit", `UPDATE recovery_reconciliation_proofs SET reconciler='replacement-reconciler',proof_json=jsonb_set(proof_json,'{reconciler}','"replacement-reconciler"') WHERE recovery_epoch=1`},
		{"missing_original_audit", `DELETE FROM audit_events WHERE sequence=(SELECT audit_sequence+1 FROM recovery_reconciliation_proofs WHERE recovery_epoch=1)`},
		{"wrong_original_payload", `UPDATE audit_events SET payload_digest='sha256:'||repeat('0',64) WHERE sequence=(SELECT audit_sequence+1 FROM recovery_reconciliation_proofs WHERE recovery_epoch=1)`},
		{"wrong_original_digest", `UPDATE audit_events SET event_digest='sha256:'||repeat('0',64) WHERE sequence=(SELECT audit_sequence+1 FROM recovery_reconciliation_proofs WHERE recovery_epoch=1)`},
		{"wrong_original_created_at", `UPDATE audit_events SET created_at=created_at+interval '1 second' WHERE sequence=(SELECT audit_sequence+1 FROM recovery_reconciliation_proofs WHERE recovery_epoch=1)`},
		{"missing_prior_head", `DELETE FROM audit_events WHERE sequence=(SELECT audit_sequence FROM recovery_reconciliation_proofs WHERE recovery_epoch=1)`},
		{"wrong_prior_head_digest", `UPDATE audit_events SET event_digest='sha256:'||repeat('0',64) WHERE sequence=(SELECT audit_sequence FROM recovery_reconciliation_proofs WHERE recovery_epoch=1)`},
	} {
		t.Run(fault.name, func(t *testing.T) {
			s, proof := recoveryProofIntegrityFixture(t)
			if _, err := s.pool.Exec(context.Background(), fault.sql); err != nil {
				t.Fatal(err)
			}
			assertRecoveryProofIntegrityBlocked(t, s, 1, proof.Reconciler)
		})
	}
}

func TestPostgresRecoveryProofRetainsOriginalAuditAcrossEpochs(t *testing.T) {
	s, first := recoveryProofIntegrityFixture(t)
	ctx := context.Background()
	if _, err := s.Mutate(ctx, Mutation{Audit: audit.Input{
		Type: "test.unrelated", PayloadDigest: canonical.BytesDigest([]byte("later unrelated audit")),
	}}); err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeCompletion(ctx, "independent-completer", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteRecoveryContext(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginRecoveryContext(ctx, 1); err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateRecoveryProof(ctx, 2, "next-reconciler")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []recovery.Proof{first, second} {
		got, err := s.GetRecoveryProof(ctx, want.RecoveryEpoch)
		if err != nil {
			t.Fatal(err)
		}
		wantDigest, err := canonical.Digest(want)
		if err != nil {
			t.Fatal(err)
		}
		gotDigest, err := canonical.Digest(got)
		if err != nil || gotDigest != wantDigest {
			t.Fatalf("later history changed proof %d: want=%s got=%s err=%v", want.RecoveryEpoch, wantDigest, gotDigest, err)
		}
	}
	if err := s.AuthorizeCompletion(ctx, first.Reconciler, 2); err != nil {
		t.Fatalf("an earlier epoch's proof reconciler was treated as the current reconciler: %v", err)
	}
	if _, err := s.CompleteRecoveryContext(ctx, 2, true); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresRecoveryProofCannotReplaceOriginalAuditWithLaterCopy(t *testing.T) {
	s, proof := recoveryProofIntegrityFixture(t)
	ctx := context.Background()
	input, err := auditInput("recovery.reconciliation_proved", "PlatformState", "singleton", proof)
	if err != nil {
		t.Fatal(err)
	}
	later, err := s.Mutate(ctx, Mutation{Audit: input})
	if err != nil {
		t.Fatal(err)
	}
	if later.AuditEvent.Sequence <= proof.AuditSequence+1 {
		t.Fatal("copy did not follow the original proof audit")
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM audit_events WHERE sequence=$1`, proof.AuditSequence+1); err != nil {
		t.Fatal(err)
	}
	// A later event with the same canonical proof payload cannot substitute for
	// the missing event at the original captured head+1. This does not require
	// scanning all subsequent history or treating later copies as authority.
	assertRecoveryProofIntegrityBlocked(t, s, 1, proof.Reconciler)
}

func TestPostgresRecoveryProofAllowsLargeOriginalAuditHead(t *testing.T) {
	s := newIsolatedIntegrationStore(t)
	ctx := context.Background()
	if _, err := s.BeginRecoveryContext(ctx, 0); err != nil {
		t.Fatal(err)
	}
	// Existing authority mutations permit aggregate IDs larger than the proof
	// reader's JSON budget. A proof binds this head's digest, not a new size limit
	// on the preceding business event's full JSON representation.
	if _, err := s.Mutate(ctx, Mutation{Audit: audit.Input{
		Type: "work.created", AggregateType: "WorkItem", AggregateID: strings.Repeat("w", 20<<10),
		PayloadDigest: canonical.BytesDigest([]byte("large existing aggregate identity")),
	}}); err != nil {
		t.Fatal(err)
	}
	proof, err := s.CreateRecoveryProof(ctx, 1, "original-reconciler")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRecoveryProof(ctx, 1)
	if err != nil {
		t.Fatalf("successfully created proof rejected its legal original audit head: %v", err)
	}
	wantDigest, err := canonical.Digest(proof)
	if err != nil {
		t.Fatal(err)
	}
	gotDigest, err := canonical.Digest(got)
	if err != nil || gotDigest != wantDigest {
		t.Fatalf("original proof changed: want=%s got=%s err=%v", wantDigest, gotDigest, err)
	}
	if err := s.AuthorizeCompletion(ctx, "independent-completer", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteRecoveryContext(ctx, 1, true); err != nil {
		t.Fatal(err)
	}
}

func recoveryProofIntegrityFixture(t *testing.T) (*Store, recovery.Proof) {
	t.Helper()
	s := newIsolatedIntegrationStore(t)
	ctx := context.Background()
	if _, err := s.BeginRecoveryContext(ctx, 0); err != nil {
		t.Fatal(err)
	}
	proof, err := s.CreateRecoveryProof(ctx, 1, "original-reconciler")
	if err != nil {
		t.Fatal(err)
	}
	return s, proof
}

func assertRecoveryProofIntegrityBlocked(t *testing.T, s *Store, epoch uint64, reconciler string) {
	t.Helper()
	ctx := context.Background()
	var completionsBefore, completionsAfter uint64
	const completionCount = `SELECT count(*) FROM audit_events WHERE event_type='recovery.completed'`
	if err := s.pool.QueryRow(ctx, completionCount).Scan(&completionsBefore); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetRecoveryProof(ctx, epoch); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Errorf("read accepted inconsistent retained proof: %v", err)
	}
	for _, subject := range []string{reconciler, "independent-completer"} {
		if err := s.AuthorizeCompletion(ctx, subject, epoch); !errors.Is(err, ErrRecoveryFactsUnresolved) {
			t.Errorf("authorization did not reject proof integrity for %q: %v", subject, err)
		}
	}
	if _, err := s.CompleteRecoveryContext(ctx, epoch, true); !errors.Is(err, ErrRecoveryFactsUnresolved) {
		t.Errorf("completion accepted inconsistent retained proof: %v", err)
	}
	state, err := s.GetRecoveryContext(ctx)
	if err != nil || state.Epoch != epoch || state.Mode != recovery.RecoveryReconciliation {
		t.Errorf("inconsistent proof did not preserve active recovery: state=%#v err=%v", state, err)
	}
	if err := s.pool.QueryRow(ctx, completionCount).Scan(&completionsAfter); err != nil {
		t.Fatal(err)
	}
	if completionsAfter != completionsBefore {
		t.Errorf("rejected proof appended a completion audit: before=%d after=%d", completionsBefore, completionsAfter)
	}
}
