# Brief: corpus approvals pass `--check` but are not found at Prepare

Date: 2026-09-13. Owner of the decision: saga (Fable). Implementer: saga-opus. One commit. Diagnose first and report the cause before fixing.

## 1. Problem

First live consumption of a corpus approval (ADR 0010) failed. The owner approved at HEAD 069af89 from a plain terminal: `approved 40 of 40 tasks for binary sha256:6d2defeaaf108160, task set sha256:fc22a4d4e333155c` (101 records in `~/.saga/bench/approved/fc22a4d4…/`, `approved_at` 2026-09-12T18:26Z). The dev launcher's preflight (`approve-corpus --check --saga-bin <same hash>`) then reported all covered and the run started. Every arm B run ended `infra` with `not pre-approved: <gate>:G1 <gate>:G2 …` (all gates of every task); arm A ran normally (20 runs, 2.73 usd). Evidence: `/tmp/saga-dev-2/` (provenance.txt, run.log, archive/B/*/sonnet/claude-code/B/1/run.json, archive/B/exclusions.jsonl). Nothing was spent in arm B.

So `--check` and Prepare's `baselineCheck` (`saga gate check --json` in the workspace, env from `c.Env(in.ConfigDir)`) compute different approval identities, or look in different stores. `ApprovalIdentity` (internal/gate/approval.go:97) hashes contract path, gate id, oracle hash, platform, `PATH` and witness hash; the consumer is internal/gate/check.go:191 with `os.Getenv("PATH")`.

## 2. Suspects, in the order to test

1. `PATH` at Prepare is not the canonical composed one that approve-corpus used (the control shim or the per-run config dir leaking in, or `c.Env` composing after the check leg was written). Print both.
2. Witness hash: `Witnesses(root, g)` over the staged workspace may include a file that differs between the approve-time staging and the run-time staging (base commit hash or timestamp in a file, `.saga/` contents, `request.md`, cache paths).
3. Oracle hash: the per-run randomised integrity probe or runner files being staged before the check at Prepare but not at approve time (or vice versa).
4. Store: `SAGA_APPROVAL_DIR` not set to the corpus store in `c.Env`, or `approvalStoreConsistent` (check.go:51) rejecting it (exit 6 vs 4; the runner maps exit 4 only).
5. Contract path constant vs an absolute path.

Reproduce without the model: stage one task the way Prepare does (there is a stub harness under `scripts/testdata`), run `gate check --json` with the exact env, and diff the identity inputs against approve-corpus's for the same task. The fix must make `--check` and Prepare use one function for the identity inputs so they cannot diverge again.

## 3. Tests

A test that approves a task into a temp store through `approve-corpus`, then runs the adapter's Prepare (stub harness) against that store and asserts the baseline check exits 1 or 5, never 4. If any suspect above was the cause, a test that pins it (for example the witness set at approve time equals the witness set at Prepare).

## 4. Docs, same commit

`docs/specs/IMPLEMENTATION-STATUS.md`; one line under ADR 0010 Consequences naming the cause; `docs/12-experiment-protocol.md` §13 amendment only if the cause changes what an approval means.

## 5. Report

Cause in two sentences; hash; whether the binary hash changed (if the fix touches the binary, it will, and the owner must re-approve; say so); test names. Under 15 lines. Do not run any live bench.
