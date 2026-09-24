GO ?= go

.PHONY: fmt fmt-check test vet build check run-control-plane capabilities

fmt:
	gofmt -w cmd internal

fmt-check:
	@test -z "$$(gofmt -l cmd internal)"

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

build:
	$(GO) build ./cmd/control-plane
	$(GO) build ./cmd/eng
	$(GO) build ./cmd/worker

check: fmt-check test vet build

run-control-plane:
	$(GO) run ./cmd/control-plane

capabilities:
	$(GO) run ./cmd/eng capabilities
