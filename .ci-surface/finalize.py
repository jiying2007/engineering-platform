from pathlib import Path
import sys,re,json,subprocess
root=Path('.').resolve();sys.path.insert(0,str(root/'scripts'))
from authority_context_inventory import inventory
from verify_context_inventory import ROW
p=root/'internal/store/postgres/store.go'
s=p.read_text();s=s.replace('// Every production connection must have an independent database-side\n\t// deadline even when a legacy Store method still uses context.Background.','// Caller Context and independent database-side ceilings are complementary.\n\t// A caller without a deadline must not leave an unbounded statement or lock wait.')
p.write_text(s)
p=root/'scripts/verify_context_inventory.py';s=p.read_text().replace('    "Concrete convenience wrapper", "Test support helper",','    "Test support helper",');p.write_text(s)
p=root/'scripts/test_verify_context_inventory.py';s=p.read_text();anchor='            doc.write_text(good)\n            src.write_text(src.read_text()+"// changed source\\n")'
replacement='''            # Retired wrapper categories cannot be restored by updating counts.
            retired=good.replace("Explicitly timed command root", "Concrete convenience wrapper")
            doc.write_text(retired)
            with self.assertRaises(ValueError):verify(root,doc)
            doc.write_text(good)
            src.write_text(src.read_text()+"// changed source\\n")'''
assert anchor in s;s=s.replace(anchor,replacement);p.write_text(s)
subprocess.run(['gofmt','-w','internal/store/context_only_surface_test.go','internal/store/postgres/store.go'],cwd=root,check=True)
p=root/'docs/implementation/CONTEXT_LIFECYCLE_INVENTORY_V1.md';s=p.read_text()
old=ROW.findall(s);retained=[(path,name,category.strip()) for path,line,name,category in old if category.strip()!='Concrete convenience wrapper']
assert len(old)==133 and len(retained)==58
report=inventory(root)
current=[f for f in report['context_or_call_candidates'] if f['kind']=='context_constructor']
assert [(f['path'],f['name']) for f in current]==[(p,n) for p,n,c in retained]
s=re.sub(r'`sha256:[0-9a-f]{64}`','`'+report['source_files_digest']+'`',s)
s=s.replace('Base: `c449d7c3f1bfaab3f9f07b61d1992d7248c6d285`, plus the scoped Publisher\nGit and v2 host-admission corrections in this change. This closes the *enumeration* of the current','Base: `2772d0d9c1122d004c7854993bae88ac53d3a138`, followed by removal of\n75 unreferenced context-free concrete wrappers. There are now 58 direct origins,\ndown from 133; no new lifetime is introduced. This closes the *enumeration* of the current')
s=s.replace('| Concrete convenience wrapper | 75 |\n','')
s=s.replace('Concrete conveniences delegate to the same Context implementations; mandatory\nCore/Action/Gateway/Publisher interfaces do not offer a network fallback to them.\nThe existing all-production Core selector/method-value regression is retained.\nThis source inventory is not an alias-sensitive or interprocedural call graph.','Concrete Core and Action types now expose only the required Context variants\nfor these 75 operations, matching the mandatory interfaces. All 329 former test\ncall sites supply their Context explicitly; the existing production selector guard\nand a new concrete-method-set regression reject reintroduction. There is no\nfixture-only wrapper or compatibility shim. The retired wrapper category is no\nlonger accepted by the inventory verifier. This source inventory is not an\nalias-sensitive or interprocedural call graph.')
start=s.index('| Source location |');end=s.find('\n## ',start)
assert end>start
s=s[:start]+'| Source location | Constructor | Classification |\n| --- | --- | --- |\n'+''.join(f"| `{f['path']}:{f['line']}` | `{f['name']}` | {cat} |\n" for f,(_,_,cat) in zip(current,retained))+s[end:]
p.write_text(s)
p=root/'docs/status/IMPLEMENTATION_STATUS.md';s=p.read_text()
s=s.replace('The direct Context inventory is updated to 132 constructors and checked in CI','The direct Context inventory is now 58 constructors after removal of 75 unused\ncontext-free wrappers from the previous 133-site source, and is checked in CI')
s=s.replace('cancellation checks. Concrete convenience wrappers remain for direct fixtures,\nnot a network fallback. Memory historical Steering also matches PostgreSQL\'s','cancellation checks. All 75 context-free concrete wrappers are now removed;\n329 test call sites explicitly use the same Context methods, with no fixture\ncompatibility shim. A concrete-method-set regression prevents those unscoped\nentry points from returning. Memory historical Steering also matches PostgreSQL\'s')
s=s.replace('original RWMutex is acquired, before ledger access or mutation. Context-free\nconcrete methods delegate to the same implementation; there is no second ledger.','original RWMutex is acquired, before ledger access or mutation. The redundant\ncontext-free methods are removed; the scoped implementation and single ledger\nremain unchanged.')
s=s.replace('## Typed embedded routing and Skill maturity\n','''## Concrete authority Context surface cleanup

The 75 retired wrappers had zero production references in the type-resolved
Linux source review. Removal covers Memory Core/Action, PostgreSQL Core/Action,
Action MemoryRepository and Action Service GET. Only the wrappers and stale
comments are removed from production: existing Context implementations,
transactions, cancellation/settlement budgets, recovery state and retry policy
are unchanged. Tests supply explicit Context at the former 329 call sites;
no test is removed and no context-free test compatibility layer is added.
The concrete-method-set regression requires all 75 scoped replacements and
rejects their unscoped names. The five remaining direct-origin categories total
58, retain their original classifications, and remain bound to current bytes.

This is concrete surface and compatibility-residue cleanup, not selected-host
lifecycle/latency or full interprocedural cancellation qualification. Internal
blockers [228,229] remain OPEN/BLOCKED; real Provider and production gates remain
separate. It does not supply post-entry publication recovery or authorize retry.

## Typed embedded routing and Skill maturity
''')
p.write_text(s)
print(json.dumps({'direct_sites':len(current),'source_digest':report['source_files_digest'],'test_call_sites':329,'wrappers_removed':75},indent=2))
