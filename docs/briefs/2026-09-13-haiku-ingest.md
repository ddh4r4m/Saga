# Brief: ingest the Haiku exploration cell of 2026-09-13

Date: 2026-09-13. Owner of the decision: saga (Fable). Implementer: saga-opus. One commit, after saga has reviewed the staged tree and said go. This cell is exploration, not the experiment (closed, docs/12 §13 b085526); the first line of NOTES.md says so and nothing in it may be quoted as a result anywhere else.

## 1. Source

`/tmp/saga-explore-20260913-181737/`, launched by the owner from their terminal at 18:17 IST with `BUDGET_USD=15 scripts/bench-explore.sh` (commit 9d2c51e, binary 1d243607…, task set fc22a4d4…, prereg e7718283…, model claude-haiku-4-5-20251001, tier user, K=5, `--on-limit wait`, estimate 9.70 with model ratio 0.2). 197 of 200 rows ran: A 99, B 98, three rows not run because the 1.5 × estimate scheduling cap (14.55 usd) was reached; exit 3; spent 14.79 usd (A 7.35, B 7.44). 0 infra, 0 limit waits. The runner wrote compare.md and per-arm reports; carry them exactly as written.

The report's headline, quoted here only so the ingest knows what it is handling: false_done A = 0.400, B = 0.158, Δ = −0.242, 95 percent CI −0.410 to −0.095, Wilcoxon n = 11 p = 0.0068; pass@1 A 0.620, B 0.867; tokens per solved A 421k, B 289k; scope violations A 0.141, B 0.051; cheat flags 0 and 0; safety denies 0 and 0. It is exploratory: not pre-registered, model chosen after the pilot, three rows unrun, one cell. The NOTES say that in the first paragraph and repeat it beside every number.

## 2. Target

`bench/results/explore-2026-09-13-haiku/` mirroring the pilot layout: A/, B/, compare.md, compare.json, per-arm report and status as written, provenance.txt, PURPOSE.txt, run-log.txt, native.jsonl per run, NOTES.md, MASKED.md (standing rule). No figure.

NOTES.md, in this order: (1) the exploration status and why the cell exists (a weaker model to exercise the bench); (2) provenance head verbatim; (3) the three not-run rows by task, arm and k, and the cap that stopped them; (4) per-task-per-arm table as in the pilot notes (n, pass, false-done, abandon, budget, median cost, median wall); (5) **what the weaker model exercised, which is the point of the cell**: every abandon row (which tasks, graded how, and whether any abandon was on a possible task), every gate block and release count from hook-trace.jsonl with any block the agent could not clear, every claim verdict shape (counts of verified, unverified, contradicted, abstain per arm; every contradicted-on-oracle-pass row re-derived offline under the new referent rules with its cause, since Haiku's phrasing is the first out-of-sample test of those rules), every harness-limit or error subtype seen, integrity results, gate_config_present on every arm B row; (6) bench defects or detector errors found on Haiku rows, listed with row ids, or a sentence that none were found; (7) the base-rate observation as counts only (claims per arm, false among them) with the plain statement that the pilot's model had a base rate near the floor and this model does not, and that a claim about the gate on this model would need its own pre-registration; (8) scans.

## 3. Report

Hash after go; before that: the per-task table, the section 5 counts, the section 6 list, and the scan result. Under 30 lines.
