#!/bin/bash
# Approve the frozen bench corpus, once, from a plain terminal.
#
# This is the owner's act (ADR 0010). It records consent for every
# `CHECK:` line of every task in the frozen set, bound to one saga
# binary; after it, `scripts/bench-smoke.sh` runs from any shell,
# because no human act remains inside a run. Re-run it when the task set
# is re-frozen or when the binary changes; both are in the store's key,
# so stale records are simply not found and the next run says so rather
# than grading anything.
#
# The binary is built by scripts/build-saga.sh into a stable location, so
# the same source tree gives the same bytes and the approvals keep
# matching. Do not approve a binary built any other way: the identity
# binds the bytes.
#
# usage: bench-approve.sh ['<tasks-glob>']    default: bench/tasks/*
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
TASKS=${1:-$ROOT/bench/tasks/*}
SAGA_BUILD=${SAGA_BUILD:-${SAGA_HOME:-$HOME/.saga}/bench/build/saga}

for m in SAGA_AGENT_SHELL CLAUDECODE CLAUDE_CODE_ENTRYPOINT CODEX_SANDBOX CODEX_CI GEMINI_CLI CURSOR_AGENT; do
  if [ -n "${!m:-}" ]; then
    echo "bench-approve: $m is set; approving is a human act and is refused inside an agent shell" >&2
    exit 3
  fi
done

bash "$ROOT/scripts/build-saga.sh" "$SAGA_BUILD"
HASH=$(shasum -a 256 "$SAGA_BUILD" | cut -d' ' -f1)
echo "bench-approve: binary $SAGA_BUILD"
echo "bench-approve: binary sha256:$HASH"

# saga refuses again on its own if a marker slipped past the loop above;
# the check here is for the message, not for the guarantee.
"$SAGA_BUILD" bench approve-corpus --saga-bin "$SAGA_BUILD" "$TASKS"
echo "bench-approve: done; scripts/bench-smoke.sh now runs from any shell for this binary and this task set"
