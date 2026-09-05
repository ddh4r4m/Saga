#!/bin/bash
# canary: d475ff26595a9476
# Idempotent, offline. Node 24 runs .ts test files natively; nothing to install.
set -e
node --version >/dev/null
[ -f package.json ] || { echo "run from the repo root" >&2; exit 1; }
exit 0
