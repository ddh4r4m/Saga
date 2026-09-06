# Brief: a canonical bench PATH, so corpus approvals do not depend on the approver's shell

Date: 2026-09-06. Owner of the decision: saga (Fable). Implementer: saga-opus. One commit.

## 1. Problem

The owner ran `scripts/bench-approve.sh` at 7d80087: 101 records in `~/.saga/bench/approved/fc22a4d4…/` for binary a54e45f8 (the orchestrator's own build of the tree hashes identically, so the reproducible build holds). `saga bench approve-corpus --check` from the orchestrator's shell then reported `covered 0 of 40` for the same binary and task set. The approval identity (gate-spec §8) hashes the full inherited `PATH`; ADR 0010 stabilised only the bench's own entry, and the rest of `PATH` differs between the owner's terminal and any other shell, so every identity differs. The approvals are therefore bound to the shell that approved, which defeats the ADR's purpose.

## 2. Decision (taken)

The bench composes one canonical `PATH` and uses it both for the approval identity at `approve-corpus` and for the agent's environment at Prepare, so the identity depends on the binary, the task and the tool locations, never on the caller's shell:

`PATH = <bench bin dir for this binary> : <dir of node> : <dir of python3> : /usr/bin : /bin : /usr/sbin : /sbin`

where the `node` and `python3` directories are resolved with `exec.LookPath` at approve time and again at Prepare, deduplicated, in that order. The control arm prepends its shim directory to the same list (the shim never enters an identity). The resolved list is recorded in `harness.json` (`bench_path`) and printed by `approve-corpus` (`path: …`) so a mismatch is visible. `claude` itself is found by the launcher, not by the agent's `PATH`; the launcher passes its absolute path as today.

If `node` or `python3` resolve to different directories at Prepare than at approval (a toolchain change), the identities differ, the run is `infra` with `not pre-approved`, and the message names the path difference; that is the correct failure.

## 3. Tests

`TestBenchPathIsCanonical` (two shells with different inherited `PATH` values compose the same bench `PATH`); `TestApprovalIdentityIgnoresTheCallersShell` (approve with one inherited `PATH` into a temp store, check with another, covered); `TestToolchainMoveInvalidatesApproval` (a fake `node` in a different directory at check time reads not approved with the path named). The launcher's `--check` leg and the probes launcher use the same composition.

## 4. Docs, same commit

`docs/specs/bench-spec.md` §3.2 (the canonical `PATH`), ADR 0010 gets a one-line addendum under Consequences (saga writes it; you may add the sentence "PATH is composed by the bench, not inherited; see brief 2026-09-06-canonical-bench-path" if that is simpler), `docs/specs/IMPLEMENTATION-STATUS.md`.

## 5. Report

Hash; the canonical `PATH` printed by `approve-corpus --check` on this machine; test names. Under 15 lines. The owner re-approves once after this commit.
