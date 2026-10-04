package testsupport

// ContinuationCodexProtocol is local TEST-ONLY code. The first process preserves
// unfinished source until an explicit interrupt; the second process requires
// those same bytes and a new-run continuation prompt before finishing.
const ContinuationCodexProtocol = `
import json,os,sys
from pathlib import Path
if sys.argv[1:]==['--version']:
 print('codex-cli 0.157.1');sys.exit(0)
def emit(v): print(json.dumps(v),flush=True)
second=Path('partial.txt').exists()
for line in sys.stdin:
 r=json.loads(line);m=r.get('method');p=r.get('params',{})
 if 'id' not in r: continue
 answer={}
 if m=='initialize': answer={'userAgent':'continuation-test-only'}
 elif m=='account/rateLimits/read': answer={'rateLimits':{'primary':{}}}
 elif m=='thread/start':
  assert not (Path(os.environ['CODEX_HOME'])/'auth.json').exists()
  answer={'thread':{'id':'thread'}}
 elif m=='turn/start':
  prompt=''.join(x.get('text','') for x in p['input'])
  if second:
   assert 'Explicit source continuation' in prompt
   assert Path('partial.txt').read_text()=='unfinished source from first process\n'
   assert Path('hello.txt').read_text()=='first change retained\n'
   Path('hello.txt').write_text('first change retained\nsecond change completed\n')
  else:
   assert 'Explicit source continuation' not in prompt
   Path('partial.txt').write_text('unfinished source from first process\n')
   Path('hello.txt').write_text('first change retained\n')
  answer={'turn':{'id':'turn'}}
 elif m=='turn/interrupt':
  assert not second and p['threadId']=='thread' and p['turnId']=='turn'
 else: sys.exit(11)
 emit({'id':r['id'],'result':answer})
 if m=='turn/interrupt':
  emit({'method':'turn/completed','params':{'threadId':'thread','turn':{'id':'turn','status':'interrupted'}}})
 if m=='turn/start' and second:
  emit({'method':'item/completed','params':{'threadId':'thread','turnId':'turn','item':{'id':'result','type':'agentMessage','text':'test-only source continuation completed'}}})
  emit({'method':'turn/completed','params':{'threadId':'thread','turn':{'id':'turn','status':'completed'}}})
`
