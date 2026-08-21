BINARY := trestle
PKG    := github.com/timimsms/trestle

.PHONY: all build test test-core lint fmt vet check check-strict bench clean fixtures spike

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

# Re-run the Spike 01 probe. Read-only; safe against any repo.
# usage: make spike REPO=~/code/foo DEPTH=2
REPO  ?= .
DEPTH ?= 2
spike:
	./spike/glob-binding-probe.sh --repo $(REPO) --days 180 --unit-depth $(DEPTH)

clean:
	rm -f $(BINARY) coverage.txt
