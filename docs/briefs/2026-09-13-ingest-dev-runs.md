# Brief: ingest the two dev runs of 2026-09-13

Date: 2026-09-13. Owner of the decision: saga (Fable). Implementer: saga-opus. Two commits (one per run), or one if the second run is done before you start; tell me before each commit.

## 1. What

Mirror `bench/results/dev-2026-09-06-1/` (A, B, compare.md, NOTES.md, run-log.txt, transcripts). Sources:

- `/tmp/saga-dev-2/` → `bench/results/dev-2026-09-13-1/`. Commit 069af89, binary 6d2defea…, task set fc22a4d4…, prereg c44b5310…. Arm A complete (20 runs, 2.73 usd, 18 completed, 2 abandons on py-0007 and py-0020). Arm B is 20 infra rows, spent 0, cause: the corpus store was keyed on the selection hash at run time and on the frozen set hash at approve time (brief 2026-09-13-corpus-approval-not-consumed, fixed f42db38). There is no compare.md (the runner refused to pair a partial arm; keep its message in NOTES). Keep arm B's archive as it is; it is the evidence of the defect.
- `/tmp/saga-dev-3/` → `bench/results/dev-2026-09-13-2/`, once `/tmp/saga-dev-3.log` ends with `exit:`. Commit f42db38, binary 9fcaeaeb…, same task set and prereg. Both arms should be complete; run `saga bench compare` as the launcher did (its compare.md is under the OUT dir if it succeeded).

NOTES.md for each: status line as in the 2026-09-06 notes (who launched, from where, commit, binary, model, K, wall cap = task's own, cost per arm, provenance head verbatim); the results table in the same columns; findings numbered, each with the row ids and the transcript lines that show it; nothing speculative. For dev-3, table anything that differs from dev-2 arm A on the same task (same model, same day, K=1: differences are noise unless the transcript says otherwise, say so). Say plainly for each arm B row whether gate_config_present is true and the config sha matches the staged one; that is the first live run where it should.

Transcripts: copy final_message.txt and trace.jsonl per run as before. Scans: secrets (sk-ant, OAUTH, TOKEN, api key shapes) and home paths (the owner's home directory) across every ingested file; mask a home path as SAGA_MASK_HOME as in probes-2026-09-06 and list every masking in NOTES.

## 2. Report

Per run: hash; the results table; findings in one line each; scan result. Under 20 lines per run. Do not edit code; if you find a defect, describe it in NOTES and in the report and stop.
