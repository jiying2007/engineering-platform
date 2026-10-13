"""Exact-preimage source change; no network, credentials or ref mutation."""
import hashlib
import re
import subprocess
import sys
from pathlib import Path

BASE = '91ba1c958c15c85c75f71ef931b9f4fbdca0f365'
def sha(data):
    return hashlib.sha1(b'blob '+str(len(data)).encode()+b'\0'+data).hexdigest()
def once(text, old, new):
    assert text.count(old) == 1, old[:100]
    return text.replace(old, new, 1)
p = Path('internal/githubpublish/github.go')
raw = p.read_bytes()
assert sha(raw) == '1da7581c0d1daf93126d9a6708a14a88285dc6ec'
s = raw.decode()
s = once(s, 'if status == http.StatusNotFound {', 'if githubRefAbsent(status, err) {')
s = once(s, 'func (g *githubRemote) request(ctx context.Context, token, method, endpoint string, query url.Values, input, output any) (int, error) {\n', 'func (g *githubRemote) request(ctx context.Context, token, method, endpoint string, query url.Values, input, output any) (int, error) {\n\tif err := ctx.Err(); err != nil {\n\t\treturn 0, err\n\t}\n')
s = once(s, '\tresponse, err := g.client.Do(request)', '\tif err := ctx.Err(); err != nil {\n\t\treturn 0, err\n\t}\n\tresponse, err := g.client.Do(request)')
start = s.index('\tdefer response.Body.Close()\n', s.index('func (g *githubRemote) request('))
end = s.index('\n}\n\nfunc (g *githubRemote) stageBundle', start)
original = s[start:end]
assert 'io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))' in original
assert original.endswith('\treturn response.StatusCode, nil')
s = s[:start]+'\treturn finishGitHubResponse(ctx, response, output)'+s[end:]
p.write_text(s)
subprocess.run(['gofmt','-w','internal/githubpublish/github.go','internal/githubpublish/github_response.go','internal/githubpublish/github_response_test.go'],check=True)

sys.path.insert(0, 'scripts')
from authority_context_inventory import inventory
from verify_context_inventory import ROW
p = Path('docs/implementation/CONTEXT_LIFECYCLE_INVENTORY_V1.md')
raw = p.read_bytes()
assert sha(raw) == 'a138a6ef323710a896895918883833cc37e82a96'
s = raw.decode()
report = inventory(Path('.').resolve())
old_rows = [(p,int(line),name) for p,line,name,_ in ROW.findall(s)]
new_rows = [(f['path'],f['line'],f['name']) for f in report['context_or_call_candidates'] if f['kind']=='context_constructor']
assert old_rows == new_rows and len(old_rows) == 133
old = 'sha256:6d82aacb91feef00839249e4407711ebb9b46d4a1fa1ec24830a06dd3c2d12ae'
s = once(s,old,report['source_files_digest'])
s += '''
## Complete HTTP observations before absence classification

Publisher REST exchange retains the initiating Context before transport and while
reading its bounded response. A complete status verdict is distinct from body,
close, limit or cancellation failure; a failed response carrying HTTP 404 cannot
become a ref-absence fact. Successful decoding uses a fresh target committed only
after read/close/decode/cancellation checks. No independent lifecycle or automatic
retry is introduced. A complete negative observation still does not prove original
dispatcher quiescence or authorize replay; internal blockers [228,229] remain OPEN.
'''
p.write_text(s)
p = Path('scripts/verify_publication_recovery_log.py')
raw = p.read_bytes()
assert sha(raw) == '9b1215756b4bdf698c6602103e18e2b2b6914f3c'
s = raw.decode()
names = re.findall(r'^func (TestPublisherGitHubResponse\w+)\(', Path('internal/githubpublish/github_response_test.go').read_text(), re.M)
assert len(names) == 6
s = once(s, '\nPUBLISHER_REQUIRED = {\n', '\nPUBLISHER_REQUIRED = {\n'+''.join('    "'+n+'",\n' for n in names))
p.write_text(s)
p = Path('docs/status/IMPLEMENTATION_STATUS.md')
raw = p.read_bytes()
assert sha(raw) == '112e82cf9ab99deb008fbb40cab60a3ffaec31ac'
s = raw.decode()
note = '''Publisher REST now distinguishes complete HTTP status observations from failed
body/close/cancellation readback. An incomplete 404 cannot become ref absence or
permit a later publication attempt. Failed or cancelled response decoding cannot
partially replace accepted output. Loopback HTTP truncation and local-only Git
attempt probes test this boundary; no live Provider or post-entry recovery
qualification is implied, and no absence-to-retry authority is added.

'''
s = once(s, '## Typed embedded routing and Skill maturity\n', note+'## Typed embedded routing and Skill maturity\n')
p.write_text(s)
print('Exact original response path amended; 133 origin rows and all prior gates preserved')
