"""One-off exact-source transfer. Creates immutable blobs only; never commits or changes refs."""
import base64
import hashlib
import json
import os
from pathlib import Path
import subprocess

assert os.environ['GITHUB_REPOSITORY']=='jiying2007/engineering-platform'
assert os.environ['GITHUB_REF']=='refs/heads/fix/publication-failure-readback-20261012'
expected={
 'docs/implementation/FAILURE_REPORT_AND_PUBLICATION_READBACK_V1.md':'a189621fd6b04af3b237b3ad1f9080f492868e6f',
 'internal/githubpublish/github.go':'e75248ac0f8605ad1f6868313142d5b39bed3999',
 'internal/githubpublish/open_pull_absence_test.go':'b4d2c15c6525dbc387d2487707739f6887a40f19',
 'internal/store/production_context_surface_test.go':'80807364e78ddb485d3bd22ea61f99e154c3ffca',
 'internal/workeragent/codex.go':'48b682105bc928d037a150de1333274bca855275',
 'internal/workeragent/failure_settlement.go':'25ffda0abbcab513349359efe8aeffb37d395a37',
 'internal/workeragent/failure_settlement_test.go':'58cf5b18c1aa8911cf47b9a5ae7774eeecf2ec9d',
 'internal/workeragent/offline.go':'0781c27324542f6bdbd87268108ce081dfd90308',
}
def blob(raw):return hashlib.sha1(b'blob '+str(len(raw)).encode()+b'\0'+raw).hexdigest()
preimages={'internal/githubpublish/github.go':'718c9e6a0cfce943bf38886d1979c8ab255865c5','internal/workeragent/codex.go':'d6c472ba124b3028b673f8c36878f70872d726e7','internal/workeragent/offline.go':'e6f6bd1583b955b2824807d4a374e9be2a98485a'}
for path,want in preimages.items():assert blob(Path(path).read_bytes())==want,path
p=Path('internal/githubpublish/github.go');s=p.read_text();offset=s.index('func (g *githubRemote) findOpenPull(');at=s.index('\tif len(pulls) == 0 {',offset)
s=s[:at]+'''\t// A 200/null is a missing observation, not the explicit empty array that
\t// permits later absence checks and PR creation. Keep Publish and Observe on
\t// the same fail-closed readback contract as historical PR lookup.
\tif pulls == nil {
\t\treturn pullRecord{}, false, fmt.Errorf("GitHub open PR response must be an array")
\t}
'''+s[at:];p.write_text(s)
for kind in ['codex','offline']:
 p=Path('internal/workeragent/'+kind+'.go');s=p.read_text()
 old='\t\t\tcleanup, stop := context.WithTimeout(context.Background(), 6*time.Second)\n\t\t\tdefer stop()\n\t\t\t_ = c.Call(cleanup, http.MethodPost, "/api/v1/worker/'+kind+'/fail", permit.Token, nil)'
 new='\t\t\terr = errors.Join(err, settleExecutionFailure(ctx, c, "/api/v1/worker/'+kind+'/fail", permit.Token))'
 assert s.count(old)==1,kind;p.write_text(s.replace(old,new,1))
for path,want in expected.items():
 p=Path(path);assert p.is_file() and not p.is_symlink();assert blob(p.read_bytes())==want,path
subprocess.run(['git','add','--',*expected],check=True)
subprocess.run(['git','rm','--cached','--','.ep-transfer/export.py','.github/workflows/readback-source-once.yml'],check=True)
tree=subprocess.check_output(['git','write-tree'],text=True).strip()
assert tree=='111821b88ea2279298bd527975761451900ea5c4',tree
out=Path(os.environ['RUNNER_TEMP'])/'readback-objects';out.mkdir()
result={'version':1,'source_head':os.environ['GITHUB_SHA'],'candidate_tree':tree,'files':[],'ref_mutation_performed':False}
for path,want in expected.items():
 raw=Path(path).read_bytes();request=out/'request.json';request.write_text(json.dumps({'encoding':'base64','content':base64.b64encode(raw).decode()}))
 response=json.loads(subprocess.check_output(['gh','api','--method','POST','repos/jiying2007/engineering-platform/git/blobs','--input',str(request)],text=True,timeout=30))
 assert response['sha']==want,path
 result['files'].append({'path':path,'sha':want,'sha256':hashlib.sha256(raw).hexdigest(),'size':len(raw)})
request.unlink()
(out/'objects.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps(result,indent=2))
