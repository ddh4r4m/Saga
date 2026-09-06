# Week 1 status, written the night of 2026-09-06

For the owner, to read in the morning. Everything here is on `main`, nothing is pushed, the tag `prereg-v1` sits on b83390b. 118 commits on the branch, 68 of them today, 18 briefs under `docs/briefs/`, 10 ADRs.

## 1. What is established

The eight-week experiment of ADR 0009 can start. Every readiness row of docs/12 §10 is closed on evidence except the ones this section names as yours. Concretely:

- **A frozen corpus of 40 tasks** (20 TypeScript, 20 Python; 8 impossible, 7 hack-bait, 25 plain), every one adversarially reviewed, with the set hash in `bench/tasks/TASKSET.sha256` (fc22a4d4…) and a run refusing a changed task.
- **Grading that cannot be forged from inside the workspace.** The review found that every Python task could be marked solved with no work by a package init that silences `unittest`, and that both older impossible tasks were passable by an object whose equality always answers true. Both are closed with controls that keep them closed, plus a per-run randomised integrity probe in the grader for both languages.
- **A claim verifier that reads the same events in both arms** (from the harness's own stream, never from hooks), with corpus precision 1.00 on every claim kind and a false contradiction rate of 0.000 on the forty dev-run rows after five detector fixes found live.
- **A gate arm that runs as pre-registered.** This took the whole day. In order: the request hash detached every contract (smoke 2); the Stop step held an honest NOT-DONE and its release continued the conversation (smoke 3); an unproven gate blocked Stop (dev run); and beneath all of it, arm B's staged config never reached the gate because the gate reads config only at the base commit and `.saga` was gitignored, so every arm B number before bc02a72 ran on the gate's defaults. Fixed, with two proofs in every archive that the config was read.
- **Cost, overhead and reconciliation.** Cost is the harness's own figure; the Sonnet 5 price row is fitted to it exactly; the arm B ledger reconciles token for token; hook overhead is measured per invocation and printed beside the primary (arm B about 3 percent of run wall, 32 injected tokens per run).
- **Harness facts closed live** on Claude Code 2.1.263: a JSON deny is honoured on a non-zero hook exit, a deny reaches the model as an error tool result, PostToolUseFailure's real payload is captured, hooks fire and doctor sees them, uninstall matches install, and `additionalContext` on Stop continues the conversation.
- **Automation.** ADR 0010: you approve the frozen corpus once; runs consume approvals and never create one; the launcher runs from any shell once every task is approved; the build is reproducible and the bench composes its own `PATH`, so the approval belongs to the binary and this machine's toolchain, not to a shell.

## 2. The numbers that exist

All Sonnet, all K at most 2, none of them the pilot. Each directory under `bench/results/` has notes and its manifests.

| run | arms | runs | usd | what it showed |
|---|---|---|---|---|
| smoke-2026-09-05 | 3 tasks, K=2 | 9 | 2.30 | first live run; the RED: none hold |
| smoke-2026-09-06 | 3 tasks, K=2 | 12 | 0.71 | arm B excluded at Prepare (request hash); the verifier blind in arm A |
| smoke-2026-09-06-2 | 3 tasks, K=2 | 12 | 2.40 | arm B completes; the held abandon and the release loop |
| smoke-2026-09-06-3 | 3 tasks, K=2 | 12 | 1.02 | gate arm matches bare on every task; first registered archive |
| dev-2026-09-06-1 | 20 tasks, K=1 | 40 | 5.58 | 17 new tasks live; three arm B timeouts (the base-config defect); false-done A 2, B 1 |

What can be said from the dev run, with the caveat that arm B ran on the gate's defaults: on the fourteen tasks both arms completed, both passed thirteen and failed the same one; bare produced two false dones and the gate arm one; the gate arm cost 0.044 usd more per run at the median (p = 0.03 on 20 pairs) and 11 s more wall. Nothing here is a result; it is the shape the pilot will measure properly.

## 3. What is yours

1. **Re-approve the corpus once** at the commit you find as HEAD in the morning: `scripts/bench-approve.sh`. Your first approval was bound to your terminal's `PATH` and to an older binary; both are fixed.
2. **Say go.** I then run, from this session: the twenty-task dev run under the staged config with the tasks' own wall limits (about 6 usd, 30 minutes), ingest it, and if it is clean, the pilot as pre-registered (`scripts/bench-pilot.sh`: 20 tasks, K=5, Opus 5, about 200 runs; the wrapper prints the runner's estimate, 48.50 usd for the cell, and refuses without a budget you set; expect 50 to 100 usd and two to three hours).
3. **The docs/10 decisions that are still open:** the budget ceiling for the eight weeks, when to push the repository (nothing has left this machine), and name and licence. None blocks the pilot.

## 4. What I would not do yet

- Run tasks 21 to 40 live before the pilot. They are frozen and reviewed, and the protocol runs them in week 6; spending on them now buys nothing the pilot needs.
- Push. The first artefact worth publishing is the pilot report generated once from its rows, per commitment 1.
- Touch `max_blocks`, the claim decision table or the price table's other rows. All three are pre-registered as they stand.

## 5. Process record

Delegation worked in the shape the `saga-delegate` skill describes: I diagnosed and decided, saga-opus implemented from briefs and reported, and I reviewed by tabling the tree on disk rather than reading reports. That review caught two batches reported complete that were not (two of forty runners untouched; a fixture and a scanner shape that would have hidden the exact defect under test). The worker found things I had missed or got wrong and said so plainly six times, including the base-config defect that changes the meaning of every earlier arm B number. One process defect was mine: committing while the worker had files staged swept them into my commit (764d2d9); the rule now is that neither session commits while the other is busy and neither uses `git add -A`.
