# Tasks and evidence status

| Task | Status | Evidence |
| --- | --- | --- |
| ATT-01 projection over existing validated v6 status | VERIFIED — source-only | internal/production/attention.go; branch-only run #41265017089 |
| ATT-02 authenticated and read-only GET route | VERIFIED — source-only | internal/api/operational_attention.go; server.go; authenticated.go; same run |
| ATT-03 positive/negative projection and authorization tests | VERIFIED — source-only | real `go test -count=1 -timeout 6m ./internal/production ./internal/api` PASS |
| ATT-04 docs/rollback & contract review | WRITTEN — review pending | requirements/design/negative-results; review not implied by CI |
| ATT-05 gofmt + Go 1.26.10 / vet | PASS — branch-only | [run #41265017089](https://github.com/jiying2007/engineering-platform/actions/runs/41265017089), exact source af27045fea4f83f4dc3b9ae400cfea2ce8499287 |
| ATT-06 PR-head required five-job CI / actual review / fresh-main | WAITING_CI | Open normal protected PR on current main. Previously restricted Codex CLI startup is now explicitly authorized **only for zero-model-turn compatibility qualification**, with no `turn/start`, credentials, or shared quota; any contrary evidence stops this path. |
| ATT-07 event-level dedup and actual notification delivery | OUT OF SCOPE | requires incident identity and verified source/owner policy |

The branch-only workflow was temporary and removed after source verification. Existing Go source blobs remain unchanged; the proof is not a trusted canonical CI artifact. Do not raise Product qualification, settle #228/#229, or claim any real notification delivery. 
