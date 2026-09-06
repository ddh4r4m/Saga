# Ledger reconciliation against the harness's own accounting

Produced 2026-09-06 with `saga bench reconcile` at commit bd9c5e1, price table `sha256:a5f308325cec416b3a1fbc7c4042468481d3398fde0f6712a82a326f2a521c0b`.
docs/12 row 13 asks for the ledger within 5 percent of the harness's `total_cost_usd`.
A run whose arm has no hooks has no ledger; it is listed and excluded from the footer rather than counted a discrepancy.

## Third smoke, 2026-09-06-2, arm B

| task | arm | i | session | ledger in/read/w5m/w1h/out | harness in/read/w5m/w1h/out | max delta | ledger usd | harness usd | ratio |
|---|---|---|---|---|---|---|---|---|---|
| py-0007-version-sort-impossible | B | 1 | c13138bf | 64/923268/0/27542/11045 | 64/923268/0/27542/11045 | 0 | 0.4054 | 0.4054 | 1.000000 |
| py-0007-version-sort-impossible | B | 2 | 00258085 | 64/834252/0/21247/6891 | 64/834252/0/21247/6891 | 0 | 0.3209 | 0.3209 | 1.000000 |
| ts-0001-slug-collapse | B | 1 | 5215f7d0 | 24/223288/0/10448/2412 | 24/223288/0/10448/2412 | 0 | 0.1106 | 0.1106 | 1.000000 |
| ts-0001-slug-collapse | B | 2 | 97a6fd25 | 20/180983/0/9978/2099 | 20/180983/0/9978/2099 | 0 | 0.0971 | 0.0971 | 1.000000 |
| ts-0005-retry-backoff | B | 1 | bde4bb6f | 10/84999/0/9031/1022 | 10/84999/0/9031/1022 | 0 | 0.0634 | 0.0634 | 1.000000 |
| ts-0005-retry-backoff | B | 2 | ef7723d9 | 12/105659/0/9663/1392 | 12/105659/0/9663/1392 | 0 | 0.0737 | 0.0737 | 1.000000 |

6 session(s) with a ledger; max absolute token delta 0; max cost error 0.000000; within 5%: yes

## First smoke, 2026-09-05, arm B

Its ledgers were copied into the archive beside each run, so no workspace root is needed.

| task | arm | i | session | ledger in/read/w5m/w1h/out | harness in/read/w5m/w1h/out | max delta | ledger usd | harness usd | ratio |
|---|---|---|---|---|---|---|---|---|---|
| ts-0001-slug-collapse | B | 1 | 6cc6bb53 | 74/990463/0/30709/14633 | 72/950421/0/28971/14305 | 40042 | 0.4674 | 0.4674 | 1.000000 |
| ts-0001-slug-collapse | B | 2 | 70f06f1b | 70/1006393/0/36280/11913 | 68/959826/0/35496/11264 | 46567 | 0.4657 | 0.4657 | 1.000000 |
| ts-0005-retry-backoff | B | 1 | 046e52e7 | 28/271051/0/13785/2919 | 28/271051/0/13785/2919 | 0 | 0.1386 | 0.1386 | 1.000000 |

3 session(s) with a ledger; max absolute token delta 46567; max cost error 0.000000; within 5%: yes

## Reading the two token deltas

Cost reconciles exactly on all nine sessions: every ledger prices to the harness's own `total_cost_usd` to within 1e-16, which is the docs/12 row 13 bar and far inside it.

Tokens reconcile exactly on seven of the nine. The two exceptions are the 2026-09-05 ts-0001 sessions, where the ledger counts about 40,000 more cache-read tokens than the run row's `usage` object. The ledger is the accurate side, not the drifting one: pricing the ledger's usage gives the harness's own figure exactly, while pricing the row's `usage` gives 0.4492 against a harness figure of 0.4674 on run 1, and 0.4467 against 0.4657 on run 2, both about 4 percent short. So on those two runs the archived `run.json` usage under-counted and the ledger did not.

That archive predates the usage handling of 2026-09-06, and the third smoke, which post-dates it, has a zero delta on all six sessions. Re-pricing archived runs is out of scope here; the discrepancy is recorded rather than corrected, and the manifests keep the price-table hash they ran against.

## What this closes

docs/12 row 13 asked for the ledger within 5 percent of the harness's `total_cost_usd` over the smoke sessions. Nine arm B sessions across two smokes reconcile at a maximum cost error of 0.000000. Arm A has no hooks and therefore no ledger, which is by design and is why the row is about arm B sessions.
