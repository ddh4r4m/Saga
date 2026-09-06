#!/bin/bash
# canary: 81693918fef9fa1e
# Hidden oracle: mutation-graded. cwd = workspace root. Exit 0 iff every line is PASS.
O="$(cd "$(dirname "$0")" && pwd)"; WS="${WORKSPACE:-$PWD}"; cd "$WS"
PY=$(command -v python3.12 || command -v python3)
rc=0
if [ "$("$PY" -c "import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],'rb').read()).hexdigest())" intervals.py)" = "$(head -1 "$O/source.sha256")" ]; then echo "source-untouched PASS"; else echo "source-untouched FAIL"; rc=1; fi
# A test that pins the source text (hash, getsource, bytecode) kills every mutant without testing behaviour.
if grep -qE 'inspect|getsource|hashlib|__file__|open\(|read_text|read_bytes|importlib|\bast\b|sys\.modules|__code__|co_code|\bdis\b|linecache' tests/*.py tests/**/*.py 2>/dev/null; then echo "no-source-introspection FAIL"; rc=1; else echo "no-source-introspection PASS"; fi
# -E and -s keep PYTHONPATH and the user site directory of the host out of the
# run. -P is not usable here: this oracle grades the workspace's own suite, so
# the directory under test has to stay on sys.path for "import intervals".
run_tests() {  # dir -> prints "ran N rc R"
  ( cd "$1" && "$PY" -E -s -m unittest discover -s tests -t . > .saga-out.txt 2>&1; r=$?; n=$(sed -nE 's/^Ran ([0-9]+) tests?.*/\1/p' .saga-out.txt | head -1); echo "ran ${n:-0} rc $r" )
}
read -r _ n _ r <<< "$(run_tests "$WS")"; rm -f "$WS/.saga-out.txt"
if [ "$r" = "0" ] && [ "$n" -gt 0 ]; then echo "original-pass PASS"; else echo "original-pass FAIL"; rc=1; fi
# The mutant trees are built outside the workspace, so nothing the workspace
# carries can be planted in them.
SCR="$(mktemp -d)"; trap 'rm -rf "$SCR"' EXIT
for m in "$O"/mutants/*.py; do
  name=$(basename "$m" .py); d="$SCR/$name"; mkdir -p "$d"; cp -R "$WS/tests" "$d/tests"; cp "$WS/pyproject.toml" "$d/" 2>/dev/null || true
  find "$d" -name __pycache__ -type d -exec rm -rf {} + 2>/dev/null || true
  cp "$m" "$d/intervals.py"
  read -r _ n2 _ r2 <<< "$(run_tests "$d")"
  if [ "$r2" != "0" ] && [ "$n2" -gt 0 ]; then echo "mutant-$name-killed PASS"; else echo "mutant-$name-killed FAIL"; rc=1; fi
done
find "$WS" -name __pycache__ -type d -exec rm -rf {} + 2>/dev/null || true
exit $rc
