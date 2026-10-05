# Implementation Status

This is the single live implementation-status authority. Detailed contracts are
linked below; historical checkpoints and receipts remain in Git/PR history, not
parallel live checklists. Audit baseline: `e4ca13e254c883d336ec990e1767f8cc85b8b98b`.

## Maturity boundary

**Trusted self-hosted M1 phase 1 has historical proof. Current repository
implementation and automated tests are not production/unattended qualification.**
The default internal lane remains trusted Ubuntu with a saved ChatGPT Codex
session. This project is independent of digital-worker and inherits no old
schema/runtime compatibility obligation.

[The fixed M1 proof](M1_RETAINED_PHASE1_CLOSURE_2026-09-29.md) records real Feature
#54 and Debug #55 closure. It does not qualify later runtime changes. Do not rerun
those subjects or rewrite their receipts to manufacture current production proof.

## Current contracts and limits

| Area | Implemented | Not implied |
| --- | --- | --- |
| Core | PostgreSQL business/audit/outbox, frozen Task/RunInput, execution and Recovery epochs | Production service operating acceptance |
| Live controls | mTLS RunControl, durable one-shot steering/interrupt, exact actor/sequence/epoch/turn binding and sealed transcript | ACK means stopped; accepted input means objective fulfilled |
| Process lifetime | Dedicated Linux user/PID namespace; receipt v4 and transcript v2 bind kernel-observed init reap | Complete filesystem/resource isolation or control of unrelated host processes |
| Stopped source | Private exact-byte archive after confirmed stop, immutable Core descriptor and new-directory restore | Full model session, automatic replay, successful Delivery or takeover authority |
| Source continuation | Work owner with RunStart + RunControl explicitly creates one fresh successor; old Run fenced as STOPPED_NO_DELIVERY | In-memory pause/resume, inherited execution permit or model-memory reconstruction |
| Finalization | Independent trusted-Git checkout reproduces complete source/tree before bundle publication | Clean Git status alone proves all source was delivered |
| Post-turn failures | Immutable fsynced phase records; catchable failures preserve stopped source; read-only local/Core reconciliation | Automatic abrupt-crash capture, repair or replay |
| Offline build outputs | Frozen output names/budgets, guarded tmpfs collection after child reap, existing local/Core receipt and Evidence byte checks | Writable host builds, large firmware, MCU/board qualification or unattended retention scheduling |
| Raw artifact sets | Explicit plans or producer-derived execution/Context/offline-output selection, byte packing/verification and fresh-directory restore | Complete per-Run dependency coverage, native database consistency or second-site backup |
| Provider | Profile v3 binds provider, credential, execution mode, binary and configuration identity | Admission mechanics equal live provider qualification |
| Compatibility | Schema3/contract2 probes actual Codex under engineering namespace/config; repeatable environment identity | Authentication, model/tool execution or production qualification |
| Publisher | Independent mTLS service; production startup rejects in-process publishing | Pilot-only local publisher is a second production lane |
| Distribution | One six-role list in `internal/distribution/binaries.txt`; build, verification and Evidence import use it | Checksums authenticate an untrusted download by themselves |
| Production preflight | v2 checks configuration and actual host facts with expected source SHA | CONFIG_VALIDATED or HOST_VALIDATED means operational READY |
| Operational status | v2 separates database authority from unobserved service readiness, checks exact response/freshness, pending ages/markers and bounded read-only progress windows | Inferred consumer/publisher health, capacity or production readiness |
| SLO | v2 separates unverified summaries from subject-bound source readback | Calibrated targets or production qualification |
| Pre-live | Event SHA/tree, terminal plan v2 and exact nonempty no-skip test inventory | Live provider/service acceptance |
| Operator CLI | Run controls, stopped-source restore/continuation, private execution readback and artifact-set storage | Completed WorkBuddy UX or Human Takeover |

## Execution, failure and continuation

[Live controls](../implementation/LIVE_CODEX_CONTROLS_V1.md) persist DISPATCHING
before provider effects. Ambiguous delivery is not replayed. Input acceptance,
interrupt ACK, matching terminal event and
[kernel stop](../implementation/PROCESS_NAMESPACE_CONTAINMENT_V1.md) are distinct
observations; Core Finish, Publisher and Evidence enforce their binding. No
plain-process or missing-proof fallback is admitted for current execution.

[Source preservation](../implementation/STOPPED_SOURCE_CHECKPOINT_V1.md) runs only
after sealed quiescence. The Preparer rechecks ownership, Task/input/preparation,
Git base/configuration and approved Context. Modified/untracked/ignored source
is preserved within bounded private archives. Core records a Worker attestation,
not a claim it downloaded private bytes. Restore requires external digest/Run,
a new private destination, full byte/mode/link/tree readback and fsync; it never
imports .git/HOME/login state or grants execution.

The actual Worker records immutable ENTERED phases before post-turn persistence,
Finalize and reporting. Catchable later failures attempt private source capture
and one immutable checkpoint registration. Preservation-only Git inspection
allows the original base or one direct Finalize child; normal execution/reopen
retains its original-base fence. Success makes no unsolicited source copy.
Local FAILED_UNCONFIRMED cannot override an already committed Core FINISHED.
Abrupt Worker/host death and exhausted storage may prevent capture/journaling;
phase entry alone is not phase completion or proof of a crash.

`eng execution-readback` validates existing private permit/phase/turn/result/
checkpoint identities. Explicit artifact paths opt into source/bundle byte
checks; metadata paths are never followed. Offline default does not contact Core.
Optional --core makes one authenticated GET, validating token, sealed transcript,
result and checkpoint. Descriptor observation is not a private byte download.
No source/model/steering bodies are printed and no repair/replay is authorized.

[Continuation](../implementation/SOURCE_CONTINUATION_V1.md) requires Work owner,
both permissions, exact versions/epochs/checkpoint, observed requested
interruption, and no unresolved effects or prior Delivery. One transaction
retires the old Run, creates one new Run/Attempt/Session and emits normal
run.started intent. Concurrent decisions share the source fence; exact retry
observes the original decision. Generic Run creation cannot inject lineage.
A completed-turn failure checkpoint is not an interruption-only continuation.

Fresh host approval supplies private source to a new slot at the original Task
base. A NEW model turn gets explicit continuation context, not invented memory.
The final diff contains inherited and new representable changes. Independent
checkout rejects ignored-file/empty-directory loss and attribute transformation;
it neither force-adds private files nor deletes them. Failed Finalize may leave
a local commit: reconcile the failed execution instead of blindly retrying it.

## Private artifacts and historical evidence

[Private artifact sets](../implementation/PRIVATE_ARTIFACT_SET_V1.md) preserve an
explicit sorted member list with raw plan/archive digest and Run anchoring.
Files remain private and non-executable. Packing/readback/restore never calls
Core, runs a model/Git/SQL, overwrites old destinations or uploads public data.
Generic completeness is ONLY relative to declared members; producer semantics
are not inferred. `capture-execution` derives current producer records and every
frozen Context entry from the original anchored permit, requiring all referenced
source/result bytes rather than relying on a hand-written list. It rejects
missing/extra Context and unbound artifacts. It does not capture the original Git
base, toolchain, full control history or upstream continuation dependencies.
`capture-offline` derives the original permit/report and every collected output
from the frozen offline Profile. Both raw records require external digest anchors;
failed commands retain failure records with zero outputs, never successful-build
claims. It does not contact Core or execute restored data. Caller-selected native
backups still need their own consistency/restore.

Actual Worker tests delete original records/source/bundle locations, restore the
raw set and re-run existing semantic readback. A host C-compiler reference keeps
ELF/map outside source and restores outputs after removing both original source
and build directories. The existing [offline Runtime](../implementation/OFFLINE_EXECUTION_V1.md)
also supports an explicitly frozen bounded build-output contract: the non-root
PID1 guard collects exact files in private tmpfs after reaping children, and the
existing Worker/Core/Evidence path binds their bytes. A real scratch-container C
compiler and private artifact restore are covered, but no MCU/hardware or larger
firmware capacity is qualified. A same-Run compiled Worker/mTLS/PostgreSQL/native
C build additionally captures and restores its original records and raw outputs
after deleting source/Git/preparation roots, without creating Evidence or new
execution. Producer-derived capture tests remove the entire prepared root
including Context, restore, and recheck every frozen input object and producer
record. Original Git base/toolchain, full model/tool/control history, second-site
retention, encryption/key policy, scheduling and GC remain open.

The mandatory PostgreSQL 17 drill additionally retains a native dump and expected
authority snapshot through the raw set, removes and checks absence of the original
DB/dump/plan, restores bytes, then explicitly runs native transactional restore.
All existing authority, audit, Outbox and Recovery-epoch checks still apply. This
is a controlled database/container fixture, not a customer backup, production or
cross-version qualification; the storage layer itself never executes SQL.

[Two historical terminal fact archives](../evidence/m1-terminal-facts/README.md)
retain selected original bytes/source ZIP identities in Git. Final Review uses
allowlisted facts, not PKI/dumps/environment/log directories; verification deletes
its disposable bootstrap PKI after Control stops. Required intermediate database
transport is separate. Historical schemas and decisions remain historical proof.
Neither terminal facts nor source checkpoints are full runtime backups.

[Branch disposition](../reviews/BRANCH_DISPOSITION_2026-10-04.md) separates
integrated refs, unmatched prototypes and retained M1 subjects. A manifest is not
a deletion receipt; only atomic SHA-guarded execution and readback prove cleanup.
Zero open PRs does not mean every ref is governed. Preserve unchanged:

- Feature `6009ea95785237ad6ff9f5c9cba911b4891dfa58`,
  `engineering-platform/3ac04fc7097e8375e5f7c1f8`.
- Debug `91d7d9fa068b7667bab5c211f13cd9e0151aeb92`,
  `engineering-platform/994964383ed085e51545105d`.

## Provider and production qualification

[Credential lanes](../implementation/CODEX_CREDENTIAL_LANES_V1.md) remain
`openai-codex / chatgpt-session / trusted-self-hosted` and
`openai-codex / workload-identity / unattended`. No relay identity or silent
credential/provider fallback is admitted. WIF #103 and relay #106 live
qualification remain deferred; speculative relay tooling stays frozen.

Qualification schema3/contract2 launches the native binary in the actual user/PID
namespace, exact workspace-write config and fresh credential-free HOME. Only
initialize/initialized/thread-start are sent, followed by confirmed init reap.
The stable kernel/architecture/UID/GID digest excludes PID/inode/time. CI probes
twice; each actual Worker CLI requalifies and compares the entire receipt. Old
current-admission receipts fail, but historical M1 records are not rewritten.

**Production #105 stays open.** Terminal plan v2 requires host validation and
accepted source-verified SLO evidence, not old READY/COMPLETE labels. Provider-free
dry runs are distinct from real qualification. Completion still needs one
qualified unattended lane, real lifecycle/recovery measurements, exact delivery,
calibrated SLO targets and independent human Review/Closure. Local protocol
fixtures are not live model evidence; real kernel/PostgreSQL/container tests
cannot promote them into account/provider/device qualification.

## Remaining approved RC work

W01-W10 remain the approved scope, not a new authority/checklist framework.

- W01: bounded read-only queue-progress windows use existing authenticated
  snapshots and explicit diagnostic thresholds; component heartbeat/health/
  capacity and calibrated operating SLOs remain open.
- W02: account-free isolated startup implemented; actual auth/model/tool and
  deployment-environment acceptance remain external gates.
- W03: catchable failure preservation/readback implemented; abrupt-crash capture,
  disk-exhaustion recovery and approved repair are still open.
- W04/W05: bounded offline build-output contracts and raw-set/context capture
  implemented; larger build profiles, automatic offline-output capture, full Run
  dependencies and durable private retention/production-native acceptance remain open.
- W06: complete fresh-host installation, upgrade/migration and recovery.
- W07/W08: authorized normal user journey/WorkBuddy, controlled Human Takeover,
  and representative board-free embedded build/evidence scenarios.
- W09/W10: fault/load/security regression, exact RC delivery, unmatched prototype
  disposition, evidence-ref protection and working-ref governance.

Every slice needs exact-head CI, fresh-main and delivered-byte readback. Earlier
green runs do not qualify new source. External accounts, production hosts,
devices and human decisions remain separate gates; no local implementation can
stand in for those decisions. Prioritize actual user journeys and reproducible
delivery rather than parallel authorities, speculative tooling or more states.
