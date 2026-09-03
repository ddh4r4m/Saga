#!/bin/sh
# Measures `saga hook claude-code PreToolUse` wall time over N spawns in a
# scratch repository (JSON in on stdin, JSON out) and prints p50 and p95.
set -eu
BIN="${1:?binary}"
N="${2:-50}"
BIN="$(cd "$(dirname "$BIN")" && pwd)/$(basename "$BIN")"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/repo/.git"
(cd "$TMP/repo" && "$BIN" init >/dev/null)
PAYLOAD="{\"session_id\":\"bench\",\"hook_event_name\":\"PreToolUse\",\"cwd\":\"$TMP/repo\",\"tool_name\":\"Bash\",\"tool_input\":{\"command\":\"ls -la\"},\"tool_use_id\":\"tu\"}"
# warm-up
for i in 1 2 3 4 5; do (cd "$TMP/repo" && printf '%s' "$PAYLOAD" | "$BIN" hook claude-code PreToolUse >/dev/null); done
python3 - "$BIN" "$TMP/repo" "$N" "$PAYLOAD" <<'PY'
import subprocess, sys, time, statistics
bin_, cwd, n, payload = sys.argv[1], sys.argv[2], int(sys.argv[3]), sys.argv[4]
samples = []
for _ in range(n):
    t0 = time.perf_counter()
    p = subprocess.run([bin_, "hook", "claude-code", "PreToolUse"], input=payload.encode(), cwd=cwd, capture_output=True)
    samples.append((time.perf_counter() - t0) * 1000)
    assert p.stdout.strip() == b"{}", p.stdout
base = []
for _ in range(n):
    t0 = time.perf_counter()
    subprocess.run([bin_, "version"], cwd=cwd, capture_output=True)
    base.append((time.perf_counter() - t0) * 1000)
base.sort()
print(f"baseline `saga version` spawn over {n}: p50 {base[len(base)//2]:.1f} ms  p95 {base[min(len(base)-1, int(round(0.95*len(base)))-1)]:.1f} ms")
samples.sort()
p50 = samples[len(samples)//2]
p95 = samples[min(len(samples)-1, int(round(0.95*len(samples)))-1)]
print(f"saga hook PreToolUse cold start over {n} spawns: p50 {p50:.1f} ms  p95 {p95:.1f} ms  min {samples[0]:.1f} ms  max {samples[-1]:.1f} ms")
PY
