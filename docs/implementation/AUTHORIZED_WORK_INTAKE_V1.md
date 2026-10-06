# Authorized work intake v1

This is a thin convenience layer over the existing authenticated Core API. It
is not a WorkBuddy-specific backend, second schema or new authority.

\`eng work-intake submit INTAKE.json\` reads one bounded owner-controlled strict
JSON document. It pre-validates Material readiness, Verification coverage,
embedded routing and Context identities before any network write. The
\`human_owner\` is never accepted from the file: it is derived from the mTLS
client certificate.

After local validation it performs exactly three existing calls in order:
Work -> TaskContract -> Run. The returned server route/digest/readiness and Run
identity are checked before success. It does not start a Worker, model turn,
publisher or production service.

There is no automatic POST retry and no compensating delete. If a later phase
fails after Work or Task creation, the error records the exact phase and which
objects were already observed. Operators must inspect those exact IDs before
manual retry.

A WorkBuddy or other office UI may export the strict intake JSON and approved
Context identities to the Ubuntu engineering host. Core access policy, Material,
server routing, Verification and mTLS remain authoritative.
