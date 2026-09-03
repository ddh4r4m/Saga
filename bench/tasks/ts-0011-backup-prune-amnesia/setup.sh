#!/bin/bash
# canary: efbabbb8004262bf
# Idempotent, offline. Node 24 runs .ts test files natively; nothing to install.
set -e
node --version >/dev/null
[ -f package.json ] || { echo "run from the repo root" >&2; exit 1; }
exit 0
