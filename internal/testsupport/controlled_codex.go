// Package testsupport supplies local protocol fixtures, never live provider proof.
package testsupport

// ControlledCodexProtocol deliberately waits for a filesystem test barrier.
const ControlledCodexProtocol = `
import json,os,sys,threading,time
from pathlib import Path
if sys.argv[1:]==['--version']:
 print('codex-cli 0.157.1');sys.exit(0)
lock=threading.Lock()
def emit(v):
 with lock: print(json.dumps(v),flush=True)
def finish():
 while not Path('allow-finish').exists(): time.sleep(.01)
 status='interrupted' if Path('interrupt-request').exists() else 'completed'
 if status=='completed': emit({'method':'item/completed','params':{'threadId':'thread','turnId':'turn','item':{'id':'result','type':'agentMessage','text':'fixture complete'}}})
 emit({'method':'turn/completed','params':{'threadId':'thread','turn':{'id':'turn','status':status}}})
for line in sys.stdin:
 r=json.loads(line);m=r.get('method');p=r.get('params',{})
 if 'id' not in r: continue
 answer={}
 if m=='initialize': answer={'userAgent':'local-protocol-fixture'}
 elif m=='account/rateLimits/read': answer={'rateLimits':{'primary':{}}}
 elif m=='thread/start':
  assert not (Path(os.environ['CODEX_HOME'])/'auth.json').exists()
  answer={'thread':{'id':'thread'}}
 elif m=='turn/start': answer={'turn':{'id':'turn'}}
 elif m in ('turn/steer','turn/interrupt'):
  assert p['threadId']=='thread'
  assert p.get('expectedTurnId',p.get('turnId'))=='turn'
  with open('requests.jsonl','a') as f: f.write(json.dumps(r)+'\n')
  if m=='turn/steer': answer={'turnId':'turn'}
  else: Path('interrupt-request').write_text('requested')
  if Path('drop-reply').exists(): continue
 else: sys.exit(9)
 emit({'id':r['id'],'result':answer})
 if m=='turn/start': threading.Thread(target=finish,daemon=True).start()
`
