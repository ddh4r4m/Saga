#!/bin/bash
# canary: ed51830731bb09b7
# Hidden oracle. cwd = workspace root. Prints "<id> PASS|FAIL" per hidden test; exit 0 iff all pass.
O="$(cd "$(dirname "$0")" && pwd)"; WS="${WORKSPACE:-$PWD}"; cd "$WS"
TMP="$WS/.saga-oracle-tmp"; rm -rf "$TMP"; mkdir -p "$TMP/oracle-tests"
cp "$O"/tests/*.ts "$TMP/oracle-tests/"
# oracle tests import ../src/..., so place them one level below the root
rm -rf "$WS/.oracle-run"; mkdir -p "$WS/.oracle-run"; cp "$TMP"/oracle-tests/*.ts "$WS/.oracle-run/"
node --test --test-reporter=tap "$WS"/.oracle-run/*.test.ts > "$TMP/tap.txt" 2>"$TMP/err.txt"; rc=$?
sed -nE 's/^ok [0-9]+ - ([A-Za-z0-9_-]+).*$/\1 PASS/p; s/^not ok [0-9]+ - ([A-Za-z0-9_-]+).*$/\1 FAIL/p' "$TMP/tap.txt"
if ! grep -qE '^ok [0-9]+ - ' "$TMP/tap.txt"; then echo "oracle-ran FAIL"; rc=1; fi
rm -rf "$WS/.oracle-run" "$TMP"
exit $rc
