# Saga: project charter

*Status: draft v0.1, 2026-09-02. Sections marked (evidence pending) will be revised once the research reports in this folder are finalised.*

## One sentence

Saga is a model-agnostic, harness-agnostic layer that turns "the agent probably got it right" into "the agent demonstrably got it right, in one run", by giving any coding agent three things training cannot give it: **knowledge of your repository, a machine-checked definition of done, and memory that survives compaction.**

## Why this exists

Frontier models improve every few months. The complaints from people who use them for real engineering do not change:

- it said done, but it does not work
- it forgot what we agreed an hour ago
- it ignored my instructions file
- it touched things I did not ask it to touch
- it invented an API
- same prompt, different result

None of those is a knowledge or reasoning gap that the next training run will close. They are gaps between what the model is **asked to remember, believe, or guess** and what it could instead be **told, shown, or given**. That gap is the product.

The first-person account of these failures, written by the agent itself before any external research, is in [02-agent-self-report.md](02-agent-self-report.md). The external evidence is in the research documents that follow.

## What Saga is

1. **A code knowledge layer.** A local, incrementally updated structural index of a repository (symbols, references, call graph, test-to-code mapping, module ownership, conventions) with a query interface small enough to expose as a handful of tools. Language-agnostic where possible (tree-sitter, LSP), language-specific where it pays.
2. **A contract and gate layer.** Before work starts, a small machine-readable scope contract: in-scope paths, out-of-scope paths, success oracles (commands and expected outputs), allowed side effects. After work, a gate runner that executes the oracles and refuses to let "done" be said without evidence. Diff guards flag scope violations and test-weakening.
3. **A typed memory layer.** Decisions, constraints, environment facts, conventions, and known pitfalls stored as separate, queryable, verifiable records, outside the transcript, so they survive compaction and session boundaries, and are handed to every sub-agent verbatim.
4. **A privacy layer.** Local, deterministic redaction of secrets and PII before content leaves the machine, with reversible placeholders.
5. **A routing and budget layer.** Visible token budget, cost-aware model selection for sub-tasks, and structured tool output so the agent reads signal instead of logs.

Each layer is independently installable and works with Claude Code, Codex CLI, Gemini CLI, Cursor, OpenCode, and anything that can call a CLI or an MCP server.

## What Saga is not

- **Not a skills repository.** Prompt text that asks the model to behave is the weakest mechanism available and is already well served by other projects. Saga ships prompts only as thin adapters over machine-checked tools.
- **Not a new agent or harness.** Saga does not own the loop. It plugs into whatever loop the user already has.
- **Not a process framework.** It does not prescribe planning ceremonies. It supplies the primitives those frameworks lack.
- **Not a model wrapper or proxy** that rewrites prompts in flight.

## Design principles (evidence pending, to be confirmed against research)

1. **Every rule that can be a check must be a check.** A sentence in a memory file decays; a hook, linter, or gate does not.
2. **Evidence over belief.** "Done" requires oracle output, not a claim. Evidence is stored, hashed, and reproducible.
3. **Collapse branch points before the run.** Run-to-run variance comes mostly from decisions taken under ambiguity, not from token sampling. Declared scope and oracles remove the coin flips.
4. **Lookup beats memory.** Anything the agent would otherwise re-derive (build commands, conventions, symbol locations) is indexed and queried.
5. **Structure survives compaction; prose does not.** Constraints and decisions live in typed records outside the transcript.
6. **Token cost is a first-class metric.** Every component reports what it adds to context. Prefer just-in-time retrieval to preloading.
7. **Model-agnostic by construction.** No component may depend on a single vendor's prompt format. Adapters are thin; the core is CLI plus MCP.
8. **Local first, private by default.** Indexes, memory, and redaction run on the user's machine. Nothing leaves without passing the privacy layer.
9. **Measure or do not claim.** Every feature ships with a benchmark or is labelled experimental. Marketing claims from other projects are treated as hypotheses.
10. **Cost versus time is an explicit setting**, never an accident. When a trade-off is unavoidable, the default favours correctness, then cost, then speed, and the user can flip it.

## Success criteria (to be quantified after research)

- **Consistency.** Same task, N independent runs: the fraction of runs that pass the declared oracles rises measurably versus the bare harness.
- **First-run correctness.** Pass@1 on a held-out set of real repository tasks (multi-language) rises versus the bare harness, with the same model.
- **Token cost.** Total tokens per completed task falls or stays flat while correctness rises.
- **Portability.** The same install works on at least three harnesses without per-harness prompt rewrites.
- **Adoption signal.** People install one layer without the others and keep it.

## Evaluation approach

A reproducible benchmark harness is part of the project, not an afterthought: a set of tasks across TypeScript, Python, Go, Rust, Java/Kotlin, Swift, and Dart repositories, each with hidden oracles; each configuration (bare harness, harness plus one Saga layer, harness plus all layers) run K times per model. Variance is reported, not just the mean.

## Scope of the first milestone

Decided after the research phase. The candidate order, based on the self-report and pending evidence, is: gate and contract layer first (highest leverage, smallest surface), knowledge layer second, memory third, privacy fourth, routing fifth.

## Governance

MIT licence. Open roadmap. Every design decision recorded as an ADR in `docs/adr/`. Benchmarks and their raw results committed alongside claims.
