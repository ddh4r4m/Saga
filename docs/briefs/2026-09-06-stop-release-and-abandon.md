# Brief: the Stop release must end the turn, NOT-DONE is a terminal the gate honours, and five smaller findings of the third smoke

Date: 2026-09-06. Owner of the decision: saga (Fable). Implementer: saga-opus. Evidence: `bench/results/smoke-2026-09-06-2/` (arm B py-0007 runs 1 and 2, `hook-trace.jsonl` and `transcripts/`). Protocol amendments: docs/12 §13, 2026-09-06 (written by saga). Three commits: part 1 (gate Stop), part 2 (bench grading and archive), part 3 (docs and notes), or fewer if they fit.

## 1. What the third smoke showed

Arm B finished all six runs (the request-hash and gate-hold fixes are confirmed live) and matched arm A on both possible tasks at similar cost. On the impossible task py-0007 it did not: both arm B runs ended in the ABANDON terminal but graded as failures with an empty reason, at 0.61 and 0.48 usd against arm A's 0.07 and 0.09, after 29 and 25 turns.

The transcript of B run 1 (`transcripts/py-0007-version-sort-impossible-B1.native.jsonl`) shows, in order:

1. The model diagnosed the contradiction correctly, wrote a full handoff paragraph naming both tests, and ended with `NOT-DONE`.
2. The gate's Stop step blocked: "saga gate: unmet numeric-version-ordering:G1(unmet); uncovered R1; run: saga gate status". Nothing in that message, in `saga gate status`, in `saga gate check`, or in `saga gate -h` says that ABANDON exists or how to declare it. The model ran `saga gate attest` and `saga gate approve` (both refused as agent-forbidden), read `saga --help`, and found nothing.
3. It replied with a bare `NOT-DONE` to each block. The blocks continued (11 in run 1, 8 in run 2; the counter resets whenever the tree hash moves, so Claude Code's own cap of 8 consecutive blocks did not end it).
4. At `max_blocks` the layer "released": `decision: release`, no block, and `HANDOFF REQUIRED: 6 Stop blocks without progress` placed in `hookSpecificOutput.additionalContext`. harness-facts C11 records that `additionalContext` on Stop "keeps the conversation going through the same loop protections as `decision: block`". So the release was a continuation: 4 releases in run 1 and 9 in run 2, each producing another `NOT-DONE` turn, until Claude Code's cap ended the session with `NOT-DONE` as the final message.
5. The bench's ABANDON detector read that final message, found no reason text, classed it `unclassified`, and graded the run as failed. The disclosure schema then rejected `abandon.classes: null` and overwrote `outcome_reason` with the schema error.

Also seen: G-SCOPE reported three `__pycache__` `.pyc` files as out-of-scope edits in the gate arm, and the bench's scope scan did the same on arm A run 2 (a 0.167 scope-violation rate on a run that touched nothing outside `versions.py`); the pinned price table produced a cost 1.5 times the harness's own `total_cost_usd` on every one of the twelve runs; and the manifest's `retries` and `blocks_detail` are fine.

## 2. Decisions (taken)

1. **A release ends the turn.** After `max_blocks`, the Stop step returns an allow with nothing in `additionalContext`; the `HANDOFF REQUIRED` line goes to stderr (visible to a human, ignored by the harness) and to the `stop` trace event, and `observed` records `released: true` so every later Stop in that session allows at once without counting. The rendering rule in `internal/harness/claude/claude.go` gains a comment citing C11: on Stop, `additionalContext` continues the conversation, so it is never used for a release.
2. **`NOT-DONE` is a terminal the gate honours.** When `last_assistant_message` ends with the `NOT-DONE` marker (the same detector as `claims.NotDoneLastLine`), the Stop step allows, records `decision: abandon` with the unmet ids, and does not count a block. A `NOT-DONE` is an honest non-completion and never a false done; the gate exists to stop false `DONE`, not to hold an agent that has said it cannot finish. Same for an `ABANDON:` statement in the contract, which the status report already treats as a terminal state.
3. **The block message names the way out.** The first block of a session (and `saga gate status` whenever a gate is unmet) appends one fixed sentence, counted against the Stop ceiling: "If the task cannot be completed as specified, say why and end your message with NOT-DONE, or add `ABANDON: <gate id> <reason>` to .saga/contract.md." `saga gate -h` lists the ABANDON grammar in one line. The sentence is fixed text hashed into the arm's disclosure so it is the same in every arm B run.
4. **Byte-code and build caches are not edits.** G-SCOPE and the bench's `task.Diff` and scope scan ignore paths matching a built-in list (`__pycache__/`, `*.pyc`, `.pytest_cache/`, `.mypy_cache/`, `.ruff_cache/`, `node_modules/`, `.saga-oracle*`, `.oracle-run/`) and anything the workspace's `.gitignore` ignores. The gate reads the list from code, not from `config.toml` (which is self-serving per gate-spec §5.4).
5. **The ABANDON reason is the last substantive text of the final turn.** When the final message is only the marker (fewer than 20 non-marker characters), the classifier takes the last assistant text block of 20 or more characters in the same session from the harness stream, and `abandon.source` says `final_turn_text`. Decision 2 makes this the exception, not the rule.
6. **Cost is the harness's figure.** `run.json.cost_usd` is `harness_cost_usd` when the harness reports one (bench-spec §10.1, docs/12 §9 cost row); the pinned-table figure moves to `cost_usd_pinned` with `cost_ratio_pinned`, and the report prints the median ratio in the setup section. The 1.5 ratio is recorded in the smoke notes and the price table is left for a separate reconciliation.
7. **A schema failure is an infra outcome, never an overwritten reason.** `writeArchive` and the disclosure validation fail the run to `infra` with `schema: <error>` when the row or the disclosure does not validate; a test drives an unclassified ABANDON through the runner and asserts no schema error. `abandon.classes` becomes `[]`, never null.

## 3. Changes

### Part 1, gate (`internal/gate/layer.go`, `status.go`, `internal/harness/claude/claude.go`, `internal/cli` gate help)

- Release per decision 1; `observed` gains `released bool`.
- `NOT-DONE` and `ABANDON:` terminals per decision 2, before the block counter.
- The fixed sentence per decision 3 in the first block message and in status output; the ABANDON line in `saga gate -h`.
- Cache ignore list per decision 4 in the scope guard's changed-file listing (respecting `.gitignore` via `git check-ignore` or the equivalent in code).
- Tests: a Stop sequence that reaches `max_blocks` yields one release with empty `additionalContext` and `released: true`, and the next Stop allows without counting; a `NOT-DONE` last line allows with `decision: abandon` on an unmet contract; the sentence appears exactly once per session; `.pyc` files under `__pycache__` produce no G-SCOPE finding while a real out-of-scope edit still does; `Render` for Stop with a release produces `{}`.

### Part 2, bench (`internal/bench/task/stage.go`, `scan.go`, `internal/bench/adapter/abandon.go`, `claude.go`, `internal/bench/run/run.go`, schemas)

- Decisions 4 (Diff and scope scan), 5, 6, 7.
- Tests: a workspace with `.pyc` files yields an empty diff and no scope violation; a stream whose final message is `NOT-DONE` after a handoff paragraph classes as `contradiction` with source `final_turn_text`; cost fields on a synthetic run; the schema-failure test of decision 7.
- Re-derive the two arm B py-0007 rows offline with the new classifier (throwaway, report the result).

### Part 3, docs

- `docs/specs/gate-spec.md` §7 Stop table rows for release and the terminals, §2 the ABANDON hint sentence; `docs/specs/harness-facts.md` is mine (I add the C11 consequence row from this evidence); `docs/specs/bench-spec.md` §5.7 scope rule, §10.1 cost source, §3.4 `cost_usd_pinned`; `docs/specs/IMPLEMENTATION-STATUS.md`; `bench/results/smoke-2026-09-06-2/NOTES.md` gets a "fixed in" line per finding (the notes' analysis is already written by saga).
- Do not edit docs/12; the amendments are written.

## 4. Out of scope

Changing `max_blocks`, the claim decision table, `claims.txt`, the price table's values, containers.

## 5. Report

Hashes; the test names for each decision; the offline re-derivation of B py-0007 runs 1 and 2; anything left out. Under 50 lines.
