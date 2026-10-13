# Declared blocker metadata guard v1

## Purpose and authority

A negated sentence containing GitHub's issue-closing keyword syntax can still
request an automatic issue transition. Source readiness, PR metadata and the
final proposed squash message must not contradict one another by accident.
The existing Implementation Status readiness block is the only declaration
this check consumes. Internal blockers, external qualification and deferred
credential gates are all treated as unresolved while they remain in that block.
The check does not query or mutate GitHub issues, change readiness, approve a
closure or invent evidence. A legitimate closure still needs reviewed source
readiness and the existing independent acceptance evidence.

## Existing CI integration

The existing `go` job runs the guard immediately after the exact source checkout.
It checks the supplied GitHub event's repository identity, PR title/body or push
commit messages, and the checked-out HEAD message. PR `edited` events now rerun
checks as well as `opened`, `synchronize` and `reopened`. The existing five job
names, dependencies, test sets, permissions and runtime budgets remain unchanged.
No token, new service, issue-write job or scheduled reconciler is introduced.

The nine documented closing keywords and local, qualified-repository and full
GitHub issue URL references are recognized case-insensitively. Negation,
Markdown wrappers and code fences are deliberately not treated as exemptions.
Comma, slash and `and` reference lists are additionally rejected conservatively.
Unrelated issue numbers and other repositories are not blocked. Safe wording is
`Internal blockers [228,229] remain OPEN`; a reference alone is not a closure.

Inputs use bounded, non-following/nonblocking regular-file reads on the Linux
CI host. JSON duplicate fields and non-finite values are rejected. Messages are
limited to 256 KiB each and 4 MiB in aggregate; reference reporting is bounded.
Rejections report only source labels, line numbers and issue IDs, not full PR
or commit text. The unit tests exercise the actual CLI, including a FIFO without
a writer, symlinks, malformed inputs, all keyword/reference combinations, the
previous negated-message failure and safe OPEN notation.

## Mandatory boundary before a custom squash merge

GitHub allows a user or client to choose a different squash message after CI
passes. This CI check cannot see that future text or prevent arbitrary direct
issue changes. The publishing client must validate the exact final message it
will send, immediately before invoking the existing protected merge operation:

```sh
python3 -B scripts/verify_blocker_metadata.py \
  --status docs/status/IMPLEMENTATION_STATUS.md \
  --repository jiying2007/engineering-platform \
  --message-file /private/path/proposed-merge-message.txt
```

The report explicitly records `arbitrary_later_merge_message_covered: false`.
Intermediate PR commit messages not present in the event or checked-out HEAD are
not silently advertised as inspected. No branch protection bypass, complete
GitHub Markdown parser or guarantee against a deliberate metadata rewrite is
claimed. Source/run/Artifact provenance and human independence remain separate.

## References

- GitHub Docs, "Linking a pull request to an issue":
  https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/linking-a-pull-request-to-an-issue
- GitHub Docs, "Issue event types", the `closed` event's `commit_id`:
  https://docs.github.com/en/rest/using-the-rest-api/issue-event-types#closed

These are syntax and event references, not evidence that an unresolved work item
has met its implementation or selected-host acceptance conditions.
