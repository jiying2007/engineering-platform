#!/usr/bin/env bash
set -euo pipefail
umask 022
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
: "${1:?usage: build-distribution.sh NEW_ABSOLUTE_DIRECTORY}"
[[ "$1" = /* ]]
mkdir -- "$1"
dist="$(cd -- "$1" && pwd)"
test "$dist" = "$1"
# No role list is repeated in CI or the receipt verifier.
while IFS= read -r name; do
  [[ "$name" =~ ^[a-z][a-z0-9-]*$ ]]
  CGO_ENABLED=0 go build -trimpath -o "$dist/$name" "./cmd/$name"
done < internal/distribution/binaries.txt
python3 - "$dist" internal/distribution/binaries.txt <<'PYCODE'
import hashlib,json,pathlib,sys
out=pathlib.Path(sys.argv[1]); names=pathlib.Path(sys.argv[2]).read_text().splitlines()
assert names==sorted(set(names))
facts=[]; sums=[]
for name in names:
    data=(out/name).read_bytes(); digest=hashlib.sha256(data).hexdigest()
    facts.append(dict(path=name,digest='sha256:'+digest,size=len(data)))
    sums.append(f'{digest}  {name}\n')
(out/'file-manifest.json').write_text(json.dumps(facts,indent=2)+'\n')
(out/'SHA256SUMS').write_text(''.join(sums))
PYCODE
# Runs from delivered bytes, never fills a missing executable from the source.
"$dist/eng" distribution-verify --dir "$dist"
