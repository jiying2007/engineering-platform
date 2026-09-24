# Authenticated Control API v1

Date: 2026-09-24. Reviewed base: `3e3c320b7d3e6ceb09bbb4150035dc923b8781ea`.
This is the next bounded Core assembly slice after outbox v2. It changes the real
control-plane entrypoint, not just an optional middleware demonstration. It adds
no service, external identity dependency, extension domain or digital-worker shim.

## Modes and startup

The default is authenticated mode. Before opening PostgreSQL or migrating data,
startup requires all of:

- `DATABASE_URL`
- `CONTROL_TLS_CERT_FILE` and `CONTROL_TLS_KEY_FILE`
- `CONTROL_CLIENT_CA_FILE`
- `CONTROL_AUTH_POLICY_FILE`

TLS uses the Go standard library, TLS 1.3 minimum, RequireAndVerifyClientCert,
an operator-supplied client CA pool and disabled session tickets. Server key
files must be regular files with owner-only permissions; other configuration
files cannot be group/world writable. Reads are bounded at 1 MiB. Paths and
parent directories remain operator-controlled deployment inputs. No key, CA or
credential is generated or retained by the production binary.

`LISTEN_HOST` defaults to `127.0.0.1`, `PORT` to `8080`; authenticated mode can
explicitly bind another interface. Plain HTTP, absent CA/policy, invalid grants,
partial TLS configuration and absent DATABASE_URL do not silently fall back to
anonymous or memory-backed operation. AUTO_MIGRATE remains explicit `1`; merging
source does not run a production migration or provision credentials.

For contract development only, `INSECURE_DEV=1` explicitly permits plaintext on
literal `127.0.0.1` or `::1`. It is MEMORY-ONLY: DATABASE_URL, AUTO_MIGRATE=1 and
supplied TLS/access inputs are rejected, not ignored. `localhost`, wildcard and
other hosts are rejected in this mode. Do not give a development instance real
provider credentials, forward its port or treat it as a protected deployment.
The bare NewServer constructor remains a unit-test/development API fixture.

The command assembles the authenticated handler with PostgreSQL. Header/body
read, write and idle timeouts are bounded. SIGINT/SIGTERM trigger bounded HTTP
shutdown before closing the store. Existing cmd/eng and WorkBuddy front doors
are not mTLS clients yet; they cannot bypass the protected endpoint.

## Identity and policy

Identity comes only from a verified client leaf certificate with explicit
clientAuth EKU and exactly one URI SAN in this namespace:

`urn:engineering-platform:<logical-principal>`

A known Common Name, unsigned forwarded-client-certificate header, bearer string,
extra URI SAN, missing SAN or certificate from another CA does not confer identity.
The authenticated request rechecks the verified chain's validity interval, so an
expired certificate on an existing keep-alive connection is rejected too.

A version-1 JSON policy lists known subjects and explicit capabilities. All grants
must declare `scope: "platform"`: this is deliberately a single administrative
trust domain with platform-wide capabilities, NOT per-Work ownership, tenant
isolation or object-level ACL. Do not deploy it as a multi-tenant service.
No wildcard grants, unknown capabilities or duplicate entries are accepted.

Example READ-ONLY policy (no credential material):

```json
{
  "version": 1,
  "principals": [
    {
      "subject": "urn:engineering-platform:operator:observer",
      "scope": "platform",
      "capabilities": ["core:read"]
    }
  ]
}
```

Capabilities are separate for reads, Work/Task creation, Run start/control/complete,
checkpoint, delivery, Evidence, Verification, Closure, action execute/reconcile,
recovery begin/complete and material degradation. `internal/api/authenticated.go`
contains the exact route mapping; a new route is denied until mapped, and an AST
regression verifies that all current Core routes are explicitly accounted for.
Healthz needs no policy grant, but the actual TLS listener still requires a valid
client certificate during handshake. No proxy header authentication is supported.

Policies are immutable startup snapshots with copied grants. Revocation/removal,
CA rotation and policy changes require operator-controlled restart; there is no
hot reload, CRL/OCSP automation, credential broker or CA enrollment implementation.
Use appropriately short-lived certificates; these lifecycle gaps remain explicit.

## Body authority

POST bodies are bounded to 1 MiB and require unencoded application/json. The
same checked bytes reach existing business handlers. A precheck rejects invalid
UTF-8, duplicate object members at any depth, non-lower_snake_case keys, trailing
JSON and excessive nesting, preventing encoding/json v1's case-folding/duplicate
merging from interpreting a different principal than the authorization layer.
Existing schema decoding continues to reject unknown fields.

- New Work human_owner, Steering actor, Action requested_by and Verification
  verifier must equal the authenticated URI subject. Delegation is not implicit.
- Material degradation needs material:degrade, that principal as approver and a
  nonblank reason, in addition to task:create and existing material hard gates.
- Action execution requires the exact action/risk_class/capability tuple configured
  for this principal, plus action:execute. The Task/epoch/recovery Action Gateway
  checks are additional controls, not replaced by this admission check.
- Evidence registration requires an explicitly bound evidence_issuer plus an
  allowlist of evidence_procedures on the certificate's principal. An engineer
  with Work/Run rights cannot become CI by writing issuer="ci" in a request.
  This authenticates the issuer; it does NOT prove Artifact bytes or test truth.
- recovery:complete alone is insufficient: reconciled=true must also pass an
  independently implemented RecoveryCompletionGate for this exact epoch and
  principal. The real command has no such verifier yet and returns 503 for this
  operation. A boolean does not restore NORMAL mode on the protected endpoint.

The authenticated identity is available in request context for downstream policy
adapters. This change does not add a fully transactional principal audit receipt
for every HTTP operation. Existing business/audit/outbox transactions are retained.

## Verification and remaining gates

Tests use ephemeral in-memory CA/server/client certificates, actual TLS handshakes,
and the same server-assembly function as the command. They cover missing/foreign
certificates, unknown/ambiguous URI identities, expired certificate state, spoofed
headers/body identities, risk/capability downgrade, degradation without authority,
issuer/procedure/verifier mismatch, ambiguous JSON and missing reconciliation gate.

The authenticated Work -> Task -> Run -> Steering -> Completion -> Delivery ->
Evidence -> Verification -> Closure contract path is exercised both in memory and
with PostgreSQL in a separately owned per-test schema. These are synthetic API
contract fixtures, not real engineering/provider evidence or product pilots.

Action providers, an authoritative reconciliation verifier, a durable Worker
recipient/consumer, approved Context mounting, sandbox/environment policy and
initialized Codex thread/turn/approval execution are still missing. The command
continues to reject unconfigured Action operations rather than installing an
allow-all/fake provider. OPA/OIDC and credential lifecycle integration are not
claimed. Git/CI/Artifact-byte facts, independent Review and real Feature/Debug
pilots remain M1 gates. Do not confuse authenticated contract closure with real
code execution or production readiness.

Official references consulted for the implementation:
- https://pkg.go.dev/crypto/tls (ClientAuthType, VerifiedChains, Config)
- https://go.dev/blog/jsonv2-exp (encoding/json v1 ambiguity)
