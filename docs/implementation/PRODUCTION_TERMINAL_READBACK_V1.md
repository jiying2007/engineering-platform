# Production terminal Core readback v1

## Purpose and authority

`eng production-terminal-readback` reads an existing terminal-maintenance Closure
through the authenticated Control API, follows its original immutable identities,
and checks the complete existing Core relationship chain. The result is a
diagnostic with status `CORE_RELATIONSHIPS_VERIFIED`. It always emits
`qualification_granted: false`, `production_qualified: false`, and `ready: false`.

This reader recognizes the existing terminal-maintenance fixture in
`examples/production/terminal-maintenance/task.json.tmpl`, terminal plan v3 and
its three frozen evidence requirements. It does not create a new Task,
VerificationPlan, issuer, procedure, evidence acceptance path or production
authority. It makes no provider/model call, publication, deployment, database
mutation or acceptance decision. It does not turn a principal name into a real
human review. Tests use explicitly synthetic principals and evidence.

Related contracts:

- `docs/implementation/PRODUCTION_TERMINAL_ACCEPTANCE_V1.md`
- `docs/status/TERMINAL_BLOCKER_EVIDENCE_CONTRACT.md`
- `internal/production/acceptance.go` (`BuildTerminalPlan`)
- `internal/verification/verification.go` (`Evaluate`)
- `internal/api/server.go` (existing Review and Closure authority)

The proposed operational receipt shape in the blocker document remains a design
contract. This reader does not implement its acceptance parser or silently add
those requirements to an already frozen maintenance plan.

## Invocation

Use the ordinary authenticated Control client configuration:

```sh
export CONTROL_ENDPOINT=https://control.example:8443
export CONTROL_CLIENT_CERT_FILE=/private/control-reader/client.pem
export CONTROL_CLIENT_KEY_FILE=/private/control-reader/client.key
export CONTROL_SERVER_CA_FILE=/private/control-reader/ca.pem

eng production-terminal-readback \
  --closure EXISTING_CLOSURE_ID \
  --run EXISTING_RUN_ID \
  --subject sha256:EXACT_EXISTING_DELIVERY_SUBJECT_DIGEST
```

The IDs and digest must come from the actual retained Core execution. The example
contains placeholders, not usable evidence. The certificate's authenticated
principal requires `access.Read`. All requests use one existing direct mTLS
client, fixed endpoint and subject; the Control client rejects redirects and
does not use proxy environment variables. There is no local JSON input mode,
human-acceptance flag, qualification override or automatic repair option.

Exit zero means the entire requested Core chain was read and validated and the
diagnostic was emitted. It does **not** mean all production gates passed. Any
invalid argument, denied/missing source, malformed JSON, limit, cancellation,
digest mismatch or relationship mismatch fails with nonzero exit and emits no
diagnostic report. Printable errors omit raw transport errors, response bodies
and credential paths. An output-write failure also returns nonzero; as with any
stream, a failing output device may already have received a partial document.

## Historical read APIs

| Route | Response | Required capability |
| --- | --- | --- |
| `GET /api/v1/task-contracts/by-digest/{digest}` | `{ "contract": TaskContract, "digest": "sha256:…" }` | `access.Read` |
| `GET /api/v1/verification-plans/by-digest/{digest}` | `{ "plan": VerificationPlan, "digest": "sha256:…" }` | `access.Read` |

Both routes require a canonical lowercase SHA-256 digest, use the original HTTP
request context and the existing contextual Store lookup, and recompute the
returned immutable object's digest before responding. Bad digests are rejected
before Store access. Missing records retain the existing Store error mapping;
corrupt records produce a sanitized internal error. Cancellation before or during
the Store call cannot produce a successful record response.

The existing `GET /api/v1/task-contracts/{id}` still returns the latest revision.
It is deliberately not used for historical readback. No Store schema, migration
or new persistence authority is required.

## Read sequence and validation

The reader has one total deadline of 30 seconds, inherited cancellation, no
retries, and at most 12 bodyless GETs:

1. Read the requested Closure; require its exact requested Run and Delivery
   subject, `CLOSED` result and immutable task digest.
2. Read the Closure's Delivery; check its Work, Run and Task links, distinct base
   and result commits, nonempty unique artifacts, and recompute the Delivery
   subject digest from the immutable subject fields and artifact digests.
3. Read the historical Task **by the Closure's task digest**. Recompute its
   digest and check the Delivery's base, target and Work relationships.
4. Read the Task's VerificationPlan **by its frozen plan digest**. Recompute its
   digest, validate its acceptance criteria and exact existing maintenance
   requirement/procedure/issuer bindings, and check the Task's fixture scope.
5. Read the original Run with its Session and RunInputManifest. Require a
   completed Run, consistent Run/Task/epoch/control-owner links and a recomputed
   RunInputManifest digest.
6. Read the Work identity and owner. Work is mutable: current state, active Run
   and active Task are not historical lookup keys and may have advanced.
7. Read the exact Verification. Check its Delivery, subject and frozen plan
   links, verifier, timestamps and exactly three distinct evidence references.
8. Read each of those three Evidence records. Require distinct requirements,
   exact subject/Delivery/issuer/procedure bindings, applicable PASS results and
   nonempty, unique artifact references belonging to the Delivery. Reproduce
   the entire stored Verification with the existing `verification.Evaluate`.
9. Read the exact Review; apply existing Review validation and require PASS,
   exact Task/Delivery/Verification/subject links, ordered timestamps, and a
   reviewer different from both verifier and the read-back Work owner.
10. Read the same Closure again and require that immutable record to be
    unchanged before emitting any report.

Only the existing supported three-requirement terminal fixture is recognized.
A different terminal contract must be explicitly versioned; a similar task type
or a caller-supplied PASS flag does not broaden the reader's scope.

Accepted records are limited to 256 KiB each and 2 MiB in total; Delivery artifacts
are limited to 64. The underlying Control transport retains its independent
4 MiB response read bound. Every request checks the same reader subject before
and after transport, and every record uses strict JSON decoding (including
unknown/duplicate field rejection). The request IDs are bounded safe path
segments; source-provided references cannot redirect the reader to another path
or endpoint. The reader returns an empty report on any failure, including a
deadline that expires after the last HTTP response.

## Source retention

Every report contains the original API response bytes, the API resource path,
`source_digest` over those exact bytes, and `record_digest` over the strictly
decoded typed Core representation. `source_bytes` is a JSON base64 byte string:
pretty-printing the report preserves the exact original source bytes and digest.
It is not normalized JSON substituted for the response body.

The retained sources are Core records, including artifact **references**. The
reader does not fetch original GitHub events, provider receipts or artifact
content. Consequently `artifact_source_bytes_verified` is always false. Record
hashes support local integrity/review; they are not a signature, independent
attestation, trusted archive or evidence acceptance receipt. Store the complete
output using the deployment's existing private evidence-retention controls if a
retained audit record is needed.

`core_review_principals_differ` describes the checked stored principal
relationships only. It does not assert the original identity enrollment,
certificate possession at creation, personhood, human presence or an independent
human's actual approval. `human_presence_established` is always false.

## Complete 14-gate diagnostic coverage

All rows are emitted in the terminal plan's frozen order. `OBSERVED` describes
only the stated Core fact. It is not a production gate acceptance verdict.

| Frozen gate | v1 observation | What remains outside this reader |
| --- | --- | --- |
| `production_host_validated` | `NOT_OBSERVED` | Actual host identity, source/executable provenance and supported production baseline. |
| `operational_status_ready` | `NOT_OBSERVED` | Current authenticated operational state and its required readiness conditions. |
| `provider_live_qualified` | `NOT_OBSERVED` | Genuine live qualification for the exact selected unattended provider identity. |
| `publisher_independent` | `NOT_OBSERVED` | Genuine independent Publisher identity, permissions and execution boundary. |
| `exact_pr_head_ci` | `NOT_OBSERVED` | Original PR event, exact head and trusted CI artifact source revalidation. Accepted CI Evidence is only a retained reference here. |
| `verification_pass` | `OBSERVED` | The existing Core PASS Verification is reproduced from its exact frozen plan and three Evidence records. This does not add operational acceptance. |
| `independent_human_review_pass` | `NOT_OBSERVED` | A genuine independent person's acceptance of the exact intended subject and evidence. Distinct stored principals alone are insufficient. |
| `closure_created` | `OBSERVED` | The existing immutable Closure and its Core relationships are checked and reread. Closure does not grant the other twelve observations. |
| `shutdown_restart_recovery_proven` | `NOT_OBSERVED` | Actual shutdown/restart/recovery case evidence for the intended operational subject. |
| `source_verified_slo_evidence_accepted` | `NOT_OBSERVED` | Verified raw SLO sources and their acceptance under an authorized frozen operational requirement. A source-verified summary alone is insufficient. |
| `canary_deployment_accepted` | `NOT_OBSERVED` | Genuine canary execution and independent acceptance for the exact deployment. |
| `provider_emergency_disable_proven` | `NOT_OBSERVED` | Actual provider emergency-disable execution and observable outcome. |
| `publisher_revocation_proven` | `NOT_OBSERVED` | Actual Publisher credential revocation and observable outcome. |
| `database_rollback_restore_policy_accepted` | `NOT_OBSERVED` | Actual authoritative backup/restore/reconciliation evidence and policy acceptance. |

Operational acceptance must bind genuine observations to an appropriately
authorized and frozen operational subject without making maintenance Closure or
SLO summaries accept themselves. That contract and its genuine inputs are not
manufactured by this command. Provider/human/deployment blockers remain open
until their independent real evidence exists and the existing authority accepts
it through an explicitly supported contract.

## Verification scope

The component tests cover original-template positive readback with a newer
mutable Work, all 14 diagnostic rows, exact source-byte retention, frozen digest
and relationship tampering, duplicate/missing Evidence, wrong issuer/procedure,
foreign artifacts/subjects, non-independent review principals, malformed JSON,
identity change, changed immutable Closure, byte/count bounds, inherited
cancellation/deadline and sanitized errors.

API tests exercise real mTLS `access.Read` admission, denial before Store access,
historical Task/Plan versus latest Task lookup, request-context propagation,
corrupt Store responses and cancellation. The CLI test builds the real `eng`
executable and calls the real authenticated Core routes over mTLS with synthetic
in-memory history, including newer Work/Task state, denied access, wrong subject,
forged Evidence, unsafe IDs, rejected human-acceptance override and SIGTERM.
These tests establish implementation behavior; they do not establish production
provider, PostgreSQL, human, canary, recovery or emergency-stop qualification.
