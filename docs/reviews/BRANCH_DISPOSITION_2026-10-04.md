# Historical branch disposition

Copied from the audited main status before live-status consolidation. This is
a cleanup record, not active implementation authority. Exact ref heads must be
checked before deletion. Retained M1 evidence subjects must not be deleted.

## Superseded branch disposition

The following non-retained branches are explicitly classified as
**superseded history**. They are not pending product work, grant no authority,
and must not be merged or cherry-picked back into main.

Older implementation prototypes:

- `feat/github-ci-core-evidence-import` — replaced by #44 trusted CI import,
  then hardened by #48 and #51.
- `feat/independent-review-authority` — replaced by the stricter #46
  independent Review authority.
- `docs/retire-superseded-historical-branches` — documentation transport for
  #110; its content is already on main.

Relay/provider development branches:

- `feat/relay-provider-prequalification` — superseded by #111.
- `feat/relay-prequalification-operator-pack` — superseded by #112.
- `feat/relay-private-http-policy` — superseded by #113.
- `feat/relay-codex-config-renderer` — exploratory renderer branch; superseded
  by the v2 renderer identity merged in #114.
- `feat/relay-codex-config-renderer-v2` — no unique commits remain relative to
  its main checkpoint and it is superseded by #114.
- `feat/relay-prequalification-v2-renderer` — merged/superseded by #114.
- `feat/relay-live-handoff-manifest` — merged/superseded by #115.

Current canonical relay progression on main is therefore:

```text
#111 repository prequalification
  -> #112 non-secret operator pack
  -> #113 explicit private-HTTP transport policy
  -> #114 prequalification v2 + exact Codex renderer identity
  -> #115 frozen future live qualification manifest
```

These superseded refs may be deleted as repository hygiene. Their continued
existence is not an implementation blocker and must not be interpreted as
parallel supported implementations.

Do **not** apply this disposition to the two retained M1 result branches:

- `engineering-platform/3ac04fc7097e8375e5f7c1f8` — retained Feature result;
- `engineering-platform/994964383ed085e51545105d` — retained Debug result.

Those two branches are immutable evidence references for the already-closed M1
Feature/Debug chains and remain intentionally preserved.

## Mechanically proven retirement

`.github/retired-branches.json` is an explicit maintenance manifest, not a
runtime compatibility contract. Each candidate binds an exact branch tip,
complete Git tree and an integrated commit on main with the identical tree.
This handles squash/rebase integration without treating every non-ancestor as
unmerged product work. The manifest does not cover all superseded prototypes.

The `Retire integrated branches` workflow runs only after successful same-repo
push-to-protected-main canonical CI. It checks the exact main checkout, actual
remote heads, complete-tree identity, integrated-commit ancestry and the open-PR
inventory. It then issues one atomic Git deletion with an exact old-SHA lease
for every selected ref, followed by remote readback. A parallel update rejects
the whole push. There is no force-delete fallback, no model/provider operation,
and no inference of maturity from a successful cleanup. Failed/unknown pushes
require observation/reconciliation, never blind replay.

An absent manifest ref is an idempotent no-op. New or changed refs require an
explicit manifest review. Main, release refs and both retained M1 result refs
are excluded by code, not merely by the current data. The maintenance workflow
never bypasses branch protection.

Four older prototype branches are deliberately not in this mechanical pass:
`feat/github-ci-core-evidence-import`, `feat/independent-publisher-service`,
`feat/independent-review-authority`, and `feat/relay-codex-config-renderer`.
No identical integrated tree was found for these tips in the reviewed history.
Their superseded classification is not proof of byte-equivalent integration;
manual semantic disposition or a separately retained source archive is required
before deleting their only named development refs.

The manifest is an approved candidate list, not a deletion receipt. Completed
workflow receipts and an independent current-branch listing establish which
refs were actually removed. Changing a manifest must not rewrite past receipts.


## RC cleanup expansion — 2026-10-06

The same exact-tree retirement mechanism now accepts the `test/*` namespace
and carries reviewed RC working refs whose remote tips were independently
compared against their squash-merge commits. Inclusion requires complete-tree
identity; merge status alone is insufficient.

The two retained M1 evidence refs under `engineering-platform/*` remain outside
the retirement grammar and manifest. Unmatched historical prototypes also remain
excluded because semantic supersession is not byte-equivalent integration proof.
Current open-PR branches are never placed in this manifest.

After this change reaches protected main and canonical CI succeeds, the existing
workflow re-reads exact main, every candidate remote head, merge ancestry/tree
identity and open-PR inventory before one atomic SHA-leased deletion and remote
readback. Any drift rejects the entire mutation.


## Retained evidence ref drift guard

The two immutable M1 evidence refs stay outside the retirement grammar. Required
CI now reads their exact remote inventory and requires the fixed branch names and
object IDs in `.github/retained-evidence-refs.json`. Missing, changed or
additional `engineering-platform/*` refs fail the existing required Go job.

This is drift detection, not GitHub branch protection. The repository currently
reports both retained refs as unprotected; actual mutation prevention remains a
separate repository-administrator setting and must not be inferred from this CI
guard.


## Applied RC retirement receipt — 2026-10-06

Protected-main CI `37475533558` qualified
`310874bd0a6297310a31c919b9d530dbf502bfa6`. Its dependent maintenance run
`37476720045` used manifest digest
`sha256:478e163bc7e5aa2229cb143c2b690612b6626e80d8680105630eb446815dc521`.
The retained receipt records 37 candidates: **32
`DELETED_READBACK_VERIFIED` and 5 `ABSENT`**, with
`mutation_attempted=true`. Direct remote inventory after the run confirms those
32 refs are gone.

The remaining non-main refs deliberately include the two historical M1 evidence
refs and four divergent prototypes
(`feat/github-ci-core-evidence-import`,
`feat/independent-publisher-service`,
`feat/independent-review-authority`,
`feat/relay-codex-config-renderer`). The prototypes require semantic
supersession review; they are not eligible for the byte-identical manifest.

To avoid each governance/maintenance PR creating the next orphan, protected-main
retirement also validates the PR associated with the newly verified main commit.
Only a closed same-repository PR targeting main, with an allowed working-ref
name, exact recorded head SHA and a complete head tree identical to verified
main, can be appended in-memory to the same atomic SHA-leased deletion. This
does not broaden deletion to retained evidence refs, release refs, arbitrary
branches or open PRs. If PR association, SHA, tree or remote inventory drifts,
the mutation is rejected before deletion.
