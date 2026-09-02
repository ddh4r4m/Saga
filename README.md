# Saga

> Saga makes any coding agent prove its work: a local, model-agnostic layer for repository knowledge, machine-checked completion, memory that survives compaction, and privacy at the tool boundary, with a benchmark that keeps it honest.

**Status: research and design phase, no code yet.** The documents in `docs/` are the current output of the project. Milestones and scope will change as the benchmark reports real numbers.

## Why

Frontier models improve every release, and the complaints from people using them for real engineering do not change: it said done but does not work, it forgot what was agreed an hour ago, it ignored the instructions file, same prompt, different result. The research behind Saga found the harness matters more than the model among frontier models: an 18.5 percentage-point variance from harness choice versus 2.4 points from model choice, roughly 7.8x (see the evidence table in [doc 09](docs/09-proposal-and-roadmap.md)). Deterministic structure closes gaps that prompting cannot: a structural code index raised bug localization from 44% to 85% in one study, while independently measured prompt-only tools delivered close to a tenth of their advertised effect. Saga exists to build the mechanisms that have evidence behind them, and to publish the benchmark that keeps every future claim honest.

## What Saga is

1. **A code knowledge layer.** A local, incrementally updated structural index of a repository: symbols, references, call graph, test-to-code mapping, conventions.
2. **A contract and gate layer.** A scope contract agreed before work starts, and a gate runner that checks oracles before "done" is accepted.
3. **A typed memory layer.** Decisions, constraints, environment facts, and conventions stored as separate, queryable records that survive compaction and are handed to sub-agents verbatim.
4. **A privacy layer.** Local, deterministic redaction of secrets and PII before content leaves the machine, with reversible placeholders.
5. **A routing and budget layer.** A visible token budget and cost-aware model selection for sub-tasks.

Each layer is independently installable. See [doc 01](docs/01-charter.md) for the full charter.

## What Saga is not

- **Not a skills repository.** Prompt text that asks the model to behave is the weakest mechanism available. Saga ships prompts only as thin adapters over machine-checked tools.
- **Not a new agent or harness.** Saga does not own the loop; it plugs into whatever loop is already in use.
- **Not a process framework.** It does not prescribe planning ceremonies. It supplies the primitives those frameworks lack.
- **Not a model wrapper or proxy** that rewrites prompts in flight.

## How it plugs into harnesses

One core binary, `saga`, that knows nothing about any vendor's prompt format. It exposes the same capabilities three ways:

- **CLI** — works anywhere a shell can run, including CI.
- **MCP server** — index, memory, and gate tools for harnesses that speak MCP.
- **Thin hook adapters** — per-harness translation only (Claude Code hooks, Codex hooks/notify, Gemini CLI hooks, Cursor rules, OpenCode plugins), each under a few hundred lines with no logic beyond translation. Gate enforcement falls back to CI where a harness has no stop hook.

See [ADR 0003](docs/adr/0003-harness-agnostic-surface.md).

## Roadmap

From [doc 09, section 5](docs/09-proposal-and-roadmap.md):

| Milestone | Scope | Exit criterion |
|---|---|---|
| **M0 Measure** | `bench` + `trace` core, task set for 3 languages, one adapter (Claude Code) | Bare harness baseline published with pass^k and variance for 2 models |
| **M1 Gate + Guard** | ledger, red proof, diff guards, Stop adapters (Claude Code, Gemini CLI, Codex, CI), secret masking, package existence check | Measured effect on M0 task set; zero false-positive masking on the positive-control suite |
| **M2 Index** | tree-sitter graph, BM25, test-impact map, 4 tools, MCP server, incremental update, regime gating | Localization and resolve delta reproduced on the M0 set with control arm blocked |
| **M3 Memory** | typed records, targeted PreToolUse injection, freshness, sub-agent preamble | Compliance-over-session-length curve measured with and without |
| **M4 Shape** | output parsers for the top runners per language, error-aware truncation, result cache | Token per completed task down with correctness flat or up |
| **M5 Route** | policy file, budget, cheap-model delegation for read-only sub-tasks | Cost down at equal pass^k |
| **M6 Portability** | remaining adapters (Cursor, OpenCode, Windsurf, Cline), portable trace resume | Same install on 3+ harnesses, no per-harness prompt rewrites |

## Read the research

| # | Document | Purpose |
|---|---|---|
| 01 | [Charter](docs/01-charter.md) | What Saga is, is not, principles, success criteria |
| 02 | [Agent self-report](docs/02-agent-self-report.md) | First-person failure modes of a coding agent, with root-cause tags |
| 03 | [Harness architecture](docs/03-harness-architecture.md) | How coding agents are built; what labs and 2026 papers say is still missing |
| 04 | [Ecosystem survey](docs/04-ecosystem-survey.md) | ~60 trending tools and skills; what works, what is hype, with independent measurements |
| 05 | [Code knowledge structures](docs/05-code-knowledge-structures.md) | Indexes, graphs, memory, determinism, token efficiency, verification loops |
| 08 | [Reference repo review](docs/08-reference-repo-review.md) | mattpocock/skills and unlazy in depth: mechanisms, evidence, token cost, gaps |
| 09 | [Proposal and roadmap](docs/09-proposal-and-roadmap.md) | Architecture direction and milestones |

| ADR | Decision |
|---|---|
| [0001](docs/adr/0001-measurement-first.md) | Ship the measurement harness before any feature |
| [0002](docs/adr/0002-mechanisms-over-prompts.md) | Prefer machine-checked mechanisms over prompt text |
| [0003](docs/adr/0003-harness-agnostic-surface.md) | One core, three surfaces (CLI, MCP, thin hooks) |
| [0004](docs/adr/0004-no-llm-narrative-memory.md) | Never inject LLM-written narrative as fact |
| [0005](docs/adr/0005-index-shape.md) | Deterministic AST graph, depth one, four tools, grep stays |

Docs 06 and 07 (model gap map, GitHub issue mining) are pending and will be added when finished. See [docs/README.md](docs/README.md) for the maintained index.

## Principles

From [doc 01](docs/01-charter.md):

- Every rule that can be a check must be a check.
- Evidence over belief: "done" requires oracle output, stored and reproducible.
- Collapse branch points before the run; declared scope and oracles remove ambiguity.
- Lookup beats memory: index and query anything the agent would otherwise re-derive.
- Structure survives compaction; prose does not.
- Token cost is a first-class metric, reported per component.
- Model-agnostic by construction: the core is CLI plus MCP, adapters stay thin.
- Local first, private by default.
- Measure or do not claim; ship a benchmark or label a feature experimental.
- Cost versus time is an explicit, logged setting, never an accident.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for how to propose a component, write an ADR, and add a harness adapter.

## License

MIT. See [LICENSE](LICENSE).
