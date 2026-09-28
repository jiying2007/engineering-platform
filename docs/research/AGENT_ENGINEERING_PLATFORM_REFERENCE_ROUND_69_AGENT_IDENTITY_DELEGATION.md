# Agent Engineering Platform Reference — Round 69 (Agent Identity and Delegation)

Date: 2026-09-29  
Status: **Research / future multi-agent identity reference; not M1 Core authority**

## 1. Why this matters

Once one engineering agent can delegate work to a sub-agent or peer agent, ordinary 'the user authorized the parent' semantics become insufficient. The platform needs to preserve who delegated what, the exact narrowed scope and which identity actually performed each operation.

Round 69 surveys emerging 2026 authorization/delegation practice and maps it onto engineering-platform's existing authenticated-principal, grant, Worker, RuntimeProfile, WIF and Action Gateway model.

## 2. Open Agent Auth

Reference:
- https://github.com/alibaba/open-agent-auth

Open Agent Auth implements an emerging Agent Operation Authorization model with:
- user/workload/token cryptographic identity binding;
- request-level temporary workload identity;
- fine-grained operation authorization;
- OAuth/OIDC/WIMSE alignment;
- semantic audit trails;
- MCP integration;
- explicit user-consent traceability.

The useful pattern is to distinguish the human/accountable principal from the ephemeral workload/agent identity that actually exercises a bounded operation.

## 3. Delegation chains and monotonic scope narrowing

References:
- https://github.com/microsoft/agent-governance-toolkit
- https://github.com/cred-ninja/protocol

Current multi-agent authorization work is converging on several principles:
- authority passed to a child can only stay equal or narrow, never expand;
- each delegation hop is explicit and independently verifiable;
- short-lived credentials are preferred over static shared secrets;
- proof-of-possession can bind a credential to the intended workload/agent;
- revoking an upstream grant invalidates descendants;
- delegation receipts/audit preserve the originating accountable principal.

Conceptually:

    Human / service principal
          |
          v
    authenticated parent agent
          | narrowed delegation
          v
    child agent
          | narrowed delegation
          v
    sub-agent

At every hop:

    child_scope ⊆ parent_scope

and expiry/action/resource bounds may only become stricter.

## 4. Credential brokering rather than credential disclosure

An important pattern in emerging delegation protocols is that the agent does not need to see the raw downstream credential.

Preferred flow:

    agent
      -> requests operation/grant reference
      -> trusted broker validates identity + delegation + policy
      -> broker obtains/exercises short-lived scoped credential
      -> resource operation
      -> result + receipt returned to agent

This strongly matches engineering-platform's current credential separation and Action Gateway direction.

Do not inject broad GitHub/cloud/device credentials into a child-agent context merely because the parent was authorized.

## 5. Potential future Core projection

No new M1 entity is needed.

If real multi-agent execution becomes a product requirement, introduce a minimal immutable delegation fact such as:

    DelegationGrant
      - delegation_id
      - parent_principal / parent_runtime_identity
      - child_runtime_identity
      - parent_grant reference
      - narrowed capabilities/actions
      - resource/target constraints
      - issued_at / expires_at
      - execution/run/task binding
      - nonce/key/proof binding where used
      - issuer/policy revision
      - revocation state/reference
      - digest/signature/receipt identity

The exact wire protocol may be OAuth token exchange, WIMSE-aligned credentials, another future standard, or an internal implementation. Core should retain the semantic fact rather than depend on one draft token shape.

## 6. Delegation does not transfer engineering authority

Even a cryptographically valid child delegation only authorizes an operation ceiling.

It does not imply:
- the child result is correct;
- a child completion is Verification;
- a delegated reviewer is independent;
- Evidence may be self-issued without provenance checks;
- a privileged external mutation can bypass Action Gateway.

Reviewer independence must also check the delegation/identity lineage where relevant; a child of the implementing agent is not automatically an independent reviewer just because it has a different process or model.

## 7. Audit and Evidence requirements

For a delegated operation retain enough facts to answer:
- which accountable human/service principal originated the authority;
- which parent agent delegated;
- which child identity actually acted;
- which exact capability/resource/action scope was delegated;
- whether the scope monotonically narrowed;
- which policy/grant revision authorized the hop;
- which credential/proof mechanism was used;
- when it expired or was revoked;
- which ActionReceipt / Artifact / Evidence resulted.

## 8. Adoption posture

For current M1:
- keep one authenticated engineering Runtime identity and existing grant model;
- keep WIF/saved-login and publisher credentials separate;
- do not add an agent IAM service merely because standards are emerging.

Activate explicit delegation only when a retained pilot demonstrates real value from sub-agent/peer-agent execution.

When activated:
1. start with one parent -> one child, read-only/development-only capability;
2. enforce monotonic scope narrowing in Core policy;
3. keep downstream credentials brokered outside model context;
4. retain a delegation receipt and operation receipt;
5. add a negative test proving a child cannot exercise a capability absent from the parent or outside the narrowed grant;
6. only then support multi-hop chains.
