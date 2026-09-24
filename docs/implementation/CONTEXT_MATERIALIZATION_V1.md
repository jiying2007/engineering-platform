# Context materialization v1

Scope: the existing Core Run input and local Worker infrastructure. This adds no
service, planner, knowledge store, digital-worker compatibility layer or provider authority.

## Identity and authorization

`RunInputManifest.context_refs` is an ordered array of `ContextRef` objects:

```json
{
  "source": "docs:task-spec",
  "type": "DOCUMENT",
  "version": "revision-1",
  "digest": "sha256:<64 lowercase hex characters>",
  "trust": "APPROVED"
}
```

The placeholder digest above is illustrative, not a valid request. Production
refs must carry the SHA-256 of the original source bytes. `canonical.Digest`
continues to hash a JSON representation; `canonical.BytesDigest` hashes bytes
without JSON, base64, newline or Unicode normalization.

The source is a bounded logical identifier, not a URL or local pathname. Types
are DOCUMENT, SKILL, LOG and EVIDENCE. Versions are explicit logical revisions;
content changes require a different digest. At most 64 refs are accepted.
Duplicate source/type pairs, invalid hashes, unknown fields and legacy string
refs fail closed. Ordering is part of the Run identity; no implicit sorting or
context accumulation is performed. No old-string compatibility shim is supplied.

APPROVED is a recorded classification, not a grant. Before reading any source,
the materializer requires an explicit Authorizer to check every ref against the
frozen Run/task/input identity and authenticated-principal ACL plus current
approval/revocation state. The principal must be bound by the trusted host to the
Authorizer or request context, never supplied by context content. UNTRUSTED refs
cannot be materialized by this interface. There is no production allow-all
implementation and no implicit network/file resolver.

## Materialize and verify

1. The host opens a dedicated existing host-controlled bundle root with `os.Root`.
   Symlink root aliases and group/world-writable roots are rejected. Source
   worktree and bundle root must not contain each other.
2. The host supplies the exact frozen RunInputManifest, Resolver and Authorizer.
   The complete ref list is validated and authorized before any bytes are fetched.
3. Streaming reads enforce per-entry and total byte limits (defaults: 4 MiB and
   32 MiB; configured hard maximum: 1 GiB each). The Resolver must honor context
   cancellation even during blocked remote reads and enforce access/redirect/SSRF
   policy for any transport it implements.
4. Generated filenames use only ordinal and digest. No retrieved text is executed,
   installed as a Skill or written as AGENTS.md/provider configuration.
5. Files are created exclusively under a private staging directory and checked
   against raw-byte digests. Read, close, size and hash failures abort publication
   and clean the stage. A per-input exclusive publication lock fences concurrent
   writers. Existing bundles are never overwritten.
6. A compact manifest binds schema version, frozen Run input digest, ordered refs,
   relative generated filenames and exact byte lengths. Local path is returned
   separately; it is not part of the Run or manifest identity.
7. Files are synced and set to 0400, then the directory to 0500 before atomic
   rename to the input digest. This is local atomic visibility, not a claim of
   fully crash-durable publication on every filesystem.
8. `Verify` supports host restart/reuse: re-authorize every ref, validate the
   manifest framing/schema/identity, reject unexpected files, symlinks or writable
   entries, and re-hash every byte before handing the bundle to a Runtime.

An orphaned `.lock` or `.staging-*` after process death is NOT automatically
removed: a host recovery procedure must prove there is no live owner first.
Filesystem quota, cleanup retention and a persistent materialization receipt are
future host integration work, not silently provided by this library.

## Security and integration limits

0400/0500 are accidental-write protection. A hostile process sharing the owner
UID can chmod its files. `os.Root` confines filesystem traversal; it is not an
OS execution sandbox. Runtime isolation must use a separately controlled identity
or read-only sandbox mount, sanitized environment/config, and bounded resources.
Verification before launch does not alone eliminate a same-UID post-check race.
The root's parent tree must remain under host control for the Materializer lifetime.

The Worker orchestration path is not yet wired to call Materialize/Verify or mount
the bundle. Context addition during a live session requires a new explicit frozen
input/Steering lineage; this library does not mutate a running manifest. Concrete
source resolvers, approval registry, principal binding, receipts and real Codex
consumption remain separate acceptance gates. Library tests do not establish a
real Feature/Debug pilot or production readiness.
