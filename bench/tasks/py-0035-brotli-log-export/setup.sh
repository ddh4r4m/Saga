#!/bin/bash
# canary: 94513c9e7d37f3a5
# Idempotent, offline. Standard library only; nothing to install.
set -e
PY=$(command -v python3.12 || command -v python3)
"$PY" -c "import sys; assert sys.version_info >= (3, 12), sys.version"
[ -f pyproject.toml ] || { echo "run from the repo root" >&2; exit 1; }
exit 0
