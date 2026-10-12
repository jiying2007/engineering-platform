# Control response contract v1

The direct-mTLS Control client is a transport boundary, not proof of an
external effect or permission to retry. Caller-visible errors never include
upstream SQL, credential, response-body or parser fragments.

## Successful response

Every Raw/Call success requires one bounded, unambiguous JSON object. The
existing 4 MiB response budget is unchanged; request/configuration validators
remain at 1 MiB. Duplicate keys at every depth, invalid UTF-8, over-deep JSON,
non-object roots (including `null`), and trailing data are rejected.

Call decodes into a fresh value and assigns it only after complete validation.
A failed decode cannot mutate a previous receipt's nested map/pointer/slice;
a successful decode cannot retain omitted optional fields from a prior call.
An invalid destination pointer is rejected before sending any HTTP request.

For an explicitly tagged top-level response struct, non-omitempty fields must
be present with their exact wire names. Null is permitted only for nullable
member types. Thus `{"assignment":null}` can mean an empty worker queue, while
`null`, `{}` and case-aliased assignment keys cannot. Domain-specific nested
identity, digest, epoch and admission checks remain the consumer's authority;
this is not a general schema or authorization replacement. Raw callers have
no typed schema and retain responsibility for those domain checks.

## Unusable mutating responses

A POST with no usable successful response is `ErrPostOutcomeUnobserved`.
A malformed, truncated, encoded, wrong-media-type, oversized or ambiguous 5xx
error body retains the same conservative classification, its HTTP status via
`errors.As(*HTTPError)`, and internal transport/read causes via `errors.Is`.
This does not assert that COMMIT or a Provider operation occurred.

An intact bounded 503 with the existing allowlisted readback error code keeps
its precise `ErrCommitOutcomeUnknown` or `ErrExternalEffectUnresolved` class.
An intact ordinary error remains an HTTP error, not invented COMMIT evidence.
GET failures do not acquire a POST uncertainty class. No path in this client
performs an automatic retry. Original-ID/epoch/idempotency/audit readback and
all existing operation-specific replay prohibitions still apply.

## Qualification and exclusions

Required package tests exercise actual local mTLS response handling, including
null/missing fields, stale nested output, malformed and truncated 503 bodies,
exact error categories, body redaction and single-attempt behavior. Existing
precise COMMIT/external-effect and denial tests remain unchanged.

These tests do not prove a production SQL transaction outcome, original
external dispatcher quiescence, selected Provider behavior or unattended
readiness. Internal #228 and the remaining #229 lifecycle inventory stay open.
