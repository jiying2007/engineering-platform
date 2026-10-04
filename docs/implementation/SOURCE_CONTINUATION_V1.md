# Explicit stopped-source continuation v1

This is **source continuation in a new Run**, not automatic replay, a paused
process, model-memory restoration, or Human Takeover. It extends the existing
Work/Task/RunInput/Session, Worker preparation and Codex execution chain. No new
provider lane, daemon, external service, tool grant or credential is introduced.

## Decision boundary

Only the authenticated Work's `human_owner` with **both `run:start` and
`run:control`** may authorize continuation. Human owner must be the actual mTLS
principal URI, not a display name. The decision binds an existing stopped-source
checkpoint descriptor digest and exact Work version, Run version, execution epoch
and Recovery epoch, plus new Run/Attempt IDs and a bounded human reason.

The source must still be the Work's active RUNNING/RUNTIME Run, with unchanged
frozen Task, input, Profile and Session identity. Its Codex execution must have no
successful receipt, and must already be UNKNOWN with a **sealed, unambiguous,
explicitly requested `TURN_INTERRUPTED`** and kernel-confirmed descendant reap.
Every dispatched steering input must have an accepted result. Queued but never
dispatched inputs are retained as NOT_APPLIED; their text is **not automatically
replayed**. The owner must review them and the checkpoint before deciding what
belongs in the new turn. Missing stop proof, mere cancel ACK, missing control
reply, failed/unconfirmed seal, generic failure and active model calls are not
eligible. No unresolved external operation, existing Delivery or offline
execution may share the source Run.

A checkpoint digest recorded by Core remains a Worker attestation. The API does
not claim to have fetched private host bytes or that an operator has completed a
human Review. Actual bytes must be independently read back by the new Preparer.

## Atomic authority transition

Migration 0011 stores one immutable continuation receipt per source Run, with a
unique successor. The decision transaction locks existing authority rows, then:

1. Preserves the old input, exact control transcript and checkpoint bytes.
2. Marks the old Run ABORTED and increases its execution epoch and version;
   pauses/fences the old Session and ends its attempt without a successful result.
3. Records the observed execution disposition `STOPPED_NO_DELIVERY`. This is an
   explicit reconciliation of a **proven requested stop**, never FINISHED and
   never an assertion that an ambiguous provider call completed successfully.
4. Creates a new Run/Attempt/Session and a new frozen RunInput containing the
   checkpoint identity. Task, original Git base, Context and profiles stay fixed.
5. Atomically changes the Work's active Run, appends audit, and creates one normal
   `run.started` outbox intent. It does not start a model itself.

An exact retry from the same actor reads the original receipt; changed input or
another successor is rejected. The ordinary Run creation path refuses a
continuation reference, preventing bypass of the source decision. Late external
operation creation shares a source-Run lock and checks the retired source before
inserting: an action arriving after pre-transaction authorization cannot cross
the continuation fence. No automatic POST, provider or model retry is added.

New execution remains behind normal inbox, Worker capability, preparation,
Profile, lease, Recovery and control-transcript gates. An old model token cannot
renew, restart, finish or publish the retired source. A later interruption may
produce another explicit source-continuation decision on its own new Run; it
cannot branch the original decision or erase earlier failure history.

## Host preparation and complete change history

The new RunInput requires a **new exact host approval**. Existing approval does
not authorize the successor. Its private `continuation_archive` path is supplied
only by the host operator; no locator is taken from a model message or the API.

Preparer independently verifies the raw archive digest, descriptor digest, source
Run, Task, Profile and original base, then restores into a fresh owned slot.
It retains the fresh checkout's original `.git`, never imports archive Git
metadata, HOME or a model session. The inherited edits become a separately bound
initial source snapshot, **without a Git commit that could hide those edits**.
Reopen/pre-execution checks require the same seed and context; later file changes
fail that check. Finalization records all changes relative to the original Task
base, so inherited unfinished work remains within new Delivery and Verification.
Git's existing ignore policy still governs the result commit: retained ignored
files are source preservation, not an instruction to publish secrets/build output.

Capture/restore bounds remain those of the stopped source archive (10,000 entries,
64 MiB/file, 256 MiB source). Oversized or unsafe workspaces fail, not truncate.
The host remains trusted; this is not protection against privileged concurrent
host modification. The new prompt identifies the source Run and checkpoint,
states it is a fresh model turn, and requires rechecking all acceptance criteria.
No old steering text, model memory or unverified provider session is auto-imported.

## Operator flow

Use the existing direct-mTLS client configuration. Inspect the current source
and Work, then verify the private checkpoint on the owning host:

```sh
eng run-control inspect --run SOURCE_RUN
eng api GET /api/v1/runs/SOURCE_RUN
eng api GET /api/v1/work-items/WORK_ID
eng source-checkpoint verify --archive /private/artifacts/EXECUTION.source-checkpoint.tar \
  --digest sha256:EXPECTED_ARCHIVE_DIGEST --run SOURCE_RUN
```

Prepare an owner-private JSON decision file, using `descriptor_digest` from verified `source-checkpoint` output
anchored to the authenticated Core archive hash (not the archive hash in its place):

```json
{
  "source_run_id": "SOURCE_RUN",
  "run_id": "NEW_RUN",
  "attempt_id": "NEW_ATTEMPT",
  "checkpoint_digest": "sha256:EXPECTED_DESCRIPTOR_DIGEST",
  "expected_run_version": 1,
  "expected_work_version": 3,
  "execution_epoch": 1,
  "recovery_epoch": 0,
  "reason": "Continue the reviewed source in a new execution; retain the old stop"
}
```

IDs, digests and versions above are placeholders, not an executable approval.
Use actual current values, `chmod 600 decision.json`, then:

```sh
eng run-continue authorize --request /private/decision.json
eng run-continue status --run SOURCE_RUN
```

The response includes the immutable new `run_input`; use its exact digest to add
a new entry to the existing host-owned preparation configuration. Preserve the
normal fields and approved Context; add `continuation_archive` with the existing
private archive path. Execute the successor through the ordinary Worker flow.
The CLI does not edit host grants, start models or supply an `--actor` override.
On a lost POST response, **read status by the source Run before any exact manual
retry**. Never change the successor ID to hide an uncertain decision.

## Validation and remaining limits

Tests include private CLI input and mTLS identity/response binding, receipt
immutability, old-token fences, concurrent decisions, concurrent late external
operations, generic-route rejection, missing host approval, wrong archive/Task/
Profile, seed tampering, unchanged Git base and full inherited diff. The two-turn
integration uses actual PostgreSQL, mTLS, Preparer, Worker, namespace processes,
private archive and Git bundle. Its model endpoint is an offline local protocol
fixture: those tests are not live provider/account evidence.

WorkBuddy UX, controlled ownership transfer/Human Takeover, in-process pause or
model-memory resume, complete raw engineering artifact retention, remaining ref
governance and live runtime/provider/production-host/SLO/device qualification
remain separate. This capability grants no production readiness, does not close
#105, and does not rewrite or rerun historical M1 pilots.
