# Captured harness payloads

Payloads captured live from Claude Code, byte for byte as the harness
handed them to a hook. They are evidence, not examples: a test that
needs to know what the harness sends reads one of these rather than a
shape someone wrote from the documentation.

| File | Event | Captured | Provenance |
|---|---|---|---|
| `posttoolusefailure.json` | `PostToolUseFailure` | 2026-09-06, Claude Code 2.1.263 | P17 of `scripts/harness-probes.sh`, third run; archived beside the rest of that run's evidence in `bench/results/probes-2026-09-06/`. Verifies harness-facts C34. |

The `/tmp` paths inside are the probe's own workspace and are kept
verbatim: rewriting them would make the file no longer the capture. The
run that produced them was scanned for tokens and for the owner's home
path before it was archived.
