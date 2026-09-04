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

The research supports the original claim more strongly than expected. In one small controlled experiment (a preprint) the harness explained far more outcome variance than the model among frontier models, one further preprint shows harness improvements transferring across model families, and the failure modes users report most (false "done", compaction amnesia, ignored instructions, scope creep, destructive commands, invented packages, run-to-run variance) are all attackable outside the model. Prompt-only tools, which dominate the ecosystem by stars, deliver between a seventh and a half of what they advertise under independent measurement. The evidence table is in docs/09 §1.

## The design

Saga is one local binary with three surfaces (CLI, MCP, thin per-harness hooks) and eight layers: bench, trace, gate, guard, index, mem, shape, route. Each has a spec in docs/specs/ with schemas, CLI, adapters, test plan, and a pre-registered bench ablation. The shared contracts (hook composition order, `.saga/` layout, exit codes, one snapshot primitive, trace event catalogue, per-session token budget of 3,800 injected tokens) are in docs/specs/00-cross-spec-contracts.md. The roadmap with numeric exit criteria is docs/09 §5: M0 measure (bench + trace), M1 gate + guard + compaction state block, M2 index, M3 memory, M4 shape, M5 route, M6 portability.

## How the work was produced

Six research agents (Opus-class general-purpose), then Fable agents for every specification and for two review passes, with an Opus draft of the gate spec that Fable then reviewed and reworked. Every agent wrote to disk and returned a summary; the orchestrator only read the four earliest reports in full. Two operational incidents cost about two hours: teammates spawned from inside a nested git workspace hang silently on a trust prompt (bug reports filed via /feedback), and the usage limit was hit twice (21:20 and 02:20 resets).

## Read the red-team review before deciding anything

An adversarial review by a separate Fable agent (docs/11-red-team-review.md) argues the thesis is plausible but the eight-layer scope is not, and recommends an eight-week, one-hook experiment instead: bare harness versus bare plus gate on 40 tasks with hidden oracles, K=5, one model, primary metric false-done rate, budget under $3k, with a pre-registered kill rule. It also corrects several numbers in the proposal: the publish-tier bench is $5.5k to $15k by the spec's own run counts, not the $500 to $2,000 it stated (bench-spec §4.4 and docs/09 §5 now corrected), the M0 task set cannot detect a 5 pp effect, and eight headline rows in docs/09 §1 rest on secondary or unverified sources. The orchestrator did not act on the scope cut overnight; the owner accepted it on 2026-09-05 (decision 0 below, ADR 0009).

## Decisions taken

| # | Decision | Taken | Record |
|---|---|---|---|
| 0 | **Scope.** The red-team scope cut is accepted: bench for TypeScript and Python, trace ledger, pins and claim verification, gate Stop block with claim verification, guard bash/zsh classifier and git-tree snapshots, minimal doctor, Claude Code only; mem, index, shape, the MCP gateway, Windows and masking deferred; route dropped from the roadmap | 2026-09-05 | [ADR 0009](adr/0009-scope-cut-eight-week-experiment.md); pre-registration in [docs/12](12-experiment-protocol.md) |
| was 5 | **Language order.** TypeScript and Python only for the experiment; Go leaves M0; Swift and Dart wait for a positive result | 2026-09-05, by ADR 0009 | ADR 0009 kept/deferred table |
| was 2 | **Harness facts** verified on live releases (sprint of 2026-09-03) | 2026-09-03 | [harness-facts](specs/harness-facts.md) |

## Decisions that still need you

Re-numbered after the scope cut. Decisions on deferred layers are parked, not dropped.

1. **Claim verification judgement calls (trace-spec §5.5 to §5.9, gate-spec §6).** Now on the critical path: docs/12 readiness row 1. Three calls to confirm: contradicted maps to exit 5 (integrity) rather than 3; the verdict lives inside the watchdog section; `RISK: impossible` is a full-mode contract header, not a per-gate attribute. Also confirm docs/12 §2.1 rule 1: the bench's `claimed_done` never uses the gate's own exit status, which departs from gate-spec §10.3.
2. **Public positioning and push timing.** README now states the experiment (ADR 0009). docs/12 §12 commits to pushing the repository with the manifests in week 8, and to publishing the pilot report whatever its sign. Decide whether to push earlier, at the week 2 smoke tier, to invite review of the pre-registration before any paid run.
3. **Name and licence** are set (Saga, MIT). Confirm.
4. **Parked with mem (deferred by ADR 0009): the 3,800-token injected budget.** Only trace's 400-token and gate's 1,000-token session shares are live; both are measured in the experiment (docs/12 §2.2 `injected_tokens`). Nothing to decide until a layer that injects mid-conversation is reopened.
5. **Parked with the monorepo snapshot question: per-turn versus per-tool-call snapshots on large trees.** The corpus repositories are 25 to 257 source lines; the gate Stop step is 155 ms p50 there. The 20k-file figures in IMPLEMENTATION-STATUS stay a known limit and are not part of the experiment.

## Suggested next steps, in order

1. Read [ADR 0009](adr/0009-scope-cut-eight-week-experiment.md), then [docs/12](12-experiment-protocol.md) end to end; it is the plan for the next eight weeks and freezes on the first paid run.
2. Answer decisions 1 to 3 above; each is one line.
3. Week 1 of docs/12 §11: claim verification events, the hook exit-code probe, pins with the harness version; tag the pre-registration.
4. Week 2: control-arm blocking, two-arm interleaving, containers or the documented substitute, then the smoke tier. Its measured dollars replace every estimate in docs/12 §6.

## Known gaps and caveats

- All numbers in the research docs are as of 2026-09-02 and many come from preprints; the provenance headers say so.
- Research reports 03 to 08 were written by agents and reviewed only through their summaries; spot-check any number before quoting it externally.
- Open risks 1, 2, 3, 7 and 8 in docs/specs/REVIEW-LOG.md are closed (2 and 3 by the harness-facts sprint); risks 4 to 6 remain and are parked as decisions 4 and 5 above; risk 9 is informational.
- The em-dash style rule was applied mechanically across all docs after the fact (558 replacements); a few sentences may read slightly oddly.
- Nothing has been pushed to GitHub.
