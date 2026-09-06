# Brief: the Sonnet 5 price row, and ledger reconciliation on the arm B sessions (docs/12 row 13)

Date: 2026-09-06. Owner of the decision: saga (Fable). Implementer: saga-opus. One or two commits.

## 1. Facts established by saga from the third smoke (`bench/results/smoke-2026-09-06-2`)

1. **Price row.** A least-squares fit of `harness_cost_usd` on the five usage components over all twelve runs has rank 4 and a maximum residual of 1e-16 usd: per million tokens, input 2.00, cache read 0.20, cache write 1h 4.00, output 10.00. `cache_write_5m` is zero on every run (Claude Code writes 1h cache), so its price is not identified. The pinned row in `internal/trace/prices/default.toml` for `claude-sonnet-5` reads 3.00 / 0.30 / 3.75 / 6.00 / 15.00 with source "doc 06 A.1", which is the previous Sonnet generation's sheet. That is the whole 1.50 ratio.
2. **Ledger.** For arm B ts-0001 run 1 (session 5215f7d0), the ledger's twelve entries sum to input_fresh 24, cache_read 223288, cache_write_5m 0, cache_write_1h 10448, output 2412: identical to the harness's own `result` usage in `run.json`. Token reconciliation is exact on that session.

## 2. Decisions (taken)

1. The `claude-sonnet-5` row becomes 2.00 / 0.20 / 2.50 / 4.00 / 10.00 with `source = "fit of claude-code 2.1.263 total_cost_usd over the 12 runs of bench/results/smoke-2026-09-06-2 (rank 4, residual under 1e-9); cache_write_5m assumed at 1.25 times input, the vendor's stated ratio, unidentified in the data"` and a `cache_write_5m_reason` field carrying that last clause. The `sonnet` alias, if the table has one, maps to this row. No other row changes. The price table hash changes; the manifests of the three smokes keep the hash they recorded.
2. `saga bench reconcile <archive-dir> --workspaces <kept-root>` (new subcommand, or a function exposed through an existing one if that is cleaner) reads every run's `run.json` usage and `harness.json.harness_cost_usd`, finds the session's ledger under `<kept-root>/<task>/<i>/ws/.saga/trace/sessions/<session>/ledger.jsonl` (session id from `harness.json` or the trace), and prints one row per run: the five ledger sums, the five harness usage figures, the token deltas, the pinned-table cost of the ledger usage, the harness cost, and the ratio; then a footer with the maximum absolute token delta, the maximum cost error as a fraction, and `within 5%: yes/no`. Output also as `saga.bench.reconcile/1` JSON with a schema. A run without a ledger (arm A has no hooks) is listed as `no ledger` and excluded from the footer.
3. docs/12 row 13 reads "ledger within 5% of the harness's `total_cost_usd` over the 20 smoke sessions". We have 6 arm B sessions from the third smoke and 3 from the first (kept roots: `/var/folders/yb/b_617z_s2ml933nmr_k7m47m0000gn/T/saga-bench-3665571939` for the third; the first smoke's workspaces were copied into `bench/results/smoke-2026-09-05/B/<run>/trace/ledger.jsonl` per its notes). Run the reconciliation over both, put the tables in a new `bench/results/RECONCILIATION.md` with the date, commit hash and price-table hash, and mark row 13 closed if every session is within 5% (expected: exact on tokens, and cost exact after decision 1). You may edit docs/12 row 13 only, to close it with the hash and the count of sessions.

## 3. Tests

1. Price fit: a test that recomputes the fit over the twelve archived `(usage, harness_cost_usd)` pairs in `bench/results/smoke-2026-09-06-2` and asserts the four identified prices to 1e-9 and the residual bound, so a future harness price change breaks the test rather than the numbers.
2. `Cost` on those twelve usage vectors with the new row equals `harness_cost_usd` within 1e-9 each.
3. Reconcile on a synthetic archive with one exact session, one 4% off, one 6% off and one arm A run: rows, footer and JSON as specified; the schema validates.

## 4. Docs, same commit

`docs/specs/trace-spec.md` §3.4: one paragraph, "offline reconciliation against the harness's own accounting" as the bench's path (the Admin API path stays as written for live use). `docs/specs/bench-spec.md` §10.1 one sentence naming `saga bench reconcile`. `docs/specs/IMPLEMENTATION-STATUS.md`. Smoke notes for 2026-09-06-2: under finding 5, one line "fixed in <hash>: the row was the previous generation's sheet".

## 5. Out of scope

Other model rows, the Admin API reconciliation, re-pricing archived runs.

## 6. Report

Hashes; the fit line; the reconciliation footer for both smokes; anything left out. Under 30 lines.
