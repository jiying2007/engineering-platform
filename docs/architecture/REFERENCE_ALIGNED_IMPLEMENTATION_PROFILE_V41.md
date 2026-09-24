# Reference-Aligned Implementation Profile v41

Date: 2026-09-24
Status: **Canonical implementation companion to architecture v1.2**
Supersedes: docs/architecture/REFERENCE_ALIGNED_IMPLEMENTATION_PROFILE_V40.md

Research basis:
- Open Source / Public Standard Reference Review Rounds 1–41
- Architecture v1.2

## 1. Goal

Profile v41 retains all v40 semantics and adds optional measurement/test interoperability contracts inspired by ASAM standards.

No ASAM service or automotive-specific runtime becomes mandatory.

---

## 2. MeasurementDataArtifact

Add immutable:

~~~text
MeasurementDataArtifact
~~~

Metadata:
- format/profile/version;
- acquisition producer;
- TestAttempt/Procedure;
- Device/Sample/Target;
- start/end/timebase;
- SignalCatalogArtifact;
- conversion metadata;
- synchronization;
- events/attachments;
- content digest.

Candidate formats:
- ASAM MDF/MF4;
- HDF5;
- Parquet;
- CSV;
- vendor/custom binary.

Format does not grant Evidence authority.

---

## 3. SignalCatalogArtifact

Optional:

~~~text
SignalCatalogArtifact
~~~

Defines:
- signal/channel identity;
- quantity/unit;
- data type;
- source/interface;
- conversion/calibration;
- sampling expectations;
- semantic description;
- version/digest.

Can be derived from:
- A2L;
- DBC;
- ARXML;
- protocol definitions;
- ASAM metadata;
- project schemas.

---

## 4. Raw-to-physical conversion

Keep:

~~~text
raw capture
 -> MeasurementDataArtifact
 -> conversion/normalization transform
 -> MeasurementResult
~~~

Conversion retains:
- source digest;
- mapping/formula;
- tool/version;
- unit conversion;
- result refs.

Raw capture stays immutable.

---

## 5. TestSequenceArtifact

Optional:

~~~text
TestSequenceArtifact
~~~

Represents exchangeable test implementation:
- format/version;
- source tool;
- ProcedureRevision mapping;
- parameters/dependencies;
- digest.

Candidate:
- ASAM OTX / ISO 13209.

Internal ProcedureRevision remains canonical.

---

## 6. ProcedureImplementationRef

Add mapping from ProcedureRevision to concrete runner/package:
- pytest;
- pytest-embedded;
- OpenHTF;
- Robot Framework;
- OpenTAP;
- OTX;
- vendor script.

One Procedure may have multiple implementations.

Trust/Verification policy selects acceptable implementation/issuer.

---

## 7. TestBenchInterfaceProfile

Add optional:

~~~text
TestBenchInterfaceProfile
~~~

Capabilities:
- measurement/read/write;
- calibration;
- diagnostics;
- bus/network access;
- fault/error injection;
- model/simulator access;
- synchronized acquisition;
- variable mapping;
- unit conversion;
- lifecycle/connect/disconnect.

ASAM XIL is the main reference.

---

## 8. Verification environments

XIL/XIL-MA reinforce the existing distinction:
- MIL;
- SIL;
- PIL;
- HIL;
- PHYSICAL_DUT.

Same semantic test in different environment remains a distinct TestVariant.

---

## 9. TestDataArchiveRef

Add lightweight:

~~~text
TestDataArchiveRef
~~~

References external test-data systems/archives:
- provider;
- dataset/test ID;
- snapshot/version;
- locator/query;
- source digest/checksum where available.

ASAM ODS is a strong reference for large test-data estates.

External archive is not Evidence authority.

---

## 10. TraceDataArtifact

Add:

~~~text
TraceDataArtifact
~~~

for:
- OS/scheduler/runtime trace;
- timing/performance events;
- embedded trace channels.

ASAM TDF is a candidate interoperable format.

Trace data may feed MeasurementResult, performance Verification or Incident analysis.

---

## 11. Timebase and synchronization

Multi-source formal measurement records:
- source clocks;
- timebase;
- synchronization method;
- drift/quality;
- timestamp transform/alignment.

Do not compare separate acquisition streams under an undeclared common-time assumption.

---

## 12. ASAM strategy

Use standards only when scale/vendor interoperability justifies them:

### MDF
High-volume synchronized measurement/trace storage.

### OTX
Portable test sequence exchange.

### XIL
Multi-vendor HIL/SIL test-bench interface abstraction.

### ODS
Large test-data management/archive.

### TDF
Embedded runtime/timing trace exchange.

None replaces internal Procedure/Measurement/Evidence semantics.

---

## 13. Open-source tooling

asammdf is a strong optional MDF ingest/analysis library.

Any use binds:
- version;
- transform config;
- source/output digest;
- conversion receipt.

No tool becomes canonical measurement authority.

---

## 14. M0/M1 strategy

### M0
Freeze:
- MeasurementDataArtifact;
- SignalCatalogArtifact;
- TestSequenceArtifact;
- ProcedureImplementationRef;
- TestBenchInterfaceProfile;
- TestDataArchiveRef;
- TraceDataArtifact;
- timebase/synchronization metadata.

### M1
No ASAM dependency.

### M2/M3
Adopt selected standards for real HIL, factory, certification or multi-vendor benches.

---

## 15. Final rules added by v41

1. Measurement/test standards provide interoperability, not authority.
2. Raw acquisition remains immutable under converted/evaluated results.
3. Signal conversion semantics are versioned and provenance-bound.
4. Procedure identity is separate from one runner/test-sequence implementation.
5. HIL/SIL/PIL/MIL remain different Evidence environments.
6. Multi-source timing comparisons declare timebase/synchronization.
7. External test-data archives remain references/projections.
8. Trace/timing data is content-addressed.
9. Format conversion is an explicit transform.
10. ASAM adoption is use-case driven.

Architecture v1.2 remains canonical.
Profile v41 is the current implementation companion.
