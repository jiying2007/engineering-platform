# Independent Review gate v1

Base: `34ff7d82316d1d095040ce6f62522fe5397dee86` (#44).

This increment turns Review from an optional Closure string into a persisted,
authenticated, independently authorized Core gate. It reuses the existing
Work/Delivery/Verification/Closure lifecycle; it does not add a service or a
parallel evidence authority.

## Required lifecycle

```text
completed Run
  -> immutable Delivery subject
  -> requirement-bound Evidence
  -> PASS Verification
  -> independent ReviewReport
  -> PASS review moves Work VERIFYING -> REVIEWING
  -> exact review + verification + delivery subject
  -> Closure
  -> CLOSED
```

Closure cannot skip Review and cannot substitute a review from another delivery,
verification report, task contract or subject.

## ReviewReport

A report binds:

- review_report_id;
- delivery_receipt_id;
- verification_report_id;
- task_contract_digest;
- delivery subject_digest;
- authenticated reviewer;
- PASS or FAIL;
- bounded findings and known limits;
- creation time.

Findings have INFO, WARNING or BLOCKING severity. PASS cannot contain a BLOCKING
finding. FAIL must contain at least one BLOCKING finding. IDs are unique within
one report and all collections are bounded.

Review JSON and audit data are immutable business facts. PostgreSQL stores the
report in the same transaction as the Work state mutation. The memory store copies
slice-backed report fields at both write and read boundaries so caller mutation
cannot rewrite retained Review state.

## Independence

The authenticated API binds `reviewer` to the verified mTLS URI identity.
The reviewer must differ from:

- the Work human owner; and
- the verifier named by the exact PASS VerificationReport.

The access policy adds `review:create`, but a principal holding that capability
may hold only `review:create` and optional `core:read`. It cannot simultaneously
hold Worker, engineering mutation, Action, Evidence, Verification, Closure,
Recovery or material-degradation authority. This is a certificate/policy
separation-of-duties rule, not merely a request-body comparison.

The store repeats owner/verifier/subject checks so an internal caller cannot
bypass the HTTP authorization layer.

## PASS/FAIL state semantics

A FAIL Review is persisted and leaves Work in VERIFYING. It cannot close the Work
and the same VerificationReport cannot be reviewed a second time. Repair requires
new evidence as needed, a new VerificationReport for the same immutable Delivery
or a new Delivery if the subject changed, followed by a new independent Review.

A PASS Review atomically moves Work from VERIFYING to REVIEWING. Closure requires:

- the same DeliveryReceipt;
- the same PASS VerificationReport;
- the same PASS ReviewReport;
- the same task contract and subject digest;
- a completed Run bound to that delivery;
- current Work state REVIEWING and matching active Run/task.

Only then may Work transition REVIEWING -> CLOSED.

## Migration 0006

`0006_review_reports.sql` creates the authoritative review table and changes
`closure_receipts.review_report_id` from nullable text to a NOT NULL foreign key.

Historical Closure rows predate Review authority. Migration therefore fails
closed when any legacy closure exists. It does not synthesize a ReviewReport or
silently bless an old Closure. Operators must reconcile historical rows outside
the migration and establish a new authoritative state explicitly.

The migration is additive for deployments with no historical closures and is
ordered after offline execution migration v5.

## Failure boundaries

A report is rejected when any of the following drifts:

- verification is not PASS;
- verification does not bind the exact delivery subject;
- Work is not VERIFYING the exact active Run/task;
- reviewer equals verifier or Work owner;
- reviewer certificate lacks isolated review authority;
- report IDs or findings are malformed/duplicated;
- PASS carries a blocking finding or FAIL lacks one;
- concurrent Work version/state changes occur;
- a second review is attempted for one VerificationReport;
- Closure references a missing, failed or mismatched ReviewReport.

## What this does not prove

Independent Review is a human/independent-agent decision authority. A PASS report
does not by itself prove that the review was technically good. Assurance comes
from the distinct reviewer identity, exact frozen subject, retained evidence and
auditability.

This slice also does not:

- produce a real WIF-authenticated model-turn receipt;
- enable workspace-write Codex execution;
- approve command/file/network effects;
- complete recovery/restore drills;
- constitute retained Feature/Debug pilots;
- make an M1 or production-readiness claim.

The next connected work after this gate is to bind real model/changed-tree facts
as frozen Evidence and exercise one complete Feature and Debug lifecycle through
Verification + independent Review + Closure.
