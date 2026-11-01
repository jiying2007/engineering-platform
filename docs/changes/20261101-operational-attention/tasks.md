# Tasks and evidence status

| Task | Status | Evidence |
| --- | --- | --- |
| ATT-01 projection over existing validated v6 status | IMPLEMENTED, verify pending | internal/production/attention.go |
| ATT-02 authenticated and read-only GET route | IMPLEMENTED, verify pending | internal/api/operational_attention.go; server.go; authenticated.go |
| ATT-03 positive/negative projection and authorization tests | IMPLEMENTED, verify pending | attention_test.go; operational_attention_test.go |
| ATT-04 docs/rollback & contract review | IMPLEMENTED, verify pending | requirements/design/negative-results |
| ATT-05 gofmt / local Go tests | NOT RUN | depends on locally available qualified Go toolchain, otherwise report |
| ATT-06 PR-head source CI / independent review / fresh-main | BLOCKED by current PR CI's Codex CLI invocation | existing #302 no-Codex source qualification proposal; do not bypass |
| ATT-07 event-level dedup and actual notification delivery | OUT OF SCOPE | requires incident identity and verified source/owner policy |

Stop on source/schema/authority conflict. Don't raise Product qualification. The branch is not a merged production release until required checks pass.
