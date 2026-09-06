#!/bin/bash
# The pilot, with the pre-registered settings of docs/12 fixed here so
# nobody types them: tasks 1 to 20, K=5, Opus 5, the task's own wall
# limit, and docs/12 itself as the pre-registration. It is a thin
# wrapper over scripts/bench-smoke.sh, which does the running.
#
# Before anything is spent it builds the binary, refuses if the corpus
# is not approved for it (bash scripts/bench-approve.sh, once, from a
# plain terminal), prints a provenance head that is worth pasting whole,
# and refuses unless BUDGET_USD is at or above the runner's own
# estimate. Nothing here is a human act; the owner's act was the
# approval and the token (ADR 0010).
#
#   BUDGET_USD=60 bash scripts/bench-pilot.sh
#
# Knobs: BUDGET_USD (required), OUT, and nothing else. The settings are
# pre-registered; change them in docs/12 and here together or not at all.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
OUT=${OUT:-/tmp/saga-pilot-$(date +%Y%m%d-%H%M%S)}
mkdir -p "$OUT/bin"

# shellcheck source=scripts/bench-common.sh
. "$ROOT/scripts/bench-common.sh"

TASKS=$(task_globs 1-20)
K=5
MODEL=claude-opus-5
WALL_CAP=0

preflight "bench-pilot" "$TASKS" "$K" 2

OUT="$OUT" MODEL="$MODEL" K="$K" WALL_CAP="$WALL_CAP" TASKS="$TASKS" \
  BUDGET_USD="$BUDGET_USD" PREREG="$ROOT/docs/12-experiment-protocol.md" \
  bash "$ROOT/scripts/bench-smoke.sh"
