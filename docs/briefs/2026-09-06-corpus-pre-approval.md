# Brief: corpus pre-approval (ADR 0010): the owner approves the frozen task set once, runs need no human act

Date: 2026-09-06. Owner of the decision: saga (Fable), ADR 0010 accepted. Implementer: saga-opus. Two commits: part 1 (stable bench PATH, corpus store, `approve-corpus`, Prepare consumes only), part 2 (launcher, disclosure, docs).

## 1. Changes

### Stable bench PATH (ADR decision 1)

`internal/bench/adapter/claude.go`: the gate arm's `saga` link moves from `<config-dir>/bin/saga` to `~/.saga/bench/bin/<first 16 hex of the binary's sha256>/saga` (created `0755`, the link replaced atomically when absent or pointing elsewhere; `SAGA_HOME` or the existing home override, if one exists, keeps tests off the real home). `Env()` puts that directory first on `PATH`. The control arm's shim stays where it is. Test: two Prepare calls with the same binary yield the same `PATH`; a different binary yields a different directory.

### Corpus approval store and `saga bench approve-corpus` (decision 2)

`~/.saga/bench/approved/<taskset-sha256>/` (from `bench/tasks/TASKSET.sha256`'s `set` line; refuse when the file is absent or the set does not match the tasks given). `saga bench approve-corpus <tasks-glob> [--saga-bin <path>]`: refuses under an agent shell with the same check and message as `gate check --approve`; for each task stages a temporary workspace exactly as Prepare does (call the same function), runs `saga gate check --approve --json` with `SAGA_APPROVAL_DIR` at the store and the stable `PATH`, prints `<task> approved <n> gates` or the failure, deletes the workspace, and ends with `approved <k> of <n> tasks for binary <hash>, task set <hash>`. Exit 0 only when every task approved. Test: with a fake approve-capable saga (the pattern in `claude_test.go`), the store fills with one record per gate per task, and re-running is idempotent.

### Prepare consumes, never creates (decision 3)

`stageGate` points `SAGA_APPROVAL_DIR` at the corpus store for the task set and runs `saga gate check --json` (no `--approve`). Exit 4 (approval required) becomes `infra` with `outcome_reason` `not pre-approved: <gate ids>`; nothing else changes in the check's handling. Delete the per-run `approvedDir` creation. Test: Prepare against an empty store yields infra with that reason; against a filled store proceeds to the run.

### Launcher (decision 4)

`scripts/bench-smoke.sh`: before the agent-marker refusal, run `saga bench approve-corpus --check <tasks>` (a read-only mode that reports which tasks lack a record for the current binary without approving); refuse under an agent marker only when the check reports a missing record, with the message naming `saga bench approve-corpus`; with every task covered, proceed from any shell. Keep the token requirement. Update the stub-based test in `scripts/probes_test.go` or add one for the launcher's new branch.

### Disclosure (decision 5)

`harness.json`: `approval_store` (`{kind: "corpus", taskset_sha256, dir_sha256}`) and per gate `approved_by` and `approved_at` from the records read at Prepare; the report's setup section prints "approvals: corpus <hash>, no run approved anything" or the deviation.

## 2. Docs, same commits

`docs/specs/gate-spec.md` §8: one paragraph on corpus approval as a use of the same identity (nothing in the identity changes); `docs/specs/bench-spec.md` §3.2 Prepare and §4.2; `docs/specs/IMPLEMENTATION-STATUS.md`; `scripts/bench-smoke.sh` header comment. Do not edit docs/12 (the amendment is written) or the ADR.

## 3. Out of scope

Any change to the approval identity, to `gate approve`, or to the agent-shell check itself; CI mode (`--ci`).

## 4. Report

Hashes; test names; the `approve-corpus` output shape; the launcher's three-way behaviour (approved from an agent shell, unapproved from an agent shell, from a plain shell); anything left out. Under 40 lines.
