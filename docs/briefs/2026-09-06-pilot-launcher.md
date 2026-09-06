# Brief: one command for the pilot, and one for a dev run

Date: 2026-09-06. Owner of the decision: saga (Fable). Implementer: saga-opus. One commit.

## 1. What

`scripts/bench-pilot.sh`, a thin wrapper over `scripts/bench-smoke.sh` that fixes the pre-registered pilot settings of docs/12 §5 and §10 week 5 so nobody types them: tasks 1 to 20 by the three-glob form (`*-000?-*`, `*-001?-*`, `*-0020-*`), `K=5`, `MODEL=claude-opus-5`, `WALL_CAP=0` (the task's own limit), `--prereg docs/12-experiment-protocol.md`, `OUT` defaulting to `/tmp/saga-pilot-<date>`, and a budget guard: before running, print the runner's own cost estimate for the cell and refuse unless `BUDGET_USD` is set at or above it (the runner already has `--budget`; pass it through). The wrapper also runs `approve-corpus --check` first and stops with the `bench-approve.sh` instruction if any task is uncovered, so a stale approval is found before a cent is spent.

`scripts/bench-dev.sh`, the same wrapper with `K=1`, `MODEL=sonnet`, `WALL_CAP=0`, `OUT=/tmp/saga-dev-<date>`, and a `TASKS` override that accepts `1-20`, `21-40` or `all` and expands to the glob form.

Both print, before anything runs: commit, binary hash, task-set hash, pre-registration hash, canonical path, the estimate, and the OUT directory, one per line, so a paste of the head is a complete provenance record.

## 2. Tests

Under the stub harness in `scripts/testdata`: the pilot wrapper composes the expected `bench-smoke.sh` environment and flags (assert the argv the smoke launcher sees); the budget guard refuses below the estimate and proceeds at it; `bench-dev.sh 21-40` selects exactly tasks 21 to 40; both stop on an uncovered approval with the instruction.

## 3. Docs, same commit

`scripts/` header comments, `docs/specs/IMPLEMENTATION-STATUS.md`, and one paragraph in `docs/12-experiment-protocol.md` §10 week 5 is not to be edited; instead add the two commands to `bench/README.md` or the results directory's README if one exists (else `bench/results/README.md`, new, three lines).

## 4. Report

Hash; the provenance head the pilot wrapper prints on this machine (dry, from `--check` only, no run); test names. Under 15 lines.
