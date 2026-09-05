#!/bin/bash
# canary: dabf805e25bad727
# Idempotent, offline. Node 24 runs .ts files natively; the only dependency is vendored.
set -e
node --version >/dev/null
[ -f package.json ] || { echo "run from the repo root" >&2; exit 1; }
exit 0
