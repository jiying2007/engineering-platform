package testsupport

// CompletedCodexProtocol is a local TEST-ONLY subprocess, never a live account.
// Unlike control fixtures it leaves no protocol log inside the source tree.
const CompletedCodexProtocol = `
import json,os,sys
from pathlib import Path
if sys.argv[1:]==['--version']:
 print('codex-cli 0.157.1');sys.exit(0)
def emit(v):print(json.dumps(v),flush=True)
for line in sys.stdin:
 r=json.loads(line);m=r.get('method');p=r.get('params',{})
 if 'id' not in r:continue
 answer={}
 if m=='initialize':answer={'userAgent':'completed-test-only'}
 elif m=='account/rateLimits/read':answer={'rateLimits':{'primary':{}}}
 elif m=='thread/start':
  assert not (Path(os.environ['CODEX_HOME'])/'auth.json').exists()
  answer={'thread':{'id':'thread'}}
 elif m=='turn/start':
  Path('hello.txt').write_text('completed model source preserved\n')
  if Path('fixture-ignored-mode').exists():
   Path('.gitignore').write_text('private-output\n')
   Path('private-output').write_bytes(b'TEST-ONLY-PRIVATE\x00BYTES')
  answer={'turn':{'id':'turn'}}
 else:sys.exit(11)
 emit({'id':r['id'],'result':answer})
 if m=='turn/start':
  emit({'method':'item/completed','params':{'threadId':'thread','turnId':'turn','item':{'id':'result','type':'agentMessage','text':'test-only completed output'}}})
  emit({'method':'turn/completed','params':{'threadId':'thread','turn':{'id':'turn','status':'completed'}}})
`
