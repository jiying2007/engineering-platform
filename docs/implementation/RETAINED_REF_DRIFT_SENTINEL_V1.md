# Retained reference drift sentinel v1

Status: **READ-ONLY OPERATIONAL MONITOR — NOT BRANCH PROTECTION**

## Gap and scope

The repository intentionally retains two historical, closed M1 Feature/Debug
Git refs and six divergent superseded-prototype refs. Required PR/main CI
already checks their exact name and expected head from frozen manifests. Those
eight refs are **not** administrator-protected; CI detects unexpected changes
only when CI runs. It cannot prevent a direct ref mutation.

**Availability limit:** GitHub may delay scheduled events and automatically
**disable schedules in a public repository after 60 days of inactivity**.
Thus this is a best-effort first-party detection improvement, not a guarantee
of permanent monitoring for an arbitrarily quiet repository. After long idle
periods, an operator must verify the workflow is enabled or independently run
the two existing read-only verifier scripts from a trusted, current checkout.
An operator-controlled external scheduler is required for a strict continuous
cadence; this PR does not create, authorize or operate such a scheduler.

The additive workflow
`.github/workflows/retained-ref-drift-watch.yml` reduces the *quiet-repository
detection gap* while GitHub scheduling remains active. It runs once daily, supports operator `workflow_dispatch` and
rechecks itself on protected-main changes to its manifests/verifiers/workflow.
Runs on forks are excluded. On the canonical repository it requires exact
protected default `main` checkout and uses only `contents:read` permissions
with `persist-credentials:false`.

Two existing fail-closed verifiers query actual GitHub remote branch inventory
and compare fixed ref-name/head SHA pairs to the pinned source manifests.
Missing, newly added or changed expected refs, network errors, invalid
manifests and malformed output fail the workflow. It never creates, repairs,
force-updates, deletes or merges refs.

The successful 90-day artifact binds the exact GitHub SHA/run/attempt and
contains only the original two bounded verifier results. Its fields explicitly
state `mutation_attempted=false`, `ref_mutation_prevented=false` and
`production_qualified=false`.

## Authority and follow-up

This is *detection*, not GitHub ruleset/branch-admin enforcement, an off-site
backup, or immutable historical data custody. Production/operator sign-off
still requires administrator-controlled ref mutation protection or equivalent
archive/key custody, and external host/provisioning qualifications in #105.
Do not remove the two M1 ref subjects; do not merge superseded prototypes.
Unexpected drift should remain a failed check for human reconciliation. Never
auto-repair a changed historical ref based on a guessed trusted SHA.

The existing required CI check is unchanged. This workflow adds a cheap
out-of-band read-only check in periods with no code changes and keeps the same
`IMPLEMENTATION_STATUS.md` authority.
