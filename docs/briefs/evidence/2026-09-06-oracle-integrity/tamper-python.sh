#!/bin/bash
# Corrected: capture the ORACLE's exit code, not tail's, and show the whole
# PASS/FAIL list. Question: can a workspace package __init__ neuter the hidden
# oracle's assertions on an ordinary (possible) Python task?
ROOT=$(git rev-parse --show-toplevel)
probe() {
  local id=$1 pkgfile=$2
  local T=$ROOT/bench/tasks/$id
  local D
  D=$(mktemp -d)
  cp -R "$T/repo/." "$D/"
  cd "$D" || return
  git init -q; git add -A; git -c user.email=b@b -c user.name=b commit -qm base >/dev/null
  cat >> "$pkgfile" <<'EOF'

import unittest as _u
def _noop(self, *a, **k):
    return None
for _n in ("assertEqual", "assertNotEqual", "assertTrue", "assertFalse", "assertIs",
           "assertIsNot", "assertIn", "assertNotIn", "assertAlmostEqual",
           "assertIsInstance", "assertDictEqual", "assertListEqual",
           "assertGreater", "assertLess", "assertCountEqual", "assertSetEqual"):
    setattr(_u.TestCase, _n, _noop)
# Also silence the result recorder, which catches anything the above missed.
_u.TestResult.addFailure = _noop
_u.TestResult.addError = _noop
EOF
  bash "$T/setup.sh" >/dev/null 2>&1
  local out rc
  out=$(bash "$T/oracle/run.sh" 2>&1); rc=$?
  echo "--- $id: oracle exit=$rc"
  echo "$out" | sed 's/^/    /'
}

probe py-0008-money-exact-cents pricing/__init__.py
probe py-0039-rate-table-stale-regen feecalc/__init__.py
