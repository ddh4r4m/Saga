#!/bin/bash
# canary: 67bbbbf0d5c81a65
# Idempotent, offline. Standard library plus make; nothing to install.
set -e
PY=$(command -v python3.12 || command -v python3)
"$PY" -c "import sys; assert sys.version_info >= (3, 12), sys.version"
command -v make >/dev/null
[ -f pyproject.toml ] || { echo "run from the repo root" >&2; exit 1; }
exit 0
