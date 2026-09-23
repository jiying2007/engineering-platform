# Open Source Reference Review — Round 25: Product Families, SKU Variants and Configuration Spaces

Date: 2026-09-24
Status: **Archived research / implementation input**

References:
- FeatureIDE
- Linux/Zephyr Kconfig / Kconfiglib
- ERPNext Item Variants
- Odoo Product Variants
- Buildroot/Yocto configuration patterns

## 1. Key conclusion

A product family needs a formal separation between:

~~~text
Feature/Variant Rules
 -> Resolved Product Variant
 -> TargetRevision
 -> Release Bundle
 -> DeviceInstance
~~~

Do not encode product families as ad-hoc SKU if/else rules scattered through build scripts and test plans.

---

## 2. FeatureModelArtifact

Add immutable:

~~~text
FeatureModelArtifact
~~~

Represents the allowed product/configuration space.

May contain:
- features/options;
- mandatory/optional relations;
- mutually exclusive choices;
- dependency/require/exclude constraints;
- attributes/ranges;
- default selection;
- model version;
- content digest.

FeatureIDE-style feature modeling is a strong reference.

---

## 3. ProductVariantDefinition

Add:

~~~text
ProductVariantDefinition
~~~

Defines one market/product/SKU variant at an intent level.

Fields:
- family/product;
- SKU/market identifier;
- feature selections;
- hardware options;
- region/regulatory options;
- commercial attributes;
- source/ERP/PLM refs;
- effective window;
- content digest.

This is not yet the complete resolved engineering configuration.

---

## 4. VariantResolutionReceipt

Resolve:

~~~text
FeatureModelArtifact
 + ProductVariantDefinition
 + Target/platform defaults
 + constraints
 -> VariantResolutionReceipt
 -> ResolvedConfigurationArtifact
 -> TargetRevision
~~~

Receipt records:
- model/version;
- selected options;
- implied/defaulted options;
- rejected/conflicting options;
- resolver/version;
- final resolved digest.

This mirrors Kconfig's dependency-driven final .config concept.

---

## 5. VariantConstraintEvidence

Add:

~~~text
VariantConstraintEvidence
~~~

Proves:
- requested variant is satisfiable;
- feature constraints pass;
- forbidden combination is rejected;
- required compatibility predicates hold.

This is configuration validity Evidence, not functional qualification.

---

## 6. ERP variant identity vs engineering variant

ERPNext/Odoo distinguish:
- a template/product family;
- concrete variants with their own inventory/transactions.

engineering-platform should reference ERP SKU/Item IDs, but keep its own exact engineering resolution.

One commercial SKU may map to:
- multiple hardware revisions over time;
- multiple supplier alternatives;
- multiple software Release cohorts.

Therefore:

~~~text
SKU
!=
TargetRevision
~~~

---

## 7. Variant effectivity

Use EffectivityRule for:
- SKU;
- market/region;
- hardware revision;
- lot/serial range;
- date;
- factory;
- compliance window.

A feature/config change can affect only a subset of the product family.

---

## 8. Variant matrix

Generate projection:

~~~text
Product Family
 x Variant
 x TargetRevision
 x Release
 x Verification Coverage
 x Manufacturing/Fleet population
~~~

This is a read model, not separate mutable authority.

---

## 9. Variant verification coverage

Avoid full Cartesian explosion where possible.

Add policy-driven:

~~~text
VariantCoveragePlan
~~~

which identifies:
- equivalence classes;
- representative variants;
- mandatory edge variants;
- high-risk combinations;
- excluded combinations;
- rationale;
- coverage Evidence.

Selection is explicit and auditable.

---

## 10. Variant equivalence

If Evidence from Variant A is reused for Variant B, require:

~~~text
VariantEquivalenceDecision
~~~

Binds:
- source/target variants;
- Evidence class;
- exact equivalence scope;
- shared components/config;
- differing dimensions;
- justification;
- validation Evidence;
- policy/authority.

Default:
- no implicit cross-variant Evidence reuse.

---

## 11. Kconfig practice

Zephyr/Linux Kconfig is valuable because:
- available options and dependencies are explicit;
- invalid assignments are constrained;
- board/app fragments merge into a final resolved configuration;
- the final generated configuration is build input.

### Absorb

For embedded software configuration:

~~~text
Kconfig sources/fragments
 -> resolver
 -> final .config / resolved values
 -> ResolvedConfigurationArtifact
 -> BuildDefinitionManifest
~~~

Do not use fragment filenames as final identity.

---

## 12. Build variants

BuildDefinitionManifest binds:
- ProductVariantDefinition/TargetRevision;
- resolved config;
- toolchain;
- source;
- feature set.

Different feature resolution = different Build Definition even at same commit.

---

## 13. Hardware variants

Variant resolution may choose:
- SoC;
- flash vendor/size;
- Wi-Fi module;
- sensor set;
- motor/driver;
- charger/dock options;
- region-specific RF/power component.

Selected hardware structure maps to ProductStructureSnapshot/TargetRevision.

---

## 14. Runtime variants

Not every variant is compile-time.

Some behavior may depend on:
- RuntimeConfigurationSnapshot;
- feature flag;
- calibration;
- locale/region;
- service entitlement.

These remain exact subject inputs rather than being hidden behind SKU label.

---

## 15. Product family compatibility

A Release can declare support set:

~~~text
SupportedVariantSet
~~~

defined by:
- explicit variants;
- feature predicates;
- TargetRevision ranges;
- exclusions.

Release Admission verifies actual Device/Target/Variant belongs to supported set.

---

## 16. Variant migration

When ProductVariantDefinition or feature rules change:
- old devices keep historical resolved variant identity;
- new Target/variant revisions are created;
- effectivity decides population transition;
- no historical device is silently "reinterpreted" under a new model.

---

## 17. Variant and requirements

Requirement/AC may scope to:
- all family variants;
- specific SKU;
- feature predicate;
- region;
- hardware option.

TraceLink and Verification Plan preserve that scope.

---

## 18. Variant and manufacturing

ProductionRecipeRevision binds exact:
- ProductVariantDefinition;
- TargetRevision;
- ProductStructureSnapshot;
- resolved config.

Manufacturing cannot infer engineering variant solely from sales SKU.

---

## 19. M0/M1 impact

M0 reserves:
- FeatureModelArtifact;
- ProductVariantDefinition;
- VariantResolutionReceipt;
- VariantConstraintEvidence;
- VariantCoveragePlan;
- VariantEquivalenceDecision;
- SupportedVariantSet.

M1 can use one trivial single-variant family fixture.

---

## 20. M2/M3 impact

Apply to real:
- regional SKUs;
- flash/Wi-Fi/power alternatives;
- sensor/feature variants;
- hardware revision transitions;
- production/fleet segmentation.

---

## 21. Invariants

1. Product family rules and concrete resolved variants are distinct.
2. ERP SKU identity is not TargetRevision identity.
3. Exact resolved feature/config selection participates in Build/Release subject.
4. Variant constraints are machine-validated.
5. Cross-variant Evidence reuse requires explicit equivalence.
6. Variant coverage reduction is policy-driven and auditable.
7. Historical devices retain the variant model/revision under which they were built.
8. Release support set is explicit and checked at admission.
9. Requirement/Verification scope can be feature/variant specific.
10. Manufacturing recipes bind exact engineering variant, not sales label alone.

## 22. Conclusion

The durable product-family chain is:

> **Feature Model → Product Variant Definition → Resolved Configuration → TargetRevision → Verification Coverage → Release/Manufacturing/Fleet effectivity.**
