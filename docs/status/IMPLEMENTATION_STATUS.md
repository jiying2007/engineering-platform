# Implementation Status

Audit baseline: `e4ca13e254c883d336ec990e1767f8cc85b8b98b`.
This file is the single live status authority. Historical CI checkpoints remain
in Git history and the retained M1 proof record, not parallel live checklists.

## Maturity boundary

**Trusted self-hosted M1 phase 1 is proven. Production/unattended operation is
not qualified.** The default internal lane remains trusted Ubuntu with a saved
ChatGPT Codex session. This project is independent of digital-worker; no old
schema/runtime compatibility obligation is inherited.

The fixed historical proof is
[M1 retained phase-1 closure](M1_RETAINED_PHASE1_CLOSURE_2026-09-29.md): Feature
#54 and Debug #55 have separate real retained Closure chains. They must not be
rerun to manufacture production or alternate-provider evidence.

## Current code contracts

| Area | Current implementation | Boundary |
| --- | --- | --- |
| Core | PostgreSQL business/audit/outbox, frozen Task/RunInput, epochs and Recovery | Live service operating acceptance remains separate |
| Worker | mTLS admission/preparation, independent workspace, one non-replayable Core-bound engineering turn with actual steering/interrupt delivery and sealed control transcript | Durable pause/resume, quiescent checkpoints and Human Takeover remain a required next slice |
| Provider | Profile v3 binds provider, credential, execution mode and configuration digest | WIF #103 and relay #106 live qualification remain deferred |
| Publisher | Independent mTLS service; production startup rejects in-process publisher | Explicit pilot-only local publisher remains for historical pilot tooling |
| Delivery | One six-executable contract in `internal/distribution/binaries.txt`; CI and evidence import use that contract | Archive authenticity still requires the authenticated distribution digest |
| Distribution verification | Delivered `eng distribution-verify` checks exact files, checksums, build roles and one clean source revision; no rebuild fallback | Byte consistency is not deployment or production qualification |
| Production preflight | v2 binds expected source SHA; default checks actual host facts and references | CONFIG_VALIDATED and HOST_VALIDATED are not operational READY |
| Operational status | Authenticated PostgreSQL snapshot for recovery/leases/outbox/actions/Codex UNKNOWN | Does not invent publisher reachability or provider health |
| SLO | v2 distinguishes unverified summaries from content-addressed, subject-bound source readback | Neither state grants provider or production qualification; real provenance/targets require Verification/Review |
| CLI | `eng run-control` submits exact live inputs and reads receipts; unknown/missing commands fail nonzero | Operator-oriented CLI is not a completed WorkBuddy UX |

## Production terminal acceptance

Issue #105 remains open. Repository changes are not a production decision.
Terminal plan schema v2 keeps the one-time RELEASE maintenance fixture but
requires `production_host_validated` and
`source_verified_slo_evidence_accepted`, rather than ambiguous READY/COMPLETE
labels. Retain a fresh v2 provider-free dry run after canonical CI succeeds;
the earlier PRE_LIVE_COMPLETE v1 artifact is historical only.

Actual completion still requires an independently qualified unattended provider,
service lifecycle/recovery evidence, exact-head delivery/CI evidence, calibrated
SLO targets, independent human Review and Closure. No mock, synthetic source
record, green CI run or offline archive verification can replace those gates.

## Provider/credential posture

[Codex credential lanes](../implementation/CODEX_CREDENTIAL_LANES_V1.md) remain
canonical. Admitted code combinations are `openai-codex / chatgpt-session /
trusted-self-hosted` and `openai-codex / workload-identity / unattended`.
Admission mechanics are not live qualification of WIF. No relay identity or
silent credential/provider fallback is admitted. Existing relay prequalification
assets are frozen until a concrete live provider contract is supplied.

## Retained terminal facts

Terminal review uploads now contain only a bounded allowlisted fact archive,
not the disposable stack, PKI, environment files, logs or database dumps.
Verification destroys its bootstrap-only PKI after stopping Control; downstream
Review already creates fresh identities. Intermediate database/result-artifact
transport remains separate from the terminal fact archive.

[Two fixed historical fact archives](../evidence/m1-terminal-facts/README.md)
retain original member bytes from the actual Feature/Debug review artifacts and
are independently checked in canonical CI. Source ZIP IDs/digests remain bound;
historical schema bytes, review decisions and result refs are not rewritten.
These small Git-retained records outlive Actions retention, but are explicitly
not full runtime backups: original model output, binaries and Git bundle bytes
still need their own retained artifact policy. No production claim follows.

## Remaining remediation

Full engineering-artifact retention/restore, historical branch disposition and
full interactive pause/resume/checkpoint/takeover still require independent tested closeout.
The two retained M1 result refs are evidence subjects, not cleanup candidates.
Do not equate zero open PRs with repository hygiene or delivery maturity.

Historical immutable result subjects:

- Feature: `6009ea95785237ad6ff9f5c9cba911b4891dfa58`,
  `engineering-platform/3ac04fc7097e8375e5f7c1f8`.
- Debug: `91d7d9fa068b7667bab5c211f13cd9e0151aeb92`,
  `engineering-platform/994964383ed085e51545105d`.

The October 4 audit found non-provider-blocked implementation gaps. The current
priority is delivery correctness, explicit validation semantics, safe retained
evidence and the real engineering interaction path, not additional speculative
qualification tooling or architecture expansion.

## Live control implementation boundary

[Core-bound live Codex controls](../implementation/LIVE_CODEX_CONTROLS_V1.md)
connects real steering text and interrupt requests from RunControl through
PostgreSQL, the Worker and app-server RPC. Exact sequence/epoch/lease/turn binding,
persist-before-dispatch, no UNKNOWN replay, ACK-versus-terminal observations and
sealed transcript binding are enforced. New result publication and Evidence
import require that transcript. The existing metadata-only pause/resume/takeover
API rejects Codex Runs; process exit is not proof of descendant quiescence.

The real local subprocess used in integration tests is a fake provider protocol,
not an actual account or model. Live provider qualification, pause/resume and
quiescent checkpoint/takeover remain unproven. This is a bounded working control
slice, not a declaration of complete production or interactive maturity.
