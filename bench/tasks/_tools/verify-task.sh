#!/bin/bash
# usage: verify.sh <taskdir>  -> prints one row per state: baseline, gold, broken-*, cheat-*
S="${SAGA_VERIFY_DIR:-$(mktemp -d)}"
task=$(cd "$1" && pwd); id=$(basename "$task"); W=$S/$id; rm -rf "$W"; mkdir -p "$W"
run_state() {  # name patchfile
  local name=$1 patch=$2 d=$W/$1
  rm -rf "$d"; mkdir -p "$d"; cp -R "$task/repo/." "$d/"
  ( cd "$d" && git init -q && git add -A && git -c user.email=b@b -c user.name=b commit -qm base )
  if [ -n "$patch" ]; then ( cd "$d" && git apply "$patch" ) || { echo "$name APPLY-FAILED"; return; }; fi
  ( cd "$d" && bash "$task/setup.sh" >/dev/null 2>&1 ) || { echo "$name SETUP-FAILED"; return; }
  ( cd "$d" && bash "$task/oracle/run.sh" > "$W/$name.oracle.txt" 2>"$W/$name.oracle.err" ); local rc=$?
  echo "$name exit=$rc tests=[$(grep -cE ' PASS$' "$W/$name.oracle.txt") pass/$(grep -cE ' FAIL$' "$W/$name.oracle.txt") fail]"
}
run_state baseline ""
for p in "$task"/controls/gold.patch "$task"/controls/broken-*.patch "$task"/controls/cheat-*.patch; do
  [ -f "$p" ] || continue
  run_state "$(basename "$p" .patch)" "$p"
done
# determinism: gold twice
if [ -f "$task/controls/gold.patch" ]; then
  d=$W/gold; ( cd "$d" && bash "$task/oracle/run.sh" > "$W/gold2.oracle.txt" 2>/dev/null )
  if cmp -s "$W/gold.oracle.txt" "$W/gold2.oracle.txt"; then echo "determinism identical"; else echo "determinism DIFFERS"; fi
fi
