# Stopped source checkpoint v1

This is exact source preservation after a stopped unsuccessful engineering turn.
It is not a paused process, a model session snapshot, automatic resume, successful
Delivery, or a Human Takeover grant. It extends the existing Worker attestation
and stopped control transcript rather than introducing a second authority.

## Actual execution path

`ExecuteCodex` first validates/reopens the prepared workspace, then runs the
existing namespaced app-server and live controls. When that turn returns an
error, a valid sealed control transcript with a quiescent process-scope proof
permits a bounded attempt to capture the now-stopped source. Missing proof or
failed sealing cannot initiate capture. Capture still runs synchronously before
the Worker returns; it is not a daemon, retry queue or automatic model replay.

The Preparer rechecks the authenticated owner, frozen Task/RunInput/preparation,
managed workspace identity and Git base/configuration, and verified approved
Context. Changed source bytes are intentionally allowed; changed Git authority
is not. Source and archive locations come only from that owned preparation,
never a model-supplied path. The archive lives under the private Worker artifact
root using a deterministic execution-ID filename and exclusive creation.

The captured descriptor binds Task and RunInput digests, exact base commit,
original source identity, execution/profile/epoch/thread/turn, sealed transcript,
captured source tree, raw archive digest and size. The Worker saves a local
reconciliation record then attempts exactly one authenticated Core registration.
Lost replies and changed readback are errors, not retries or successful saves.
The error returned to the operator preserves the archive path/digest and whether
Core registration was confirmed. The original execution still follows its
unsuccessful/UNKNOWN path; no model result is finalized or published.

Migration 0010 adds one immutable source-checkpoint record per execution. Core
registration checks the same Worker, prepared Task/input/base, sealed runtime
and transcript proof. Exact duplicate submissions only read the original record;
changed identities or archive descriptors conflict. Registration may record an
already-stopped artifact after authority revocation, but cannot restore authority.
Authenticated `GET /api/v1/runs/RUN_ID/codex` exposes `source_checkpoint` alongside
the unsuccessful execution. Core records a Worker attestation: it does not claim
to have fetched/recomputed the host's private archive bytes.

## Archive and restore

A versioned canonical manifest and exact file bytes occupy one bounded TAR:

- Capture includes modified tracked files, untracked/ignored files, empty files
  and directories, executable bits and safe relative symlinks.
- The root `.git` is excluded. Nested Git metadata, special files, external or
  cyclic links, including indirect link-chain escapes, are rejected.
- Maximums are 10,000 entries, 64 MiB per file, 256 MiB source bytes and
  320 MiB archive. Larger workspaces are rejected rather than truncated.
- Capture checks file stability and a second complete source inventory, fsyncs,
  then independently reads back the exact manifest and archive digest.
- Readback requires an externally obtained expected raw digest and Run ID.
  Recomputed checksums do not excuse wrong file/member/subject bindings.
- Restore verifies the archive before creating a **new private destination**.
  It never overlays a running workspace, follows extraction links, imports Git
  configuration, or launches a model. An incomplete copy stays explicitly marked
  `INCOMPLETE`; success requires file/manifest/full-source readback and fsync.

Read the archive digest from authenticated Core/local reconciliation evidence,
not an untrusted archive's self-declared value. On a host holding the archive:

```sh
eng run-control inspect --run RUN_ID
eng source-checkpoint verify \
  --archive /private/artifacts/EXECUTION.source-checkpoint.tar \
  --digest sha256:EXPECTED_RAW_ARCHIVE_DIGEST --run RUN_ID
eng source-checkpoint restore \
  --archive /private/artifacts/EXECUTION.source-checkpoint.tar \
  --digest sha256:EXPECTED_RAW_ARCHIVE_DIGEST --run RUN_ID \
  --destination /private/recovery/NEW_DIRECTORY
```

The destination parent must already exist and be owner-private. Output explicitly
states `execution_authorized=false`. Source is copied to `NEW_DIRECTORY/source`;
`RESTORED.json` binds the verified descriptor. Existing paths cause rejection.
A failed registration does not make local bytes disappear, but must be reconciled
using the exact execution/descriptor, never by replaying the model or inventing a
new command ID. This version does not add automatic registration replay.

## Security and maturity boundary

These are **private local source archives**, never automatic public Git/Actions
uploads. Source files and steering text may contain proprietary or sensitive
material. This is not a secret-redaction engine: operators must apply their
access, encryption, retention and sharing policy to the artifact store. HOME,
login bootstrap files, process memory, Git metadata and external files are not
collected. A source archive does not replace complete original model output,
executable, Git bundle or database backup retention.

The kernel proof covers descendants of the owned namespace, not unrelated host
processes. Capture relies on the existing trusted Worker/host filesystem boundary;
it does not defend against a privileged host administrator concurrently rewriting
source/storage. Portable metadata is the source bytes, path/kind, link target and
execute bit, not UID/GID, timestamps, ACLs or sparse layout.

Actual `ExecuteCodex` + Preparer + namespace/subprocess + archive/restore tests
exercise successful registration, lost registration replies and changed readback.
Separate real PostgreSQL+mTLS tests use the production Worker-turn segment and
verify authorization, revocation-safe recording, immutable registration, audit,
and restored bytes. The model endpoint is a deliberate local protocol fixture:
these tests are not live provider, account or production-host qualification.

Remaining: authorized continuation/resume and ownership transfer with a new
explicit execution decision, process/session reconstruction, WorkBuddy UX, full
raw engineering-artifact retention and live production acceptance. None is
implicitly granted by `SOURCE_BYTES_RESTORED` or the existence of a checkpoint.
