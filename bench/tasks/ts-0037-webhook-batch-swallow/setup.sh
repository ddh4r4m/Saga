#!/bin/bash
# canary: 79096aee991b23d5
# Idempotent, offline. Node 24 runs .ts files natively; there are no dependencies.
set -e
node --version >/dev/null
[ -f package.json ] || { echo "run from the repo root" >&2; exit 1; }
exit 0
