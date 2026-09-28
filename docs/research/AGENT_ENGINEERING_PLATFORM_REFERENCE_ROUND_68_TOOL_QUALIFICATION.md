# Agent Engineering Platform Reference — Round 68 (Tool and MCP Qualification)

Date: 2026-09-29  
Status: **Research / future ToolProfile qualification reference; not tool authority**

## 1. Why this is separate from runtime policy

Runtime gateways can allow/deny a tool call, but they do not answer whether the tool package/server itself is trustworthy enough to admit into a Formal Run.

Round 68 focuses on **ToolProfile supply-chain and behavior qualification**.

## 2. Docker MCP Gateway

References:
- https://github.com/docker/mcp-gateway
- https://github.com/docker/mcp-gateway/blob/main/docs/security.md

Important security patterns:
- MCP servers run with explicit isolation/lifecycle management;
- secrets managed outside ordinary environment-variable sprawl;
- tool/prompt/resource name collisions are rejected;
- backend tool schemas are treated as untrusted input;
- policy applies consistently across direct, dynamic and code-generated tool paths;
- remote URL/private-network/metadata protections;
- signature verification and mutable-tag concerns for MCP images;
- default secret scanning around tool-call arguments/responses;
- logging avoids raw sensitive argument values by default.

These strengthen the existing rule that every Formal Run binds an exact ToolProfile rather than an ambient tool catalog.

## 3. Static and dynamic agent-component scanners

### Snyk Agent Scan

Reference:
- https://github.com/snyk/agent-scan

Scans MCP servers, agent components and skills for prompt-injection and security risk. A particularly important operational lesson is that **scanning an MCP config can itself execute code or contact remote servers**; untrusted inspection therefore belongs in a disposable sandbox with explicit consent/network controls.

### MCPRadar

Reference:
- https://github.com/yatuk/mcpradar

Relevant capabilities:
- protocol-surface inspection;
- poisoning/injection/schema/security checks;
- source-code analysis;
- configuration/hook/permission analysis;
- package download without install scripts;
- OSV/dependency checks;
- CycloneDX SBOM and provenance;
- disposable sandbox for untrusted stdio servers;
- snapshots and tool fingerprints;
- cosmetic vs behavioral vs security drift classification;
- JSON/SARIF/policy outputs.

### MCP Gateway Registry security scanning

Reference:
- https://github.com/agentic-community/mcp-gateway-registry

Useful as an example of registry + scanner integration for MCP servers, A2A agents and Agent Skills. Registry metadata and scan findings remain discovery/qualification facts, not capability grants.

## 4. ToolProfile qualification facts

A future exact ToolProfile should be able to bind, where applicable:
- logical tool/server identity;
- package/image/repository source;
- immutable version/digest;
- publisher/issuer identity;
- signature/attestation verification result;
- tool schema/resource/prompt fingerprint;
- exposed capabilities;
- filesystem mounts and write scope;
- network/egress scope;
- secrets/credentials required and injection method;
- process/container/runtime identity;
- dependency lock/SBOM/vulnerability snapshot;
- static security findings;
- dynamic protocol findings;
- known behavioral fingerprint/baseline;
- scan tool/version/policy identity;
- qualification timestamp/freshness policy.

## 5. Qualification is not authorization

Keep the separation:

    ToolQualificationFacts
          |
          v
    ToolAdmissionPolicy
          |
          v
    approved ToolProfile
          |
          v
    TaskContract / Run action ceiling
          |
          v
    per-call Action/Runtime policy

A high scan grade or clean SBOM does not grant a tool access to source, secrets, GitHub, devices or networks.

## 6. Behavioral drift / rug-pull protection

Tool identity cannot be path/name only.

If a previously approved MCP server changes:
- package/image digest;
- tool descriptions;
- input/output schema;
- prompt/resources;
- requested mounts/permissions;
- network destinations;
- dependency tree;

the previous qualification must not silently carry forward.

Recommended model:

    ToolProfileRevision
       -> immutable fingerprints
       -> qualification receipt
       -> freshness/expiry
       -> supersedes prior revision

Behavioral/schema drift can require requalification even when the human-facing tool name is unchanged.

## 7. Scan safety

Security tooling itself needs a threat model:
- prefer source/archive analysis before execution;
- never run package install/lifecycle scripts merely to inspect metadata;
- start untrusted stdio servers only in disposable isolation;
- default-deny host mounts;
- bound CPU/memory/process/output/time;
- protect loopback/private/cloud-metadata destinations;
- retain evidence of which paths were actually inspected;
- report incomplete/unreachable as missing evidence, not clean.

## 8. engineering-platform adoption

For M1, no new MCP gateway or scanner dependency is required.

After M1:
1. freeze the minimal canonical ToolProfile fields already required by real Skills;
2. add one security qualification adapter for an external MCP/tool package;
3. retain raw scan/SBOM/signature artifacts as qualification Evidence;
4. test deliberate schema/description/package drift and require requalification;
5. keep per-call privileged mutations under the existing Action Gateway rather than delegating business authority to a generic MCP firewall.
