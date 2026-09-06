#!/bin/bash
# Build the saga binary the bench uses, reproducibly.
#
# The gate's approval identity binds the binary's bytes (gate-spec 8),
# and the bench's corpus approvals are keyed by that hash (ADR 0010), so
# a binary that changes for a reason unrelated to behaviour stales every
# approval the owner gave. Stamping the commit into the binary did
# exactly that: a docs-only commit changed the hash and the next run
# would have reported `not pre-approved` for the whole corpus.
#
# So: no version stamp, -trimpath so no absolute path is embedded, and
# -buildvcs=false so the commit and the dirty flag are not either. The
# manifest already records the commit as `bench_version.git`, and
# `saga --version` prints the built-in default; nothing that a reader
# needs is lost.
#
# usage: build-saga.sh <output-path>
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
OUT=${1:?usage: build-saga.sh <output-path>}

mkdir -p "$(dirname "$OUT")"
# CGO off so the toolchain's C compiler cannot vary the bytes either.
CGO_ENABLED=0 go -C "$ROOT" build -trimpath -buildvcs=false -ldflags "-s -w" -o "$OUT" ./cmd/saga
