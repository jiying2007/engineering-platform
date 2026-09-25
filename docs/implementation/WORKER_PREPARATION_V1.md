# Worker preparation v1

Reviewed base: `71a4877e2eb4973196cd612852fba19277919197` (Worker admission #33).
This joins the existing Worker lease, approved Context materializer and Workspace
into a command-level preparation path. No new service, domain or provider is added.

## Executable path

```
authenticated Work/Task/Run -> existing atomic outbox/inbox relay
 -> worker --prepare-only claims under a dedicated grant
 -> exact operator approval + trusted local Git snapshot
 -> bounded Context byte resolution / hash verification
 -> live lease renewal + recheck
 -> atomic preparation attestation + input receipt + audit
 -> eng api GET /api/v1/runs/<run>/preparation
```

The preparation lane deliberately stops before an agent/model/build invocation.
It does not complete Run, create Delivery/Evidence/Verification/Closure, approve
external actions, or turn a self-reported boolean into execution permission.

## Roles and truth boundaries

- Control Plane checks authenticated subject/profile, frozen intent/task/input,
  current Run/Session, recovery epoch, exact lease generation and database expiry.
- Worker actually fetches the approved exact local commit, hashes checkout bytes,
  materializes approved Context bytes and runs Verify again before reporting.
- The remote record is named `WORKER_ATTESTED_PREPARATION`. The worker is the
  attester for filesystem observations; the server rechecks identities and the
  deterministic Context manifest hash but cannot observe remote filesystem bytes.
- `execution_started` and `os_isolated` MUST be false in this version. The existing
  `INPUT_VALIDATED` subreceipt retains `context_bytes_verified=false`; the stronger
  preparation record is separately attributed, not silently substituted for it.

A preparation receipt is historical evidence of preparation, not a lease to use
that directory later. A future executor must acquire fresh Core authority, verify
current approvals and bytes, enforce OS isolation, and produce its own execution
receipt. The prepared Codex adapter is not invoked by this lane.

## Independent Workspace recipe

`independent-git-snapshot-v1` replaces linked-worktree creation. The public
Workspace API remains, but each owned slot now has an independent `.git` object
database, detached exact HEAD, separate Runtime HOME and separate Git HOME.
The host fetches an exact 40-hex commit from an operator-approved local repository
into a fresh repository with an empty template; no local-clone hardlinks,
alternates, shared worktree metadata or source checkout configuration are copied.
Submodule gitlinks fail closed rather than producing an incomplete checkout.

The host-selected absolute Git executable runs with a constructed environment,
no inherited GIT_DIR/GIT_CONFIG parameters/proxies/secrets, no global/system Git
configuration, no checkout hooks/filters/fsmonitor, and only local file transport.
Git command output and time are bounded; Linux process-group cancellation prevents
an abandoned fetch child. Non-Linux execution is rejected by this recipe.

After checkout, clean checks do NOT execute Git in mutable repository config.
They check the ownership record, detached HEAD and config digest, reject shared
metadata/hooks, and recompute sorted source byte/type/executable/symlink-text
snapshots. Untracked files are included; symlinks are hashed without following.
Limits are 100,000 entries, nesting 64, 256 MiB/file and 1 GiB hashed content.
These limits do not cap Git's temporary pack/disk allocation: deploy the preparation
root with filesystem quota and reserve sufficient space. No resource sandbox is
claimed. Source `.git`, source repository, Git binary and root parents must be
operator-controlled; this is not a parser sandbox for hostile Git metadata.

Root/slot traversal is anchored by os.Root, canonical-directory identity checks
and a random owner record. Cleanup removes only the owned slot without invoking
Git or touching the original source checkout. Old linked slots are NOT silently
adopted or deleted by the new manager; drain old workers and preserve/clean their
old slots through an explicitly reviewed operator procedure before upgrade.
The parent tree and same-UID host remain trusted: this is not protection against
malicious owner processes, mounts, kernel compromise or arbitrary executable code.

## Operator configuration and permission

All standard mTLS Worker client variables from WORKER_ADMISSION_V1 still apply.
The worker additionally requires an operator-owned JSON file:

```
WORKER_PREPARATION_CONFIG=/absolute/operator/worker-preparation.json
worker --prepare-only --profile worker/preparation --once
```

`--admission-only` and `--prepare-only` are mutually exclusive; admission mode
rejects a preparation configuration rather than silently ignoring it. The Worker
still rejects DATABASE_URL. Its certificate subject must match the config subject.

Configuration schema (values are illustrative placeholders, not valid hashes):

```json
{
  "version": 1,
  "worker_subject": "urn:engineering-platform:worker:preparer",
  "root": "/srv/engineering/preparation",
  "git_executable": "/usr/bin/git",
  "context_source": "/srv/engineering/context-source",
  "approvals": [{
    "run_id": "<exact-run-id>",
    "task_contract_digest": "sha256:<exact-64-lowercase-hex>",
    "run_input_manifest_digest": "sha256:<exact-64-lowercase-hex>",
    "repository": "<logical-repository-from-task>",
    "repository_path": "/srv/engineering/sources/approved-repo",
    "context_refs": []
  }]
}
```

The root and content source must already exist, be canonical and not shared
writable. Approvals list the exact ordered ContextRefs, never wildcard sources.
Source files are the existing `<64-digest-hex>.bin` readonly content-addressed
format. Approval digest excludes machine locators; they are only local inputs.
The local snapshot is loaded at startup, not an online approval/ revocation UI.
The profile must be dedicated to preparation workers: input-only workers on the
same profile can otherwise legitimately consume that input first.

Add `worker:prepare` in addition to `worker:poll` / `worker:report` and explicit
`worker_profiles`. Missing companion grants are rejected. The new endpoints are
only registered in the authenticated API:

- POST `/api/v1/worker/prepare-claim`: verifies migration readiness BEFORE claim.
- POST `/api/v1/worker/prepared`: exact profile and preparation grant required.
- GET `/api/v1/runs/{id}/preparation`: existing platform read capability.

Single administrative trust domain limitations remain; no tenant/Work-row ACL.
The preparation worker credential must only be issued to a host authorized to
read the approved local repositories and context source.

## Lease, persistence and failure recovery

Preparation has a 90-second operation bound (Git phase at most 60 seconds), with
renewal every five seconds. Existing 30-second leases and two-minute generation
ceiling remain authoritative. Failed renewal cancels preparation; the renewal
loop is joined before reporting. There is no detached execution after denial.
An in-flight local preparation may finish a step before cooperative cancellation;
it still cannot submit a valid first receipt after authority is lost.

The additive version-4 migration creates worker_preparations. Existing version-3
admission continues; preparation claims fail unavailable until v4 is explicitly
applied. Merging code does not migrate a production database. Stop old consumers,
apply through the existing operator migration path and restart reviewed profiles.
No old INPUT_VALIDATED receipt is promoted into a preparation attestation.

First report locks recovery/inbox/Run/Session, recomputes frozen identity and
validates the manifest, appends audit and inserts preparation plus input receipts
in one transaction. Final database-time expiry is checked after journal waits.
Any failure rolls back all records. Identical retry returns retained history,
including during recovery; changed facts, actor, generation or identity conflict.
The client only retries that identical report once for transport/5xx ambiguity.
It never repeats local preparation or a claim automatically after ambiguity.

Successful local preparation retains `<root>/workspaces/<slot>/prepared.json`,
including the local workspace owner record and bundle locator. Remote Facts omit
machine paths. Transport ambiguity retains that slot for operator reconciliation:
read the remote exact receipt before deciding whether to retain or remove it.
This version has no automatic disk retention sweeper, startup scavenger or crash
resume. A cancelled known pre-report preparation cleans its owned workspace;
bundles may remain as immutable reusable cache. Process death can leave an orphaned
stage/lock; do not delete it until absence of a live owner is established.

## Regression and remaining gates

CI preserves all earlier PostgreSQL/race/outbox/Runtime/Worker checks, then adds
five workspace/preparation repetitions, three shuffled preparation DB repetitions
and three actual command repetitions. Tests use actual Git, source bytes, mTLS,
PostgreSQL, compiled Worker/eng and the real Control Plane relay/lifecycle.
They prove exact retained receipt readback, no accidental Run completion/Evidence,
dedicated-grant rejection, no source hooks/ambient Git config, ownership fencing,
digest/tamper failures, receipt replay, audit rollback and expiry after lock waits.

Local formatting is not Go 1.25 compilation proof. Authoritative new evidence is
exact PR CI followed by fresh main CI. No test helper/model output is represented
as real Codex qualification. Next execution gates remain OS-isolated supervised
Runtime, real version-pinned Codex/schema, live Action/reconciliation authority,
Git/CI/Artifact-byte evidence, independent Review, Feature/Debug and restore pilots.
**Preparation is implemented; M1 and production readiness are not claimed.**
