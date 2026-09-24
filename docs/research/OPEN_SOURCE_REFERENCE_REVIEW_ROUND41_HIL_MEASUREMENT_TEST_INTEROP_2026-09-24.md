# Public Standard / Open Source Reference Review — Round 41: HIL, Measurement and Test Interoperability

Date: 2026-09-24
Status: **Archived research / implementation input**

References:
- ASAM MDF
- ASAM ODS
- ASAM OTX Extensions
- ASAM XIL / XIL-MA
- ASAM TDF
- asammdf open-source tooling
- existing Procedure, Measurement, TestReportArtifact, Evidence and DeviceLab semantics

## 1. Key conclusion

Embedded/HIL interoperability benefits from standard exchange formats and test-bench interfaces, but engineering-platform should not adopt automotive test standards as internal authority.

The durable separation is:

~~~text
Internal Procedure / Measurement / Evidence authority
        |
        +-> standard measurement/trace artifacts
        +-> standard test-sequence import/export
        +-> test-bench interface adapters
        +-> external test-data archive references
~~~

---

## 2. MeasurementDataArtifact

Add generic immutable:

~~~text
MeasurementDataArtifact
~~~

Metadata:
- format/profile;
- format version;
- producer/acquisition system;
- TestAttempt/Procedure;
- Device/Sample/Target;
- start/end/timebase;
- channel/signal catalog ref;
- raw/physical conversion metadata;
- clock/synchronization refs;
- attachments/events;
- content digest.

Candidate formats:
- ASAM MDF/MF4;
- CSV/Parquet/HDF5 where simpler;
- vendor binary format;
- custom raw capture.

Format is interoperability, not Evidence authority.

---

## 3. ASAM MDF practice

ASAM MDF supports:
- raw and calculated measurement data;
- raw-to-physical conversion rules;
- multiple channel/data types;
- bus events;
- synchronization;
- metadata/attachments;
- high-volume storage.

Current official ASAM MDF version is 4.3.0, released 2025-09-23.

### Absorb

Where measurement volume/complexity justifies it:
- use MDF/MF4 as MeasurementDataArtifact format;
- retain raw values and conversion metadata;
- preserve timebase/synchronization;
- derive canonical MeasurementResult from exact MDF Artifact.

Do not require MDF for trivial CI scalar measurements.

---

## 4. SignalCatalogArtifact

Add optional:

~~~text
SignalCatalogArtifact
~~~

Defines machine-readable channels/signals:
- stable signal ID/name;
- quantity/unit;
- datatype;
- source/interface;
- conversion/calibration;
- sampling expectations;
- semantic description;
- version/digest.

This avoids interpreting the same raw channel differently across tools.

It may map from:
- ASAM metadata;
- A2L/DBC/ARXML;
- project protocol definitions;
- custom signal schema.

---

## 5. Measurement conversion

Raw measurement conversion is a transform:

~~~text
raw acquisition
 -> MeasurementDataArtifact
 -> conversion/normalization
 -> MeasurementResult
~~~

Record:
- source Artifact;
- conversion definition;
- tool/version;
- unit transform;
- output/result refs.

Never throw away raw acquisition merely because physical values were calculated.

---

## 6. TestSequenceArtifact

Add optional immutable:

~~~text
TestSequenceArtifact
~~~

Represents an exchangeable executable/documented test sequence.

Metadata:
- format;
- format version;
- source tool;
- ProcedureRevision mapping;
- parameters;
- external libraries/dependencies;
- content digest.

Candidate:
- ASAM OTX/ISO 13209 for diagnostic/calibration/EOL test sequence exchange.

Internal ProcedureRevision remains canonical.
An imported OTX file is an implementation Artifact of a Procedure.

---

## 7. Procedure ↔ test-sequence mapping

Use explicit:

~~~text
ProcedureImplementationRef
~~~

to link ProcedureRevision to:
- pytest;
- Robot;
- OpenHTF;
- OpenTAP;
- OTX;
- vendor test script.

Multiple implementations may exist.

Verification policy chooses which implementation/issuer is trusted for a given scope.

---

## 8. TestBenchInterfaceProfile

Add:

~~~text
TestBenchInterfaceProfile
~~~

Defines abstract bench capabilities:
- measurement/read/write;
- calibration access;
- diagnostic access;
- network/bus access;
- electrical-error injection;
- model/simulator access;
- synchronized acquisition;
- supported variable mappings;
- unit conversion;
- lifecycle/connect/disconnect semantics.

ASAM XIL is a strong reference for this abstraction.

---

## 9. XIL practice

ASAM XIL abstracts test applications from concrete HIL/SIL test benches through standardized ports and framework services.

Useful ideas:
- port-independent variable access;
- mapping identifiers;
- unit/data-type conversion;
- synchronized acquisition from multiple sources;
- test-bench configuration/lifecycle.

### Decision

DeviceLabProvider/TestProcedureService may later expose a XIL-inspired adapter contract.

Do not require ASAM XIL compliance for simple labgrid/serial/power benches.

---

## 10. HIL/SIL/MIL interface

ASAM XIL-MA explicitly covers:
- HIL;
- PIL;
- SIL;
- MIL;
- simulator/model access.

This reinforces v10 VerificationEnvironmentClass and allows one Procedure definition to bind different environment implementations while retaining different TestVariants.

---

## 11. TestDataArchiveRef

ASAM ODS provides a mature model for managing:
- measurement data;
- simulation/fleet test data;
- instrumentation;
- test stand calibration;
- testing workflows;
- large data archives.

Add lightweight:

~~~text
TestDataArchiveRef
~~~

for external test-data systems:
- provider/archive;
- dataset/test ID;
- version/snapshot;
- query/locator;
- observed_at;
- source digest/checksum if available.

Core authority remains in exact Artifacts/Evidence references.

---

## 12. TraceDataArtifact

Add Artifact specialization:

~~~text
TraceDataArtifact
~~~

for:
- OS/runtime trace;
- scheduler trace;
- timing/performance events;
- embedded trace channels.

Candidate interoperable format:
- ASAM TDF, which is based on MDF and targets embedded run-time trace/timing data.

Trace Artifact can feed:
- performance MeasurementResult;
- latency/timing Verification;
- Incident analysis.

---

## 13. Time synchronization

Multi-source HIL Evidence must identify:
- clock/timebase;
- synchronization method;
- clock quality/drift;
- source timestamps;
- alignment/transformation.

Do not compare events from separate sources as if timestamps are identical without a declared synchronization basis.

Add optional:

~~~text
TimebaseDefinition
~~~

or fields on MeasurementDataArtifact/TestAttempt.

---

## 14. Test-data normalization

Canonical path:

~~~text
Acquisition/Test Bench
 -> MeasurementDataArtifact / TraceDataArtifact
 -> format adapter
 -> MeasurementResult / TestAttemptResult
 -> Evidence
 -> Verification
~~~

Standard files remain immutable input artifacts.

Normalization does not rewrite them.

---

## 15. Open-source tooling

asammdf is a mature open-source Python reader/editor for MDF/MF4 supporting MDF 2/3/4, CAN/LIN extraction, filtering, cutting and export.

Potential M2 tool for:
- MDF ingest;
- analysis;
- conversion;
- selected export.

Tool output remains subject to normal version/provenance controls.

---

## 16. M0/M1 impact

M0 reserves:
- MeasurementDataArtifact;
- SignalCatalogArtifact;
- TestSequenceArtifact;
- ProcedureImplementationRef;
- TestBenchInterfaceProfile;
- TestDataArchiveRef;
- TraceDataArtifact;
- timebase/synchronization metadata.

M1 does not require ASAM tooling.

---

## 17. M2/M3 impact

For HIL/production/certification:
- adopt MDF when high-volume synchronized signals justify it;
- optionally use OTX for portable test sequences;
- use XIL-inspired bench abstraction when multiple vendors/test benches appear;
- use ODS/external test archives only when result volume justifies them;
- use TDF for timing/trace interoperability where useful.

---

## 18. Invariants

1. Standard measurement/test formats are interoperability Artifacts, not Verification authority.
2. Raw acquisition is preserved beneath converted/evaluated measurements.
3. Signal definitions and conversion semantics are versioned.
4. Procedure authority is separate from its pytest/OTX/OpenTAP/vendor implementation.
5. HIL/SIL/MIL remain different Verification variants/environments.
6. Multi-source measurements expose timebase/synchronization assumptions.
7. External test archives remain references/projections over exact immutable data.
8. Trace/timing data is content-addressed and provenance-bound.
9. Standard format conversion creates explicit transform provenance.
10. ASAM adoption is milestone/use-case driven, not mandatory for every embedded test.

## 19. Conclusion

The durable test-interoperability rule is:

> **Keep Procedure, Measurement and Evidence semantics internal and exact; use standards such as MDF, OTX, XIL, ODS and TDF only to make acquisition, test execution and data exchange portable across tools and laboratories.**
