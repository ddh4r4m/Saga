#!/bin/bash
# End-to-end bench smoke on Claude Code: arm A is bare Claude Code (no
# hooks, no contract, `saga` shimmed off the PATH), arm B is Claude Code
# with the Saga hooks installed and the task contract in .saga/. Three
# tasks, K runs each, arms interleaved per task (bench-spec 3.2).
#
# The owner's act is the corpus approval and the token, not the run
# (ADR 0010). Approve the frozen task set once, from a plain terminal:
#
#   saga bench approve-corpus 'bench/tasks/*'
#
# After that this script runs from any shell, agent sessions included,
# because no human act remains in a run: arm B's baseline consumes those
# records and never writes one. Under a harness marker the script first
# asks `saga bench approve-corpus --check`, which writes nothing, and
# refuses only when a task in the set has no record for the saga binary
# it is about to build.
#
# Authentication: a fresh CLAUDE_CONFIG_DIR has no login, so export
# CLAUDE_CODE_OAUTH_TOKEN (from `claude setup-token`) or ANTHROPIC_API_KEY
# before running. The bench never reads ~/.claude.
#
# Environment knobs: OUT (archive root), MODEL (default sonnet), K
# (default 2), WALL_CAP (seconds per run, default 300), TASKS, PREREG,
# BUDGET_USD (passed to `bench run --budget`, which refuses when its own
# estimate exceeds it), TIER (default user), ON_LIMIT (wait or stop,
# default wait).
#
# TIER must be named rather than defaulted for anything larger than a
# dev run: the runner caps the `user` tier at 20 usd (bench-spec 4.4),
# and the first pilot launch of 2026-09-13 passed the approval check and
# the budget guard with BUDGET_USD=65 and was then refused by the runner
# with `the user tier caps at 20 usd`, exit 3, before any spend.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
OUT=${OUT:-/tmp/saga-bench-smoke-$(date +%Y%m%d-%H%M%S)}
MODEL=${MODEL:-sonnet}
K=${K:-2}
WALL_CAP=${WALL_CAP:-300}
TIER=${TIER:-user}
ON_LIMIT=${ON_LIMIT:-wait}
TASKS=${TASKS:-$ROOT/bench/tasks/ts-0001-slug-collapse,$ROOT/bench/tasks/ts-0005-retry-backoff,$ROOT/bench/tasks/py-0007-version-sort-impossible}

AGENT_MARKER=""
for m in SAGA_AGENT_SHELL CLAUDECODE CLAUDE_CODE_ENTRYPOINT CODEX_SANDBOX CODEX_CI GEMINI_CLI CURSOR_AGENT; do
  if [ -n "${!m:-}" ]; then AGENT_MARKER=$m; break; fi
done
if [ -z "${CLAUDE_CODE_OAUTH_TOKEN:-}" ] && [ -z "${ANTHROPIC_API_KEY:-}" ]; then
  echo "bench-smoke: export CLAUDE_CODE_OAUTH_TOKEN (claude setup-token) or ANTHROPIC_API_KEY first; a fresh config dir has no login" >&2
  exit 6
fi
command -v claude >/dev/null || { echo "bench-smoke: claude not on PATH" >&2; exit 6; }
command -v node >/dev/null || { echo "bench-smoke: node not on PATH (ts tasks)" >&2; exit 6; }
command -v python3 >/dev/null || { echo "bench-smoke: python3 not on PATH (py tasks)" >&2; exit 6; }

mkdir -p "$OUT/bin"
# The shared build: no version stamp, so a docs-only commit does not
# change the bytes and stale every corpus approval (ADR 0010; the
# manifest still records the commit as bench_version.git).
bash "$ROOT/scripts/build-saga.sh" "$OUT/bin/saga"
SAGA="$OUT/bin/saga"

# ADR 0010: the run needs no human act, but it does need the corpus to
# have been approved for this binary. --check writes nothing. Under an
# agent marker a gap is fatal, because nobody there can close it; from a
# plain terminal it is a warning naming the command that closes it.
set +e
APPROVAL=$("$SAGA" bench approve-corpus --check --saga-bin "$SAGA" "$TASKS" 2>&1)
APPROVAL_CODE=$?
set -e
echo "$APPROVAL" | tee -a "$OUT/run.log" >/dev/null
if [ "$APPROVAL_CODE" -ne 0 ]; then
  if [ -n "$AGENT_MARKER" ]; then
    {
      echo "bench-smoke: $AGENT_MARKER is set and the corpus is not approved for this binary."
      echo "bench-smoke: run this from a plain terminal, once:"
      echo "bench-smoke:   $SAGA bench approve-corpus --saga-bin $SAGA '$TASKS'"
      echo "$APPROVAL"
    } >&2
    exit 6
  fi
  echo "bench-smoke: corpus not fully approved; approve it with \`$SAGA bench approve-corpus --saga-bin $SAGA '$TASKS'\`" | tee -a "$OUT/run.log" >&2
fi

{
  echo "bench-smoke $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "saga $($SAGA --version 2>&1 | head -1); claude $(claude --version 2>&1 | head -1)"
  echo "model=$MODEL k=$K tier=$TIER wall_cap=${WALL_CAP}s out=$OUT"
  echo "prereg=docs/12-experiment-protocol.md ($(shasum -a 256 "$ROOT/docs/12-experiment-protocol.md" | cut -d" " -f1))"
} | tee "$OUT/run.log"

set +e
"$SAGA" bench run \
  --tasks "$TASKS" \
  --adapter claude-code \
  --k "$K" \
  --arm A:bare --arm B:gate \
  --model "$MODEL" \
  --tier "$TIER" \
  --on-limit "$ON_LIMIT" \
  --wall-cap "$WALL_CAP" \
  --saga-bin "$SAGA" \
  --bench-git "$(git -C "$ROOT" rev-parse --short HEAD)" \
  --keep \
  --prereg "${PREREG:-$ROOT/docs/12-experiment-protocol.md}" \
  ${BUDGET_USD:+--budget "$BUDGET_USD"} \
  --out "$OUT/archive" 2>&1 | tee -a "$OUT/run.log"
code=${PIPESTATUS[0]}
set -e
echo "bench run exit $code" | tee -a "$OUT/run.log"

if [ -f "$OUT/archive/A/rows.jsonl" ] && [ -f "$OUT/archive/B/rows.jsonl" ]; then
  if "$SAGA" bench compare "$OUT/archive/A" "$OUT/archive/B" --out "$OUT/archive" 2>&1 | tee -a "$OUT/run.log"; then
    echo "compare written to $OUT/archive/compare.md" | tee -a "$OUT/run.log"
  else
    echo "compare failed (exit ${PIPESTATUS[0]}); no compare.md (a partial arm cannot be paired)" | tee -a "$OUT/run.log"
  fi
fi
echo "archive: $OUT/archive"
exit "$code"
