# Architecture Review — Round 9: External Security and Supply-Chain Cross-Check

Date: 2026-09-23
Reviewed baseline: docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_1.md
Depends on: Round 2–8 reviews
Focus: cross-check current architecture against OWASP agent/LLM security guidance, SLSA provenance requirements and NIST SSDF supply-chain practices.

## Result

The current architectural direction is consistent with the external guidance reviewed.

No major reversal is required.

The cross-check mainly strengthens several existing P0 decisions:
- security controls must be deterministic and external to the LLM;
- agent/tool privileges must be least-privilege and resource-scoped;
- high-impact actions need human authority;
- untrusted repository/tool/web content must not become authority;
- build provenance must identify exact output artifacts and trusted builders;
- provenance/signing secrets must remain outside user-controlled build steps;
- release components need provenance and secure-development records;
- adversarial testing should be part of platform acceptance.

---

## 1. Prompt Injection: current architecture is correctly fail-outside-model

OWASP guidance treats prompt injection as a structural risk of LLM systems and recommends:
- least-privilege tools;
- tool parameter validation;
- separation of untrusted external content;
- human approval for high-risk actions;
- deterministic controls outside the model.

This supports the platform rules already established:

~~~text
untrusted context
  -> Runtime may reason over it
  -> Runtime may request an action
  -> deterministic Policy / Action Gateway decides
~~~

Do not weaken this into:
"the system prompt tells Codex not to do dangerous things."

The Runtime must continue to be treated as an untrusted decision proposer for authority-bearing actions.

References:
- https://cheatsheetseries.owasp.org/cheatsheets/LLM_Prompt_Injection_Prevention_Cheat_Sheet.html
- https://genai.owasp.org/llmrisk/llm01-prompt-injection/
- https://cheatsheetseries.owasp.org/cheatsheets/AI_Agent_Security_Cheat_Sheet.html

---

## 2. Tool/MCP responses must remain untrusted data

OWASP tool-poisoning guidance reinforces a point that should be explicit in engineering-platform:

A trusted-looking tool response can itself contain malicious instructions.

Therefore:
- external tool/MCP responses are data, not authority;
- prefer typed/validated structured outputs;
- response content cannot widen capability grants;
- privileged tool execution remains server-side/policy-enforced;
- arbitrary MCP/tool servers are not dynamically trusted in Formal Runs.

This should apply to future WorkBuddy, Knowledge and Skill integrations as well.

Reference:
- https://community.owasp.org/attacks/MCP_Tool_Poisoning

---

## 3. SLSA reinforces the Evidence Issuer / trusted builder split

SLSA provenance guidance distinguishes:
- the artifact subject/digest;
- build definition/inputs;
- builder identity;
- provenance authenticity;
- resistance to tenant/build-step forgery.

This aligns directly with the proposed:
- Artifact digest;
- Build Environment/Profile;
- Evidence/Provenance Issuer Registry;
- trusted build/verifier identity;
- signatures/attestations;
- separation between untrusted Run sandbox and trusted provenance generator.

For release-grade provenance, the platform should prefer a trusted build/verifier service to generate or verify provenance rather than allowing user-controlled build scripts to assert their own authoritative provenance.

References:
- https://slsa.dev/spec/v1.0/requirements
- https://slsa.dev/spec/v1.2-rc2/build-provenance

---

## 4. Signing/provenance secrets must remain outside build steps

SLSA higher-assurance requirements explicitly keep authentication/signing secrets outside user-controlled build environments.

That reinforces the existing design:

~~~text
Runtime / user build steps
    X no production signing key

Trusted Signing/Provenance Service
    -> privileged transform
    -> signed Artifact / attestation
~~~

This is a hard boundary, not merely an implementation preference.

---

## 5. NIST SSDF supports secure environment + provenance + risk/decision records

NIST SSDF includes practices around:
- maintaining secure software development environments;
- collecting/sharing provenance for release components;
- tracking security requirements, risks and design decisions.

This supports:
- controlled Worker/build environments;
- Artifact provenance;
- typed Decision/Risk records;
- Assurance Profiles;
- release/closure manifests.

Reference:
- https://csrc.nist.gov/projects/ssdf

---

## 6. New P1 — Formal Runs need explicit agent loop/tool-chain limits

OWASP agent guidance calls out denial-of-wallet/resource abuse and recommends cost, retry and tool-chain limits.

The platform already has admission control/cost tracking, but Formal Run policy should also support:
- max wall-clock duration;
- max model/API budget;
- max tool/action count where appropriate;
- max retry count;
- max recursive/sub-agent depth when multi-agent is introduced;
- max external network/data-transfer quota;
- max concurrent privileged operations.

Exceeding a limit should pause/request authority or terminate according to policy, not silently continue.

This is operational safety, not engineering Verification.

---

## 7. New P1 — Provider/tool changes require adversarial re-validation

Security properties can change when:
- Runtime provider/model changes;
- prompt/runtime adapter changes;
- tool schema changes;
- Session Gateway changes;
- Action Gateway changes;
- Context Resolver/RAG changes;
- MCP/tool server is added;
- sandbox/network policy changes.

Maintain an adversarial regression suite for:
- direct prompt injection;
- indirect repository/document injection;
- malicious tool output;
- secret exfiltration attempt;
- policy bypass request;
- unexpected cross-project resource request;
- unbounded loop/cost escalation;
- privilege-escalation tool chaining.

A feature being functionally compatible does not prove security compatibility.

---

## 8. Future M4/M5 risk — Skill and Knowledge poisoning

Skills and Knowledge are deliberately deferred, which is good.

When introduced:

### Skill package
must have:
- origin/owner;
- version;
- content digest;
- required capabilities;
- trust level;
- review/approval status;
- compatibility/runtime constraints.

A Skill cannot self-grant capability.

### Knowledge item
must preserve:
- source provenance;
- trust class;
- revision/supersession;
- confidentiality;
- freshness;
- curator/issuer;
- content digest.

Retrieved Knowledge remains context, not policy authority unless explicitly promoted through a controlled Decision process.

This prevents future RAG/knowledge poisoning from bypassing the architecture.

---

## 9. Future multi-agent rule

If M4 eventually introduces multi-agent workflows:

- one agent's text is not another agent's authority;
- inter-agent messages need typed source/role metadata;
- capabilities remain assigned by Control Plane, not delegated by agents;
- privileged action result is always checked by Platform Action Gateway;
- agent recursion/delegation must have hard depth/cost limits;
- high-impact decisions still require the same Human Authority and Verification rules.

Do not create a separate security model for multi-agent execution.

---

## 10. Cross-check verdict

External guidance supports, rather than weakens, the current architecture.

Keep these principles as non-negotiable:

1. Treat Runtime/agent as an untrusted proposer for privileged action.
2. Enforce policy outside the model.
3. Treat external context/tool output as untrusted data.
4. Keep credentials/signing keys outside user-controlled agent/build steps.
5. Bind provenance to exact Artifact digest and trusted builder/issuer.
6. Require human authority for high-impact/irreversible action.
7. Continuously adversarial-test agent/tool/context boundaries.

No new Plane or standalone "AI Security Agent" is needed.
