GO ?= go

.PHONY: build fmt fmt-check test vet check release legacy-check

build:
	mkdir -p bin
	cd tool && CGO_ENABLED=1 $(GO) build -buildvcs=false -trimpath -o ../bin/eng ./cmd/eng

fmt:
	cd tool && gofmt -w cmd internal

fmt-check:
	@test -z "$$(cd tool && gofmt -l cmd internal)"

test:
	cd tool && CGO_ENABLED=1 $(GO) test -race -count=1 -timeout 3m ./...

vet:
	cd tool && CGO_ENABLED=1 $(GO) vet ./...

check: fmt-check test vet build

release:
	ENG_GO_BIN="$(GO)" bash tool/scripts/build-release.sh

# Historical regression only; this does not build the active eng product.
legacy-check:
	$(GO) test ./...
	$(GO) vet ./...
	$(GO) build ./cmd/...
