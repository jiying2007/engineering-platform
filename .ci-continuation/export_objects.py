"""Temporary immutable-blob export only. Never creates a commit or changes a ref."""
import base64
import hashlib
import json
import os
from pathlib import Path
import subprocess

assert os.environ['GITHUB_REPOSITORY']=='jiying2007/engineering-platform'
paths=['.github/workflows/ci.yml','docs/status/IMPLEMENTATION_STATUS.md','internal/store/memory.go','internal/rcdelivery/model.go','internal/rcdelivery/model_test.go','internal/gateway/authority.go','internal/gateway/session_epoch_test.go','internal/rcdelivery/readiness.go','internal/rcdelivery/readiness_test.go','internal/rcdelivery/readiness_binding_test.go','internal/store/memory_cancel_during_wait_test.go','internal/store/postgres/pool_context_test.go','internal/action/recovery_state_matrix_test.go','docs/implementation/ACTION_RECOVERY_STATE_MATRIX_V1.md','scripts/authority_context_inventory.py','scripts/test_authority_context_inventory.py']
out=Path(os.environ['RUNNER_TEMP'])/'continuation-candidate';out.mkdir(exist_ok=True)
result={'source_head':subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip(),'files':[],'ref_mutation_performed':False}
for name in paths:
 p=Path(name)
 assert p.is_file() and not p.is_symlink(), name
 raw=p.read_bytes();assert len(raw)<1<<20
 sha=hashlib.sha1(b'blob '+str(len(raw)).encode()+b'\0'+raw).hexdigest()
 request=out/'request.json'
 request.write_text(json.dumps({'encoding':'base64','content':base64.b64encode(raw).decode()}))
 response=json.loads(subprocess.check_output(['gh','api','--method','POST','repos/jiying2007/engineering-platform/git/blobs','--input',str(request)],text=True))
 assert response['sha']==sha,(name,response)
 result['files'].append({'path':name,'sha':sha,'sha256':hashlib.sha256(raw).hexdigest(),'size':len(raw)})
 target=out/'files'/name;target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes(raw)
request.unlink(missing_ok=True)
(out/'objects.json').write_text(json.dumps(result,indent=2)+'\n')
(out/'candidate.diff').write_bytes(subprocess.check_output(['git','diff','--binary']))
print(json.dumps(result,indent=2))
