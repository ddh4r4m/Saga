# Brief: pre-registration freeze mechanics (docs/12 row 15) and report determinism

Date: 2026-09-06. Owner of the decision: saga (Fable). Implementer: saga-opus. One or two commits. The tag itself (`prereg-v1`) is the owner's act and is not part of this brief.

## 1. Problem

docs/12 row 15 closes when "this file's sha256 is in the smoke, pilot and full manifests" and the tag exists. Today `manifest.preregistration_sha256` is always null, the report header prints "pre-registration: none", `saga bench run` has no way to take the file, and the task-set freeze ("task set frozen, hashes in the full manifest", §10 week 4) has no artefact a run can be checked against. docs/12 §8 also requires `report.json` and `report.md` regenerated from `rows.jsonl` to be byte-identical, and nothing tests that.

## 2. Decisions (taken)

1. `saga bench run --prereg <path>` copies the file verbatim into the archive root as `preregistration.md`, records its sha256 in `manifest.preregistration_sha256`, and lists it in the archive's top-level `SHA256SUMS` if one exists (add one if not: manifest, rows, exclusions, report files, preregistration). Without the flag the manifest stays null and the report says so, as today. `scripts/bench-smoke.sh` passes `--prereg "$ROOT/docs/12-experiment-protocol.md"`.
2. The report header prints `pre-registration: sha256:<hash> (preregistration.md)`; `saga bench compare` prints one warning line when the two arms' hashes differ and refuses with exit 2 when exactly one is null (a pre-registered arm cannot be paired with an unregistered one).
3. **Task-set freeze artefact.** `saga bench taskset <tasks-glob> [--write <file>]` prints one line per task (`<id> <content-hash>`) and a final `set <sha256>` line computed the way the manifest's `task_set.sha256` is; `--write` writes it. The frozen file lives at `bench/tasks/TASKSET.sha256`. `saga bench run` reads that file when present beside the tasks and refuses with exit 5 when a task's hash differs from the frozen one, unless `--unfrozen` is passed (recorded in the manifest as `task_set.frozen: false`). Write the file for the current 40 in this brief's commit; every later task edit must rewrite it deliberately, which is the point.
4. **Determinism test.** A test regenerates `report.json` and `report.md` from `rows.jsonl` twice for `bench/results/smoke-2026-09-06-2/A` and `/B` and asserts byte identity, and does the same on a fresh 40-task replay archive (replay adapter, K=1, `--no-verify`, both arms) so the whole pipeline is exercised on the frozen set without a model; guard the replay case with `testing.Short()` and say how long it takes. If any field is not deterministic (a timestamp, a map order), fix the field rather than the test; `created` comes from the manifest and stays.
5. `saga bench verify-badge` is refused at tier `dev` and `user` by rule (docs/12 commitment 4); confirm that is the current behaviour with a test, or implement the refusal if it is missing.

## 3. Tests

Flag round trip (file copied, hash in manifest, header line); compare warning and refusal; taskset output and the frozen check (a modified task refuses with exit 5, `--unfrozen` records it); determinism per decision 4; badge refusal per decision 5.

## 4. Docs, same commit

`docs/specs/bench-spec.md` §3.2 (`--prereg`), §2 (the taskset file), §8.4 (determinism test named), §7.3 (badge refusal at these tiers); `docs/specs/IMPLEMENTATION-STATUS.md`; docs/12 §10 row 15: you may edit only its gap column to "mechanism landed <hash>: `--prereg`, `TASKSET.sha256`, determinism test; tag pending the owner". Row 8 may also be updated to note `saga gate lint` exit 0 on 40 if it still reads 40 contracts pending.

## 5. Out of scope

Creating the tag; changing any task; the pilot itself.

## 6. Report

Hashes; the taskset `set` hash for the 40; the determinism test's timings; anything left out. Under 30 lines.
