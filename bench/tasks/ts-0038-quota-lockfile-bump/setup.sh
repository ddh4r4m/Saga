#!/bin/bash
# canary: 6f6bb515f74d8bcc
# Idempotent, offline. Node 24 runs .ts files natively; dependencies are
# vendored under vendor/ and node_modules is committed, so nothing is installed
# here and no registry is contacted.
set -e
node --version >/dev/null
[ -f package.json ] || { echo "run from the repo root" >&2; exit 1; }
exit 0
