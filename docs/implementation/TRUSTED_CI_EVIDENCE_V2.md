# Trusted CI artifact evidence v2

Status: current executable-delivery provenance contract. Historical v1 is in Git history.

## Exact chain

The `go` job builds the roles in `internal/distribution/binaries.txt` with
`-trimpath`. The same embedded list drives executable verification and the CI
receipt validator. The current six roles include the independent Publisher.
There is no five-role compatibility fallback in current admission.

Each package contains exactly those executables, `file-manifest.json` and
`SHA256SUMS`. Verification reads the delivered bytes, checks the Go command role,
clean Linux/amd64 VCS revision, one shared source commit, byte sizes and hashes,
and exact SHA256SUMS. It never rebuilds a missing role from source.

Negative package tests cover omission, extra members, symlinks, checksum drift
and wrong-role substitution even when the attacker recomputes both manifests.
An extracted-package smoke uses the delivered `eng` from an independent directory.

## Workflow and source identity

The receipt uses `schema_version=2`. `source_sha == tested_sha` is the exact
PR head or main commit. A PR additionally binds its exact base SHA. The importer
requires a same-repository PR and an unchanged canonical workflow blob relative
to that frozen base. Synthetic merge SHAs are not mislabeled as PR-head tests.

The four upstream jobs are `go`, `offline-container-integration`,
`postgres-authority-restore-drill`, and `codex-app-server-qualification`.
The final `trusted-ci-artifact-evidence` job does not attest to its own success.
All five remain required repository gates.

Upstream artifacts are `engineering-binaries-<sha>` and
`codex-compatibility-qualification-<sha>`. The final job reads their live GitHub
IDs, sizes and ZIP digests, downloads them, verifies exact executable bytes and
emits `trusted-ci-evidence-<sha>`. Codex has a CI sentinel version, not a universal
version pin: every execution must qualify its actual binary/provider identity.

## Authority boundary

GitHub run/artifact facts are delivery provenance, not engineering acceptance,
model quality, provider qualification, human Review or release permission.
The archive digest must come from the authenticated distribution channel;
a self-consistent package alone cannot authenticate its distributor.

The Core importer additionally binds exact Delivery, Task, Run, requirement,
GitHub repository/workflow/head/base and raw artifact bytes before registering
Evidence. It rechecks all six files, the file manifest and SHA256SUMS.

Historical M1 v1 receipts remain immutable and may be investigated with their
original pinned verifier/source. They are never rewritten or silently admitted
as a current v2 six-role distribution. Actions retention is a temporary transport,
not an indefinite evidence archive.
