# Core Bootstrap CI Validation

Date: 2026-09-24

Purpose: trigger the repository pull-request CI against the current clean-slate Core implementation.

The validation target is:

- gofmt check
- go test ./...
- go vet ./...
- build cmd/control-plane
- build cmd/eng
- build cmd/worker

This file carries no runtime or architecture semantics.

Rerun marker: validate against updated CI workflow and current main.
