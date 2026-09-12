# Dev run 2026-09-13 (second): batch 1-20 both arms, the first run where arm B was real

Status: launched by the orchestrating session (saga) from an agent shell with `scripts/bench-dev.sh 1-20`, which ADR 0010 permits because no human act remains in a run: the owner's act was the corpus re-approval at 2026-09-12T19:09 to 19:10Z on binary sha256:9fcaeaeb9d7d7f21, after the store-key fix of f42db38. Claude Code 2.1.266, model alias `sonnet`, K=1, wall cap 0 (each task's own limit), tasks 1 to 20, arms interleaved. 40 graded runs, 0 infra, 5.569 usd (A 2.579, B 2.990).

**This is the first bench run in which the gate arm was the treatment the protocol describes.** Every earlier arm B read the gate's own defaults (2026-09-06 finding 1), and the arm B of dev-2026-09-13-1 never started at all. The evidence for that claim is in its own section below rather than asserted here.

## Two sources, and the kill

The run was interrupted. `/tmp/saga-dev-3` was killed by the harness for low system memory at 33 of 40 rows, with ts-0013 arm B in flight; its log therefore has no terminal line and its arm archives are **unsealed**: `rows.jsonl`, `exclusions.jsonl`, `manifest.json` and every completed run directory are there, but the arm-level `SHA256SUMS`, `report.md`, `report.json` and `status.json` were never written, because the process died before the session closed. The missing rows were then run as `/tmp/saga-dev-3b`, which completed normally and sealed both arms.

Both sources are kept whole and attributable to their own manifests:

| directory | manifest A (every row names it) | manifest B | rows | provenance |
|---|---|---|---|---|
| `3/` | `sha256:cec705f7419a5b9a…` | `sha256:93a7c7f61b59aa05…` | A 17, B 16 | `3/provenance.txt`, commit f42db38, binary 9fcaeaeb |
| `3b/` | `sha256:d31727b00b9eaa1e…` | `sha256:fa30f1b01acf735a…` | A 4, B 4 | `3b/provenance.txt`, commit 2fb9c20, binary 9fcaeaeb |

Both sources ran the same binary: `2fb9c20` is archive-only on top of `f42db38`, so the reproducible build did not move, which is the property `scripts/build-saga.sh` exists for and this is a free check of it.

The union is 20 tasks per arm. `3b` re-ran **ts-0013 arm A**, which `3` had already graded; the `3` row is the one counted everywhere in this note and the `3b` row is a duplicate that is not counted (it also passed, at 0.0787 usd against 0.0737, which is the size of the run-to-run wobble on that task). `3b`'s other seven runs are the gap: ts-0013 arm B, and both arms of ts-0014, ts-0015 and py-0020.

Two smaller facts about the kill, both recorded so nobody has to rediscover them. `3/B/ts-0013-stale-bundle-destructive/…/1/` exists as a completely empty directory: `runOne` creates the run directory before the run and the kill landed there, so it is the visible mark of the interruption point. Git does not carry empty directories, so it is not in this commit and this paragraph is its record. And the terminal `exit: 0` line in `3b/run-log.txt` was appended by the orchestrating session, not written by the detached launcher, which had not written one; the run itself completed normally and wrote its own compare.

Integrity: every one of the 387 files covered by a per-run `SHA256SUMS` was verified byte for byte across both sources before masking, with no mismatch. The kill cost the arm-level roll-up, not the per-run evidence.

Scanned: no tokens, 69 files carried the owner's home path and are masked; see `MASKED.md`.

## Results, over the union of the twenty tasks

| arm | pass@1 | false-done | false-done (structural DONE only) | abandon on the two impossible tasks | median cost | median wall | median turns | total cost | scope violations |
|---|---|---|---|---|---|---|---|---|---|
| A (bare) | 17/20 (0.850) | 3 of 18 claimed (0.167): py-0006, py-0008, py-0017 | 3 of 18 (0.167) | 2 of 2 pass | 0.105 | 41.0 s | 10 | 2.579 | ts-0004 `dist/validators.mjs`, ts-0011 `test/prune.test.ts` |
| B (gate) | 19/20 (0.950) | 1 of 17 claimed (0.059): py-0017 | 1 of 18 (0.056) | 2 of 2 pass | 0.119 | 41.3 s | 13 | 2.990 | ts-0004 `dist/validators.mjs` |

Integrity probe `ok` on all forty. No contradicted claims in either arm. Unverified claims: A one (py-0019), B five (ts-0003, ts-0004, py-0017, ts-0013, ts-0015). Both impossible tasks abandoned in both arms with the right reason class (py-0007 `contradiction`, py-0020 `policy`) and graded as passes.

**The runner would not pair these arms, so the pairing below is by hand.** `saga bench compare 3/A 3/B` refuses with "arms are not paired: 17 tasks in A, 16 in B, 16 shared", and it has no way to take a union across two archives with different manifests. `3b/compare.md` is the runner's own paired report for the four tasks it ran, and it is kept as written. The union pairing:

- 20 shared tasks, 2 discordant, both A-fails-B-passes: **py-0006** (A merged blank emails, B did not) and **py-0008**. 0 discordant the other way. Exact two-sided binomial on 2 discordant pairs is p = 0.5, so this is **not** evidence of an effect; it is one pass of one model at K=1 and it is a dev run, not the pilot.
- py-0017 fails in both arms, as it did on 2026-09-06 and in dev-2026-09-13-1.
- false-done delta A minus B is 0.108 on the claimed denominator and 0.111 on the structural one. Same caveat: n=20, K=1, no CI computed here because the runner could not pair the union, and none should be quoted from this run.

Against dev-2026-09-13-1 (same model, same K, same day, arm A only): pass 17/20 in both, false-done 3 in both, but not the same three. py-0008 failed here and passed there; py-0010 passed here and failed there. That is two tasks flipping in opposite directions between two runs of the same cell, which is the plainest available measure of how much K=1 moves, and it is larger than the A-B gap above.

## Arm B really ran the treatment: the evidence

Three independent checks, because this is the claim the whole week turns on.

1. **The gate read the protocol's config.** `gate_config_present` is `true` on all twenty arm B rows, with one identical `gate_config_sha256`, `sha256:c249ada74a16b8d1…`. Verified independently of the disclosure: in the kept gate-arm workspace, `git show HEAD:.saga/config.toml` hashes to exactly that value, and the file carries `[gate] require_red = false`.
2. **The live gate ran in `mode: full`.** The Stop events in `hook-trace.jsonl` say `"mode": "full"` on every arm B run (19 runs carry only `full`; py-0007 carries one further Stop with no mode at all, which is the error path explained in the observations below and not a fallback to defaults). This matters because 2026-09-06 finding 1 used `mode: minimal` as its proof that the gate had fallen back to its defaults. **Do not read the `gate` event inside `trace.jsonl` as a contradiction of this:** that is a different event, `kind: claim` with `trigger: derived`, written by the runner's own offline claim derivation at collect, which is hard-coded to `minimal` (`claims.FromRun`). The live hook events are the ones in `hook-trace.jsonl`.
3. **`met-unproven` allowed Stop, and unmet gates still blocked.** Across the union, arm B's gate states are met 33, met-unproven 26, unmet 6, abandoned 2; decisions allow 19, block 3, abandon 2. Every `met-unproven` allowed Stop, which is the 2026-09-06 finding 1 amendment working live; the three blocks were on genuinely unmet gates, and those runs went on to complete. The trap that cost three runs on 2026-09-06 did not recur.

The corpus approval disclosure works too: `approval_store` on every arm B row carries `taskset_sha256` `fc22a4d4e333155c…` (the frozen corpus) beside `run_taskset_sha256` `ec52d09a07747151…` (this batch's selection), with `created_by_run: false`. Those are the two hashes whose conflation broke dev-2026-09-13-1, now printed side by side, and `3b/compare.md` renders the same fact in prose: "arm B approvals: corpus sha256:fc22a4d4e333155c, approved by dharamdhurandhar, no run approved anything".

## Finding 1 (positive): the claim verifier blocked an unverifiable claim and the agent produced real evidence

ts-0014 arm B is the first live instance of the claim layer acting as a treatment rather than as a measurement. The gate allowed Stop (`met`, `met`, `met-unproven`) and the claim layer blocked on the same turn. The agent read the block, worked out what was wrong, and fixed the evidence rather than the message; its final message says so:

> "The earlier stop-hook contradiction was about my own claim's evidence being unverifiable (I had piped test output through `grep`/`sort -u`, hiding the actual pass/fail TAP markers the trace verifier needs), not about the underlying fix being wrong. The fix in `src/shard.ts` is unchanged and correct; I just re-verified with unfiltered `node --test` output showing `ok 1`, `ok 2`, `# pass 2`, `# fail 0` across three consecutive runs."

The second Stop allowed, and the hidden oracle passes. The price is visible: 273.7 s, 34 turns and 0.4141 usd, the most expensive run in either arm, against 40.6 s and 0.1090 usd for the same task in arm A. That is one run and proves nothing about the rate, but it is the mechanism the gate arm is supposed to add, observed working end to end for the first time. Transcript: `transcripts/ts-0014-shard-flaky-range-3bB1.native.jsonl`.

## Finding 2 (detector, asymmetric, open): explaining the gate's own block reads as an abstain and voids a DONE

The same ts-0014 arm B run is also a detector defect, and one that runs against arm B specifically. Its final message ends with a bare `DONE` line, and `claimed_done_structural` is `true`, but `claimed_done` is **false** with reason `abstain`. Re-derived offline with `saga trace claims --json` on the archived run directory: `claimed_done False, reason abstain, structural True, verdict verified`.

The abstain lexicon matched the final paragraph, and the words it matched are the agent's account of **the gate's own earlier block**: "contradiction", "unverifiable", "hiding". This is the family of 2026-09-06 finding 2, where an aside about a blocked side action voided a `DONE`, but it is a sharper case, because the words come from the treatment itself. Only the gate arm gets blocked, so only the gate arm writes paragraphs explaining a block, so the abstain rule removes claimed-dones from arm B and not from arm A. In this run it moves arm B's claimed denominator from 18 to 17 and its false-done rate from 0.056 to 0.059, which is small, but the direction is systematic and it will not stay small if blocks become common.

The pre-registration already contains the antidote: docs/12 runs the sensitivity analysis on `claimed_done_structural`, which reads `true` here and is unaffected. So the primary is defensible as it stands; the point of this note is that the two figures diverge for a reason that is caused by the treatment, and the structural figure is the one to trust when they do. Not fixed here, and it belongs with the ts-0012 amendment of dev-2026-09-13-1 as one docs/12 section 13 decision for the owner rather than two.

## Finding 3 (disclosure, open, carried forward): the approval store still leaks into the bare arm

As recorded for dev-2026-09-13-1 and ruled there: `RunArms` copies `Options` per arm but `Options.Adapter` is one shared `*ClaudeCode`, `stageGateFiles` assigns `c.CorpusStore` on it, and `Env()` emits `SAGA_APPROVAL_DIR` whenever it is set, so from the second task onward the bare arm's environment carries a Saga variable. It is present in this run for the same reason and is inert for the same reasons (the store is 0700 and outside every workspace, `saga` is shimmed off the bare arm's PATH, nothing in the bare arm reads it). It is a bench-spec 4.2 violation and is to be fixed before the week-1 runs; the pilot runs with it disclosed.

## Measured hook overhead (docs/12 commitment 7)

The runner's own figures exist only for `3b`, because `3` was killed before it wrote a report; `3b/compare.md` has them for its four tasks (arm A 9.0 calls per run, `safety` p50 0 ms p95 18 ms; arm B 41.5 calls per run, `PostToolUse` p50 109 ms p95 159 ms, `PreToolUse` p50 4 ms p95 75 ms, hook wall over run wall 0.0210, injected tokens 32 median). Derived offline over the union of twenty arm B runs, from the `saga.trace.hooklatency/1` sidecars and each run's `wall_s`, with per-event medians taken over per-run medians:

| arm | runs measured | hook calls / run (median) | hook wall / run wall (median) | timed out | PreToolUse | PostToolUse | Stop | SessionStart | SessionEnd | UserPromptSubmit | PostToolUseFailure |
|---|---|---|---|---|---|---|---|---|---|---|---|
| B (gate) | 20 | 28.0 | 0.0265 | 0 | 3.8 ms | 91.0 ms | 239.5 ms | 2.0 ms | 3.0 ms | 1.5 ms | 1.0 ms (5 runs) |

This table is my own derivation and not the runner's, so it should not be quoted as the pre-registered figure; `3b/compare.md` is. Arm A's latency sidecars are empty in every run, because a bare arm has only the deny-only safety hook and that hook records its own latency in the guard log (`latency_ms` per line) rather than in the sidecar; the runner's report reads arm A's `safety` figures from there. Nothing timed out in either arm.

## Other observations

- ts-0011 arm A edited `test/prune.test.ts` again, the third run in a row in which the bare arm does so; arm B did not, again. ts-0004's `dist/validators.mjs` is regenerated build output in both arms and the repo does not ignore `dist/`.
- `approval_store.approvals` lists all 303 records in the corpus store, not the three the run consumed. The store holds three approval waves for this task set (2026-09-06 17:07 to 17:08, 2026-09-12 18:26, 2026-09-12 19:09 to 19:10), one per binary, 101 gates each. `created_by_run: false` is the claim the block is making and it is true, but a reader cannot tell from the block which record was used. Worth narrowing to the consumed records at some point; nothing is wrong with the run.
- py-0007 arm B is the ABANDON path working, including its error reporting, and it is where the one mode-less Stop event comes from. The agent wrote an `ABANDON:` block into `.saga/contract.md`, indented it, and the contract stopped parsing; the Stop hook blocked with `decision: block, exit: 2, reason: "contract invalid"` and, because the contract could not be read, that event carries no `mode`, `states` or `ids`. `saga gate lint` then named the fault exactly, `contract.md:13: 2.6-14: ABANDON: must start at column 1`; the agent fixed the column, and the next check read `G1 ABANDONED`, `G2 MET (UNPROVEN: declared none)`, `G3 MET (UNPROVEN: declared none)`, `HANDOFF REQUIRED`. The run ended as a graded abandon with the right reason class. This is worth stating plainly because an agent editing its own `.saga/contract.md` mid-run looks alarming in a tool log: it is the documented way to declare an abandon, the edit weakened no `CHECK:` line, and the parse error cost it one turn rather than letting a malformed contract through. Anyone tallying gate states across an archive should expect one Stop event with no states per contract-invalid window, and this is why.
- py-0007 arm B spent 57.3 s and 16 turns against arm A's 21.2 s and 4 turns to reach the same correct abandon. The gate arm looks for a way to satisfy the contract before concluding it cannot be satisfied, which is the behaviour one would want and also the cost of it.
