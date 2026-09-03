# Saga

> Saga makes any coding agent prove its work: a local, model-agnostic layer for repository knowledge, machine-checked completion, memory that survives compaction, and privacy at the tool boundary, with a benchmark that keeps it honest.

**Status: M0 in progress.** The documents in `docs/` are the design; the Go module at the repository root is the first implementation step (trace event log, ledger, composed hook, Claude Code adapter, `doctor`). `docs/specs/IMPLEMENTATION-STATUS.md` lists what is implemented against each spec section.

## Why

Frontier models improve every release, and the complaints do not change: it said done but does not work, it forgot what was agreed an hour ago, it ignored the instructions file, same prompt, different result. The research behind Saga found the harness matters more than the model: in one controlled study, harness choice explained 7.8 times more score variance than model choice (evidence table in [doc 09](docs/09-proposal-and-roadmap.md)). Deterministic structure closes gaps prompting cannot: a structural code index raised bug localization from 44% to 85% in one study, while independently measured prompt-only tools delivered close to a tenth of their advertised effect. Saga builds the mechanisms with evidence behind them, and publishes the benchmark that keeps every claim honest.

## What Saga is

1. **A code knowledge layer.** A local, incrementally updated structural index: symbols, references, call graph, test-to-code mapping, conventions.
2. **A contract and gate layer.** A scope contract agreed before work starts, and a gate runner that checks oracles before "done" is accepted.
3. **A typed memory layer.** Decisions, constraints, and conventions as queryable records that survive compaction and are handed to sub-agents verbatim.
4. **A privacy layer.** Local, deterministic redaction of secrets and PII before content leaves the machine.
5. **A routing and budget layer.** A visible token budget and cost-aware model selection for sub-tasks.

Each layer installs independently. See [doc 01](docs/01-charter.md) for the full charter.

## What Saga is not

- **Not a skills repository.** Prompt text asking the model to behave is the weakest mechanism available; Saga ships prompts only as thin adapters over checked tools.
- **Not a new agent or harness.** Saga does not own the loop; it plugs into whatever loop is already in use.
- **Not a process framework.** It supplies primitives, not planning ceremonies.
- **Not a model wrapper or proxy** that rewrites prompts in flight.

## How it plugs into harnesses

One core binary, `saga`, knows nothing about any vendor's prompt format. It exposes the same capabilities three ways:

- **CLI**: works anywhere a shell can run, including CI.
- **MCP server**: index, memory, and gate tools for harnesses that speak MCP.
- **Thin hook adapters**: per-harness translation only (Claude Code, Codex, Gemini CLI, Cursor, OpenCode), each a few hundred lines with no logic beyond translation. Gate enforcement falls back to CI where a harness has no stop hook.

See [ADR 0003](docs/adr/0003-harness-agnostic-surface.md).

## Roadmap

From [doc 09, section 5](docs/09-proposal-and-roadmap.md):

| Milestone | Scope | Exit criterion |
|---|---|---|
| **M0 Measure** | `bench` core; `trace` with per-turn cost ledger, version pinning, watchdog; `doctor`; 3-language task set; Claude Code adapter | Baseline published with pass^k and variance for 2 models; ledger reconciles with provider usage; canary runs on a schedule |
| **M1 Gate + Guard + compaction survival** | ledger grammar, red proof, claim verification, diff guards, Stop adapters; post-expansion command classifier, per-turn snapshots and `saga undo`, secret masking, package check; PreCompact state block | Measured effect on M0 set; zero escapes on the destructive-command control suite; no false-positive masking; passes on Windows |
| **M2 Index** | tree-sitter graph, BM25, test-impact map, 4 tools, MCP server, external API cache | Localization/resolve delta reproduced, control arm blocked |
| **M3 Memory** | typed records, targeted injection, freshness, sub-agent preamble, procedures | Compliance-over-session curve measured with and without; drift on prose-only rules reported |
| **M4 Shape** | output parsers, error-aware truncation, result cache, comment stripper | Tokens per task down, correctness flat or up |
| **M5 Route** | policy file, budget, effort pinning, cheap-model delegation, schema repair | Cost down at equal pass^k; no silent model change |
| **M6 Portability** | remaining adapters (Cursor, OpenCode, Devin Desktop, Cline, Kilo), trace resume, MCP gateway | Same install on 3+ harnesses |

## Building from source

Requires Go 1.26 or later; no cgo for this milestone.

```sh
make build            # static binary at bin/saga
make test             # go test ./...
make lint             # gofmt and go vet
make bench-hook       # hook cold start p50/p95 over 50 spawns
```

Then, in a repository you want traced:

```sh
saga init                                   # writes .saga/ (gitignore, config skeleton)
saga install --harness claude-code --dry-run  # shows the hook bindings; drop --dry-run to write them
saga doctor                                 # environment, hook registration, usage source, pins
saga trace ledger                           # per-call cost ledger of the latest session
```

## Read the research

| # | Document | Purpose |
|---|---|---|
| 01 | [Charter](docs/01-charter.md) | What Saga is, is not, principles |
| 02 | [Agent self-report](docs/02-agent-self-report.md) | First-person failure modes |
| 03 | [Harness architecture](docs/03-harness-architecture.md) | How agents are built; what is missing |
| 04 | [Ecosystem survey](docs/04-ecosystem-survey.md) | ~60 tools; what works, what is hype |
| 05 | [Code knowledge structures](docs/05-code-knowledge-structures.md) | Indexes, graphs, memory, determinism |
| 08 | [Reference repo review](docs/08-reference-repo-review.md) | Two reference repos: mechanisms, evidence, gaps |
| 09 | [Proposal and roadmap](docs/09-proposal-and-roadmap.md) | Architecture direction and milestones |

| ADR | Decision |
|---|---|
| [0001](docs/adr/0001-measurement-first.md) | Ship the measurement harness before any feature |
| [0002](docs/adr/0002-mechanisms-over-prompts.md) | Prefer machine-checked mechanisms over prompt text |
| [0003](docs/adr/0003-harness-agnostic-surface.md) | One core, three surfaces: CLI, MCP, thin hooks |
| [0004](docs/adr/0004-no-llm-narrative-memory.md) | Never inject LLM-written narrative as fact |
| [0005](docs/adr/0005-index-shape.md) | Deterministic AST graph, depth one, four tools |

Docs 06 and 07 are pending. See [docs/README.md](docs/README.md) for the maintained index.

## Principles

From [doc 01](docs/01-charter.md):

- Every rule that can be a check must be a check.
- Evidence over belief: "done" requires stored, reproducible oracle output.
- Collapse branch points before the run; scope and oracles remove ambiguity.
- Lookup beats memory; structure survives compaction, prose does not.
- Token cost is a first-class metric, reported per component.
- Model-agnostic by construction: CLI plus MCP, thin adapters.
- Local first, private by default.
- Measure or do not claim; cost versus time is explicit and logged.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT. See [LICENSE](LICENSE).
