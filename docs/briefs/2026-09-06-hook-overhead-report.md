# Brief: measured hook overhead beside the primary (docs/12 commitment 7)

Date: 2026-09-06. Owner of the decision: saga (Fable). Implementer: saga-opus. Two commits: part 1 (recording), part 2 (report and docs).

## 1. Problem

docs/12 §12 commitment 7: "Measured hook overhead (p50 and p95 per event, wall overhead, injected tokens) is reported beside the primary, not in an appendix." Today nothing records hook wall time: `hook-trace.jsonl` events carry no duration (the only `latency_ms` is the harness's `model_call` field and it is null), and `saga bench report` has no overhead table. Injected tokens exist in part: the gate's Stop and status messages carry `message_tokens_est`, the SessionStart context and the claim block do not, and the contract sentence and the safety hook are not counted anywhere.

## 2. Decisions (taken)

1. **Every hook invocation records its own wall time.** The composed hook (`saga hook <harness> <event>`) measures from process start to the byte before it writes its JSON reply and stamps `hook_ms` (integer, milliseconds) and `hook_steps` (map step name to ms, only steps that ran) on every event it writes during that invocation. Schema: optional fields on the envelope (`schema/trace/1/envelope.json`), so old archives stay valid and read as "not recorded". The safety hook's log line gains `latency_ms` the same way.
2. **Injected tokens are counted per run from what the trace holds.** `injected_tokens_est` per run = sum over the run's events of `message_tokens_est` (gate Stop and status), the claim block message (add `message_tokens_est` to the claim event when it blocks), SessionStart `additionalContext` (add `context_tokens_est` to the session event when it injects), plus the fixed contract sentence of the staged prompt (one constant per arm, from `adapter.ContractSentence`). Arm A's figure is the constant 0 plus nothing, and the report says so.
3. **The report shows overhead beside the primary.** In `report.md` section 3, immediately after the primary line, a table per arm: hook invocations per run (median), `hook_ms` p50 and p95 per event type (SessionStart, PreToolUse, PostToolUse, PostToolUseFailure, Stop, safety), hook wall per run as a median share of run wall, `injected_tokens_est` median per run, and the same for arm A (safety hook only). `report.json` gains `overhead` per arm with those keys (schema required, values null with a `_reason` when the archive predates recording). `compare.md` carries the same table with the B minus A delta on the share and the injected tokens.
4. Archives from before this brief report `overhead: null` with reason `hook_ms not recorded before <hash>`; nothing is back-filled.

## 3. Changes

- `internal/hook` (the composed entry): timing per decision 1; `internal/trace` recorder: stamp the fields; `internal/guard` safety hook: `latency_ms`.
- `internal/trace/claims` block message and the SessionStart step: the token estimate fields per decision 2.
- `internal/bench/run`: per-run `overhead` block computed at collect from `hook-trace.jsonl` and `guard.jsonl` (schema on `run.json`, required, nullable with reason).
- `internal/bench/report` and `metrics`: aggregation and the table per decision 3.

## 4. Tests

1. A composed-hook invocation on a synthetic payload writes events with `hook_ms` at or above the measured step sum and `hook_steps` naming the steps that ran.
2. Per-run overhead from a synthetic `hook-trace.jsonl` with known durations: p50, p95, share and injected tokens exact.
3. Report on an archive with recording and on `bench/results/smoke-2026-09-06-2` (no recording): the first prints numbers, the second prints null with the reason; both validate.
4. The safety hook's log line carries `latency_ms` and the arm A overhead row shows it.

## 5. Docs, same commits

`docs/specs/trace-spec.md` §2 envelope fields (`hook_ms`, `hook_steps`) and §11.2 overhead; `docs/specs/bench-spec.md` §7.2 section 3 (the overhead table) and §5 metric list (`overhead`, secondary, pre-registered by docs/12 commitment 7); `docs/specs/IMPLEMENTATION-STATUS.md`. Do not edit docs/12.

## 6. Out of scope

The harness's own `model_call` latency (null from the harness), tokens the model reads from `saga` command output (that is the model's own tool use, not injection), any change to what the gate says.

## 7. Report

Hashes; test names; the overhead table as printed for a replay archive with recording; anything left out. Under 30 lines.
