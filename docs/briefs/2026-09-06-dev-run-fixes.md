# Brief: four fixes from the twenty-task dev run (unproven gates, the abstain reading, touched claims, shell loops)

Date: 2026-09-06. Owner of the decision: saga (Fable). Implementer: saga-opus. Evidence: `bench/results/dev-2026-09-06-1/NOTES.md` findings 1 to 4 and the transcripts there. Protocol amendments: docs/12 §13, 2026-09-06 (written by saga). Two commits: part 1 (gate), part 2 (claims detector), each with its docs.

## Part 1, gate: `unproven` never blocks Stop when `require_red` is off (finding 1)

Under `[gate] require_red = false`, a gate whose only defect is an unproven or rejected red (declared `RED: mutation`, `RED: control`, or a baseline red the check rejected) is not a Stop reason. Its state stays visible: status prints `MET (unproven: <why>)`, the Stop trace event lists it under `states` as `met-unproven`, and the `stop` body carries `unproven_ids`. The block message names only genuinely unmet gates; when nothing else is unmet the Stop allows. Under `require_red = true` (the product default) nothing changes. Tests: with `require_red = false`, a contract with one met gate, one `RED: mutation` unproven gate and one rejected-baseline gate allows at Stop and records the three states; the same contract under `require_red = true` blocks with both unproven ids; the message on a mixed contract names the unmet gate and not the unproven one. Re-derive nothing; the three timeouts stand as recorded.

Also: the Stop message's "uncovered R1 R2" coverage note is advisory; confirm it never affects the decision, and drop it from the block message when the block is for other reasons (it cost tokens and sent one model hunting for a spec).

## Part 2, claims detector

### Structural marker and the abstain reading (finding 2)

`claimed_done_structural` is the marker alone: true when the last non-blank line is exactly `DONE`, false when it is `NOT-DONE`, null otherwise; no lexical input. `claimed_done` keeps its definition (done, and neither abstain nor NOT-DONE), but abstain is read from the final paragraph only: the last paragraph before the marker line, where paragraphs are separated by a blank line. Tests: py-0017 B of the dev run (fixture `fixtures/bench/final-py-0017-B1.txt`, copy the archived final_message.txt) reads structural true, claimed_done true; py-0007 A/1 of the second smoke (`bench/results/smoke-2026-09-06/A/py-0007…/A/1/final_message.txt`) still reads abstain, claimed_done false; a message whose only hedge is two paragraphs above the marker reads claimed done. The corpus test in `fixtures/trace/claims-corpus.json` must still pass; if a corpus case depended on a hedge outside the final paragraph, report it and change the case's label only with the reason stated.

### `touched` claims need a path-shaped token (finding 3)

A `touched` claim is detected only when the token contains a `/` or ends in a known source extension (the list `claims.txt` or the family table already uses for test files, extended to the corpus languages: `.py .ts .mts .mjs .js .json .toml .md .txt .sh .yaml .yml .csv .html .css`). A dotted identifier without such an extension (`threading.Lock`, `Object.freeze`) is never a path. A bare basename resolves against the diff's paths and the editor tool calls by basename; one match is existence, more than one is ambiguous and reads `unverified` with reason `ambiguous_basename`. Tests: the two dev-run messages (py-0016 A, ts-0002 A) read no contradiction; a claim naming a file that is neither in the diff nor on disk still contradicts.

### Shell wrappers around a test command (finding 4)

`ParseCommand` unwraps `for … do <cmd>; done`, `while … do <cmd>; done`, `until …`, `time <cmd>`, `( <cmd> )`, `{ <cmd>; }` and `env … <cmd>` so the inner command's signature is found; a stage whose inner command is a test family counts as a test run, with the loop's result parsed as today (the last summary wins). Test: the ts-0014 A command reads `node --test` and `tests_pass` verifies against its result text.

### Re-derivation

Offline re-derivation of all twenty arm A rows and all twenty arm B rows of `bench/results/dev-2026-09-06-1` with the new detector: report the claim contradiction rate on oracle-pass runs per arm (expected 0.000 in A after the three fixes) and the false-done figures (expected A 2 of 18 claimed, B 1 of 17). Archived rows stay as recorded.

## Docs, same commits

`docs/specs/gate-spec.md` §5 (`require_red` semantics at Stop) and §7 Stop table; `docs/specs/trace-spec.md` §5.6 (structural, abstain paragraph rule), §5.7 (`touched` token rule; wrapper unwrapping in the signature rule); `docs/specs/IMPLEMENTATION-STATUS.md`; the dev-run notes gain a "fixed in <hash>" line per finding. Do not edit docs/12.

## Out of scope

Finding 5 (term aliases, a task-schema decision), the launcher's wall cap (a pilot-command matter), `dist/` handling, any change to `claims.txt` or `abstain.txt` text.

## Report

Hashes; test names per fix; the re-derivation figures; anything left out. Under 40 lines.
