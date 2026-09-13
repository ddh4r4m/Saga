# Shared front matter for the pilot and dev-run wrappers. Sourced, not
# executed: it composes the settings, prints the provenance head, and
# refuses before a cent is spent.

# task_globs <selector> prints the comma-separated glob form for
# `1-20`, `21-40` or `all`. The glob form is used rather than a literal
# list so a re-freeze that renames nothing needs no edit here.
task_globs() {
  case "${1:-1-20}" in
    1-20)  echo "$ROOT/bench/tasks/*-000?-*,$ROOT/bench/tasks/*-001?-*,$ROOT/bench/tasks/*-0020-*" ;;
    # 0021 to 0029, not 002?: `*-002?-*` also matches 0020, so the two
    # batches overlapped on py-0020 and `21-40` selected 21 tasks. Found
    # in the dev run of 2026-09-13, fixed after the experiment closed;
    # the archives that used the old globs record the set they ran.
    21-40) echo "$ROOT/bench/tasks/*-002[1-9]-*,$ROOT/bench/tasks/*-003?-*,$ROOT/bench/tasks/*-0040-*" ;;
    all)   echo "$ROOT/bench/tasks/*" ;;
    *)     echo "$1" ;;
  esac
}

# preflight builds the binary, prints the provenance head, and refuses
# on every problem it finds rather than on the first: an operator who
# has both an unapproved corpus and no budget should learn both in one
# go, not one per attempt. Nothing is spent by any of it. Sets SAGA and
# ESTIMATE. PURPOSE, when set, is printed on the head. TIER is read, defaulted to user, and checked against the
# runner's own cap, because a guard the launcher does not apply is one
# the operator meets separately and later: the first pilot launch of
# 2026-09-13 passed both checks here and was then refused inside
# `bench run` with `the user tier caps at 20 usd`.
preflight() {  # label tasks k arms
  local label=$1 tasks=$2 k=$3 arms=$4
  TIER=${TIER:-user}
  ON_LIMIT=${ON_LIMIT:-wait}

  bash "$ROOT/scripts/build-saga.sh" "$OUT/bin/saga"
  SAGA="$OUT/bin/saga"

  # The approval state of the corpus for this binary (ADR 0010). A stale
  # approval is found here, before anything is spent, rather than as an
  # arm of infra rows halfway through.
  set +e
  local approval approval_code
  approval=$("$SAGA" bench approve-corpus --check --saga-bin "$SAGA" "$tasks" 2>&1)
  approval_code=$?
  set -e

  # The runner's own estimate, so the number in front of the operator is
  # the number the runner will refuse on.
  # The model is passed so the estimate is priced for the model actually
  # being run: the cost hints were calibrated on Opus (docs/12 §6), and
  # without the ratio a Haiku cell was priced as an Opus one and refused
  # by the user tier's cap (2026-09-13).
  ESTIMATE=$("$SAGA" bench estimate "$tasks" --k "$k" --arms "$arms" --model "${MODEL:-}" 2>"$OUT/estimate.log")
  RATIO=$(sed -n 's/^ratio //p' "$OUT/estimate.log")

  {
    echo "$label $(date -u +%Y-%m-%dT%H:%M:%SZ)"
    echo "commit:    $(git -C "$ROOT" rev-parse --short HEAD)$(git -C "$ROOT" diff --quiet || echo ' (dirty)')"
    echo "binary:    sha256:$(shasum -a 256 "$SAGA" | cut -c1-64)"
    echo "task set:  $(grep '^set ' "$ROOT/bench/tasks/TASKSET.sha256" | cut -d' ' -f2)"
    echo "prereg:    sha256:$(shasum -a 256 "$ROOT/docs/12-experiment-protocol.md" | cut -c1-64)"
    echo "path:      $(printf '%s\n' "$approval" | sed -n 's/^path: //p')"
    echo "tier:      $TIER"
    # An exploration cell says so on the head, so a pasted provenance
    # block can never be mistaken for a pre-registered run.
    [ -n "${PURPOSE:-}" ] && echo "purpose:   $PURPOSE"
    echo "on_limit:  $ON_LIMIT"
    echo "estimate:  $ESTIMATE usd (k=$k, $arms arms, model $MODEL, model ratio ${RATIO:-1.0000})"
    echo "budget:    ${BUDGET_USD:-<unset>} usd"
    echo "out:       $OUT"
  } | tee "$OUT/provenance.txt"

  local refused=0
  if [ "$approval_code" -ne 0 ]; then
    refused=6
    {
      echo "$label: the corpus is not approved for this binary."
      echo "$label: run this once, from a plain terminal:  bash scripts/bench-approve.sh"
      printf '%s\n' "$approval" | grep -v '^path: ' | tail -5
    } >&2
  fi
  # The budget is the operator's own statement of what they will spend.
  # It is required rather than defaulted, because a default is a number
  # nobody chose.
  if [ -z "${BUDGET_USD:-}" ]; then
    [ "$refused" -eq 0 ] && refused=3
    echo "$label: set BUDGET_USD to at least the estimate ($ESTIMATE) to proceed" >&2
  elif awk -v b="$BUDGET_USD" -v e="$ESTIMATE" 'BEGIN{exit !(b < e)}'; then
    [ "$refused" -eq 0 ] && refused=3
    echo "$label: BUDGET_USD=$BUDGET_USD is below the estimate $ESTIMATE" >&2
  elif [ "$TIER" = "user" ] && awk -v b="$BUDGET_USD" 'BEGIN{exit !(b > 20)}'; then
    # The runner's own cap (bench-spec 4.4), applied here so it joins the
    # other refusals in one go instead of arriving separately from inside
    # `bench run` after the launcher has said everything is fine.
    [ "$refused" -eq 0 ] && refused=3
    echo "$label: the user tier caps at 20 usd (section 4.4); BUDGET_USD=$BUDGET_USD needs TIER=dev" >&2
  fi
  if [ "$refused" -ne 0 ]; then
    echo "$label: nothing has been spent." >&2
    exit "$refused"
  fi
}
