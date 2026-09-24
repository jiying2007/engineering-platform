# Public / Open Source Reference Review — Round 34: Long-Term Compatibility and Support Matrices

Date: 2026-09-24
Status: **Archived research / implementation input**

References:
- Kubernetes Version Skew Policy
- Android VINTF compatibility matrices
- Kubernetes conformance program
- existing InterfaceCompatibilityPolicy, SupportedVariantSet, SystemUpdateManifest and SecuritySupportPolicy

## 1. Key conclusion

Long-lived products need more than pairwise API compatibility.

The platform must be able to answer:
- which component versions may coexist?
- which product generations are supported?
- how far may one component lag another?
- what upgrade order is required?
- which old versions still receive fixes?
- which paths are unsupported even if endpoints compile?

The durable model is a machine-readable compatibility envelope plus support policy.

---

## 2. CompatibilityEnvelope

Add immutable:

~~~text
CompatibilityEnvelope
~~~

Defines a valid multi-component version/configuration space.

May include:
- component identities;
- allowed version ranges;
- min/max skew;
- required interface revisions;
- Target/variant constraints;
- runtime/config predicates;
- unsupported combinations;
- upgrade-order constraints;
- deprecation/removal dates;
- policy version;
- content digest.

This is broader than one InterfaceCompatibilityPolicy.

---

## 3. VersionSkewPolicy

Add:

~~~text
VersionSkewPolicy
~~~

Examples:
- main MCU may be at most one protocol generation ahead of motor MCU;
- Linux application supports MCU protocol v3-v5;
- dock firmware must not be newer than main MCU control protocol support;
- mobile app supports device API for N generations.

Kubernetes is a strong reference because it explicitly documents component-specific allowed skew and upgrade order.

---

## 4. UpgradeOrderConstraint

Some valid end states are unsafe to reach in the wrong order.

Represent:

~~~text
UpgradeOrderConstraint
~~~

Examples:
- server/control plane before client/node;
- bootloader before application;
- main MCU before motor MCU;
- config schema migration before service switch;
- new credential/trust root before key retirement.

SystemUpdateManifest references these constraints.

---

## 5. ProductSupportPolicy

Add generic:

~~~text
ProductSupportPolicy
~~~

Fields:
- product/variant/Target family;
- supported Release lines;
- support start/end;
- maintenance/LTS branches;
- allowed upgrade sources/targets;
- minimum supported component versions;
- deprecation/EOL dates;
- security support relation;
- rollback/downgrade policy;
- migration exceptions.

SecuritySupportPolicy may reference or specialize it.

---

## 6. CompatibilityMatrixProjection

Generate:

~~~text
Component A version
 x Component B version
 x Variant/Target
 x Interface revisions
 x Release line
 -> SUPPORTED / CONDITIONALLY_SUPPORTED / UNSUPPORTED / UNKNOWN
~~~

This is a projection from policies/evidence, not a manually edited spreadsheet.

---

## 7. Android VINTF practice

Android VINTF separates:
- framework compatibility matrix;
- device compatibility matrix;
- manifests;
- runtime/build/test enforcement.

### Absorb

For embedded multi-component products:
- provider requirements and consumer requirements should both be machine-readable;
- compatibility should be checked at:
  - build/integration;
  - update admission;
  - runtime/boot where possible;
  - qualification tests.

This is stronger than CI-only checking.

---

## 8. Kubernetes version-skew practice

Kubernetes documents:
- explicit max skew;
- which components may be newer/older;
- supported release branches;
- upgrade order.

### Absorb

Compatibility rules should encode directionality.

Examples:

~~~text
client must not be newer than server
~~~

and:

~~~text
client may lag server by up to N versions
~~~

are different rules.

Do not model skew as absolute version difference only.

---

## 9. CrossVersionQualificationEvidence

Add:

~~~text
CrossVersionQualificationEvidence
~~~

Binds:
- exact component version set;
- CompatibilityEnvelope;
- Target/variant;
- Procedures/tests;
- result;
- environment;
- coverage;
- issuer.

A policy may require representative cross-version tests before declaring a combination supported.

---

## 10. Conformance profile

For stable external/internal platform contracts, define:

~~~text
ConformanceProfile
~~~

as a selected mandatory behavior/test set for a version line.

Kubernetes conformance shows value in:
- versioned required tests;
- machine-checkable submissions;
- preserving historical conformance evidence.

Use only where interface ecosystem size justifies it.

---

## 11. Support is not compatibility

Keep separate:

~~~text
Technically compatible
!=
Currently supported
~~~

A very old combination may still work but be outside support/security maintenance.

Conversely a combination may be nominally supported but require a migration step before use.

Both status dimensions should be visible.

---

## 12. Long-term test strategy

Maintain:
- golden cross-version fixtures;
- oldest-supported/newest-supported pairs;
- upgrade/downgrade paths;
- mixed-version transition states;
- rollback;
- persisted-data migration.

Test selection uses VersionSkewPolicy and ProductSupportPolicy.

---

## 13. Compatibility after EOL

When a version exits support:
- historical Evidence remains;
- existing devices remain identifiable;
- new Release/rollout may block targeting;
- ProductSecurityCase may use different remediation policy;
- migration Work may be generated.

Do not rewrite old support status as if the version never existed.

---

## 14. Multi-generation devices

A fleet may contain:
- HW V1/V2/V3;
- multiple MCU generations;
- different flash/Wi-Fi suppliers;
- old/new bootloaders;
- different app/cloud protocol generations.

SupportedVariantSet + CompatibilityEnvelope + ProductSupportPolicy define the usable population.

---

## 15. Runtime negotiation

If components negotiate capabilities/version at runtime:
- capture negotiated result;
- compare to CompatibilityEnvelope;
- expose mismatch/drift;
- use as Evidence.

Negotiation does not override unsupported policy unless explicit compatibility path permits it.

---

## 16. M0/M1 impact

M0 reserves:
- CompatibilityEnvelope;
- VersionSkewPolicy;
- UpgradeOrderConstraint;
- ProductSupportPolicy;
- CrossVersionQualificationEvidence;
- optional ConformanceProfile.

M1 uses one simple self-compatibility fixture.

---

## 17. M2/M3 impact

Apply to real:
- Linux/main MCU/motor MCU/dock version sets;
- OTA upgrade ordering;
- app/cloud/device compatibility;
- multi-generation fleet support;
- EOL migration.

---

## 18. Invariants

1. Pairwise API compatibility is insufficient for multi-component product support.
2. Version skew rules are directional and component-specific.
3. Valid end state does not imply every upgrade order is valid.
4. Technical compatibility and support status are distinct.
5. Compatibility matrices are derived from policy/evidence.
6. Cross-version support is backed by exact-version qualification Evidence.
7. EOL changes future policy; it does not rewrite historical state.
8. Runtime negotiation is observed Evidence, not authority to bypass support policy.
9. SystemUpdateManifest obeys compatibility envelope and upgrade-order constraints.
10. Fleet targeting checks exact device/variant/component versions against current support policy.

## 19. Conclusion

The durable long-term-support rule is:

> **A product version is supportable only when the entire component set lies inside a declared compatibility envelope and there is a verified path to reach and recover from that state.**
