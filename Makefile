BINARY := trestle
PKG    := github.com/timimsms/trestle

.PHONY: all build test test-core lint fmt vet check check-strict self-check render bench clean fixtures spike

all: fmt vet test build

build:
	go build -o $(BINARY) ./cmd/trestle

test:
	go test ./...

# `internal/check` is the product and must stay I/O-free. Run it alone to make
# an accidental filesystem dependency obvious rather than buried in a full run.
test-core:
	go test -race -count=1 ./internal/check/...

bench:
	go test -run=XXX -bench=. -benchmem ./internal/check/...

fmt:
	gofmt -l -w .

vet:
	go vet ./...

lint:
	golangci-lint run

# Self-check: run the built binary against the worked example, which has a real
# code tree as of Phase 4. Expect exit 0 with one UNBOUND warning on `tenant`.
# `|| true` keeps a warning from failing the target; `check-strict` is the one
# that treats it as a failure.
check: build
	cd examples/repairs-platform && ../../$(BINARY) check --format=human || true

check-strict: build
	cd examples/repairs-platform && ../../$(BINARY) check --strict

# Trestle checks Trestle. This is the only invocation in the repo whose config
# was not written to make something pass, so it is the one that can still
# surprise us. It runs --strict: this repo holds itself to the standard it
# recommends, and a warning here is a modeling gap worth fixing now.
self-check: build
	./$(BINARY) check --strict

# Re-render this repo's own diagram. Output is gitignored — it is a generated
# artifact, and CONVENTIONS tells authors to edit the .d2 instead.
render: build
	./$(BINARY) render

# Re-run the Spike 01 probe. Read-only; safe against any repo.
# usage: make spike REPO=~/code/foo DEPTH=2
REPO  ?= .
DEPTH ?= 2
spike:
	./spike/glob-binding-probe.sh --repo $(REPO) --days 180 --unit-depth $(DEPTH)

clean:
	rm -f $(BINARY) coverage.txt
