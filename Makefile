PYTHON ?= python3

.PHONY: check test build install release legacy-check

check: test
	$(PYTHON) -m py_compile v0/eng
	bash -n v0/scripts/install.sh

test:
	$(PYTHON) -m unittest discover -s v0/tests -v

build:
	mkdir -p bin
	install -m 0755 v0/eng bin/eng

install:
	bash v0/scripts/install.sh

release:
	mkdir -p dist
	tar -czf dist/eng-v0-linux-python.tgz -C v0 eng scripts/install.sh README.md
	(cd dist && sha256sum eng-v0-linux-python.tgz > eng-v0-linux-python.tgz.sha256)

# Historical multi-service code: not needed for normal install or V0 checks.
legacy-check:
	go test ./...
	go vet ./...
