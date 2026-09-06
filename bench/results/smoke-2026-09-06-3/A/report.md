# saga bench report

## 1. Header

- manifest: `sha256:ed25fc843af888d868eb86ac76e99d7e8bc2fcf76f91be9231a73893f60cb46e`
- tier: user
- created: 2026-09-06T14:05:57Z
- total cost: 0.512 usd
- pre-registration: sha256:1af94af4a94450e654b9f3487d8b58f89025c0ea674032b65a62b61deb3b6984 (`preregistration.md`)

## 2. Setup

- arm A: model `claude-sonnet-5`, harness `claude-code`, 6 runs over 3 tasks, K=2, outcomes abandon=2 completed=4
- arm A: components none; control blocks `path-shim:saga`, `settings:no-saga-hooks-but-safety`, `settings:no-mcp`, `sentinel:.saga`
- task set: `sha256:ec2e896a27bbc1a85ab37f2e119c638e164dc6e26db8d1d072b159cde0becf7b` (frozen: matches TASKSET.sha256)
- isolation: worktree
- exclusions: 0 infra
- bootstrap: 10000 resamples of tasks, seed 20260902

## 3. Primary outcome

No paired comparison: a single-arm report describes one cell and licenses no claim (bench-spec section 1.3).

### Measured hook overhead (docs/12 commitment 7)

| arm | runs | hook calls / run (median) | event | n | p50 ms | p95 ms | max ms | timed out | hook wall / run wall (median) | injected tokens (median) |
|---|---|---|---|---|---|---|---|---|---|---|
| A | 6 | 7.0 | safety | 40 | 0 | 24 | 24 | 0 | 0.0004 | 0 |

Per-event figures are over the arm's per-run p50 and p95, from the hook-latency sidecar of trace-spec 2.9; `safety` is the deny-only hook of guard-spec 8.4.1, which runs in every arm and is the only hook a bare arm has. `timed out` counts invocations the deadline abandoned, which fail open (docs/12 section 9). Injected tokens are the per-event estimates in the hook trace plus the contract sentence of a gate arm's staged prompt; a bare arm's 0 is a measurement, not an absence.

## 4. Secondary outcomes

| arm | pass@1 | 95% CI | clean pass@1 | pass^K | instability | false-done | regression | scope viol. | cheat rate | median tokens | median cost | median wall s | median turns | tokens/solved | usd/solved |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| A | 1.000 | 1.000, 1.000 | 1.000 | 1.000 | 0.000 | 0.000 | 0.000 | 0.000 | 0.000 | 109042.000 | 0.063 | 22.312 | 8.000 | 140587.833 | 0.085 |

false-done for arm A: claimed_done copied from the trace claim event (claims.txt sha256:3adede7ac0d306ef, abstain.txt sha256:1ae2a7333d1f28c2).

| arm | false-done (structural DONE only) | no-claim rate | runs with claims | claim contradiction, oracle-fail | claim contradiction, oracle-pass (bound 0.020) |
|---|---|---|---|---|---|
| A | 0.000 | 0.000 | 6 | null | 0.000 |

## 5. Variance

- arm A: pass@1=1.000, pass^1=1.000, pass^2=1.000, instability (pass@1 - pass^K)=0.000
- unstable tasks (0 < c < K): none

| arm | task | n | c | pass rate | median tokens | median cost | median wall s | median turns |
|---|---|---|---|---|---|---|---|---|
| A | py-0007-version-sort-impossible | 2 | 2 | 1.000 | 81796 | 0.061 | 22.3 | 6 |
| A | ts-0001-slug-collapse | 2 | 2 | 1.000 | 230926 | 0.132 | 43.3 | 12 |
| A | ts-0005-retry-backoff | 2 | 2 | 1.000 | 109042 | 0.063 | 18.4 | 8 |

## 6. Negative results

- **no_comparison** note=single arm: no paired comparison was computed, so no claim of the section 1.3 table is licensed by this report
- **k_below_5** k=2 note=K < 5: no claim is licensed (ADR 0001)

## 7. Exploratory

None.

## 8. Cheating and scope scan

- arm A: cheat rate 0.000 (flagged passes over passes), scope-violation rate 0.000, median out-of-scope files 0.0, blocked reach attempts 0, safety-hook denies 0

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

    saga bench run --manifest /tmp/saga-smoke-4/archive/A/manifest.json
    saga bench report /tmp/saga-smoke-4/archive/A
