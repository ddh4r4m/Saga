# saga bench report

## 1. Header

- manifest: `sha256:f50d4f9e28c15c126d99d655271e2508cebfbf3391926d67ec8839c6018c8020`
- tier: user
- created: 2026-09-12T18:31:33Z
- total cost: null usd
- pre-registration: sha256:c44b53101b2260301ba4bdebc32943eb1b61c7c21e017c5c4d05c94bc85cfb82 (`preregistration.md`)

## 2. Setup

- arm B: model `sonnet`, harness `claude-code`, 20 runs over 0 tasks, K=1, outcomes infra=20
- arm B: components `gate`; control blocks none
- task set: `sha256:ec52d09a0774715146c6653ea321e3cfcc85cdcba6f65489fcbecb7626ecfbe8` (frozen: matches TASKSET.sha256)
- isolation: worktree
- exclusions: 20 infra
- bootstrap: 10000 resamples of tasks, seed 20260902

## 3. Primary outcome

No paired comparison: a single-arm report describes one cell and licenses no claim (bench-spec section 1.3).

### Measured hook overhead (docs/12 commitment 7)

| arm | runs | hook calls / run (median) | event | n | p50 ms | p95 ms | max ms | timed out | hook wall / run wall (median) | injected tokens (median) |
|---|---|---|---|---|---|---|---|---|---|---|
| B | | | | | | | | | | no run recorded hook wall time |

Per-event figures are over the arm's per-run p50 and p95, from the hook-latency sidecar of trace-spec 2.9; `safety` is the deny-only hook of guard-spec 8.4.1, which runs in every arm and is the only hook a bare arm has. `timed out` counts invocations the deadline abandoned, which fail open (docs/12 section 9). Injected tokens are the per-event estimates in the hook trace plus the contract sentence of a gate arm's staged prompt; a bare arm's 0 is a measurement, not an absence.

## 4. Secondary outcomes

| arm | pass@1 | 95% CI | clean pass@1 | pass^K | instability | false-done | regression | scope viol. | cheat rate | median tokens | median cost | median wall s | median turns | tokens/solved | usd/solved |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| B | 0.000 | 0.000, 0.000 | 0.000 | 0.000 | 0.000 | null | 0.000 | 0.000 | null | null | null | null | null | infinity (no solved run) | infinity (no solved run) |

false-done for arm B: no run claimed done.

| arm | false-done (structural DONE only) | no-claim rate | runs with claims | claim contradiction, oracle-fail | claim contradiction, oracle-pass (bound 0.020) |
|---|---|---|---|---|---|
| B | null | 0.000 | 0 | null | null |

## 5. Variance

- arm B: pass@1=0.000, pass^1=0.000, pass^1=0.000, instability (pass@1 - pass^K)=0.000
- unstable tasks (0 < c < K): none

| arm | task | n | c | pass rate | median tokens | median cost | median wall s | median turns |
|---|---|---|---|---|---|---|---|---|

## 6. Negative results

- **no_comparison** note=single arm: no paired comparison was computed, so no claim of the section 1.3 table is licensed by this report
- **exclusions** infra=20
- **k_below_5** k=1 note=K < 5: no claim is licensed (ADR 0001)

## 7. Exploratory

None.

## 8. Cheating and scope scan

- arm B: cheat rate null (flagged passes over passes), scope-violation rate 0.000, median out-of-scope files 0.0, blocked reach attempts 0, safety-hook denies 0

Detector precision: not yet characterised on the gate-spec 10.1 labelled corpus; every value is null.

## 9. Threats

| threat | status in this run |
|---|---|
| model drift behind a stable id | snapshot and fingerprint recorded as null with reasons in harness.json; single cell, no merge across snapshots |
| contamination | created dates are post-cutoff only if the price table declares cutoffs; not checked in this runner, contamination list is 0 long |
| oracle wrong or gameable | every task passed verify-task at its content hash before the run; cheating scan ran on every row |
| control arm reaches the component | not applicable: no component blocks are configured in this runner |
| run-to-run variance | K=1; pass^k, per-task medians and bootstrap CIs are printed above |
| isolation | worktree: package caches and the operator's harness config are replaced, the host is shared (badge refused) |

## 10. Reproduce

    saga bench run --manifest /tmp/saga-dev-2/archive/B/manifest.json
    saga bench report /tmp/saga-dev-2/archive/B
