# Architecture Review — Round 8: Readiness, Risk, Assurance and Lifecycle Governance

Date: 2026-09-23
Reviewed baseline: docs/architecture/AI_NATIVE_ENGINEERING_PLATFORM_V1_1.md
Depends on: Round 2–7 reviews
Focus: requirement readiness, risk classification, verification planning, supersession/cancellation, waivers, authority delegation and closure/reopen semantics.

## Result

The technical architecture is strong, but the lifecycle still assumes that once a Requirement exists the system can safely plan and execute.

That is not always true.

A Requirement can be syntactically valid yet still be unsuitable for formal engineering because:
- acceptance is not measurable;
- target/configuration is unknown;
- risk is unclassified;
- required verification is unspecified;
- dependencies are unresolved;
- authority/owner is missing;
- constraints conflict;
- the requested change is unsafe or impossible to verify.

The platform therefore needs explicit readiness and assurance semantics before execution, and explicit supersession/waiver semantics during and after execution.

No new top-level Plane is required.

---

## P0-1 — Requirement READY needs guarded semantics

READY must not mean "required JSON fields exist".

A Requirement Revision can become READY only if required readiness checks pass.

Recommended readiness dimensions:

- product owner identified;
- goal/scope/non-goals present;
- target/product variant known or explicitly not applicable;
- acceptance criteria are testable or have an approved qualitative-review method;
- constraints are internally consistent;
- required risk classification complete;
- data/confidentiality classification complete where relevant;
- known dependencies recorded;
- required authority for unresolved choices identified;
- no blocking contradiction/open question;
- initial verification/assurance requirements derivable.

Readiness result is recorded as an immutable Decision against the exact Requirement Revision digest.

---

## P0-2 — Acceptance Criterion needs a verifiability contract

Each AC should carry more structure than free text.

Example:

~~~yaml
acceptance:
  - ac_id: AC-01
    statement: ...
    verification_class: device_statistical
    metric: docking_success_rate
    comparator: gte
    threshold:
      baseline_ref: BASE-...
    sample_requirement:
      count: 100
    target_scope:
      target_revision: ...
    authority:
      owner: product
~~~

Not every AC needs a numeric threshold, but every required AC needs a declared verification mode, such as:
- automated test
- measurement
- statistical evaluation
- independent human review
- security review
- compliance/manual inspection

"Looks good" must not silently become machine-verifiable PASS.

---

## P0-3 — Add an Assurance Profile selected from risk

Verification intensity should not be manually re-invented for every Task.

Define a versioned Assurance Profile/policy selected from risk attributes.

Potential risk dimensions:
- production impact
- safety/physical actuation
- security
- boot/update/recovery
- persistent-data migration
- hardware damage potential
- privacy/confidentiality
- regulatory/customer commitment
- blast radius
- reversibility

Example profiles:

~~~text
A0 — low-risk internal/dev
A1 — normal product code
A2 — device/firmware operational risk
A3 — boot/OTA/security/safety critical
A4 — irreversible/production critical
~~~

Profiles define minimum:
- Verification classes
- Evidence issuer trust
- Review independence
- approval/quorum
- required rollback/migration evidence
- retention/provenance level
- whether break-glass is allowed

The profile label is not itself the authority; the versioned policy bundle is.

---

## P0-4 — Verification Plan should exist before implementation completes

The Acceptance -> Evidence Matrix is valuable, but waiting until the end to discover missing Evidence is too late.

Create an immutable Verification Plan Artifact/record for a Work/Subject scope.

It maps:
- Requirement AC
- risk/Assurance Profile
- target/variant
- required procedure/revision class
- expected Evidence type
- trusted issuer class
- sample/repetition requirements
- review requirement
- release/final-stage checks

The plan may evolve by revision/Decision, but changes are auditable.

Planner/AI may propose it; Control Plane/authorized actors freeze it.

---

## P0-5 — Requirement/Task supersession must immediately affect active privilege

Scenario:

~~~text
RUN-812 executes TASK-5-r2 for REQ-r3
product creates REQ-r4 that supersedes r3
~~~

Do not allow RUN-812 to continue privileged Formal actions indefinitely under obsolete authority.

Policy should classify active Runs:
- unaffected: may continue;
- pending impact review: privileged actions paused;
- superseded: Run becomes terminal/superseded;
- allowed diagnostic continuation: execution may continue as non-authoritative/Explore-like investigation with downgraded capabilities.

The dependency/impact Decision determines which case applies.

A superseded Requirement/Task cannot remain an invisible source of current production authority.

---

## P0-6 — Task replan needs explicit old-run disposition

When TASK-r1 becomes TASK-r2:

Existing Runs need a Decision:
- ABORT
- SUPERSEDE
- KEEP_FOR_EVIDENCE_ONLY
- CONTINUE_DIAGNOSTIC
- NO_IMPACT_CONTINUE

Do not merely create a new Task Revision while old Runs keep running with full capabilities.

---

## P0-7 — Cancellation semantics must revoke future authority

Cancellation is not deletion.

When Work/Task/Run is cancelled:
- active execution lease is revoked;
- future privileged action grants are revoked;
- pending approvals become inapplicable;
- queued Temporal/external operations are cancelled where safe;
- UNKNOWN operations are reconciled;
- produced historical artifacts/evidence remain immutable but cannot silently authorize release;
- cancellation Decision/reason is retained.

A late callback from a cancelled Run cannot resurrect it.

---

## P0-8 — Waiver / Exception / Risk Acceptance needs exact scope and expiry

"Accept risk" cannot be a generic boolean.

Represent as a typed Decision:

~~~yaml
decision_type: WAIVER | RISK_ACCEPTANCE | POLICY_EXCEPTION
subject_digest: ...
risk_or_rule_id: ...
scope:
  project: ...
  environment: ...
  release_manifest: ...
reason: ...
authority: ...
valid_from: ...
valid_until: ...
max_uses: ...
compensating_controls: [...]
follow_up_work: [...]
~~~

Rules:
- expired waiver cannot satisfy current release policy;
- manifest/subject change invalidates it unless explicitly scoped;
- one environment's waiver cannot replay into another;
- permanent exception requires stronger authority than temporary exception;
- unresolved follow-up can block later release according to policy.

---

## P0-9 — "Known risk" and "accepted risk" are different

A risk can exist in states such as:

~~~text
IDENTIFIED
ANALYZED
MITIGATED
ACCEPTED
TRANSFERRED
CLOSED
EXPIRED_ACCEPTANCE
~~~

Do not treat "documented in review" as "accepted".

Closure/Release gates require all blocking risks either:
- mitigated/closed; or
- covered by an applicable authorized Risk Acceptance.

---

## P0-10 — Work closure conditions must be explicit

CLOSED should be guarded by a Closure Policy.

Possible required predicates:
- all required current Task Revisions are terminal;
- current Subject/Release manifests are frozen;
- required AC have applicable Verification;
- required Reviews complete;
- blocking risks resolved/accepted;
- required release completed or explicitly "no release required";
- required follow-up Work linked;
- Closure Manifest successfully generated/finalized.

Closure is a Decision over the exact current closure graph, not a UI state toggle.

---

## P0-11 — Reopen does not rewrite historical closure

A closed Work may need reopening because:
- field defect
- evidence revoked
- issuer compromised
- requirement changed
- release rollback
- audit finding

Do not move the historical closure record back to "open" as if closure never happened.

Preferred model:
- immutable Closure Manifest remains;
- create a Reopen/Impact Decision;
- transition current Work projection or create follow-up Work according to policy;
- preserve the historical closed snapshot and reason it ceased to be sufficient.

---

## P0-12 — Release eligibility and Work completion are distinct

A Work may be engineering-complete but:
- not intended for production release;
- waiting for a coordinated product train;
- superseded by another implementation;
- integrated but feature-flagged off.

Conversely, a Release may aggregate several Works.

Do not make Work.CLOSED == production released a universal invariant.

Closure policy and Release policy are separate.

---

## P0-13 — Plan approval must bind exact Plan/Task graph digest

If WorkBuddy exposes approve_plan, the approval must bind an immutable plan representation.

The plan should identify:
- Task Revisions / DAG
- repositories/targets
- risk/Assurance Profile
- Verification Plan digest
- major dependencies
- expected privileged resources
- important open risks

Changing the plan materially creates a new plan digest and may require renewed approval according to policy.

Do not let "approved plan" float across arbitrary replanning.

---

## P0-14 — Authority delegation needs explicit lifecycle

Human-required gates can deadlock if one person is unavailable.

Define controlled delegation:
- delegator
- delegate
- authority scope
- project/environment
- valid_from / valid_until
- max risk/operation class
- revocation
- prohibition on re-delegation where appropriate

Delegation never implicitly bypasses separation-of-duties.

Example:
if requester != approver is required, delegating requester authority to the same approver cannot collapse the rule.

---

## P0-15 — Clarification answers have authority and scope

A Runtime may ask a question whose answer changes:
- implementation detail
- Task contract
- Requirement acceptance

The answer record must identify:
- question ID
- exact question/context digest
- required authority
- answering actor
- answer
- scope
- resulting classification: Steering / Task Revision / Requirement Revision
- effective revision/change reference

Do not inject free-form WorkBuddy answers directly into the Runtime without classification.

---

## P1-1 — Add approval/clarification SLA and escalation metadata

Long-running work may spend most time waiting for humans.

Track:
- requested_at
- required authority
- due/SLA target
- escalation route
- reminder policy
- fallback/delegation eligibility

This is operational metadata, not a reason to auto-approve.

---

## P1-2 — Explicit ownership per Work/Task

Record:
- product owner
- engineering owner
- verification owner
- release owner where applicable

Ownership is not necessarily approval authority, but it provides routing and accountability.

---

## P1-3 — Architecture/technical Decisions should be queryable categories

The generic Decision object can cover:
- architecture decision
- technical trade-off
- risk acceptance
- no-impact decision
- waiver
- release authorization
- closure
- reopen
- break-glass

Use typed categories and structured fields, rather than one unstructured rationale blob.

ADR documents may reference Decision IDs where appropriate.

---

## P1-4 — Context freshness should be visible

Reference context may be immutable snapshots yet obsolete.

Store:
- source snapshot time/revision
- freshness policy/class
- superseded-by relation where known

Context Resolver can warn when:
- datasheet revision is old;
- design document was superseded;
- known-issue guidance has a newer version.

Stale reference context is not automatically invalid, but should be visible.

---

## P1-5 — Partial acceptance should not be hidden

If a Requirement has multiple AC and only a subset is delivered, do not represent it as fully CLOSED without an explicit Requirement Revision/Scope Decision.

Options:
- new Requirement Revision narrows scope;
- split Work/Requirement;
- typed partial-acceptance Decision where business policy permits.

The Closure Manifest must make incomplete/waived AC visible.

---

## P1-6 — Risk debt and follow-up need durable links

Temporary workarounds often create:
- TODO
- known limitation
- deferred test
- accepted risk
- later cleanup requirement

Risk Acceptance/Waiver may require follow_up_work_id.

The platform should expose overdue follow-up rather than burying it in release notes.

---

# Readiness gate example

A formal Requirement Revision could project:

~~~yaml
readiness:
  status: READY
  decision_id: DEC-...
  requirement_digest: ...
  assurance_profile: A2
  unresolved_blockers: []
  verification_plan_digest: ...
~~~

If any authority-bearing input changes, readiness is recomputed for the new revision; old readiness Decision remains historical.

---

# Lifecycle guard examples

## Start Formal Run

Require:
- Requirement Revision READY
- Task Revision current/applicable
- Work not cancelled/superseded
- policy/Assurance Profile resolved
- Run Input Manifest frozen
- Worker eligible
- required privileged grants available

## Enter VERIFYING

Require:
- implementation result frozen
- current Subject Manifest created
- external side effects reconciled enough for Verification
- required Procedure/Baseline refs resolved

## Enter RELEASE_READY

Require:
- all release-required Verification applicable/pass
- required Review complete
- blocking risks resolved/accepted
- Release Bundle/Target/compatibility valid
- no unresolved quarantine

## Close Work

Require Closure Policy, not merely Release completion.

---

# M0 additions

Add to contract freeze:

1. Requirement readiness schema/Decision type.
2. Acceptance Criterion verification-mode schema.
3. Assurance Profile schema.
4. Verification Plan schema.
5. Plan Manifest/digest if approve_plan remains a public operation.
6. Supersession/cancellation impact rules.
7. Run disposition Decision schema.
8. Risk / Waiver / Exception Decision schemas.
9. Closure/Reopen Decision and guard rules.
10. Authority Delegation schema.
11. Clarification Question/Answer schema and classifier result.
12. Typed Decision categories.

---

# Additional no-go checks

M1/M2 should fail readiness if:

- Formal Run can start from a non-READY Requirement;
- AC can reach PASS without a declared verification mode;
- high-risk Task can downgrade its Assurance Profile without authorized Decision;
- superseded Requirement leaves old Run with unchanged privileged authority;
- cancelled Run can still dispatch privileged actions;
- expired waiver still satisfies release gate;
- known risk is treated as accepted without applicable Decision;
- plan approval remains valid after material Task/Verification Plan change;
- delegated actor can collapse required separation-of-duties;
- free-form clarification changes Acceptance without creating Requirement Revision;
- Work can close with missing required AC and no explicit scope/waiver record.

---

# Assessment

The architecture now needs to distinguish four forms of "done":

1. Requirement ready — the request is sufficiently well-defined to engineer.
2. Engineering done — the current Task/Run result is frozen.
3. Assurance done — required Verification/Review/risk handling is complete.
4. Release/closure done — release and/or business closure policy is satisfied.

Collapsing these into one status would reintroduce the ambiguity the Control Plane is intended to eliminate.

The next v1.2 baseline should therefore add readiness/risk/assurance lifecycle semantics, but still avoid a new top-level plane or workflow product abstraction.
