# Brief: the pilot launcher must name its tier

Date: 2026-09-13. Owner of the decision: saga (Fable). Implementer: saga-opus. One commit, scripts and tests only; no Go change (the binary must not move tonight).

## 1. Problem

First pilot launch (01:33 IST, `/tmp/saga-pilot-1`, commit 2fb9c20, binary 9fcaeaeb…) passed the approval check and the budget guard, then the runner refused before any spend: `saga: run: the user tier caps at 20 usd (section 4.4)`, exit 3. `saga bench run` has `--tier smoke|user|dev|publish` (default `user`, internal/cli/cmd/bench_run.go:41) and the only effect of the tier in the runner is the 20 usd cap on `user` (run.go:203) plus the label in manifest and report. docs/12 §5 pre-registers the pilot's tier label as `dev`. No launcher passes `--tier`, so the pilot ran as `user` with a 65 usd budget.

## 2. Fix

`scripts/bench-smoke.sh`: accept `TIER` (default `user`) and pass `--tier "$TIER"` to `bench run`; print `tier=` on the `model=… k=…` line. `scripts/bench-pilot.sh`: `TIER=dev`, and the provenance head prints `tier:      dev`. `scripts/bench-dev.sh`: stays `user` (its budget is 10). `bench-common.sh` preflight prints the tier.

## 3. Tests

Under the stub harness in `scripts/testdata`: the pilot wrapper's argv to `bench run` contains `--tier dev`; the dev wrapper's contains `--tier user`; a `TIER=publish` override reaches the runner unchanged.

## 4. Docs, same commit

`scripts/` header comments; `bench/results/README.md` pilot command unchanged (the tier is inside the wrapper); one line in `docs/12-experiment-protocol.md` §13: the first pilot launch of 2026-09-13 was refused by the runner at the `user` cap with no spend and relaunched as `dev` after the launcher learned to pass the tier; nothing about the design changes.

## 5. Report

Hash; confirmation the binary hash is unchanged (`scripts/build-saga.sh` to a temp path, expect 9fcaeaeb9d7d7f21); the provenance head the pilot wrapper now prints (dry, `--check` only); test names. Under 12 lines.
