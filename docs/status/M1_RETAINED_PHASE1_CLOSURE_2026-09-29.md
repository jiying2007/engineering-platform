# M1 retained phase-1 closure evidence — 2026-09-29

Status: **COMPLETE for the trusted self-hosted M1 phase-1 lane.**

This record freezes the two real retained closure chains used to reassess M1.
It is evidence bookkeeping, not a replacement for GitHub Actions artifacts,
Core receipts, or immutable retained result commits.

## Feature retained chain

Requirement: issue #54, subsystem-aware embedded routing.

- frozen base: `75b6e24a409999129a0e89e6aa56e13733109034`
- engineering run: `36505950006`
- retained result commit: `6009ea95785237ad6ff9f5c9cba911b4891dfa58`
- retained transport PR: #91
- exact PR-head CI: `36513182863` PASS
- post-FINISHED recovery run: `36515098424` PASS
- verification run: `36516020897` PASS
- independent review run: `36517048600` PASS
- independent reviewer actor: `jiying2007`
- ClosureReceipt: `m1-feature-routing-closure`
- WorkItem: `m1-feature-routing-work` -> `CLOSED`
- post-Closure promotion PR: #100
- current-main integration commit: `0ba3c80574bad246d3c617a15996075b60d793a9`

The engineering workflow itself ended in a post-publication mechanical self-copy
failure after Core had already reached FINISHED and Action Gateway publication
was CONFIRMED. Recovery `36515098424` validated the retained FINISHED subject
without replaying the model turn. Verification consumed the recovered subject.

## Debug retained chain

Requirement: issue #55, reject non-canonical DEVICE_TEST firmware identity.

- authoritative pre-task reproduction run: `36518555837` PASS
- frozen base: `9d6309c6eff62d3489959ed574dda31536578568`
- engineering run: `36518908635` PASS
- retained result commit: `91d7d9fa068b7667bab5c211f13cd9e0151aeb92`
- retained transport PR: #98
- exact PR-head CI: `36526250474` PASS
- verification run: `36536077026` PASS
- independent review run: `36536314395` PASS
- independent reviewer actor: `jiying2007`
- review decision comment: `5885615313`
- ClosureReceipt: `m1-debug-firmware-identity-closure`
- WorkItem: `m1-debug-firmware-identity-work` -> `CLOSED`
- post-Closure promotion PR: #101
- current-main integration commit: `ee0249fe25b860a83beba9fe06b7acdc80cd75c6`

The Debug chain additionally binds the authoritative failing reproduction to the
exact frozen base before Task creation and revalidates that lineage during
Verification.

## Main integration state

After both post-Closure promotions:

- protected main: `ee0249fe25b860a83beba9fe06b7acdc80cd75c6`
- fresh-main CI: `36538546170` PASS
- Feature transport PR #91: closed as superseded by #100
- Debug transport PR #98: closed as superseded by #101

The immutable retained result branches/SHAs remain evidence subjects; the
promotion PRs are separate post-Closure integration decisions.

## M1 reassessment

The trusted self-hosted M1 phase-1 acceptance objective is satisfied:

1. real Feature requirement -> Core-bound Codex -> retained result -> independent
   publication -> exact-head CI -> Evidence -> Verification -> independent Review
   -> Closure;
2. real Debug reproduction -> frozen Debug Task -> Core-bound Codex -> retained
   result -> independent publication -> exact-head CI -> Evidence -> Verification
   -> independent Review -> Closure;
3. both retained results were promoted to current protected main only after
   Closure and passed current-main CI.

This does **not** qualify managed-workspace WIF or production/unattended
operation. The trusted self-hosted lane explicitly trusts the Linux host/account
and uses one-job ephemeral repository runners. Managed-workspace WIF still
requires a real administrator-provisioned provider/rule and a successful live
WIF model turn before that lane may be called qualified.

Production readiness therefore remains a separate decision requiring at least
the intended unattended credential/isolation posture, deployment ownership,
operational SLOs, rollout/rollback policy, and production service evidence.
