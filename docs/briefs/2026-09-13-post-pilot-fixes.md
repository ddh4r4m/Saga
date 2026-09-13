# Brief: the bench fixes left open after the pilot

Date: 2026-09-13. Owner of the decision: saga (Fable). Implementer: saga-opus. Four parts, one commit each, in this order; report after each. The experiment is closed (docs/12 §13, b085526), so nothing here re-grades, regenerates or edits any archive under `bench/results/`; every change applies to future runs only and says so in its docs line, dated. Go changes move the binary; that is fine now (no live run is planned; the owner re-approves before the next one). Diagnose before fixing where the brief says so.

## Part 1. The approval store must not reach the bare arm (bench-spec 4.2)

Dev-2 finding 3 (`bench/results/dev-2026-09-13-1/NOTES.md`): `RunArms` shares one `*ClaudeCode` adapter across arms, `stageGateFiles` sets `c.CorpusStore` on it, and `Env()` emits `SAGA_APPROVAL_DIR` whenever it is set, so every arm A run after the first arm B Prepare carries the variable. Fix: the corpus store is per Prepare (carried on the `PrepareInput` or a per-arm adapter clone), never on shared adapter state; the bare arm's environment has no `SAGA_APPROVAL_DIR` and no other gate-arm variable. Tests: `TestBareArmEnvHasNoGateVariables` (run arm B then arm A under the stub harness, assert arm A's harness.json `env_vars` lacks `SAGA_APPROVAL_DIR`) and a check in the existing control-arm blocking test that asserts the whole bare env against an allowlist. Docs: bench-spec 4.2 one sentence; IMPLEMENTATION-STATUS.

## Part 2. The three report defects (`bench/results/pilot-2026-09-13/NOTES.md`, "Report defects")

1. §4 secondary table: a column is computed over the runs that carry a verdict, with the count of runs lacking one printed beside it (`0.067 (99 of 100 runs carry a verdict)`), never nulled by a single missing verdict; the primary already does this. 2. §9 threats row "control arm reaches the component": read the manifest's control blocks and the arm A `blocked_reach_attempts` total, and print them; the text "no component blocks are configured" appears only when the manifest lists none. 3. §6 `component_unused`: evaluate it from the rows, a treatment run is exposed when its hook-trace carries at least one gate Stop event (or a gate `check` event), print the exposed share per treatment arm, and keep the "not evaluated" wording only for a runner that records no hook trace. Tests for each on fixtures; the report's golden files updated. Also the runner-side report of the void archive rule: nothing. Docs: bench-spec §5 report section, one line each.

## Part 3. `task_globs` puts py-0020 in both batches

`scripts/bench-common.sh`: `*-002?-*` matches `0020`; `21-40` selects 21 tasks. Fix the globs so `1-20` and `21-40` partition the forty exactly, with a test that counts both selections against `bench/tasks/` and asserts 20 and 20 with no overlap. Scripts only.

## Part 4. The three claim-verifier contradiction shapes (diagnose first)

Pilot NOTES table: on arm A oracle-pass runs, `tests_pass` contradicted with `status fail` 6, `ran` contradicted with `not_executed` 9, `touched` contradicted with `not_in_diff` 5; arm B 1 (`touched`). Re-derive every one of the 20 offline from the archived rows (the claim text, the trace events the verifier matched, and what it should have matched), group them by cause, and message me a table (row, claim, verdict, cause, proposed rule) before changing anything. For the `tests_pass` shape my earlier reading stands: the sentence refers to the last same-family command, so a later crashing invocation must not contradict a claim about an earlier green one unless the claim names it; propose the rule precisely. For `ran` and `touched` I have no reading yet; find the cause. Then, after my ruling, implement with fixtures taken from these rows (masked), tests, `docs/specs/claims-spec` (or wherever the decision table lives) updated with dated "post-experiment" entries, and a corpus precision re-check on the labelled corpus. The archived verdicts stay as they are; the fix applies forward.

## Report

Per part: hash; test names; for part 4 the diagnosis table first, then the hash. Under 20 lines each.
