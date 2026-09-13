#!/bin/bash
# An exploration cell. **Nothing this script produces is the
# pre-registered experiment, which closed on 2026-09-13 (docs/12 §13).**
# Its numbers exercise the bench; they may not be quoted as results.
#
#   BUDGET_USD=20 bash scripts/bench-explore.sh
#   BUDGET_USD=20 bash scripts/bench-explore.sh 1-9
#
# Why Haiku 4.5. A weaker model should carry a higher false-done base
# rate and a wider spread of failure shapes than Opus did, which is what
# exercises the parts of the bench that only fire when a run goes wrong:
# the claim detectors and their referent rules, the gate's block and
# release path, ABANDON grading on the impossible tasks, and the
# session-limit handling. Doing that on Opus costs 50 usd a pass; on
# Haiku it is a few. Price row: `internal/trace/prices/default.toml`,
# `id = "claude-haiku-4-5-20251001"`, in 1.00 out 5.00, source line
# "doc 06 A.1 (Haiku 4.5 $1/$5; cache multipliers per doc 05 section
# 4.2)".
#
# Tier `user`, so the runner's own 20 usd cap applies by construction
# (bench-spec §4.4) and no exploration can grow into a pilot-sized
# spend by accident.
#
# The default batch is the first nine tasks, because that is what fits.
# The runner's estimate is `Σ K × cost_hint_usd × arms` and knows
# nothing about the model, so it prices a Haiku run as if it were the
# Opus run the hints were calibrated on: at K=5 over two arms, tasks
# 1-9 estimate 19.00 usd and tasks 1-20 estimate 48.50, which the tier
# cap refuses. Ask for a larger batch and the launcher will say so
# before spending anything; that refusal is the cap working, not a
# fault. Selectors: 1-9 (the default), 1-20, 21-40, all, or a literal
# comma-separated glob.
#
# Knobs: BUDGET_USD (required, at most 20), OUT, MODEL, K.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
OUT=${OUT:-/tmp/saga-explore-$(date +%Y%m%d-%H%M%S)}
mkdir -p "$OUT/bin"

# shellcheck source=scripts/bench-common.sh
. "$ROOT/scripts/bench-common.sh"

SELECTOR=${1:-1-9}
case "$SELECTOR" in
  # The first nine tasks, one decade of the corpus: the largest batch
  # whose estimate fits the user tier's cap at K=5.
  1-9) TASKS="$ROOT/bench/tasks/*-000?-*" ;;
  *)   TASKS=$(task_globs "$SELECTOR") ;;
esac
K=${K:-5}
MODEL=${MODEL:-claude-haiku-4-5-20251001}
WALL_CAP=0
TIER=user
PURPOSE="exploration, not pre-registered"

preflight "bench-explore" "$TASKS" "$K" 2

# The archive gets the same sentence as the head, so a directory found
# later on disk says what it is without anyone remembering.
cat > "$OUT/PURPOSE.txt" <<'PURPOSE_EOF'
Exploration cell, not the pre-registered experiment.

The experiment of docs/12 closed on 2026-09-13 (§13 amendment, commit
b085526) and its result is bench/results/pilot-2026-09-13. This run was
launched with scripts/bench-explore.sh to exercise the bench on a weaker
and cheaper model, at tier `user` with the 20 usd cap.

No number in this directory is a result. None of it may be quoted in a
report, a README, a badge or a claim about Saga. It is not paired with
the pilot, it is not pre-registered, and `saga bench badge` refuses it
by rule at any tier below `publish`.
PURPOSE_EOF

OUT="$OUT" MODEL="$MODEL" K="$K" WALL_CAP="$WALL_CAP" TASKS="$TASKS" TIER="$TIER" \
  BUDGET_USD="$BUDGET_USD" PREREG="$ROOT/docs/12-experiment-protocol.md" \
  bash "$ROOT/scripts/bench-smoke.sh"
