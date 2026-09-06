# saga bench report

## 1. Header

- manifest: `sha256:f3e4a33cf77cb97cd259dca30822176d6dbb0e4757b52c0b68f68f4b7d541905`
- tier: user
- created: 2026-09-06T12:08:50Z
- total cost: 1.607 usd
- pre-registration: none (no preregistration.md in this archive)

## 2. Setup

- arm B: model `claude-sonnet-5`, harness `claude-code`, 6 runs over 3 tasks, K=2, outcomes abandon=2 completed=4
- arm B: components `gate`; control blocks none
- task set: `sha256:ec2e896a27bbc1a85ab37f2e119c638e164dc6e26db8d1d072b159cde0becf7b`
- isolation: worktree
- exclusions: 0 infra
- bootstrap: 10000 resamples of tasks, seed 20260902

## 3. Primary outcome

No paired comparison: a single-arm report describes one cell and licenses no claim (bench-spec section 1.3).

## 4. Secondary outcomes

| arm | pass@1 | 95% CI | clean pass@1 | pass^K | instability | false-done | regression | scope viol. | cheat rate | median tokens | median cost | median wall s | median turns | tokens/solved | usd/solved |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| B | 0.667 | 0.000, 1.000 | 0.667 | 0.667 | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | 214626.000 | 0.156 | 34.629 | 11.000 | 616353.250 | 0.402 |

false-done for arm B: claimed_done copied from the trace claim event (claims.txt sha256:3adede7ac0d306ef, abstain.txt sha256:1ae2a7333d1f28c2).

| arm | false-done (structural DONE only) | no-claim rate | runs with claims | claim contradiction, oracle-fail | claim contradiction, oracle-pass (bound 0.020) |
|---|---|---|---|---|---|
| B | 0.000 | 0.000 | 6 | 0.000 | 0.000 |

## 5. Variance

- arm B: pass@1=0.667, pass^1=0.667, pass^2=0.667, instability (pass@1 - pass^K)=0.000
- unstable tasks (0 < c < K): none

| arm | task | n | c | pass rate | median tokens | median cost | median wall s | median turns |
|---|---|---|---|---|---|---|---|---|
| B | py-0007-version-sort-impossible | 2 | 0 | 0.000 | 912186 | 0.545 | 160.8 | 27 |
| B | ts-0001-slug-collapse | 2 | 2 | 1.000 | 214626 | 0.156 | 34.6 | 11 |
| B | ts-0005-retry-backoff | 2 | 2 | 1.000 | 105894 | 0.103 | 21.7 | 8 |

## 6. Negative results

- **no_comparison** note=single arm: no paired comparison was computed, so no claim of the section 1.3 table is licensed by this report
- **possibly_broken_task** pass_all_arms=0.000 task=py-0007-version-sort-impossible
- **k_below_5** k=2 note=K < 5: no claim is licensed (ADR 0001)

## 7. Exploratory

None.

## 8. Cheating and scope scan

- arm B: cheat rate 0.000 (flagged passes over passes), scope-violation rate 0.000, median out-of-scope files 0.0, blocked reach attempts 0

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

    saga bench run --manifest /tmp/saga-smoke-3/archive/B/manifest.json
    saga bench report /tmp/saga-smoke-3/archive/B
