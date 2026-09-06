# Brief: the remaining small readiness rows and a live-probe launcher for the owner

Date: 2026-09-06. Owner of the decision: saga (Fable). Implementer: saga-opus. Two commits: part 1 (rows 4, 14, 16 in Go), part 2 (`scripts/harness-probes.sh` and the doctor live probe).

## Part 1: three small rows

### Row 4 leftover: interleaving order written as a field

`run.RunArms` executes A₁ B₁ A₂ B₂ per task; the order is deterministic but not recorded. Add `sequence` (integer, 1-based position in the invocation's execution order across arms and tasks) to the run row (`internal/bench/run/row.go`, `schema/bench/1/run.json`, required) and `execution_order` to the manifest (`arms` already lists arm ids; add `interleaving = "per-task-alternating"` and the first eight `(task, arm, i)` triples as a sample is not needed: the sequence numbers reconstruct it). Test: a two-arm replay run yields sequences 1, 2, 3, 4 for A₁ B₁ A₂ B₂ of the first task. Close row 4 in docs/12 §10 yourself with the hash (this is the one docs/12 edit you may make; leave the wording "closed 2026-09-06" plus the hash).

### Row 14: provider retries

bench-spec §3.3 pre-registers "3 retries with backoff on 429/5xx; retries recorded; 4th failure → infra". Claude Code retries inside the harness and its stream-json carries no retry events in any of the twelve archived transcripts (subtypes seen: init, thinking_tokens, task_started, task_notification). Decision: the disclosure's `retries` block becomes `{policy: "harness-internal", count: null, count_reason: "claude-code retries inside the harness and does not emit retry events in stream-json (checked on 2.1.263)"}` for the claude-code adapter, and the runner classifies a `result` with `is_error` true and subtype naming an API failure (`error_during_execution` with an API error text, or any subtype beginning `error_` other than `error_max_turns` and `error_max_budget_usd`) as `infra` with `outcome_reason` carrying the subtype and the first 200 characters of the error. Test: a synthetic stream whose result is `{"type":"result","subtype":"error_during_execution","is_error":true,"result":"API Error: 529 overloaded"}` yields `infra`. Amend bench-spec §3.3's row to say the harness retries and the bench records the terminal failure only. Close row 14 with the hash (same permission as above).

### Row 16 leftover: the abandon lexicon hash joins the manifest

`internal/bench/adapter/abandon.go` `reasonPatterns` is the closed lexicon that classes an ABANDON. Serialise it canonically (class name, then each pattern source string, sorted within class, classes in the `ReasonClasses` order), hash it, expose `adapter.AbandonLexiconHash`, and write it to the manifest as `abandon_lexicon_sha256` beside `claims_list_sha256` and `abstain_list_sha256` (schema required). Test: the hash is stable across two processes (a golden value in the test, updated deliberately when the lexicon changes) and changes when a pattern is added. Close row 16 with the hash.

## Part 2: `scripts/harness-probes.sh` (owner-run, cents not dollars)

Readiness rows 9, 10 and 17 need one live Claude Code turn each on the pinned version. Write a launcher in the shape of `scripts/bench-smoke.sh` (same refusal under harness markers, same token requirement, same private `HOME` and `CLAUDE_CONFIG_DIR`, never `~/.claude`, `--model sonnet`, `--max-turns 4`, `--max-budget-usd 0.10` per probe) that runs three probes into `OUT` and prints one line per probe with `PASS`, `FAIL` or `INCONCLUSIVE` and the evidence path:

1. **P9, JSON honoured on a non-zero exit.** Register a PreToolUse hook script that prints `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"saga-probe-p9"}}` and exits 1, then prompt for one Bash call (`echo saga-probe`). PASS when the tool did not run and the transcript shows the deny reason; FAIL when the tool ran (fail-open on non-zero); record which. Then the same with exit 2 and plain-text stderr, the documented block path, as the control. Write the finding as a proposed row for `docs/specs/harness-facts.md` in `OUT/harness-facts-proposed.md`; do not edit harness-facts yourself, I will.
2. **Doctor hooks fire (row 10).** `saga install` into the private config, then one `claude -p` turn with a single Bash call, then `saga doctor --json` in that workspace must report the session's `tool_call` and `tool_result` events and the Stop event with the claim verdict; add a `--live-session <id>` flag to doctor if it has no way to name the session, or read the latest session, your call, documented. PASS when all three events are present within 10 s of the turn's end. Also run `saga uninstall --dry-run` and diff its listing against what `install` wrote; PASS when identical (implement `uninstall --dry-run` if it is missing; it is listed as a gap).
3. **P9b, PostToolUseFailure live payload (row 17).** With the composed hook installed, prompt for a Bash call that fails (`false`, then `exit 3`), capture the raw PostToolUseFailure stdin the hook received into `OUT/posttoolusefailure.json` (redact nothing but the token env, which the hook never sees anyway), and compare its fields with harness-facts C34; PASS when the field set matches. Then replace `fixtures/trace/` synthesised payload with the captured one in a follow-up commit after I have read it: for this brief, write the capture only.

Also print the pinned version and binary hash at the top of the output like the smoke launcher does. Dry-run the script logic under a stub `claude` (a shell script that emits a minimal stream-json and calls the hooks the way the harness does) in a Go test or a bash test so the launcher is known to work before the owner spends a cent; the stub lives under `scripts/testdata/`.

## Docs, same commits

`docs/specs/IMPLEMENTATION-STATUS.md` lines for rows 4, 14, 16 and the launcher; `docs/specs/bench-spec.md` §3.3; `scripts/README` line if one exists (else a header comment).

## Out of scope

Editing `docs/specs/harness-facts.md` (I do it from the probe output), containers, tasks, anything under `internal/gate`.

## Report

Both hashes; the three tests' key values; the launcher's dry-run output; anything left out. Under 40 lines.
