# Captured harness payloads

Payloads captured live from Claude Code, byte for byte as the harness
handed them to a hook. They are evidence, not examples: a test that
needs to know what the harness sends reads one of these rather than a
shape someone wrote from the documentation.

| File | Event | Captured | Provenance |
|---|---|---|---|
| `posttoolusefailure.json` | `PostToolUseFailure` | 2026-09-06, Claude Code 2.1.263 | P17 of `scripts/harness-probes.sh`, third run; archived beside the rest of that run's evidence in `bench/results/probes-2026-09-06/`. Verifies harness-facts C34. |
| `session-limit-clean.json` | `result`, account session window already closed | 2026-09-12, Claude Code 2.1.266 | The pilot of 2026-09-13, arm A ts-0004 run 1 (row 71 of 200): 1 turn, 0 tool calls, 0.6 s, cost 0, `subtype: "success"` with `is_error: true`. Verifies harness-facts C38. |
| `session-limit-after-work.json` | `result`, window closed mid-run | 2026-09-12, Claude Code 2.1.266 | The same pilot, arm A ts-0003 run 5 (row 69, the transition): 12 turns, 11 tool calls, 23.3 s, cost 0.1250 usd, then the same one-line reply. It is here because a detector keyed on cost or tool use would miss exactly this row. |
| `session-limit.txt` | the reply text | 2026-09-12, Claude Code 2.1.266 | The `result` string, byte-identical in both rows above and in all 132 of the pilot's void rows. |

The `/tmp` paths inside are the probe's own workspace and are kept
verbatim: rewriting them would make the file no longer the capture. The
run that produced them was scanned for tokens and for the owner's home
path before it was archived. The two `session-limit-*.json` captures carry
the harness's own `/var/folders` session paths and no home path; their
archive is `bench/results/pilot-2026-09-13-void/`.
