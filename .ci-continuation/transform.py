"""Temporary exact-original transforms. No credentials, refs, or remote writes."""
from pathlib import Path
import hashlib
import json
import subprocess

PREIMAGES = {
 'internal/store/memory.go': 'b779e5522ffb441c2c0fd43942c2860a085d374e',
 'internal/rcdelivery/model.go': '91a226a24ab0175050a174bf10a69d5d968a7103',
 'internal/rcdelivery/model_test.go': '1e7ee9138f2918d89e3f23d7c70e55ee544c4871',
 '.github/workflows/ci.yml': '2e8a6d2a831a04a7e80f96ad69570b3dea5c4c27',
 'docs/status/IMPLEMENTATION_STATUS.md': 'bfd94a1fa6ec4cf86e4cb28718d18c02cf8bd505',
}
for name, expected in PREIMAGES.items():
 raw=Path(name).read_bytes()
 assert hashlib.sha1(b'blob '+str(len(raw)).encode()+b'\0'+raw).hexdigest()==expected, name

def once(s,old,new):
 assert s.count(old)==1, (old[:100],s.count(old))
 return s.replace(old,new,1)

subprocess.run(['go','run','./.ci-continuation/memory.go'],check=True)
p=Path('internal/rcdelivery/model.go');s=p.read_text()
s=once(s,'\tVersion = 1','\tVersion = 2')
s=once(s,'"INTERNAL_RC_DELIVERY_BOUND_EXTERNAL_GATES_OPEN"','"CI_DELIVERY_PROVENANCE_BOUND"')
s=once(s,'\tStatus                          string              `json:"status"`','\tStatus                          string              `json:"status"`\n\tReadiness ReadinessSnapshot `json:"readiness"`')
s=once(s,'\tbinaries, codex, err := selectArtifacts(receipt)','\treadiness, err := ReadinessFromStatus(implementationStatus)\n\tif err != nil { return zero, err }\n\tbinaries, codex, err := selectArtifacts(receipt)')
s=once(s,'\t\tVersion: Version, Status: Status, Repository: receipt.Repository,','\t\tVersion: Version, Status: Status, Repository: receipt.Repository,\n\t\tReadiness: readiness,')
s=once(s,'\tif err := envelope.Verify(); err != nil {','\tif err := envelope.VerifyStatusBytes(implementationStatus); err != nil {')
s=once(s,'\td := e.Delivery\n','\td := e.Delivery\n\tif err := d.Readiness.Validate(); err != nil { return err }\n')
s+='''
// VerifyStatusBytes additionally checks the exact authoritative status bytes.
// Verify alone is structural integrity, not authentication of supplied hashes.
func (e Envelope) VerifyStatusBytes(status []byte) error {
 if err := e.Verify(); err != nil { return err }
 if canonical.BytesDigest(status) != e.Delivery.ImplementationStatusDigest {
  return fmt.Errorf("implementation status byte digest mismatch")
 }
 observed, err := ReadinessFromStatus(status)
 if err != nil { return err }
 observedDigest, err := canonical.Digest(observed)
 if err != nil { return err }
 expectedDigest, err := canonical.Digest(e.Delivery.Readiness)
 if err != nil || observedDigest != expectedDigest {
  return fmt.Errorf("declared readiness differs from original status bytes")
 }
 return nil
}
''';p.write_text(s)
p=Path('internal/rcdelivery/model_test.go');s=p.read_text()
old='[]byte("# Implementation Status\\ncurrent\\n")';assert s.count(old)==3
p.write_text(s.replace(old,'readinessTestStatus()'))
p=Path('.github/workflows/ci.yml');s=p.read_text()
s=once(s,'            .delivery.source_sha==$sha and\n','''            .delivery.version==2 and
            .delivery.status=="CI_DELIVERY_PROVENANCE_BOUND" and
            ((.delivery.readiness.internal_readiness=="BLOCKED" and (.delivery.readiness.internal_blockers|length)>0) or
             (.delivery.readiness.internal_readiness=="CANDIDATE_ONLY" and (.delivery.readiness.internal_blockers|length)==0)) and
            (.delivery.readiness.external_qualification_gates|length)>0 and
            .delivery.source_sha==$sha and
''')
s=once(s,'      - name: Module hygiene\n','''      - name: Review bounded Context inventory
        shell: bash
        run: |
          set -euo pipefail
          python3 -B scripts/test_authority_context_inventory.py
          python3 -B scripts/authority_context_inventory.py --root . --out "$RUNNER_TEMP/authority-context-inventory.json"
          cat "$RUNNER_TEMP/authority-context-inventory.json"
      - name: Module hygiene
''');p.write_text(s)
p=Path('docs/status/IMPLEMENTATION_STATUS.md');s=p.read_text()
snapshot={'version':1,'internal_readiness':'BLOCKED','internal_blockers':[228,229],'external_qualification_gates':[105],'deferred_credential_gates':[103,106]}
block='<!-- engineering-platform-readiness\n'+json.dumps(snapshot,indent=2)+'\n-->\n\n'
s=once(s,'# Implementation Status\n\n','# Implementation Status\n\n'+block)
s=once(s,'W01-W10 remain the approved scope, but the repository-side implementation for\nthe current small-team RC is closed. Remaining work is deliberately separated\ninto **external qualification/deployment gates** and **explicit non-goals**; it\nis not an invitation to add parallel authorities, compatibility shims or\nspeculative services.','W01-W10 remain historical accepted repository slices, not current internal\nclosure. Internal P0 #228 and P1 #229 remain open independently of external\nqualification/deployment gates and explicit non-goals. No green CI, binary\nprovenance envelope or historical milestone can erase these internal blockers.\nDo not add parallel authorities, compatibility shims or speculative services.')
s=once(s,'ELF/map reference and private artifact restore are covered, but no target MCU\ncross-toolchain, boot/timing/hardware behavior or larger production firmware\ncapacity is qualified.','ELF/map reference and private artifact restore are covered. The separate\nno-network Cortex-M0 cross-toolchain reference is a controlled fixture, not\nqualification of target-board boot/timing/hardware behavior or larger\nproduction firmware capacity.')
note='''## Response, Memory and readiness hardening

The already-merged #262 Control client rejects a root null/non-object,
validates explicitly tagged top-level required response members, and commits a
fresh decoded output only after validation. Explicit nullable assignment is a
valid empty claim; missing assignment is not. Complete allowlisted 503 codes
keep their precise classification; unusable 5xx bodies preserve outcome-
unobserved, HTTP status and internal cause. Intact ordinary errors are not
reclassified as proof of COMMIT. No retry is added. The remaining hardening in
this slice does not overwrite or reimplement #262. Nested domain checks remain
the consumer's responsibility.

Memory's existing scoped methods additionally recheck cancellation after the
original RWMutex is acquired, before ledger access or mutation. Context-free
concrete methods delegate to the same implementation; there is no second ledger.
RWMutex waiting itself is not cancellable. This closes the post-wait access
race, not all #229 lifecycle, pool-wait or production latency qualification.

The unreferenced context-free `internal/authority` duplicate is retired; the
single production Gateway retains its frozen-task authorization and now rejects
cross-Run or stale Session identities in its epoch precheck. Existing PostgreSQL
transactional admission remains authoritative. An isolated PostgreSQL regression
holds the only pooled connection until seven caller-bound read/mutation/admission
paths time out, then requires zero business/audit/Action residue, zero Provider
calls and a usable pool. This is pool-wait cancellation evidence, not a production
capacity measurement or permission to retry an uncertain operation.

Delivery envelope v2 separates CI/source provenance from declared internal
readiness, production acceptance and deferred credential alternatives. The one
machine-readable block in this document is the declared readiness authority.
Missing or inconsistent blocks fail closed; VerifyStatusBytes requires the exact
original status bytes. The envelope is neither independent issue closure nor
production qualification. #228 still needs an implemented and qualified
provider-specific effectful terminal disposition; the state matrix is an
acceptance contract and regression coverage, not that implementation.

'''
s=once(s,'## Typed embedded routing and Skill maturity\n',note+'## Typed embedded routing and Skill maturity\n');p.write_text(s)
print('Exact original Memory/model/status/CI transforms complete; no authority closure claimed')
