package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jiying2007/engineering-platform/internal/canonical"
	"github.com/jiying2007/engineering-platform/internal/recovery"
)

// All corruption is confined to the fixture's random schema. A previously
// valid aggregate proof must not hide later receipt, row or audit damage.
func TestPlannedActionProvenanceRejectsCorruptionAtEveryReadAndRecoveryGate(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*testing.T, *Store, recovery.ActionPlannedAbandonReceipt)
	}{
		{"truncated_receipt", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			plannedFixtureSQL(t, s, `UPDATE external_operations SET reconciliation_json=$2::jsonb WHERE operation_id=$1`, r.Request.OperationID, `{"kind":"RECOVERY_ACTION_PLANNED_ABANDONMENT_V1"}`)
		}},
		{"unknown_receipt_field", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			plannedFixtureSQL(t, s, `UPDATE external_operations SET reconciliation_json=reconciliation_json || '{"unrecognized":true}'::jsonb WHERE operation_id=$1`, r.Request.OperationID)
		}},
		{"self_consistent_wrong_original_key", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			r.Request.IdempotencyKey = "different-original-reservation"
			plannedFixtureReceipt(t, s, r)
		}},
		{"self_consistent_wrong_original_digest", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			r.Request.OriginalRequestDigest = canonical.BytesDigest([]byte("different-original-request"))
			plannedFixtureReceipt(t, s, r)
		}},
		{"self_consistent_wrong_run", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			r.Request.RunID = "different-original-run"
			plannedFixtureReceipt(t, s, r)
		}},
		{"self_consistent_wrong_execution_epoch", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			r.Request.ExecutionEpoch++
			plannedFixtureReceipt(t, s, r)
		}},
		{"self_consistent_wrong_recovery_epochs", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			r.Request.OriginalRecoveryEpoch++
			r.Request.RecoveryEpoch++
			plannedFixtureReceipt(t, s, r)
		}},
		{"changed_reconciler", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			r.Reconciler = "urn:engineering-platform:operator:replacement"
			plannedFixtureReceipt(t, s, r)
		}},
		{"changed_receipt_time", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			r.CreatedAt = r.CreatedAt.Add(time.Microsecond)
			plannedFixtureReceipt(t, s, r)
		}},
		{"changed_row_time", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			plannedFixtureSQL(t, s, `UPDATE external_operations SET updated_at=updated_at+interval '1 microsecond' WHERE operation_id=$1`, r.Request.OperationID)
		}},
		{"changed_external_ref", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			plannedFixtureSQL(t, s, `UPDATE external_operations SET external_ref='unexpected-effect' WHERE operation_id=$1`, r.Request.OperationID)
		}},
		{"changed_observation", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			plannedFixtureSQL(t, s, `UPDATE external_operations SET observed_state='unexpected-observation' WHERE operation_id=$1`, r.Request.OperationID)
		}},
		{"success_receipt_present", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			plannedFixtureSQL(t, s, `UPDATE external_operations SET receipt_json='{}'::jsonb WHERE operation_id=$1`, r.Request.OperationID)
		}},
		{"missing_receipt", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			plannedFixtureSQL(t, s, `UPDATE external_operations SET reconciliation_json=NULL,state='MANUAL' WHERE operation_id=$1`, r.Request.OperationID)
		}},
		{"missing_audit", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			plannedFixtureSQL(t, s, `DELETE FROM audit_events WHERE event_type='recovery.action.planned.abandoned' AND aggregate_id=$1`, r.Request.OperationID)
		}},
		{"missing_kind_and_audit", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			plannedFixtureSQL(t, s, `UPDATE external_operations SET reconciliation_json=reconciliation_json-'kind' WHERE operation_id=$1`, r.Request.OperationID)
			plannedFixtureSQL(t, s, `DELETE FROM audit_events WHERE event_type='recovery.action.planned.abandoned' AND aggregate_id=$1`, r.Request.OperationID)
		}},
		{"json_null_receipt_and_missing_audit", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			plannedFixtureSQL(t, s, `UPDATE external_operations SET reconciliation_json='null'::jsonb WHERE operation_id=$1`, r.Request.OperationID)
			plannedFixtureSQL(t, s, `DELETE FROM audit_events WHERE event_type='recovery.action.planned.abandoned' AND aggregate_id=$1`, r.Request.OperationID)
		}},
		{"unknown_kind_and_missing_audit", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			plannedFixtureSQL(t, s, `UPDATE external_operations SET reconciliation_json=jsonb_set(reconciliation_json,'{kind}','"UNRECOGNIZED_RECEIPT"') WHERE operation_id=$1`, r.Request.OperationID)
			plannedFixtureSQL(t, s, `DELETE FROM audit_events WHERE event_type='recovery.action.planned.abandoned' AND aggregate_id=$1`, r.Request.OperationID)
		}},
		{"impersonated_uninvoked_kind_and_missing_audit", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			plannedFixtureSQL(t, s, `UPDATE external_operations SET reconciliation_json=jsonb_set(reconciliation_json,'{kind}','"RECOVERY_GITHUB_PUBLICATION_NOT_INVOKED_V1"') WHERE operation_id=$1`, r.Request.OperationID)
			plannedFixtureSQL(t, s, `DELETE FROM audit_events WHERE event_type='recovery.action.planned.abandoned' AND aggregate_id=$1`, r.Request.OperationID)
		}},
		{"wrong_audit_payload", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			plannedFixtureSQL(t, s, `UPDATE audit_events SET payload_digest=$2 WHERE event_type='recovery.action.planned.abandoned' AND aggregate_id=$1`, r.Request.OperationID, canonical.BytesDigest([]byte("substituted-payload")))
		}},
		{"wrong_audit_event_digest", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			plannedFixtureSQL(t, s, `UPDATE audit_events SET event_digest=$2 WHERE event_type='recovery.action.planned.abandoned' AND aggregate_id=$1`, r.Request.OperationID, canonical.BytesDigest([]byte("substituted-event")))
		}},
		{"wrong_audit_time", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			plannedFixtureSQL(t, s, `UPDATE audit_events SET created_at=created_at+interval '1 microsecond' WHERE event_type='recovery.action.planned.abandoned' AND aggregate_id=$1`, r.Request.OperationID)
		}},
		{"wrong_audit_causation", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			plannedFixtureSQL(t, s, `UPDATE audit_events SET causation_id='another-cause' WHERE event_type='recovery.action.planned.abandoned' AND aggregate_id=$1`, r.Request.OperationID)
		}},
		{"duplicate_audit", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			input, err := auditInput("recovery.action.planned.abandoned", "ExternalOperation", r.Request.OperationID, r)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Mutate(context.Background(), Mutation{Audit: input}); err != nil {
				t.Fatal(err)
			}
		}},
		{"orphan_audit", func(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
			plannedFixtureSQL(t, s, `DELETE FROM external_operations WHERE operation_id=$1`, r.Request.OperationID)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, op, req := actionPlannedRecoveryFixture(t)
			ctx := context.Background()
			if err := s.CreateContext(ctx, *op); err != nil {
				t.Fatal(err)
			}
			if _, err := s.BeginRecoveryContext(ctx, 0); err != nil {
				t.Fatal(err)
			}
			receipt, err := s.ReconcilePlannedActionAbandoned(ctx, plannedReconciler, req)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.CreateRecoveryProof(ctx, 1, "independent-proof"); err != nil {
				t.Fatal(err)
			}
			if err = s.AuthorizeCompletion(ctx, "independent-completer", 1); err != nil {
				t.Fatal(err)
			}
			tc.mutate(t, s, receipt)
			var auditBefore int
			if err = s.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events").Scan(&auditBefore); err != nil {
				t.Fatal(err)
			}
			if _, err = s.GetPlannedActionAbandonReceipt(ctx, op.ID); err == nil {
				t.Fatal("corrupt receipt accepted by direct readback")
			}
			if _, err = s.ReconcilePlannedActionAbandoned(ctx, plannedReconciler, req); err == nil {
				t.Fatal("corrupt receipt accepted by exact repeat")
			}
			if _, err = s.ReadOperationalStatus(ctx); !errors.Is(err, ErrRecoveryFactsUnresolved) {
				t.Fatalf("corrupt receipt passed status: %v", err)
			}
			if _, err = s.CreateRecoveryProof(ctx, 1, "independent-proof-after-corruption"); !errors.Is(err, ErrRecoveryFactsUnresolved) {
				t.Fatalf("corrupt receipt passed aggregate proof: %v", err)
			}
			if err = s.AuthorizeCompletion(ctx, "independent-completer", 1); !errors.Is(err, ErrRecoveryFactsUnresolved) {
				t.Fatalf("corrupt receipt passed authorization: %v", err)
			}
			if _, err = s.CompleteRecoveryContext(ctx, 1, true); !errors.Is(err, ErrRecoveryFactsUnresolved) {
				t.Fatalf("corrupt receipt completed Recovery: %v", err)
			}
			state, err := s.GetRecoveryContext(ctx)
			if err != nil || state.Mode != recovery.RecoveryReconciliation {
				t.Fatalf("failure changed recovery mode: %+v %v", state, err)
			}
			var auditAfter int
			if err = s.pool.QueryRow(ctx, "SELECT count(*) FROM audit_events").Scan(&auditAfter); err != nil || auditAfter != auditBefore {
				t.Fatalf("rejected read changed journal: %d/%d %v", auditBefore, auditAfter, err)
			}
		})
	}
}

func plannedFixtureSQL(t *testing.T, s *Store, query string, args ...any) {
	t.Helper()
	if _, err := s.pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

func plannedFixtureReceipt(t *testing.T, s *Store, r recovery.ActionPlannedAbandonReceipt) {
	t.Helper()
	var err error
	r.RequestDigest, err = r.Request.Digest()
	if err != nil || r.Validate() != nil {
		t.Fatal("invalid replacement fixture", err)
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	plannedFixtureSQL(t, s, `UPDATE external_operations SET reconciliation_json=$2::jsonb WHERE operation_id=$1`, r.Request.OperationID, string(raw))
}

func TestPlannedReconcilerCannotCompleteViaDifferentProofPrincipal(t *testing.T) {
	s, op, req := actionPlannedRecoveryFixture(t)
	ctx := context.Background()
	if err := s.CreateContext(ctx, *op); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginRecoveryContext(ctx, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReconcilePlannedActionAbandoned(ctx, plannedReconciler, req); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRecoveryProof(ctx, 1, "independent-proof"); err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeCompletion(ctx, plannedReconciler, 1); err == nil {
		t.Fatal("original reconciler completed through another principal's proof")
	}
	if err := s.AuthorizeCompletion(ctx, "independent-completer", 1); err != nil {
		t.Fatal(err)
	}
}

func TestPlannedActionAuditWithoutReceiptCannotBeOverwritten(t *testing.T) {
	s, op, req := actionPlannedRecoveryFixture(t)
	ctx := context.Background()
	if err := s.CreateContext(ctx, *op); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginRecoveryContext(ctx, 0); err != nil {
		t.Fatal(err)
	}
	receipt, err := s.ReconcilePlannedActionAbandoned(ctx, plannedReconciler, req)
	if err != nil {
		t.Fatal(err)
	}
	// An inconsistent restore can retain the audit with an older PLANNED row.
	plannedFixtureSQL(t, s, `UPDATE external_operations SET state='PLANNED',reconciliation_json=NULL WHERE operation_id=$1`, op.ID)
	if _, err = s.ReconcilePlannedActionAbandoned(ctx, plannedReconciler, req); err == nil {
		t.Fatal("surviving original audit was overwritten by a new settlement")
	}
	var count int
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE event_type='recovery.action.planned.abandoned' AND aggregate_id=$1`, receipt.Request.OperationID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("original audit changed: %d %v", count, err)
	}
}
