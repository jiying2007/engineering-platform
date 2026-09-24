# Implementation Status

Stage: **Authenticated Core + durable Worker + actual approved Context/workspace preparation; no real M1 pilot**
Reviewed base: `71a4877e2eb4973196cd612852fba19277919197`.

## Scope and assembly

Core architecture, Embedded Domain Capability Model, Core M0/M1 plan and
ADR-001/002/003 remain canonical. Independent of digital-worker; no new domain
or service is added. WorkBuddy remains a future authenticated front door.

| Area | Implemented | Remaining acceptance boundary |
| --- | --- | --- |
| Core | Work/Task/frozen VerificationPlan and RunInput, epoch/CAS, Session/Steering/Checkpoint, Delivery/Evidence/Verification/Closure | Memory deep-copy/conformance; independent real evidence and Review |
| PostgreSQL | Core/action persistence, business + audit + outbox; Worker inbox and separately attributed preparation receipt transactions | Operational restore/recovery drills; no source merge runs production migration |
| Outbox / Worker admission | Actual Control Plane atomic relay; certificate/profile-bound claims, bounded renewal, generation/recovery fences and deterministic input receipts | Input receipt is not execution authority |
| mTLS API / clients | Explicit URI identity and capabilities, bound actors/issuers, actual Worker and eng API, no ambient proxy/redirect/insecure fallback | Single administrative trust domain; credential lifecycle and Work-level ACL |
| Worker preparation (this change) | Actual prepare-only command: exact operator approval, local Context byte verification, independent exact-base workspace, periodic lease renewal, recheck, same-transaction preparation/input/audit and CLI readback | Worker filesystem attestation, not server byte observation; no model execution or OS sandbox |
| Workspace (this change) | Independent fresh Git objects instead of linked worktrees; explicit sanitized local Git, owner-fenced slots, source/config byte checks without mutable Git execution, bounded cancellation | Trusted Git/source metadata and host parents; disk quota/retention and separate OS isolation |
| Context | Structured refs, raw-byte hash, materialize/Verify plus exact operator LocalSource; now consumed by preparation Worker | Startup approval snapshot only; online lifecycle and future sandbox mount |
| Codex Runtime | Bounded correlated JSONL, isolated environment, typed initialize/thread/turn/steer/interrupt and deny-only approvals | Pinned real binary/schema qualification; live Core execution authority and approval grants |
| Action / recovery | Durable action ledger and contract/epoch/recovery components; authenticated grants and independent completion gate | Real provider/effect boundary and concrete trusted recovery completion verifier absent |
| Embedded Skills | Six capabilities/eight defined Skills; explicit material/routing | No retained Feature/Debug pilot; no PILOTED/PROVEN claim |

The preparation receipt is WORKER_ATTESTED_PREPARATION. execution_started and
os_isolated must be false. Its nested INPUT_VALIDATED receipt keeps its original
meaning; no Context-byte or execution assertions are silently promoted there.
No preparation transition completes a Run or fabricates engineering Evidence.
Use dedicated preparation profiles to avoid input-only workers consuming them.

## Retained checkpoints and new verification

- #29 Context: PR `35998634850`; main `35998821289`.
- #30 outbox: PR `36003408959`; main `36003703669`.
- #31 authentication: PR `36007219869`; main `36007507189`.
- #32 prepared protocol: final head `c995062b477c45c8e0da311a179c0fb61efad431`,
  PR CI `36023841471`; offline helper is not real Codex qualification.
- #33 admission: main `71a4877e2eb4973196cd612852fba19277919197`,
  PR CI `36031414023`, fresh main CI `36031860646`, all passed.
- This preparation change requires its exact final PR CI and fresh main CI.
  No earlier green checkpoint or local formatting substitutes for that proof.

Existing full Go 1.25/PostgreSQL 17 race tests, outbox repetitions, ten Runtime/
Context repetitions and Worker admission/client/command repetitions remain.
Added tests cover actual prepared bytes/workspace through compiled Worker/eng,
real mTLS and an isolated schema with the live Control Plane relay; dedicated
permission rejection; hash/approval/tamper failures; config/hook isolation;
concurrent ownership/reports; audit rollback; final expiry after journal waits;
migration replay and no false execution/Evidence. No shared public schema reset.

## Next delivery gates

1. OS-isolated supervised execution using a fresh Core authorization, rechecked
   approved Context/workspace and actual execution receipts. Preparation history
   must not grant a stale worker permission to launch later.
2. Pinned real Codex binary/schema qualification, authorized interactive approval
   integration and current Action/reconciliation authority at the effect boundary.
3. Actual Git/CI/Artifact-byte facts and independent Review, memory-store
   conformance, retained reconciliation and operational restore/recovery drills.
4. Real Feature/Debug pilots before assessing M1. No new extension domains.

The old local Worker ZIP is superseded by #33. The new workspace recipe does not
adopt old linked-worktree slots: drain and reconcile them explicitly. Preparation
requires explicit version-4 migration, operator source/approval/configuration and
a dedicated credential/profile. No production deployment or credential is created
by merging these files. **M1 and production readiness remain unclaimed.**

Details: `docs/implementation/WORKER_PREPARATION_V1.md` and the retained
WORKER_ADMISSION_V1 / PREPARED_RUNTIME_PROTOCOL_V1 / AUTHENTICATED_CONTROL_API_V1 /
OUTBOX_DISPATCH_AUTHORITY_V2 / CONTEXT_MATERIALIZATION_V1 contracts.
