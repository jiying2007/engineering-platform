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
