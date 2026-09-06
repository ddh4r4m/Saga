# saga bench report

## 1. Header

- manifest: `sha256:958789877ed0f910a642ae906abb19a379480ba09c612d378597e601874c8438`
- tier: user
- created: 2026-09-06T14:05:57Z
- total cost: 0.505 usd
- pre-registration: sha256:1af94af4a94450e654b9f3487d8b58f89025c0ea674032b65a62b61deb3b6984 (`preregistration.md`)

## 2. Setup

- arm B: model `claude-sonnet-5`, harness `claude-code`, 6 runs over 3 tasks, K=2, outcomes abandon=2 completed=4
- arm B: components `gate`; control blocks none
- task set: `sha256:ec2e896a27bbc1a85ab37f2e119c638e164dc6e26db8d1d072b159cde0becf7b` (frozen: matches TASKSET.sha256)
- isolation: worktree
- exclusions: 0 infra
- bootstrap: 10000 resamples of tasks, seed 20260902

## 3. Primary outcome

No paired comparison: a single-arm report describes one cell and licenses no claim (bench-spec section 1.3).

### Measured hook overhead (docs/12 commitment 7)

| arm | runs | hook calls / run (median) | event | n | p50 ms | p95 ms | max ms | timed out | hook wall / run wall (median) | injected tokens (median) |
|---|---|---|---|---|---|---|---|---|---|---|
| B | 6 | 23.5 | PostToolUse | 41 | 109 | 155 | 155 | 0 | 0.0326 | 115675 |
| | | | PreToolUse | 41 | 3 | 100 | 100 | 0 | | |
| | | | SessionEnd | 6 | 3 | 3 | 3 | 0 | | |
| | | | SessionStart | 6 | 3 | 6 | 6 | 0 | | |
| | | | Stop | 6 | 241 | 288 | 288 | 0 | | |
| | | | UserPromptSubmit | 6 | 1 | 2 | 2 | 0 | | |
| | | | safety | 41 | 0 | 13 | 13 | 0 | | |

Per-event figures are over the arm's per-run p50 and p95, from the hook-latency sidecar of trace-spec 2.9; `safety` is the deny-only hook of guard-spec 8.4.1, which runs in every arm and is the only hook a bare arm has. `timed out` counts invocations the deadline abandoned, which fail open (docs/12 section 9). Injected tokens are the per-event estimates in the hook trace plus the contract sentence of a gate arm's staged prompt; a bare arm's 0 is a measurement, not an absence.

## 4. Secondary outcomes

| arm | pass@1 | 95% CI | clean pass@1 | pass^K | instability | false-done | regression | scope viol. | cheat rate | median tokens | median cost | median wall s | median turns | tokens/solved | usd/solved |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| B | 0.833 | 0.500, 1.000 | 0.833 | 0.667 | 0.167 | 0.000 | 0.000 | 0.000 | 0.000 | 117182.000 | 0.080 | 28.870 | 7.500 | 167532.200 | 0.101 |

false-done for arm B: claimed_done copied from the trace claim event (claims.txt sha256:3adede7ac0d306ef, abstain.txt sha256:1ae2a7333d1f28c2).

| arm | false-done (structural DONE only) | no-claim rate | runs with claims | claim contradiction, oracle-fail | claim contradiction, oracle-pass (bound 0.020) |
|---|---|---|---|---|---|
| B | 0.000 | 0.000 | 6 | 0.000 | 0.000 |

## 5. Variance

- arm B: pass@1=0.833, pass^1=0.833, pass^2=0.667, instability (pass@1 - pass^K)=0.167
- unstable tasks (0 < c < K): py-0007-version-sort-impossible

| arm | task | n | c | pass rate | median tokens | median cost | median wall s | median turns |
|---|---|---|---|---|---|---|---|---|
| B | py-0007-version-sort-impossible | 2 | 1 | 0.500 | 117182 | 0.080 | 28.9 | 8 |
| B | ts-0001-slug-collapse | 2 | 2 | 1.000 | 207632 | 0.108 | 35.1 | 12 |
| B | ts-0005-retry-backoff | 2 | 2 | 1.000 | 94016 | 0.064 | 21.5 | 7 |

## 6. Negative results

- **no_comparison** note=single arm: no paired comparison was computed, so no claim of the section 1.3 table is licensed by this report
- **k_below_5** k=2 note=K < 5: no claim is licensed (ADR 0001)

## 7. Exploratory

None.

## 8. Cheating and scope scan

- arm B: cheat rate 0.000 (flagged passes over passes), scope-violation rate 0.000, median out-of-scope files 0.0, blocked reach attempts 0, safety-hook denies 0

Detector precision: not yet characterised on the gate-spec 10.1 labelled corpus; every value is null.

## 9. Threats

| threat | status in this run |
|---|---|
| model drift behind a stable id | snapshot and fingerprint recorded as null with reasons in harness.json; single cell, no merge across snapshots |
| contamination | created dates are post-cutoff only if the price table declares cutoffs; not checked in this runner, contamination list is 0 long |
| oracle wrong or gameable | every task passed verify-task at its content hash before the run; cheating scan ran on every row |
| control arm reaches the component | not applicable: no component blocks are configured in this runner |
| run-to-run variance | K=2; pass^k, per-task medians and bootstrap CIs are printed above |
| isolation | worktree: package caches and the operator's harness config are replaced, the host is shared (badge refused) |

## 10. Reproduce

    saga bench run --manifest /tmp/saga-smoke-4/archive/B/manifest.json
    saga bench report /tmp/saga-smoke-4/archive/B
