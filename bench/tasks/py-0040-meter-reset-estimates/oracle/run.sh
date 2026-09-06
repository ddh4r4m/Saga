#!/bin/bash
# canary: 69b875b0f024e134
# Hidden oracle. cwd = workspace root. Prints "<id> PASS|FAIL" per hidden test; exit 0 iff all pass.
O="$(cd "$(dirname "$0")" && pwd)"; WS="${WORKSPACE:-$PWD}"; cd "$WS"
PY=$(command -v python3.12 || command -v python3)
# The runner tree is made fresh outside the workspace on entry, so nothing the
# workspace carries can be planted in it or picked up as part of the oracle.
R=$(mktemp -d); trap 'rm -rf "$R"' EXIT
mkdir -p "$R/saga_oracle_run"; : > "$R/saga_oracle_run/__init__.py"
cp "$O"/tests/test_*.py "$R/saga_oracle_run/"
cp "$O"/tests/saga_oracle_main.py "$R/saga_oracle_main.py"
OUT="$WS/.saga-oracle-out.txt"
# --buffer keeps anything the code under test prints out of the result lines.
# -E and -s keep PYTHONPATH and the user site directory of the host out of the
# run; -P keeps the runner script's own directory off sys.path.
"$PY" -E -s -P "$R/saga_oracle_main.py" "$R" "$WS" oracle > "$OUT" 2>&1; rc=$?
sed -nE 's/^(test_[A-Za-z0-9_]+) \(.*\) \.\.\. ok$/\1 PASS/p; s/^(test_[A-Za-z0-9_]+) \(.*\) \.\.\. (FAIL|ERROR)$/\1 FAIL/p' "$OUT"
if ! grep -qE '^test_[A-Za-z0-9_]+ \(.*\) \.\.\. ok$' "$OUT"; then echo "oracle-ran FAIL"; rc=1; fi
rm -rf "$OUT" "$WS"/__pycache__ "$WS"/*/__pycache__
exit $rc
