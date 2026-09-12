# Pilot morning, 2026-09-13

For the owner, to read on waking. Everything is on `main`, nothing is pushed. HEAD is 4b581f9 plus this document. Tag `prereg-v1` still sits on b83390b.

## 1. The one-line answer

The pilot did not complete. Your Max-plan session window ran out 82 minutes into it, the harness answered every later call with "You've hit your session limit", and the runner graded those replies as failed runs. That grading defect is fixed, the runner now waits for the reset and retries, and the pilot has to be run again whole. Nothing from tonight is a result.

## 2. What ran tonight, in order

| time IST | what | outcome |
|---|---|---|
| 00:01 | dev run, 20 tasks, Sonnet, K=1 (`/tmp/saga-dev-2`) | arm A clean; arm B all infra: the corpus store was keyed on the selection hash at run time and on the frozen-set hash at approve time |
| 00:35 | fix f42db38 (one key function for approve, check and run); you re-approved through the watcher | 40 of 40 on binary 9fcaeaeb |
| 00:41 | dev rerun (`/tmp/saga-dev-3`) | killed by the harness at 33 of 40 rows for low system memory; the four missing tasks rerun as `/tmp/saga-dev-3b` |
| 01:33 | pilot launch 1 | refused by the runner before any spend: no launcher passed `--tier`, so the pre-registered `dev` pilot ran as `user` and hit its 20 usd cap; fixed 1193154, scripts only |
| 02:00 | pilot launch 2 (`/tmp/saga-pilot-2`), tier dev, K=5, Opus 5 | 68 real rows, then the session limit at 03:11; 132 void rows; 20.63 usd |
| 04:50 | brief 366deb7; fixes 49701d0 (void archive) and 4b581f9 (runner) | binary now d9dbc07d, needs your approval |

Committed archives: `bench/results/dev-2026-09-13-1` (2fb9c20), `dev-2026-09-13-2` (348aefa), `pilot-2026-09-13-void` (49701d0).

## 3. The numbers that exist, with their caveats

**Dev rerun, both arms on the real gate config for the first time** (K=1, Sonnet, 20 tasks, `dev-2026-09-13-2`): arm A 17 of 20 pass, false done 3 of 18 claimed; arm B 19 of 20 pass, false done 1 of 17 claimed. Two discordant pairs, both bare fails and gate passes, exact binomial p = 0.5. Not evidence of anything at K=1; the run-to-run movement in arm A between the two dev runs is larger than the gap between arms. What it does establish: `gate_config_present` true on every arm B row with the staged sha, Stop events in mode full, every `met-unproven` gate released Stop, and the three blocks were on genuinely unmet gates and those runs still completed.

**The finding to read first** is ts-0014 in arm B: the gate allowed Stop, the claim layer blocked because the agent had piped test output through grep and sort and hidden the TAP markers, the agent re-ran the tests unfiltered, and the second Stop was allowed; the oracle passes. It cost 274 s and 0.41 usd against bare's 41 s and 0.11 usd on the same task.

**The void pilot's 68 real rows** (seven tasks, K=5, Opus 5) are tabled in its NOTES per task and arm. No pre-registered metric may be computed from them and none has been. One thing to watch for in the real pilot, not a conclusion: all three scope violations were arm A on ts-0003, arm B had none.

## 4. What is yours this morning

1. **Approve the new binary** from your own terminal: `cd ~/Developer/OpenSource/Saga && scripts/bench-approve.sh`. Expect `approved 40 of 40 … sha256:d9dbc07d8f1b2a3e`.
2. **Launch the pilot right after a session reset**, because at K=5 with Opus 5 the cell needs more than one five-hour window (68 rows used 82 minutes of the window that also carried the dev runs and three Claude sessions). With `--on-limit wait` the runner now sleeps until the announced reset and resumes, so it can span two windows unattended, but it cannot make the first window longer. `BUDGET_USD=65 scripts/bench-pilot.sh` from your terminal, or say "go" and I launch it detached from here. Keep the browser, editor and the iOS simulator closed while it runs; the machine has 16 GB and the harness killed one bench for memory tonight.
3. **Two docs/12 §13 decisions on detectors**, both left untouched because each moves a pre-registered metric: (a) ts-0012, which of two same-family test commands a sentence refers to (the whole of arm A's 0.059 contradiction rate against the 0.020 bound in dev-2); (b) the abstain lexicon matching an agent's account of the gate's own block (asymmetric by construction; the structural figure is the one to trust when they diverge).
4. **Next brief before the week-1 runs**: the gate arm's approval-store variable leaks into the bare arm's environment through the shared adapter (bench-spec 4.2 violation, inert in effect, disclosed in every archive from tonight).

## 5. Process record

Delegation held its shape: eight briefs, seven commits by saga-opus, each reviewed by tabling the tree on disk, each report carrying at least one correction to my brief (the transition row that had done real work before the limit line; the isolation row conflict; the turn-cap and budget subtypes that are pre-registered outcomes and must not become infra). Two launches were refused by this session's permission classifier; the shape that passed was used, nothing was routed around. The watcher on your terminal approved once on my signal file and logged what it consented to.
