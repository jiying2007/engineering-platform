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

This closes explicit raw-set storage/readback, not all W04/W05: producer-side
frozen output contracts, complete per-Run dependency coverage (including original
Git base/input/context), scheduled backup, approved second-site storage, key
management/encryption policy, native database consistency/restore, full model
output retention, crash capture and retention/GC lifecycle remain separate.
Omitted files are NOT covered. Use native database backups, never live data files.
Never upload private sets to public GitHub/Actions. Source, steering and artifacts
can contain secrets; these archives are not auto-redacted or encrypted. Public
terminal fact allowlists and historical M1 archives are unchanged. #105 is open.
