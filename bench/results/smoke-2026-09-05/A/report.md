# saga bench report

## 1. Header

- manifest: `sha256:f605c417d2c8e44a4ecdb1a65ba37bc0a864957a061dd5a7f18a17cb44b2a561`
- tier: user
- created: 2026-09-04T21:03:05Z
- total cost: 0.751 usd
- pre-registration: none (no preregistration.md in this archive)

## 2. Setup

- arm A: model `claude-sonnet-5`, harness `claude-code`, 6 runs over 3 tasks, K=2, outcomes completed=6
- arm A: components none; control blocks `path-shim:saga`
- task set: `sha256:f393a1b0467fc785022e7fd8539caa124f7e17f4c0aade15adf53a1fc30f8632`
- isolation: worktree
- exclusions: 0 infra
- bootstrap: 10000 resamples of tasks, seed 20260902

## 3. Primary outcome

No paired comparison: a single-arm report describes one cell and licenses no claim (bench-spec section 1.3).

## 4. Secondary outcomes

| arm | pass@1 | 95% CI | clean pass@1 | pass^K | instability | false-done | regression | scope viol. | cheat rate | median tokens | median cost | median wall s | median turns | tokens/solved | usd/solved |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| A | 0.667 | 0.000, 1.000 | 0.667 | 0.667 | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | 92222.000 | 0.093 | 16.590 | 7.500 | 203288.750 | 0.188 |

false-done for arm A: claimed_done from the bench abstain list (placeholder for trace-spec 5.6).

## 5. Variance

- arm A: pass@1=0.667, pass^1=0.667, pass^2=0.667, instability (pass@1 - pass^K)=0.000
- unstable tasks (0 < c < K): none

| arm | task | n | c | pass rate | median tokens | median cost | median wall s | median turns |
|---|---|---|---|---|---|---|---|---|
| A | py-0007-version-sort-impossible | 2 | 0 | 0.000 | 61456 | 0.075 | 16.2 | 4 |
| A | ts-0001-slug-collapse | 2 | 2 | 1.000 | 252900 | 0.208 | 40.3 | 13 |
| A | ts-0005-retry-backoff | 2 | 2 | 1.000 | 92222 | 0.093 | 16.6 | 8 |

## 6. Negative results

- **no_comparison** note=single arm: no paired comparison was computed, so no claim of the section 1.3 table is licensed by this report
- **possibly_broken_task** pass_all_arms=0.000 task=py-0007-version-sort-impossible
- **k_below_5** k=2 note=K < 5: no claim is licensed (ADR 0001)

## 7. Exploratory

None.

## 8. Cheating and scope scan

- arm A: cheat rate 0.000 (flagged passes over passes), scope-violation rate 0.000, median out-of-scope files 0.0, blocked reach attempts 0

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

    saga bench run --manifest /tmp/saga-smoke/archive/A/manifest.json
    saga bench report /tmp/saga-smoke/archive/A
