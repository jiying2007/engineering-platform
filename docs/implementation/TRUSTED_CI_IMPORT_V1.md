# Trusted GitHub CI Evidence Import v1

Base: `e895fc6dddb509a51897b5fe32b21216312ebb9d` (#43).

This increment converts already-retained GitHub CI provenance into one exact
Core Evidence item without creating a second evidence domain or trusting a
caller-supplied PASS string.

## Authority

Only the dedicated mTLS principal

`urn:engineering-platform:github-ci-importer`

may own the reserved issuer/procedure pair:

- issuer: `github-actions-importer`
- procedure: `github.actions.trusted-ci.v1`

Access-policy validation rejects that issuer/procedure on another principal and
rejects mixing the reserved procedure with other evidence procedures. Human
engineering principals should never hold the importer certificate.

The importer still uses the existing authenticated `POST /api/v1/evidence`.
The Core server independently re-resolves Delivery -> Task -> frozen
VerificationPlan and requires the exact `requirement_id`, procedure, optional
issuer, subject and delivery artifact binding introduced by #43.

## Accepted provenance

v1 is intentionally narrow and self-hosted. It accepts only:

- repository `jiying2007/engineering-platform`;
- workflow name `CI`;
- workflow path `.github/workflows/ci.yml`;
- event `push`;
- branch `main`;
- `source_sha == tested_sha == DeliveryReceipt.result_commit`;
- completed/successful exact GitHub run and run attempt.

PR merge commits are not accepted as final-delivery evidence. A delivery must be
verified by the fresh main push for its exact result commit.

## Live GitHub verification

`eng import-ci-evidence` reads the retained envelope only to discover the run
identity, then queries `https://api.github.com` directly with a no-proxy,
no-redirect bounded client. Public-repository reads need no token. If rate limits
require authentication, `GITHUB_TOKEN_FILE` may point to one owner-private
regular file; the token is not accepted on the command line.

The importer requires the live run to be completed/successful and rechecks:

- repository, workflow name/path, event, main branch, result SHA and run attempt;
- the exact successful `go`, `offline-container-integration`, and
  `codex-app-server-0.155.0-qualification` jobs;
- each job's live `head_sha`;
- live GitHub artifact IDs, names, sizes and GitHub SHA-256 digests;
- non-expired artifacts belonging to the exact run/head SHA.

Responses larger than the fixed bound or requiring pagination beyond the first
100 jobs/artifacts fail closed rather than silently verifying a partial view.

## Artifact byte verification

The operator downloads three artifacts from the same run:

1. `trusted-ci-evidence-<sha>`
2. `engineering-binaries-<sha>`
3. `codex-0.155.0-qualification-<sha>`

The importer does not follow artifact download redirects itself. Each supplied
ZIP must be a regular local file whose complete bytes match the GitHub-reported
artifact digest and size.

The trusted envelope ZIP must contain only `ci-evidence-envelope.json`. Its
canonical receipt digest is reverified.

The binary ZIP is independently checked again instead of trusting the workflow
that produced the envelope. It must contain exactly the five retained executable
facts plus `file-manifest.json` and `SHA256SUMS`; every executable byte size
and raw SHA-256 must match the envelope, and the manifest must contain the same
sorted facts.

The Codex ZIP must contain the qualification receipt and npm integrity file. The
receipt must still match the pinned 0.155.0 qualification contract: exact release
identity, raw binary/schema digests, fresh stdio process, no daemon/per-thread
config override, successful initialize/thread-start, stable/experimental checks,
and credential-safe profile proof.

Symlinked artifact paths are rejected. Critical archives are re-hashed after
content validation to detect ordinary replacement races. The importer host and
same-privilege local account remain trusted; this is not a hostile-kernel
filesystem attestation mechanism.

## Delivery binding

Before import, the immutable DeliveryReceipt must already include one artifact
whose digest is the GitHub-reported digest of the exact
`trusted-ci-evidence-<result_commit>` ZIP, for example:

```json
{
  "artifact_id": "ci-provenance",
  "digest": "sha256:...",
  "media_type": "application/zip"
}
```

The importer re-computes the Delivery subject digest and refuses a mismatched
record. The generated Core Evidence references only that exact delivery artifact.
The envelope transitively binds the tested jobs, binary archive and Codex
qualification archive; the live GitHub API independently rechecks their IDs and
digests.

## Command

With the dedicated importer mTLS certificate configured through the existing
Control API environment:

```sh
eng import-ci-evidence \
  --delivery delivery-123 \
  --requirement req-ci-main \
  --evidence ev-ci-main-123 \
  --artifact ci-provenance \
  --envelope-zip trusted-ci-evidence.zip \
  --binaries-zip engineering-binaries.zip \
  --codex-zip codex-qualification.zip
```

Required Control API variables remain:

- `CONTROL_ENDPOINT`
- `CONTROL_CLIENT_CERT_FILE`
- `CONTROL_CLIENT_KEY_FILE`
- `CONTROL_SERVER_CA_FILE`

Optional GitHub API authentication:

- `GITHUB_TOKEN_FILE`

The command refuses to run unless the mTLS certificate URI is exactly the
dedicated importer subject.

## Result semantics

A successful import creates one ordinary `EvidenceRef`:

- exact `delivery_receipt_id`;
- exact frozen `requirement_id`;
- exact Delivery subject digest;
- reserved importer issuer/procedure;
- `result=PASS`;
- `applicable=true`;
- the trusted CI envelope delivery artifact reference.

This PASS means only that the frozen CI provenance requirement represented by
`github.actions.trusted-ci.v1` was satisfied. It does not by itself mean the
whole VerificationPlan passes, the engineering change is correct, independent
Review passed, or the Work may close.

## Remaining closure work

After this importer lands, the remaining platform-level gaps are materially
smaller:

1. configure managed-workspace Codex WIF and retain the first real authenticated
   read-only model-turn receipt;
2. bind real Codex execution to a Core Run lease and workspace-write sandbox;
3. add changed-tree/model-output evidence procedures and independent Review;
4. exercise UNKNOWN reconciliation plus backup/restore;
5. retain one real Feature pilot and one real Debug pilot.

No production deployment, certificate issuance, database migration, WIF admin
change or Evidence import occurs merely by merging this source change.
