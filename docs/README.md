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

## Layer specifications

| Spec | Milestone | Status |
|---|---|---|
| [bench](specs/bench-spec.md) | M0 | v0.1, Fable |
| [trace](specs/trace-spec.md) | M0 | v0.1, Fable |
| [gate](specs/gate-spec.md) ([review log](specs/gate-spec-review.md)) | M1 | v0.2, Opus draft, Fable-reviewed |
| [guard](specs/guard-spec.md) | M1 | v0.1, Fable |
| [index](specs/index-spec.md) | M2 | v0.1, Fable |
| [mem](specs/mem-spec.md) | M1 (state block), M3 | v0.1, Fable |
| [shape](specs/shape-spec.md) | M4 | v0.1, Fable |
| [route](specs/route-spec.md) | M5 | v0.1, Fable |

## Architecture decision records

See [adr/](adr/): 0001 measurement first, 0002 mechanisms over prompts, 0003 harness-agnostic surface, 0004 no LLM narrative memory, 0005 index shape, 0006 guard command validation, 0007 cost ledger and canary.

Vendored reference repositories live in `../research/vendor/` (git-ignored, see MANIFEST.md there).
