#!/bin/bash
# canary: e1d33b66472d7b11
# Hidden oracle. cwd = workspace root. Prints "<id> PASS|FAIL" per hidden test; exit 0 iff all pass.
O="$(cd "$(dirname "$0")" && pwd)"; WS="${WORKSPACE:-$PWD}"; cd "$WS"
PY=$(command -v python3.12 || command -v python3)
R="$WS/saga_oracle_run"; rm -rf "$R"; mkdir -p "$R"; touch "$R/__init__.py"; cp "$O"/tests/test_*.py "$R/"
OUT="$WS/.saga-oracle-out.txt"
"$PY" -m unittest discover -v -s saga_oracle_run -t . > "$OUT" 2>&1; rc=$?
sed -nE 's/^(test_[A-Za-z0-9_]+) \(.*\) \.\.\. ok$/\1 PASS/p; s/^(test_[A-Za-z0-9_]+) \(.*\) \.\.\. (FAIL|ERROR)$/\1 FAIL/p' "$OUT"
if ! grep -qE '^test_[A-Za-z0-9_]+ \(.*\) \.\.\. ok$' "$OUT"; then echo "oracle-ran FAIL"; rc=1; fi
rm -rf "$R" "$OUT" "$WS"/__pycache__ "$WS"/*/__pycache__
exit $rc
