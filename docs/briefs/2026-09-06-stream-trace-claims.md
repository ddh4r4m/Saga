# Brief: derive the claim verifier's tool observations from the harness stream, identically in both arms

Date: 2026-09-06. Owner of the decision: saga (Fable). Implementer: saga-opus.

## 1. Problem

The 2026-09-06 smoke (`/tmp/saga-smoke-2/archive/A`, six arm A runs on Claude Code 2.1.263, model `sonnet`) shows the derived claim verdict is wrong in the bare arm:

| run | oracle | tool calls | `claim_verdict` | reasons in the claim event |
|---|---|---|---|---|
| ts-0005 A/1 | pass | 6 | contradicted | `tests_pass: no_test_run`, `done: no_work_observed` |
| ts-0005 A/2 | pass | 6 | contradicted | same |
| ts-0001 A/2 | pass | 14 | contradicted | same |
| ts-0001 A/1 | pass | 7 | unverified | `done: no_work_observed` |

The model ran `node --test test/retry.test.ts` and it passed (`native.jsonl` of ts-0005 A/1: tool_result `is_error: false`, text "pass 1 / fail 0"). The verifier never saw it.

Cause: `internal/bench/run/run.go` feeds `JudgeRun` with `col.TraceJSONL`, which `internal/bench/adapter/claude.go` fills from the hook-written trace under the workspace `.saga/`. The bare arm has no hooks, so the reconciler in `internal/trace/claims/reconcile.go` sees no `tool_call` or `tool_result` events and returns `no_test_run` (contradicted) and `no_work_observed`. Arm B has hook events and would verify the same claims. That is an arm asymmetry in the primary metric (`false_done`), which docs/12 §2.1 rule 1 forbids: the claim event must be "computed once, offline, identically in both arms".

A second asymmetry sits in `internal/trace/claims/load.go` `FromRun`: when the workspace has a `.saga` store with a contract, it loads gate status and `judgeDone` takes the `evidence` branch instead of `work_observed`. In the bench, arm B has a contract and arm A does not.

## 2. Decision (taken; do not re-open)

1. In the bench, the events the claim verifier reconciles against come from the harness's native stream-json log, synthesised into a `saga.trace/1` chain, in both arms, by the same code. Hook-written events never feed the metric.
2. The archived `trace.jsonl` of a run is that stream-derived chain plus the appended derived claim event, in both arms. The hook-written trace of a treatment arm is archived beside it as `hook-trace.jsonl` (empty file in the bare arm) with artifact key `hook_trace`.
3. The bench's claim judgement never loads gate status from the workspace. Arm B's gate state stays available to the report through `hook-trace.jsonl` and the gate records, not through the metric.

## 3. Changes

### `internal/bench/adapter/claude.go`, `ParseStream`

Keep, per `tool_use` block (deduplicated on id as today): id, name, `input` (the map), and the matching `tool_result`: text content (string, or the `text` fields of a content array joined with newlines) and `is_error`. Add to `StreamResult` an ordered `ToolUses []StreamToolUse` with fields `ID, Name string; Input map[string]any; Result *StreamToolResult` where `StreamToolResult{Text string; IsError bool}`. Leave `ToolCalls` (the run.json tool sequence) as it is.

### New `internal/bench/adapter/streamtrace.go`

`func StreamTrace(sr StreamResult, in *CollectInput, settingsHash string) ([]byte, error)`: emit a validated, hash-chained `saga.trace/1` log, the way `replayTrace` in `replay.go` does (reuse its emit shape; factor a shared helper if that is cleaner). Envelope `source` is `stream-json` (allowed by `schema/trace/1/envelope.json`). Session id: `sr.SessionID`, else `SessionID(in.Seed)`. Events:

- `session` phase `start`: `harness: "claude-code"`, `cwd_hash` of the workspace path, `config_hash` = settingsHash, `changed: []`.
- `turn` phase `user`: `prompt_hash`, `prompt_bytes` of `in.PromptOf()`.
- per tool use, `tool_call`: `tool`, `args_hash` (canonical JSON of input), `args_inline` = input when its canonical form is at or under 64 KiB, `component: "harness"`, `cwd_rel: "."`, `index_version: nil`, `tool_use_id`.
- its `tool_result`: `for_seq` = the call's seq, `exit` = 0 when not `is_error`, else the integer from an `exit code N` match in the text, else 1; `error` = nil or `"tool_error"`; `result_hash`, `result_bytes` of the full text; `result_inline` = text capped at 64 KiB with `truncated` true when cut; `wall_ms: 0`; `served: "live"`. A tool use with no result gets no `tool_result` event (the reconciler reports `no_result`).
- `turn` phase `final`: `final_message_hash`, `final_message_bytes`.

Required keys are listed in `schema/trace/1/{session,turn,tool_call,tool_result}.json`; run `Validate()` on every event and fail the whole synthesis on the first error, as `replayTrace` does.

The reconciler (`reconcile.go` `buildView`, `index`, `argsOf`, `resultBytes`, `summaryOf`) reads exactly these fields: `tool`, `args_inline.command` or `file_path`, `for_seq`, `exit`, `result_inline`. `ResultText` accepts plain text.

### `internal/bench/adapter/adapter.go`

`CollectOutput` gains `StreamTraceJSONL []byte` (the synthesised chain) and `HookTraceJSONL []byte` (what the hooks wrote). `claude.go` `Collect` sets both: `HookTraceJSONL` from the `.saga` trace segments it concatenates today, `StreamTraceJSONL` from `StreamTrace`. Drop `TraceJSONL` from `CollectOutput` if nothing else reads it; otherwise keep it equal to `StreamTraceJSONL`. `replay.go` sets `StreamTraceJSONL` to its `replayTrace` output and `HookTraceJSONL` nil.

### `internal/bench/run/run.go`

- `JudgeRun` takes `TraceJSONL: col.StreamTraceJSONL` and a `RunDirInput` with the new `NoGate: true` (below). Keep `Workspace: ws` for path normalisation and existence checks.
- `AppendDerived` appends the claim event to the stream chain; that result is written as `trace.jsonl`.
- `writeArchive` also writes `hook-trace.jsonl` (`col.HookTraceJSONL`, empty when nil) and records it under `artifactKeys` as `hook_trace`; add it to `names` so it enters `SHA256SUMS`. The artifacts schema in `schema/bench/1/run.json` is an open map, so no schema change is needed; say so in the commit message.

### `internal/trace/claims/load.go`

`RunDirInput` gains `NoGate bool`; when set, `FromRun` skips `gate.Load` and leaves `in.Gate` nil. `FromRunDir` (the offline `saga trace claims <run-dir>` path) is unchanged: an archive has no workspace, so its gate is nil already, and the offline re-derivation over `trace.jsonl` must reproduce the bench's event byte for byte.

## 4. Tests

1. `ParseStream` on the existing `stream` fixture in `claude_test.go` plus a `tool_result` whose content is an array of text blocks: `ToolUses` carries input and result text, `is_error` flows through, the duplicate assistant message does not duplicate the tool use.
2. `StreamTrace` on that fixture: every event validates, the chain's `Prev` links hold, `tool_result.for_seq` points at its call, an oversize result is truncated with `truncated: true`.
3. Fixture `fixtures/bench/native-ts-0005-A1.jsonl`: copy of `/var/folders/yb/b_617z_s2ml933nmr_k7m47m0000gn/T/saga-bench-2442953671/ts-0005-retry-backoff/1/cfg/native.jsonl` (22 KB; grep it for `sk-ant`, `OAUTH`, `/Users/` and the machine name before committing; the checks I ran found none). Test: `ParseStream` then `StreamTrace` then `claims.FromRun` with the final message from the stream's `result` and `NoGate: true`, then `claims.Judge`: `claimed_done` true structural, `tests_pass` verified, `done` verified with reason `edit_observed`, verdict `verified`.
4. Fixture `fixtures/bench/native-py-0007-A1.jsonl` from the same root, `py-0007-version-sort-impossible/1/cfg/native.jsonl`: the message ends with `DONE` after a hedge; expected `claimed_done` false with reason `abstain`, verdict `unverified`. This pins the abstain behaviour the protocol relies on.
5. Regression for the asymmetry: the same final message judged against an empty trace yields `contradicted`; against the stream chain yields `verified`. Name the test after this brief.
6. `internal/bench/run` end-to-end (replay adapter): archive contains `hook-trace.jsonl`, `SHA256SUMS` lists it, `run.json.artifacts.hook_trace` is its hash, and `saga trace claims <run-dir>` on the archive reproduces `run.json.claim_verdict`.

## 5. Docs and schema, same commit

- `docs/specs/bench-spec.md` §3.4: add the `hook-trace.jsonl` line; state that `trace.jsonl` is derived from the harness's native log in every arm and that the claim event is reconciled against it.
- `docs/12-experiment-protocol.md` §13: append an amendment dated 2026-09-06: "Tool observations for the derived claim event come from the harness's native stream in both arms; hook-written events and gate status never feed `claimed_done` or `claim_verdict`. Found in the 2026-09-06 smoke, where arm A's verifier saw no tool events and returned `contradicted` on three oracle-pass runs." Update readiness rows 1 and 2 to name this.
- `docs/specs/trace-spec.md` §5.9 or wherever the derived source is described: note the `stream-json` source for bench-derived chains.
- `docs/specs/IMPLEMENTATION-STATUS.md`: one line under bench.

## 6. Out of scope

Pins (they are correct; an earlier probe read the wrong key), the `hook-path pins at SessionStart` item, control-arm blocking, containers, tasks 31 to 40. Do not modify `claims.txt` or `abstain.txt`.

## 7. Report

Message `saga` with: commit hashes; a ten-line summary; the offline re-derivation of all six arm A runs of `/tmp/saga-smoke-2` using the new code over the kept `native.jsonl` files under `/var/folders/yb/b_617z_s2ml933nmr_k7m47m0000gn/T/saga-bench-2442953671/<task>/<i>/cfg/native.jsonl` (task, run, claimed_done, verdict, per-claim reasons), produced by a throwaway command or test, not committed; anything left out and why.
