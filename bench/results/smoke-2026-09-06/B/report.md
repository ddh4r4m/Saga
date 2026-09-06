# saga bench report

## 1. Header

- manifest: `sha256:b2419323b0d0798158e18c5e1e6ac4c0d0abc2585abe8d2907c5b6f04ec648fe`
- tier: user
- created: 2026-09-06T07:10:05Z
- total cost: null usd
- pre-registration: none (no preregistration.md in this archive)

## 2. Setup

- arm B: model `sonnet`, harness `claude-code`, 6 runs over 0 tasks, K=2, outcomes infra=6
- arm B: components `gate`; control blocks none
- task set: `sha256:f393a1b0467fc785022e7fd8539caa124f7e17f4c0aade15adf53a1fc30f8632`
- isolation: worktree
- exclusions: 6 infra
- bootstrap: 10000 resamples of tasks, seed 20260902

## 3. Primary outcome

No paired comparison: a single-arm report describes one cell and licenses no claim (bench-spec section 1.3).

## 4. Secondary outcomes

| arm | pass@1 | 95% CI | clean pass@1 | pass^K | instability | false-done | regression | scope viol. | cheat rate | median tokens | median cost | median wall s | median turns | tokens/solved | usd/solved |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| B | 0.000 | 0.000, 0.000 | 0.000 | 0.000 | 0.000 | null | 0.000 | 0.000 | null | null | null | null | null | infinity (no solved run) | infinity (no solved run) |

false-done for arm B: no run claimed done.

| arm | false-done (structural DONE only) | no-claim rate | runs with claims | claim contradiction, oracle-fail | claim contradiction, oracle-pass (bound 0.020) |
|---|---|---|---|---|---|
| B | null | 0.000 | 0 | null | null |

## 5. Variance

- arm B: pass@1=0.000, pass^1=0.000, pass^2=0.000, instability (pass@1 - pass^K)=0.000
- unstable tasks (0 < c < K): none

| arm | task | n | c | pass rate | median tokens | median cost | median wall s | median turns |
|---|---|---|---|---|---|---|---|---|

## 6. Negative results

- **no_comparison** note=single arm: no paired comparison was computed, so no claim of the section 1.3 table is licensed by this report
- **exclusions** infra=6
- **k_below_5** k=2 note=K < 5: no claim is licensed (ADR 0001)

## 7. Exploratory

None.

## 8. Cheating and scope scan

- arm B: cheat rate null (flagged passes over passes), scope-violation rate 0.000, median out-of-scope files 0.0, blocked reach attempts 0

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

    saga bench run --manifest /tmp/saga-smoke-2/archive/B/manifest.json
    saga bench report /tmp/saga-smoke-2/archive/B
