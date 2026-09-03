# Morning brief: what happened overnight (2026-09-02 evening to 2026-09-03 morning)

*Written by the orchestrating session for the project owner. Read this first, then the documents it points to.*

## What exists now

The repository went from an empty README to a complete research and design package, all committed locally on `main` (not pushed).

| Area | Files | Words (approx) |
|---|---|---|
| Charter, self-report, proposal, morning brief | docs/01, 02, 09, 10 | 12k |
| Research reports (six, with provenance headers) | docs/03 to 08 | 34k |
| Architecture decision records | docs/adr/0001 to 0007 | 1.5k |
| Layer specifications (eight) plus shared contracts and review logs | docs/specs/ | 63k |
| Raw GitHub issue data for 32 repos | research/data/issues-raw/ (334 JSON files) | n/a |
| Vendored reference repos, git-ignored, pinned | research/vendor/MANIFEST.md | n/a |

## The thesis, confirmed by evidence

The research supports the original claim more strongly than expected. Among frontier models the harness explains far more outcome variance than the model, harness improvements transfer across model families, and the failure modes users report most (false "done", compaction amnesia, ignored instructions, scope creep, destructive commands, invented packages, run-to-run variance) are all attackable outside the model. Prompt-only tools, which dominate the ecosystem by stars, deliver roughly a tenth of what they advertise under independent measurement. The evidence table is in docs/09 §1.

## The design

Saga is one local binary with three surfaces (CLI, MCP, thin per-harness hooks) and eight layers: bench, trace, gate, guard, index, mem, shape, route. Each has a spec in docs/specs/ with schemas, CLI, adapters, test plan, and a pre-registered bench ablation. The shared contracts (hook composition order, `.saga/` layout, exit codes, one snapshot primitive, trace event catalogue, per-session token budget of 3,800 injected tokens) are in docs/specs/00-cross-spec-contracts.md. The roadmap with numeric exit criteria is docs/09 §5: M0 measure (bench + trace), M1 gate + guard + compaction state block, M2 index, M3 memory, M4 shape, M5 route, M6 portability.

## How the work was produced

Six research agents (Opus-class general-purpose), then Fable agents for every specification and for two review passes, with an Opus draft of the gate spec that Fable then reviewed and reworked. Every agent wrote to disk and returned a summary; the orchestrator only read the four earliest reports in full. Two operational incidents cost about two hours: teammates spawned from inside a nested git workspace hang silently on a trust prompt (bug reports filed via /feedback), and the usage limit was hit twice (21:20 and 02:20 resets).

## Read the red-team review before deciding anything

An adversarial review by a separate Fable agent (docs/11-red-team-review.md) argues the thesis is plausible but the eight-layer scope is not, and recommends an eight-week, one-hook experiment instead: bare harness versus bare plus gate on 40 tasks with hidden oracles, K=5, one model, primary metric false-done rate, budget under $3k, with a pre-registered kill rule. It also corrects several numbers in the proposal: the publish-tier bench is closer to $12k than the spec's $500 to $2,000, the M0 task set cannot detect a 5 pp effect, and eight headline rows in docs/09 §1 rest on secondary or unverified sources. The orchestrator did not act on the scope cut; that is decision 0 below.

## Decisions that need you

0. **Scope.** Accept the red-team scope cut (bench for TS and Python, trace ledger and pins, gate Stop with claim verification, guard shell classifier and git-tree snapshots, minimal doctor, Claude Code only; defer mem, index, shape, MCP gateway, Windows, masking; drop route), or keep the full eight-layer roadmap with the red-team's ship conditions as gates.

From docs/specs/REVIEW-LOG.md and docs/09 §7:

1. **Claim verification now has an owner (trace, spec v0.2 §5.5 to §5.9), with gate blocking Stop on a contradicted claim.** Three judgement calls to confirm: contradicted maps to exit 5 (integrity) rather than 3; the verdict lives inside the watchdog section; `RISK: impossible` is a full-mode contract header, not a per-gate attribute.
2. **Harness facts verified (sprint done 2026-09-03, `docs/specs/harness-facts.md`).** Every hook fact the adapters rely on was checked against live vendor docs and default-branch source for Claude Code 2.1.258, Codex rust-v0.152.1, Gemini CLI v0.58.0 and OpenCode v1.18.26, with quotes. Both open items closed in Saga's favour: Claude Code `PreToolUse` `additionalContext` works (the two pages described plain stdout versus the JSON field); Codex `updatedInput` works and `PostToolUse` `block` replaces the result. Five spec statements were contradicted and fixed: Claude Code `PostToolUse` can replace results, `PostCompact` cannot add context, `SubagentStart` can (mem's preamble now uses it, removing the `updatedInput.prompt` collision with route), hooks run outside the sandbox on Claude Code and Codex, and `ask` is unsupported on Codex `PreToolUse`. Four fail-open modes were found (hook timeout on Claude Code, non-JSON stdout on Gemini, `ask` on Codex, untrusted hooks on Codex); contracts §1.1 now requires an entry-side deadline and a trust canary. Nothing further for you to decide; REVIEW-LOG risk 9 lists what remains a probe.
3. **Token budget of 3,800 injected tokens per session** is a sum of priors. Accept as the M0 starting value to be tuned by bench, or set a different ceiling.
4. **Snapshot cost on monorepos** now sits inside every PreToolUse call. Accept `on_budget = "ask"` prompts, or default snapshots to per-turn rather than per-tool-call on large trees.
5. **Language order.** M0 is TypeScript, Python, Go. Given your own work is Swift and Flutter, decide whether Swift or Dart should displace Go in M0.
6. **Public positioning.** README states "research and design phase, no code yet". Decide whether to push now to invite review, or hold until M0 has a first measured baseline.
7. **Name and licence** are set (Saga, MIT). Confirm.

## Suggested next steps, in order

1. Read docs/09 (proposal v0.4) end to end, then docs/specs/00-cross-spec-contracts.md.
2. Answer the seven decisions above; each is a one-line ADR.
3. Start M0: implement `saga bench` and `saga trace` per their specs, with the first 30 tasks in the language you choose. The bench must exist before any other layer claims a number.
4. Run the bare-harness baseline on two models and publish the variance; that is the first public artefact worth pushing.

## Known gaps and caveats

- All numbers in the research docs are as of 2026-09-02 and many come from preprints; the provenance headers say so.
- Research reports 03 to 08 were written by agents and reviewed only through their summaries; spot-check any number before quoting it externally.
- Open risks 1, 2, 3, 7 and 8 in docs/specs/REVIEW-LOG.md are closed (2 and 3 by the harness-facts sprint); risks 4 to 6 remain and are decisions 3 to 4 above; risk 9 is informational.
- The em-dash style rule was applied mechanically across all docs after the fact (558 replacements); a few sentences may read slightly oddly.
- Nothing has been pushed to GitHub.
