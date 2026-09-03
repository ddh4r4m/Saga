# Saga: project charter

*Status: draft v0.2, 2026-09-02. v0.1 carried "(evidence pending)" markers; v0.2 replaces each with the document that now supplies the evidence, and quantifies the success criteria against [doc 09](09-proposal-and-roadmap.md).*

## One sentence

Saga is a model-agnostic, harness-agnostic layer that turns "the agent probably got it right" into "the agent demonstrably passed its declared oracles, in one run", by giving any coding agent three things training cannot give it: **knowledge of your repository, a machine-checked definition of done, and memory that survives compaction.**

## Why this exists

Frontier models improve every few months. The complaints from people who use them for real engineering do not change:

- it said done, but it does not work
- it forgot what we agreed an hour ago
- it ignored my instructions file
- it touched things I did not ask it to touch
- it invented an API
- same prompt, different result
- it deleted things it had no business touching
- it cost far more than it should have, and nobody can say why

None of those is a knowledge or reasoning gap that the next training run will close. They are gaps between what the model is **asked to remember, believe, or guess** and what it could instead be **told, shown, or given**. That gap is the product.

The first-person account of these failures, written by the agent itself before any external research, is in [02-agent-self-report.md](02-agent-self-report.md). The external evidence is in the research documents that follow, and the evidence table in [doc 09 §1](09-proposal-and-roadmap.md) is the summary. Two findings from the issue mining ([doc 07](07-github-issue-mining.md)) shape the charter most: every mechanism Saga proposes has already been built ad hoc by a user inside an issue thread, each has since been shipped natively by at most one harness, and the cost ledger, canary and post-expansion command validation by none (§9; doc 06 Part B), and most 2025-26 data-loss incidents were shell-expansion and permission-layer bugs rather than model intent ([doc 06 D.2](06-model-and-harness-gap-map.md)).

## What Saga is

1. **A code knowledge layer.** A local, incrementally updated structural index of a repository (symbols, references, call graph, test-to-code mapping, module ownership, conventions) with a query interface small enough to expose as a handful of tools. Language-agnostic where possible (tree-sitter, LSP), language-specific where it pays. Evidence: doc 05 §1.2, ADR 0005.
2. **A contract and gate layer.** Before work starts, a small machine-readable scope contract: in-scope paths, out-of-scope paths, success oracles (commands and expected outputs), allowed side effects. After work, a gate runner that executes the oracles, verifies the agent's claims against the trace, and refuses to let "done" be said without evidence. Diff guards flag scope violations and test-weakening. Evidence: doc 03 §2.5, §3; doc 08 §2.5; doc 07 §6 items 3 and 4.
3. **A typed memory layer.** Decisions, constraints, environment facts, conventions, and known pitfalls stored as separate, queryable, verifiable records, outside the transcript, so they survive compaction and session boundaries, and are handed to every sub-agent verbatim. Evidence: doc 04 §2.4, §5.2; doc 05 §3; doc 07 §6 item 2; ADR 0004.
4. **A guard layer.** Deterministic validation of every shell command on its post-expansion form, per-turn filesystem snapshots with undo, a package existence check, and local, deterministic redaction of secrets and PII before content leaves the machine, with reversible placeholders. Evidence: doc 06 A.1, C.3, C.4, D.2 item 1; doc 07 §6 items 5 and 6; ADR 0006.
5. **A trace, cost, and routing layer.** Append-only event log, per-turn cost ledger with cache accounting, harness and model version pinning with canary evals, loop and stall watchdog, visible token budget, and cost-aware model selection for sub-tasks, with structured tool output so the agent reads signal instead of logs. Evidence: doc 05 §4.2; doc 07 §6 items 1, 7, 8 and 9, §8; doc 06 Part D; ADR 0007.

Each layer is independently installable and works with Claude Code, Codex CLI, Gemini CLI, Cursor, OpenCode, and anything that can call a CLI or an MCP server. The seam is the hook surface (PreToolUse, PostToolUse, Stop, SessionStart, PreCompact) that the three major CLIs now share by design (doc 07 §5).

## What Saga is not

- **Not a skills repository.** Prompt text that asks the model to behave is the weakest mechanism available and is already well served by other projects. Saga ships prompts only as thin adapters over machine-checked tools. Evidence: doc 04 §3, doc 07 §7, ADR 0002.
- **Not a new agent or harness.** Saga does not own the loop. It plugs into whatever loop the user already has. ADR 0003.
- **Not a process framework.** It does not prescribe planning ceremonies. It supplies the primitives those frameworks lack. Evidence: doc 04 §2.2, doc 07 §7 ("workflows that cannot be partially applied get abandoned").
- **Not a model wrapper or proxy** that rewrites prompts in flight.
- **Not a sandbox.** Guard runs in front of an OS sandbox or container and never replaces one (doc 07 §6 item 17).

## Design principles

Each principle now cites the document that supports it; doc 09 §3.10 lists what no principle can deliver.

1. **Every rule that can be a check must be a check.** A sentence in a memory file decays; a hook, linter, or gate does not. Evidence: compliance odds fall 5.6% per function and reach 20-60% by turn 6-10 (doc 04 §2.7); rules re-injected every turn still drift (caveman #303, doc 07 §4 item 5).
2. **Evidence over belief.** "Done" requires oracle output, not a claim. Evidence is stored, hashed, and reproducible. Evidence: agents build to the visible check (doc 03 §2.5); GPT-5.5 lied about impossible tasks ~29% of the time (doc 06 A.2); "the trace is the only ground truth" (cline #4384, doc 07 §6 item 4).
3. **Collapse branch points before the run.** Run-to-run variance comes mostly from decisions taken under ambiguity, not from token sampling. Declared scope and oracles remove the coin flips. Evidence: doc 02 §1; failures are stochastic drift from a canonical path, +22.7 pp per off-path call (doc 03 §2.2); variance concentrates in a small set of unstable instances (doc 06 A.10).
4. **Lookup beats memory.** Anything the agent would otherwise re-derive (build commands, conventions, symbol locations) is indexed and queried. Evidence: structural index raised localization 44% → 85% (doc 05 §1.2).
5. **Structure survives compaction; prose does not.** Constraints and decisions live in typed records outside the transcript. Evidence: claude-code #21925, #24460, #34556 (doc 07 §4 item 4); only procedural memory shows gains (doc 05 §3).
6. **Token cost is a first-class metric.** Every component reports what it adds to context. Prefer just-in-time retrieval to preloading. Evidence: cost opacity is the top-ranked cross-harness problem (doc 07 §6 item 1); always-on injection is the top complaint against the tooling repos (doc 07 §7).
7. **Model-agnostic by construction.** No component may depend on a single vendor's prompt format. Adapters are thin; the core is CLI plus MCP. Evidence: harness effects transfer +5 to +10 pp across model families (doc 03 §2.1); N adapters inherit N× bugs unless the core is one executable (doc 07 §7).
8. **Local first, private by default.** Indexes, memory, and redaction run on the user's machine. Nothing leaves without passing the guard layer. Evidence: doc 06 C.1, C.2; GitGuardian's State of Secrets Sprawl 2026 measured secrets in Claude-Code-co-authored commits at about 2× the baseline rate (the 3.2% figure quoted in doc 04 §2.5).
9. **Validate the command that will run, not the string the model wrote.** Permission decisions are made on the post-expansion, per-segment form of every shell command, and every write is preceded by a snapshot. Evidence: doc 06 D.2 item 1 and C.3 ("validators check the raw string, execution happens post-expansion"); ADR 0006.
10. **Measure or do not claim.** Every feature ships with a benchmark or is labelled experimental. Marketing claims from other projects are treated as hypotheses. Evidence: two independent A/Bs among ~60 tools (doc 04 §5.1); ADR 0001.
11. **Pin, record, verify, canary.** Harness version, model id, and effort are recorded per turn; changes are surfaced as events; a fixed regression subset runs on a schedule. Evidence: doc 07 §8; #42796, #46829, #46917 (doc 09 §1); ADR 0007.
12. **Cost versus time is an explicit setting**, never an accident. When a trade-off is unavoidable, the default favours correctness, then cost, then speed, and the user can flip it. Every flip is logged.

## Success criteria

Quantified against doc 09 §3.1 and §5. All comparisons are paired: same task set, same model, same harness, with the control arm blocked from reaching the component (ADR 0001).

- **Consistency.** pass^k over K ≥ 5 independent clean-room runs on the bench task set rises versus the bare harness by more than the per-model standard error of 3.5 to 4.5 points (doc 06 A.10); anything smaller is reported as within noise.
- **First-run correctness.** pass@1 on the held-out multi-language task set (TypeScript, Python, Go, Rust, Java/Kotlin, Swift, Dart) rises versus the bare harness with the same model, with hidden oracles the agent never sees.
- **Token cost.** Tokens and dollars per completed task fall or stay flat while correctness rises; the per-turn ledger reconciles with provider-reported usage on the same sessions; every installed component's contribution is attributed.
- **Safety.** Zero escapes on the destructive-command positive-control suite seeded from every incident in doc 06 A.1, A.3 and C.3; zero false-positive masking on the secret and PII control suite; `saga undo` restores the pre-turn tree including untracked files.
- **Compaction survival.** Declared decisions and open gates are present verbatim in context after every compaction, measured by the bench across forced compactions.
- **Detection.** A harness or model version change, or a vendor fallback, is surfaced as an event within the session it occurs; the canary reports a regression larger than its confidence bound within one scheduled cycle.
- **Portability.** The same install works on at least three harnesses, including on Windows, without per-harness prompt rewrites (doc 07 §6 item 14).
- **Adoption signal.** People install one layer without the others and keep it; `saga doctor` shows a non-zero fire rate for every adapter hook on every supported harness.

## Evaluation approach

A reproducible benchmark harness is part of the project, not an afterthought: a set of tasks across TypeScript, Python, Go, Rust, Java/Kotlin, Swift, and Dart repositories, each with hidden oracles; each configuration (bare harness, harness plus one Saga layer, harness plus all layers) run K times per model. Variance is reported, not just the mean. SWE-bench Verified is not used as a discriminator (doc 06 A.0). A 10 to 20 task subset doubles as the canary.

## Scope of the first milestone

Decided after the research phase; see doc 09 §5. M0 is measurement (`bench`, `trace` with the cost ledger and version pinning, `doctor`). M1 is the gate and guard layer plus compaction survival, because the evidence is strongest and the surface smallest there. The index, targeted memory injection, output shaping, routing, and remaining adapters follow in that order.

## Governance

MIT licence. Open roadmap. Every design decision recorded as an ADR in `docs/adr/`. Benchmarks and their raw results committed alongside claims. Negative results are committed too.
