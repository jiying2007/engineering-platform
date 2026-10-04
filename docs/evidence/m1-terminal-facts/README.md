# Retained M1 terminal facts

These two **derived, bounded fact archives** preserve original UTF-8 member bytes
from the actual historical Feature and Debug final Review ZIPs. They do not
re-execute a model, reissue Review/Closure, migrate historical schemas, or qualify
any current provider. Original ZIPs and retained result commits are unchanged.

| Subject | Source review run | Source artifact | Derived file |
| --- | --- | --- | --- |
| Feature | 36517048600 | 11010594961 | `feature-closure.json.gz` |
| Debug | 36536314395 | 11018093512 | `debug-closure.json.gz` |

Each file is a single bounded gzip-compressed canonical JSON envelope, not a
fragmented archive. Each archive binds its original ZIP digest and artifact/run IDs. Each selected
member retains its original bytes, byte count and SHA-256. The independent
verifier rechecks the exact member set, canonical envelope, immutable Task/Run,
base/result commits, Delivery/Evidence/Verification/Review/Closure identities,
publication facts, and different engineering/reviewer actors. An external
expected archive digest is required. The protected Git commit containing this
directory is the durable anchor; a checksum downloaded with an untrusted file
is not authentication by itself.

Read back from the repository root without account access or a network:

```sh
(cd docs/evidence/m1-terminal-facts && sha256sum -c SHA256SUMS)
python3 -B scripts/test_retained_review_archive.py
```

## Explicit limits

This is a **terminal fact record, not a complete reproducibility or runtime
backup**. It intentionally excludes disposable PKI/private keys, environment
files, database dumps, raw logs/model output, nested ZIPs and executable bytes.
The archived receipt facts retain the digests of those engineering artifacts;
this does not preserve their raw bytes or independently replay their verifiers.
Full source/binary/model-result artifact retention remains separate work.

The raw historical source ZIPs remain in Actions with their original expiration
policy. These small derived records remain in Git after that expiry, with clear
source lineage, but must never be represented as the original ZIP or as a full
replacement for all engineering Evidence.

New final Review uploads use `retained-pilot-review-facts-*`, not the old raw
`retained-pilot-review-*` payload. PASS archives contain 14 allowlisted facts;
FAIL archives contain 13 and cannot contain a ClosureReceipt. Failed packing or
readback uploads nothing; there is no raw-directory fallback. Verification's
bootstrap PKI is deleted after Control stops because Review creates fresh PKI.
Intermediate database transport is still a distinct, short-lived workflow need.

Credential-pattern/field rejection is defense in depth, not a universal secret
classifier. Only the declared pilot terminal JSON facts are eligible; extending
the allowlist requires explicit review and new negative tests. Do not broaden it
to logs or arbitrary product materials.
