"""One-off exact patch -> immutable blob transport, never candidate execution or ref update."""
import base64,gzip,hashlib,json,os,subprocess
from pathlib import Path
assert os.environ['GITHUB_REPOSITORY']=='jiying2007/engineering-platform'
assert os.environ['GITHUB_REF']=='refs/heads/fix/runtime-settlement-causes-20261012'
expected={
 'docs/status/IMPLEMENTATION_STATUS.md':('b9c7c4b8416ff101627a18fb2b2e1869511cc766','1ffee3a1d181f9568013a673947850241c9f4255'),
 'internal/runtime/codexapp/controls.go':('d0edbcc9de75475cdb93d47727b601cbd19973be','804922426271bb999df08e833c2d6da7a276c977'),
 'internal/runtime/codexapp/controls_test.go':('c31e5e4b039ff3ef696e408fc2b0aba9b95aaadb','f2c4adee7a058c2a8d6567fb4c97003124e5f707'),
 'internal/runtime/codexapp/engineering.go':('4c68853f5c8adce3cf0fadbe1202a6e2cc4fdfe4','0e404be5fceb69af4e35a62a81eef605d2754636'),
 'internal/runtime/codexapp/engineering_exit.go':(None,'3d40811549d7ca0712c52c91ddcf744aa4ee13b8'),
 'internal/runtime/codexapp/engineering_exit_test.go':(None,'fcd875176b8c75f24de0bb1fa6c8ad99b18a81eb'),
}
def blob(raw):return hashlib.sha1(b'blob '+str(len(raw)).encode()+b'\0'+raw).hexdigest()
for path,(before,after) in expected.items():
 p=Path(path)
 if before is None:assert not p.exists(),path
 else:assert p.is_file() and not p.is_symlink() and blob(p.read_bytes())==before,path
compressed=Path('.ep-transfer/runtime.patch.gz').read_bytes();assert blob(compressed)=='f622b635cf234ee84bd87407df95d146441c68d2'
patch=gzip.decompress(compressed);assert len(patch)==20935 and hashlib.sha256(patch).hexdigest()=='809ef0e3fd2e695402546ed120318325097f8b0a88424e62665655b2db004f3e'
subprocess.run(['git','apply','--check','-'],input=patch,check=True)
subprocess.run(['git','apply','--index','-'],input=patch,check=True)
for path,(_,after) in expected.items():assert blob(Path(path).read_bytes())==after,path
subprocess.run(['git','rm','--cached','--','.ep-transfer/runtime.patch.gz','.ep-transfer/export-runtime.py','.github/workflows/runtime-source-once.yml'],check=True)
tree=subprocess.check_output(['git','write-tree'],text=True).strip();assert tree=='5e9e3963aaef4b90849047dc2c316ffce846620a'
out=Path(os.environ['RUNNER_TEMP'])/'runtime-source-objects';out.mkdir()
result={'source_head':os.environ['GITHUB_SHA'],'candidate_tree':tree,'files':[],'ref_mutation_performed':False}
for path,(_,after) in expected.items():
 raw=Path(path).read_bytes();request=out/'request.json';request.write_text(json.dumps({'encoding':'base64','content':base64.b64encode(raw).decode()}))
 response=json.loads(subprocess.check_output(['gh','api','--method','POST','repos/jiying2007/engineering-platform/git/blobs','--input',str(request)],text=True,timeout=30));assert response['sha']==after,path
 result['files'].append({'path':path,'sha':after,'sha256':hashlib.sha256(raw).hexdigest(),'size':len(raw)})
request.unlink();(out/'objects.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result,indent=2))
