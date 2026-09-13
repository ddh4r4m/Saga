# Brief: ingest the pilot of 2026-09-13

Date: 2026-09-13. Owner of the decision: saga (Fable). Implementer: saga-opus. One commit, after saga has reviewed the report and said go.

## 1. Source

`/tmp/saga-pilot-20260913-082413/`, launched by the owner from their own terminal at 08:24 IST (`BUDGET_USD=65 scripts/bench-pilot.sh` at commit 24814bd, binary d9dbc07d…, task set fc22a4d4…, prereg 88d1abdc…, tier dev, K=5, claude-opus-5, `--on-limit wait`). 200 rows, 0 infra, 0 limit waits, exit 0, 59.40 usd (A 28.15, B 31.25). The runner wrote `archive/compare.md` and each arm's report; these are the pilot report of docs/12 commitment 1 and are carried exactly as written, never regenerated or edited. A refused launch at 08:22 (`/tmp/saga-pilot-20260913-082219`, the terminal lacked the token; nothing spent) is carried as provenance and log only.

## 2. Target

`bench/results/pilot-2026-09-13/` mirroring the dev layout: A/, B/, compare.md, compare.json, per-arm report and status as written, provenance.txt, run-log.txt, transcripts (final_message.txt and trace.jsonl per run, all 200), NOTES.md, MASKED.md (the standing rule: mask the home path, SHA256SUMS untouched, both sums recorded), and `refused-0822/`.

NOTES.md: status line (launched by the owner from their terminal; the corpus approval at 08:21 IST on binary d9dbc07d); the provenance head verbatim; the primary outcome and its CI quoted from compare.md without rounding; per-task-per-arm table (n, pass, false-done, abandon, budget, median cost, median wall) as in the void notes; the four impossible-task rows (py-0007, py-0020) abandon counts per arm; anything in the rows that the report does not surface (scope violations by task, safety-hook denies by task, the one `budget` row, the one arm A run with no claimed_done verdict and why); disclosures carried forward: the arm A approval-store env leak (dev-2 finding 3), the prereg hash sequence (c44b5310 dev runs, d5adae70 void pilot, 88d1abdc this run, with the two §13 amendments between them named), the two open §13 detector decisions and how many rows of this run each touches (count them: ts-0012-shape same-family attribution; abstain lexicon hits on the gate's own block in arm B). Scans as always. No badge, no edits to the report.

## 3. Report

Hash; the per-task table; the counts for the two detector questions; scan result. Under 25 lines.
