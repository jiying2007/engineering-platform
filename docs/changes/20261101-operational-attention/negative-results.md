# Negative results and remaining limitations

1. **Not a notification service**: projections do not send, batch or deduplicate messages. No measurable end-user notification reduction is claimed until a real authorized delivery system and baseline A/B exist.
2. **Aggregate not identity**: count changes are not stable event IDs; a repeated unknown Action with the same count can be a new incident. No automatic dedup, retry or rearm based solely on these counters.
3. **No inferred incident age**: PLANNED/DISPATCHED and normal queues are suppressed from this projection without per-record timing evidence, while remaining visible in existing status and blocking canonical authority.
4. **Not product READY**: even an empty projection is no evidence of worker service health, provider qualification, field tests, SLO or external effect resolution.
5. **No high-privilege runtime**: DSH danger-full-access and worker home are not adopted. External advice cannot change Action authority.
6. **No current native verification**: CI source and Go test status must be read from actual run receipts. The repository's required `ci.yml` currently installs and starts Codex CLI; the user's existing prohibition remains in effect, so the branch shall not trigger that PR workflow until a sanctioned no-Codex verification path exists.
