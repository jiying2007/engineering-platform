# Explicit private raw artifact sets v1

`eng artifact-set pack|verify|restore` preserves the original bytes of one
explicitly declared set of files. It extends local artifact handling; it is not
a second Core ledger, backup scheduler, model tool or authority grant.

## Scope and trust

A plan binds original Run/execution/Task/input/base identities and a sorted
member list: stable artifact ID, kind, exact raw digest, byte size, and explicit
absolute source path. The raw plan digest must come from an authenticated
upstream decision or pinned local record. Hashing arbitrary input on demand is
NOT authorization. Verification/restoration likewise requires an externally
anchored archive digest and exact Run. An adjacent checksum alone does not
authenticate an untrusted archive.

There is no recursive scan or metadata-path following. Individual paths must be
canonical regular files owned by the caller, under owner-private directories.
Symlinks, hard links, special files, writable-by-others inputs, aliases, duplicate
paths/IDs, unsupported kinds, and excess sizes reject. Input files may have read
bits outside the owner (for example a Git-produced 0644 bundle) only inside that
private parent; files must not be group/world writable. All emitted files are
0600, directories 0700. Linux is required; other systems fail explicitly.

The caller must provide quiescent, host-owned inputs. All input bytes are read
again after capture. The storage code defends against accidental races, not
malicious root/same-UID modification or an untrusted filesystem/kernel.

## Format and durability

One canonical USTAR archive contains a path-free `manifest.json`, followed by
sorted `files/<artifact_id>` members. The IDs are bounded ASCII leaf names;
there are no archive-provided absolute paths, links or executable permissions.
The manifest records original subject and raw plan digest, NOT source paths or
file contents. Payloads are exact raw bytes. A fixed zero timestamp and fixed
headers make the same plan/content deterministic. A canonical re-encoding hash
rejects hidden padding/trailing data as well as extra/missing/reordered members.

Packing uses a private temporary file, file fsync, independent full readback,
atomic no-replace link publication, directory fsync and final-path readback.
A concurrent attempt cannot overwrite the winner. A failure after publication
may leave an archive or extra temporary link: observe/reconcile exact bytes;
never overwrite it or treat a failed invocation as successful durability.

Restore verifies before creating a fresh private directory, writes/fsyncs raw
files, writes the manifest last, fsyncs directories, and independently re-reads
every restored byte. All permissions are 0600 even for original executable
artifacts. Restore never runs Git, a binary, SQL, a model or a migration. Existing
destinations are rejected, including empty ones. On a failed restore, a private
partial directory may remain; the manifest's presence alone is NOT success and
must never be used as a readiness signal. No automatic overwrite/repair/replay.

Current bounds: 128 files, 1 GiB per file, 4 GiB combined payload, 256 KiB metadata.
Archives are uncompressed to bound parsing and avoid expansion ambiguity. Limits
reject rather than truncate. A larger workspace/artifact set needs a reviewed
capacity change, not untracked splitting or silent file omission.

## Operator interface

The explicit plan has this shape (identities/digests below are placeholders):

```json
{
  "version": 1,
  "subject": {
    "run_id": "original-run",
    "execution_id": "<64 lowercase hex>",
    "task_contract_digest": "sha256:<64 lowercase hex>",
    "run_input_manifest_digest": "sha256:<64 lowercase hex>",
    "base_commit": "<40 lowercase hex>"
  },
  "members": [
    {
      "artifact_id": "result.bundle",
      "kind": "git-bundle",
      "size": 12345,
      "digest": "sha256:<64 lowercase hex>",
      "source_path": "/private/artifacts/result.bundle"
    }
  ]
}
```

```sh
eng artifact-set pack --plan /private/plan.json \
  --plan-digest "$PINNED_PLAN_DIGEST" --out /private/retained/set.tar
eng artifact-set verify --archive /private/retained/set.tar \
  --archive-digest "$PINNED_ARCHIVE_DIGEST" --run "$ORIGINAL_RUN"
eng artifact-set restore --archive /private/retained/set.tar \
  --archive-digest "$PINNED_ARCHIVE_DIGEST" --run "$ORIGINAL_RUN" \
  --into /private/restores/new-set
```

Reports distinguish packed, verified and restored bytes and always state
`coverage=EXPLICIT_DECLARED_MEMBERS_ONLY`, `producer_semantics_verified=false`,
`execution_authorized=false`, `production_qualified=false`. A kind is a label,
not proof that a file is a valid firmware image or consistent database backup.
Reports contain identities/digests/counts, not absolute paths or raw payloads.

## Producer-derived execution capture

`capture-execution` reuses this exact archive format and verifier. It derives the
member plan from an externally anchored original execution permit, the existing
strict execution readback, and ALL entries in the frozen preparation Context
manifest. The Task and Run input are retained inside the original permit bytes;
recorded agent output is retained inside original turn/result records. No new
schema, permission or automatic runtime capture is introduced.

```sh
eng artifact-set capture-execution \
  --records /private/original/records --run "$ORIGINAL_RUN" \
  --execution "$EXECUTION_ID" --permit-digest "$PINNED_PERMIT_DIGEST" \
  --context /private/original/context \
  --runtime-binary /private/runtime/codex \
  --qualification-receipt /private/runtime/qualification.json \
  --archive /private/artifacts/source-checkpoint.tar \
  --bundle /private/artifacts/result.bundle --out /private/retained/execution.tar
```

Supply `--archive` exactly when a source checkpoint is recorded, and `--bundle`
exactly when a result is recorded. Omission of a referenced artifact rejects;
metadata paths never supply defaults. At least one verifiable result/checkpoint
is required. Missing or changed Context bytes/manifest, extra Context members,
unsafe paths/files, output beneath records/Context, and existing output reject.
A no-record/phase-only observation cannot be advertised as an archived result.

The command also requires private, quiescent copies of the exact Codex runtime
binary and compatibility qualification receipt frozen by the Permit. Runtime
bytes must hash to `profile.binary_digest`; the qualification JSON is strictly
decoded and its semantic digest, binary/version/model/config identities must
match the frozen Profile before its original bytes enter the archive. Restored
runtime bytes remain non-executable like every other artifact-set payload.

The ordinary selection is
`EXECUTION_RECORDS_CONTROL_HISTORY_CONTEXT_AND_FROZEN_RUNTIME`; an explicit
source-continuation successor uses
`EXECUTION_RECORDS_CONTROL_HISTORY_CONTEXT_RUNTIME_AND_UPSTREAM_CONTINUATION`.
Both selections require the exact private sealed ControlTranscript and bounded
schema-v5 EngineeringHistory (accepted `item/completed` Params plus terminal
`turn/completed`) in addition to current producer records, frozen Context and
Permit-bound Codex runtime/qualification. A continuation successor additionally
requires the exact upstream source-checkpoint archive. Result bundles are
self-contained for the frozen base+result Git graph, while stopped-source
checkpoint v2 retains its validated base graph. This remains
`full_run_backup=false`: credential/session bootstrap, bearer material,
provider-internal streaming state, broader repository history and external
service state remain outside this selection. Artifact-set's own coverage stays
`EXPLICIT_DECLARED_MEMBERS_ONLY`. No Core/network request, Git, SQL, model,
source scan, permission grant or public upload occurs. The command requires
already host-owned quiescent inputs; offline retention does not renew Context
approval or authorize execution.

The context manifest is stored as `files/context-manifest.json`, with original
canonical manifest bytes and original entry names. This name avoids collision
with the outer archive manifest. All producer record names and raw bytes remain
unchanged. On explicit restore, existing `execution-readback` consumes the
restored files; the context manifest and every referenced entry can be checked
against the original preparation/Run input. Do not automatically resolve its
historical paths or grant runtime access to restored material.

The derived private plan exists only in an owned temporary staging directory.
Packing uses the existing independent readback and atomic no-overwrite path.
The selection is rechecked after packing, including record absences and Context
inventory. An error after publication may leave an archive, but never reports
successful capture. Only this call's temporary staging is cleaned; published
or original material is never deleted to conceal failure.

Actual Worker/Preparer/kernel-namespace tests cover completed execution, Finalize
failure, result reply loss and checkpoint reply loss. They delete the entire
original prepared root (source, HOME, records, Context and artifacts), restore
only the retained set and verify producer records and every frozen Context byte.
Protocol and Core transport in these tests are explicitly synthetic fixtures,
not live provider or authenticated service proof.

## W04/W05 integration and remaining gates

Actual Worker/Preparer tests preserve their emitted permit, phase, result,
source-checkpoint and Git-bundle bytes. The original source/HOME/records and
artifacts are removed before restoring. Existing `execution-readback` then
revalidates the restored records and referenced source/bundle bytes, without
calling Core or replaying a model. Test model/Core transport remains explicit
fixtures, not live provider evidence. Raw record payloads remain private and
may themselves contain historical paths or sensitive context.

A separate C compiler fixture produces ELF/map outside a read-only source file's
source directory. The source bytes/inventory stay unchanged. After deleting both
source and build directories, the output set restores byte-for-byte without
automatically enabling executable permissions. This is a host reference case,
NOT a new Runtime out-of-tree build profile, MCU qualification or board evidence.
The shipping committed-source readback fence is unchanged.

The existing mandatory PostgreSQL 17 authority-restore drill also consumes the
private set. Native `pg_dump` runs as the test caller's UID/GID; it produces a
consistent custom-format dump plus the expected authority snapshot. Packing is
followed by deletion and absence checks of the original database and dump/plan
directory. Restore reads only the retained archive, then the explicitly
privileged test invokes native `pg_restore --single-transaction` into the fresh
fixture database. Existing checks compare Work/Delivery/Evidence/Verification/
Review/Closure, operation and recovery proof, audit sequence/digest, migrations
and Outbox; recovered-epoch completion must still pass.

This is an actual PostgreSQL/container integration with synthetic test records,
not production-host or customer-data recovery. The archive's execution ID names
a TEST-only backup operation, not a model run. Raw-set byte verification still
does not execute SQL or assert producer semantics. The native recovery decision
and existing Recovery completion authority remain separate. The test admin pool
stays open until scoped database cleanup completes; no backup file is uploaded.

This closes explicit raw-set storage/readback, not all W04/W05: producer-side
frozen output contracts, complete credential/session/provider-internal state
coverage, approved off-host/second-site placement, destructive retention/GC policy,
key management/encryption policy, production/cross-version database acceptance,
provider credential/session/internal-streaming retention and crash capture remain
separate. Periodic verified replica scheduling is implemented but does not delete
or qualify a second site.
Omitted files are NOT covered. Use native database backups, never live data files.
Never upload private sets to public GitHub/Actions. Source, steering and artifacts
can contain secrets; these archives are not auto-redacted or encrypted. Public
terminal fact allowlists and historical M1 archives are unchanged. #105 is open.

## Scheduled verified replica retention

`eng artifact-retention replicate` adds a deliberately narrow unattended
retention primitive around already-created artifact-set archives. It takes an
owner-controlled strict configuration whose **raw config digest must be pinned
externally**. The config names two existing owner-private roots, an explicit
sorted list of archive leaf names, exact original Run IDs and exact archive
digests, plus a bounded total byte budget.

The command performs two phases. First it verifies every primary archive and
every already-present replica with the existing artifact-set verifier and checks
the whole sweep against the explicit byte budget. Only after all existing state
is valid does it create missing replicas. Copying uses a private temporary file,
streaming digest/size verification, fsync, independent archive readback,
no-replace publication, directory fsync, final replica readback and a second
primary readback. A nonblocking lock on the replica root serializes concurrent
sweeps. Existing mismatched files, aliases, links, unsafe roots, changed inputs,
cancellation and budget overflow fail closed.

The report contains only config digest, counts and verified bytes. It always
states `deletion_performed=false`, `execution_authorized=false`,
`production_qualified=false` and `second_site_qualified=false`. Replication
does **not** scan for archives, choose retention policy, delete/overwrite either
root, execute restored data, call Core, Git, SQL, a model or a network service.
A replica root on the same filesystem is not evidence of a second site.

The production examples include a disabled-by-default maintenance systemd
service/timer running as the existing `engineering-preparation` identity.
Installation only copies these templates; an operator must provision the
owner-private config/roots, pin the raw config digest, review the example cadence
and explicitly enable the timer. The example six-hour cadence is an operating
example, not a measured SLO. The service can write only the replica root under
`ProtectSystem=strict`; the primary remains read-only.

This closes the code path for unattended **verified replication scheduling**.
Retention expiry/GC, destructive deletion, encryption/key custody and actual
off-host/second-site placement remain separate explicit gates. No automatic GC
is intentionally coupled to successful replication.

## Frozen offline build producer capture

`eng artifact-set capture-offline` reads only the two existing deterministic
`offline-<execution>.json` and `offline-<hash(execution + ":report")>.json`
producer records. Both raw digests must be independently anchored. The original
permit binds Task, RunInput, preparation and exact sandbox Profile. The complete
local report must match that permit, including the frozen output names/budgets,
child-reap flag, actual command exit, stdout/stderr and every output byte/hash.
A caller cannot omit an output or substitute a different Profile after the run.
No expired permit is renewed and no local record is promoted to a Core receipt.

```sh
eng artifact-set capture-offline --records /private/original-workspace \
  --run "$ORIGINAL_RUN" --execution "$OFFLINE_EXECUTION_ID" \
  --permit-digest "$PINNED_RAW_PERMIT_DIGEST" \
  --report-digest "$PINNED_RAW_REPORT_DIGEST" \
  --out /private/retained/offline.tar
```

The unmodified permit/report bytes plus every collected raw file enter the
existing archive format. Output archive IDs are `output-000.bin`, `output-001.bin`
and so on in frozen contract order; the original case-sensitive names, bounds
and digests remain in the report. These IDs are data, not executable paths.
Failed-command reports are preserved with their nonzero exit, state
`NOT_COLLECTED_EXIT_NONZERO` and **zero** output members. They never become
successful builds because capture or restore returned zero.

A private temporary directory materializes bounded inline output bytes for the
existing Pack implementation; input ownership/link checks precede these writes.
All source records are reread after packing. Only this call's staging is removed;
original records and a published archive are never deleted on failure. Like
Pack, post-publication failures require observation, not overwrite or blind
retry. The generated raw-plan digest includes actual temporary source paths;
separate captures need not have identical archive digests. Original record and
output byte digests, not temporary-path identities, are the stable producer facts.

Reports state `selection=FROZEN_OFFLINE_RECORDS_AND_COLLECTED_OUTPUTS`, command
exit/output state/count, both external anchors, result and output-contract digests.
`core_observation=NOT_OBSERVED`, `full_run_backup=false`, and execution/production
flags remain false. Storage does not independently attest the original runtime.
Offline execution may already have sent these output bytes to authorized Core
readers; this command does not retract or reclassify that earlier exposure.
No network, Core call, compile, SQL, Git, recursive scan or public upload occurs.

This selection does NOT include source/Git-base bytes, Context objects, toolchain,
upstream dependencies, complete logs or a database backup. The original embedded
stdout/stderr are retained only within existing bounded reports. Successful capture
is not Evidence/Verification/Review or a complete Run backup. Additional files in
the private workspace are ignored, never inferred as approved archive members.

Mandatory command integration runs both the existing probe and a real scratch-
container host C compiler through the actual compiled Worker, mTLS and PostgreSQL.
The same Run's local records are captured by compiled `eng`, original preparation,
source and Git directories are removed, then the archive alone is restored. The
restored permit/report and every raw output are compared with the previously
observed Core receipt. No Evidence or new execution is created. This is synthetic
test-task authority with real infrastructure, not a model, MCU or board qualification.
