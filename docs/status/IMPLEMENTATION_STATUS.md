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
| Worker | mTLS admission/preparation, independent workspace, one non-replayable Core-bound engineering turn with actual steering/interrupt delivery and sealed control transcript | Kernel-backed stop, private checkpoints and explicit new-Run source continuation are implemented; model-memory resume and Human Takeover remain separate |
| Provider | Profile v3 binds provider, credential, execution mode and configuration digest | WIF #103 and relay #106 live qualification remain deferred |
| Publisher | Independent mTLS service; production startup rejects in-process publisher | Explicit pilot-only local publisher remains for historical pilot tooling |
| Delivery | One six-executable contract in `internal/distribution/binaries.txt`; CI and evidence import use that contract | Archive authenticity still requires the authenticated distribution digest |
| Distribution verification | Delivered `eng distribution-verify` checks exact files, checksums, build roles and one clean source revision; no rebuild fallback | Byte consistency is not deployment or production qualification |
| Production preflight | v2 binds expected source SHA; default checks actual host facts and references | CONFIG_VALIDATED and HOST_VALIDATED are not operational READY |
| Operational status | Authenticated PostgreSQL snapshot for recovery/leases/outbox/actions/Codex UNKNOWN | Does not invent publisher reachability or provider health |
| SLO | v2 distinguishes unverified summaries from content-addressed, subject-bound source readback | Neither state grants provider or production qualification; real provenance/targets require Verification/Review |
| CLI | `eng run-control`, `source-checkpoint` and `run-continue` expose explicit controls, source readback and new-Run decisions | Operator-oriented CLI is not a completed WorkBuddy UX |

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
model-memory resume, Human Takeover and WorkBuddy UX still require independent tested closeout. Source checkpoint/continuation is not full model-session restoration.
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
API rejects Codex Runs. Bare process exit is insufficient; receipt v4/transcript v2
now require the separately described kernel namespace-reap proof.

The real local subprocess used in integration tests is a fake provider protocol,
not an actual account or model. Live provider qualification, in-process pause/model-memory resume and
Human Takeover remain unproven. This is a bounded working control
slice, not a declaration of complete production or interactive maturity.

## Process-tree lifecycle

Core-bound engineering now requires a dedicated user/PID namespace and binds
its actual init-reap proof to receipt schema 4 and transcript v2. No missing
namespace or process-group fallback is admitted. The local account-free probe
and kernel tests cover this boundary; full pause/model-memory resume/takeover and
real-provider compatibility are not inferred from those tests. See
[process containment](../implementation/PROCESS_NAMESPACE_CONTAINMENT_V1.md).

## Stopped source checkpoints

The actual Worker now attempts bounded private source capture after an unsuccessful
turn only when Core sealed the exact quiescent runtime transcript. Preparation
ownership, base/config and approved Context are rechecked before capturing changed,
untracked and ignored source bytes. Migration 0010 records an immutable descriptor
and exact retry/readback; this never promotes the execution to FINISHED.

`eng source-checkpoint verify|restore` checks the externally anchored archive and
Run identity and restores only to a fresh private directory. It launches no model
and grants no execution or takeover permission. Local raw bytes survive a failed
registration reply without a blind retry. The archive is source-only, private,
and not automatically uploaded: full model/binary/Git/database artifact retention
and genuine pause/resume/controlled takeover remain separate unfinished work.

The detailed contract is [Stopped source checkpoint v1](../implementation/STOPPED_SOURCE_CHECKPOINT_V1.md).

## Explicit source continuation

[Stopped-source continuation](../implementation/SOURCE_CONTINUATION_V1.md) now
connects a proven requested stop to a new authorized Run. The mTLS Work owner
must hold both RunStart and RunControl and bind exact versions/epochs/checkpoint.
One atomic decision fences the old Run, records STOPPED_NO_DELIVERY without
changing its transcript, creates one successor and normal outbox intent. Lost
control replies and unresolved effects remain ineligible; exact retries only
observe the original decision. Generic Run creation cannot inject this lineage.

A new exact host approval supplies the private archive. Preparer verifies raw
bytes, restores an owned fresh slot, binds its seed and preserves original Git
base/configuration. Normal Worker authorization launches a NEW model turn;
final diff includes inherited unfinished work. This is not automatic replay,
model-memory/session resume, Human Takeover or live provider qualification.
