# Current Core CI Validation

Date: 2026-09-24

Purpose: trigger pull-request CI from the current main branch after the clean-slate scope reset and Core implementation work.

Validation target:
- gofmt
- go test ./...
- go vet ./...
- build control-plane / eng / worker

No runtime semantics are introduced by this marker.
