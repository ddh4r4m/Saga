# ADR 0010: The owner approves the frozen corpus once; runs need no human act

**Status:** accepted, 2026-09-06

## Context

gate-spec §8 makes approval a human act: `saga gate check --approve` records consent for a gate's `CHECK:` line under an identity that hashes the contract path, gate id, oracle hash, platform, the inherited `PATH` and the witness hash, and the command refuses under an agent shell (environment markers and a walk of the parent-process chain). The bench's Prepare step runs that command in every fresh workspace with a per-run approval store, so every bench run needs the owner at a terminal. That is why `scripts/bench-smoke.sh` refuses under an agent marker, why the four smokes, the probe runs and the dev run of 2026-09-06 were all launched by hand, and why the orchestrating session cannot run the pilot.

The property worth keeping is that an agent never approves its own baselines. The property worth dropping is that consent is re-given for the same contract, the same check, the same binary and the same platform on every run: the approval identity already contains nothing workspace-specific except `PATH`, and `PATH` varies per run only because the bench puts a per-run directory on it.

## Decision

1. **Stable bench PATH.** The bench's `saga` link for a gate arm lives at a path derived from the binary's hash (`~/.saga/bench/bin/<sha256-16>/saga`), not under the per-run config directory, so the identity's `PATH` component is the same for every run of the same binary and changes when the binary does. The control arm's shim stays per run; it never enters an approval identity.
2. **Corpus approval store.** `~/.saga/bench/approved/<taskset-sha256>/` holds the approval records for a frozen task set. `saga bench approve-corpus <tasks-glob>` (a human act, refused under an agent shell exactly as `check --approve` is) stages each task the way Prepare does (base commit carrying config and contract), runs `saga gate check --approve` against that store with the stable `PATH`, and prints one line per task and a final count. It is re-run when the task set or the binary changes; the store is keyed so that stale records are simply not found.
3. **Prepare consumes, never creates.** A gate-arm run points `SAGA_APPROVAL_DIR` at the corpus store and runs `saga gate check` without `--approve`. A missing record is `infra` with reason `not pre-approved: <gate>`; the runner never approves. The agent's own `saga gate approve` stays agent-forbidden and the store is `0700`, outside every workspace.
4. **The launcher's refusal narrows.** `scripts/bench-smoke.sh` refuses under an agent marker only when a task in the set lacks a corpus approval for the current binary; with every task approved it runs from any shell, because no human act remains in the run. The pre-registration's rule that live runs are the owner's act is amended to: the owner's act is the corpus approval and the token.
5. **Disclosure.** `harness.json` records `approval_store` (corpus, with the task-set hash) and `approved_by` per gate from the records, so a report can show that no run approved anything.

## Consequences

- The orchestrating session, or any automation holding the owner's token, can run smokes, dev runs and the pilot without the owner present; the owner's consent is given once per freeze and once per binary, which is what consent for `CHECK:` lines means.
- A binary change silently invalidates the corpus approvals (by construction) and the next run says so as `infra`, never as a graded outcome.
- The four smokes and the dev run of 2026-09-06 keep their per-run approvals in their archives; nothing is re-graded.
- Rejected alternative: reading approvals from the working tree or skipping approval in the bench. Both remove the consent property for every user to buy convenience for one; see the ruling of 2026-09-06 on reading gate config from the working tree, rejected for the same reason.
