# Trusted CI artifact evidence v1

Base: `810d01c7e1c89efe22aa7c3c99bcbd2398b35380` (#36).

This slice retains exact Git, CI and artifact-byte provenance for each accepted
workflow run. It does not turn CI facts into engineering acceptance, model review
or release authority.

## Evidence chain

After the required jobs complete successfully:

1. the Go job builds five candidate binaries with `-trimpath` and records each
   raw file SHA-256 and byte size;
2. GitHub stores that binary set as `engineering-binaries-<tested sha>`;
3. the Codex qualification job independently retains the exact 0.155.0 native
   binary/schema qualification artifact;
4. a final evidence job queries the GitHub Actions API for this exact run, not a
   user-supplied status document;
5. it requires the exact `go`, `offline-container-integration` and
   `codex-app-server-0.155.0-qualification` jobs to be present and successful;
6. it requires GitHub-reported SHA-256 digests for exactly the binary and Codex
   qualification artifacts;
7. it downloads the binary artifact and verifies the embedded raw-byte manifest;
8. `cmd/ci-evidence` strict-decodes, sorts and validates the facts, then emits a
   canonical receipt digest;
9. GitHub retains the envelope for 90 days.

The envelope is immutable content. It contains no access token, runner path,
certificate, API credential or timestamp supplied by a Worker.

## Git identity

Pull-request evidence deliberately records two identities:

- `source_sha`: exact PR head requested for review;
- `tested_sha`: the checkout actually exercised by GitHub Actions, normally the
  synthetic PR merge commit;
- `base_sha`: the PR base SHA.

For a push to main, `source_sha == tested_sha` and no base SHA is accepted.
This prevents a green synthetic merge from being mislabeled as testing some
different source revision.

## Required CI facts

The receipt accepts exactly these required jobs and only successful conclusions:

- `go`
- `offline-container-integration`
- `codex-app-server-0.155.0-qualification`

A renamed, missing, skipped, cancelled or failed required job invalidates the
receipt. The evidence job itself is not used as evidence for itself.

## Required artifacts

Exactly two upstream artifacts are retained in v1:

- `engineering-binaries-*`
- `codex-0.155.0-qualification-*`

Their IDs, byte sizes and artifact ZIP digests are returned by GitHub's Actions
API. The binary artifact additionally contains a raw-byte manifest for:

- `codex-qualifier`
- `control-plane`
- `eng`
- `sandbox-guard`
- `worker`

Both layers are intentional: the GitHub digest binds the uploaded archive, while
the inner manifest binds executable bytes independently of archive metadata.

## Trust boundary

This is `GITHUB_ATTESTED_CI_PROVENANCE` in the architectural sense, not a Core
Evidence record. GitHub Actions and its API are the attester for job/artifact
facts. The repository code validates structure and exact required facts, but does
not independently prove GitHub itself was uncompromised.

The workflow uses the job-scoped GitHub token with `contents:read` and
`actions:read`; it does not use a PAT or repository write token. Downloaded
artifact content is treated as untrusted until the manifest and schema validate.

No CI receipt:

- completes a Run;
- satisfies an acceptance criterion by itself;
- creates Delivery/Evidence/Verification/Closure;
- proves model output quality;
- authorizes deployment or an external side effect.

A later importer must bind this envelope to the exact Run, VerificationPlan and
trusted GitHub repository/workflow identity before registering engineering
Evidence.

## Next gates

With this slice, Git/CI/artifact fact capture is no longer a hand-written status
claim. Remaining connected work is:

1. bind the qualified Codex binary to a current Core execution reservation and
   perform a real authenticated model turn under an explicit egress/credential
   boundary;
2. route interactive approvals through Action Gateway;
3. import this CI provenance plus actual changed-tree/artifact facts into Core
   Evidence and independent Review;
4. exercise reconciliation and backup/restore;
5. retain one Feature and one Debug pilot before assessing M1.
