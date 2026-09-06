#!/bin/bash
# canary: 5bc066014590baf5
O="$(cd "$(dirname "$0")" && pwd)"; WS="${WORKSPACE:-$PWD}"; cd "$WS"
TMP="$WS/.saga-oracle-tmp"; rm -rf "$TMP" "$WS/.oracle-run"; mkdir -p "$TMP" "$WS/.oracle-run"
cp "$O"/tests/*.mjs "$WS/.oracle-run/"
SAGA_ORACLE_FIXTURE="$O/fixture-schema" node --test --test-reporter=tap "$WS"/.oracle-run/*.test.mjs > "$TMP/tap.txt" 2>"$TMP/err.txt"; rc=$?
sed -nE 's/^ok [0-9]+ - ([A-Za-z0-9_-]+).*$/\1 PASS/p; s/^not ok [0-9]+ - ([A-Za-z0-9_-]+).*$/\1 FAIL/p' "$TMP/tap.txt"
if ! grep -qE '^ok [0-9]+ - ' "$TMP/tap.txt"; then echo "oracle-ran FAIL"; rc=1; fi
rm -rf "$WS/.oracle-run" "$TMP"
exit $rc
