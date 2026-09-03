# Saga build targets (ADR 0008 section 5). golangci-lint is not on the
# dependency allow-list, so lint is gofmt plus go vet.
MODULE  := github.com/ddh4r4m/saga
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
BIN     := bin/saga

.PHONY: build test lint vet fmt clean bench-hook

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/saga

test:
	go test ./...

lint: vet
	@fmt_out="$$(gofmt -l . 2>/dev/null | grep -v '^research/' || true)"; \
	if [ -n "$$fmt_out" ]; then echo "gofmt needed:"; echo "$$fmt_out"; exit 1; fi

vet:
	go vet ./...

fmt:
	gofmt -w cmd internal adapters schema

clean:
	rm -rf bin

# Cold-start measurement of the hook path: 50 spawns of PreToolUse in a
# scratch repo, p50 and p95 in milliseconds (trace-spec section 11.1).
bench-hook: build
	@./scripts/bench-hook.sh $(BIN) 50
