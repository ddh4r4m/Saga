# Saga documentation

| # | Document | Purpose |
|---|---|---|
| 01 | [Charter](01-charter.md) | What Saga is, is not, principles, success criteria |
| 02 | [Agent self-report](02-agent-self-report.md) | First-person failure modes of a coding agent, with root-cause tags |
| 03 | [Harness architecture](03-harness-architecture.md) | How coding agents are built; what the labs and 2026 papers say is still missing |
| 04 | [Ecosystem survey](04-ecosystem-survey.md) | ~60 trending tools and skills; what works, what is hype, with independent measurements |
| 05 | [Code knowledge structures](05-code-knowledge-structures.md) | Indexes, graphs, memory, determinism, token efficiency, verification loops |
| 06 | [Model and harness gap map](06-model-and-harness-gap-map.md) | Per-model and per-harness failure modes, security and privacy ledger, when to use which |
| 07 | [GitHub issue mining](07-github-issue-mining.md) | 32 repos: what harnesses and tooling have and have not solved, design lessons, tooling-repo mistakes |
| 08 | [Reference repo review](08-reference-repo-review.md) | mattpocock/skills and unlazy in depth: mechanisms, evidence, token cost, gaps |
| 09 | [Proposal and roadmap](09-proposal-and-roadmap.md) | Evidence-backed architecture, per-model and per-harness gap maps, milestones, open questions |
| 10 | [Morning brief](10-morning-brief.md) | What was produced overnight, decisions needed, next steps |
| 11 | [Red-team review](11-red-team-review.md) | Adversarial review: five failure arguments, overstated claims, recommended scope cut and ship conditions |
| 12 | [Experiment protocol](12-experiment-protocol.md) | Pre-registration of the eight-week experiment: hypotheses, arms, task set, budget, kill rule, analysis plan, readiness checklist, timeline |
| 13 | [Week 1 status](13-week1-status.md) | Night of 2026-09-06: what is established, the numbers that exist, what is the owner's to do, and the process record |
| 14 | [Pilot morning](14-pilot-morning.md) | 2026-09-13: the void pilot, the defects fixed live, the completed pilot and its pre-registered null reading, what is the owner's to decide |

## Layer specifications

Scope as of ADR 0009 (2026-09-05): the experiment in docs/12 uses bench, trace, gate and guard on Claude Code only. Deferred specs stay at their paths and are not edited until the experiment reports.

| Spec | Scope (ADR 0009) | Status |
|---|---|---|
| [bench](specs/bench-spec.md) | kept: TypeScript and Python, `smoke` and `dev` tiers | v0.1, Fable |
| [trace](specs/trace-spec.md) | kept: ledger, pins, claim verification; canary, replay and resume deferred | v0.2, Fable |
| [gate](specs/gate-spec.md) ([review log](specs/gate-spec-review.md)) | kept: contract, Stop block, claim verification; red proof as a requirement, diff guards as blockers, approvals deferred | v0.3, Opus draft, Fable-reviewed |
| [guard](specs/guard-spec.md) | kept: bash and zsh classifier, git-tree snapshot; PowerShell, cmd, Windows, masking, package check, MCP gateway deferred | v0.1, Fable |
| [index](specs/index-spec.md) | deferred | v0.1, Fable, frozen |
| [mem](specs/mem-spec.md) | deferred, state block included | v0.1, Fable, frozen |
| [shape](specs/shape-spec.md) | deferred | v0.1, Fable, frozen |
| [route](specs/route-spec.md) | dropped from the roadmap | v0.1, Fable, record only |

## Architecture decision records

See [adr/](adr/): 0001 measurement first, 0002 mechanisms over prompts, 0003 harness-agnostic surface, 0004 no LLM narrative memory, 0005 index shape, 0006 guard command validation, 0007 cost ledger and canary, 0008 implementation language, 0009 scope cut to the eight-week experiment.

Vendored reference repositories live in `../research/vendor/` (git-ignored, see MANIFEST.md there).
