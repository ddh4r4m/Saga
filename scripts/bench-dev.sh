#!/bin/bash
# A dev run: one pass over a batch of the corpus with Sonnet, for
# finding defects rather than for a result. Same preflight as the pilot,
# the same wrapper over scripts/bench-smoke.sh.
#
#   BUDGET_USD=8 bash scripts/bench-dev.sh 1-20
#
# The argument selects the batch: 1-20, 21-40, all, or a literal
# comma-separated glob. Knobs: BUDGET_USD (required), OUT.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
OUT=${OUT:-/tmp/saga-dev-$(date +%Y%m%d-%H%M%S)}
mkdir -p "$OUT/bin"

# shellcheck source=scripts/bench-common.sh
. "$ROOT/scripts/bench-common.sh"

TASKS=$(task_globs "${1:-1-20}")
K=1
MODEL=sonnet
WALL_CAP=0
# A dev run stays on the `user` tier: its budget is well under the 20 usd
# cap and nothing it produces is pre-registered. It is overridable,
# unlike the pilot's, precisely because nothing here is pre-registered.
TIER=${TIER:-user}

preflight "bench-dev" "$TASKS" "$K" 2

OUT="$OUT" MODEL="$MODEL" K="$K" WALL_CAP="$WALL_CAP" TASKS="$TASKS" TIER="$TIER" \
  BUDGET_USD="$BUDGET_USD" PREREG="$ROOT/docs/12-experiment-protocol.md" \
  bash "$ROOT/scripts/bench-smoke.sh"
