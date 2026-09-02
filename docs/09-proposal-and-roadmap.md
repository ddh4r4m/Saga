# Saga: proposal and roadmap

*Draft v0.2, 2026-09-02. Synthesises docs 02, 03, 04, 05 and 08. Sections that depend on the per-model gap map (doc 06) and the GitHub issue mining (doc 07) are marked and will be revised when those land.*

---

## 1. The thesis, now with evidence

The charter's claim was that most reported agent failures are not model failures. The research supports it more strongly than expected:

| Claim | Evidence | Source |
|---|---|---|
| The harness matters more than the model, among frontier models | Harness variance 18.5 pp² vs model variance 2.4 pp² (7.8×) on SWE-bench Verified; harness changes transfer +5 to +10 pp across model families | doc 03 §2.1 |
| Failures are drift, not incapacity | Each off-path tool call raises the chance of the next by 22.7 pp; a mid-run restart monitor gained +8.8 pp | doc 03 §2.2 |
| Agents build to the visible check | With a visible oracle, near-perfect scores masked dead code; with it hidden, incompleteness went undetected | doc 03 §2.5 |
| Prompt text decays | Compliance odds fall 5.6% per function written; 95% at turn 1 to 20–60% by turn 6–10; LLM-generated AGENTS.md reduces resolve rate 3% while adding 20% cost | doc 04 §2.7 |
| Prompt-only tools deliver a tenth of what they claim | caveman −8.5% output tokens (claimed 65%); ponytail −10% cost (claimed 54%); no quality change either way | doc 04 §3 |
| Deterministic structure works | Structural index: localization 44% → 85%, resolve 41.9% → 50.4% at lower cost; test-impact map −70% regressions; type-checker loop +3.5 to +37% pass@1 | doc 05 §1.2, §7 |
| Memory storage is solved, injection is not | 8,785 stored observations never surfaced; self-memory underperformed plain retrieval; only procedural "what worked" memory shows gains | doc 04 §2.4, doc 05 §3 |
| Model-level determinism is impossible on hosted APIs | 80 unique outputs in 1,000 temperature-0 runs; cause is batch-variant kernels and MoE routing | doc 05 §4.1 |
| Verification is gameable by prompting alone | One anti-hack line cut reward hacking 52% → 18%, but Opus 4.6 fabricated content where "a prompt to not do it did not make this go away" | doc 03 §3 |
| Nobody measures | Of ~60 tools, two have an independent paired A/B, both from one JetBrains team | doc 04 §5.1 |

The gap that training will not close in the next one to two years is therefore concrete: **repository-specific structure, machine-checked completion, drift detection, targeted rule injection, and privacy at the tool boundary.** Every one of those lives outside the model, and the labs' own harnesses only partially cover them.

## 2. What Saga will not do, restated with reasons

- **No prompt-only skills as the product.** Evidence: ~10% cost effect, 0% quality effect, decays within a session. Saga ships prompt text only as thin adapters over checked mechanisms, each with a measured token cost.
- **No LLM-extracted knowledge graph of the code.** Evidence: 10⁷–10⁸ tokens to build, relation errors, and no advantage over deterministic AST graphs for code. Code already has a schema.
- **No LLM-written narrative memory injected as fact.** Evidence: net negative on coding outcomes.
- **No multi-agent write parallelism by default.** Evidence: raises total tokens, fragments decisions; read-only sub-agents and a separate-context reviewer are the parts that work.
- **No unsigned auto-executing hooks.** Evidence: the two largest "enhancer" repos shipped a CVSS 10.0 RCE and a malware clone.

## 3. Architecture

One local binary, `saga`, that exposes the same capabilities three ways: as a **CLI** (works with anything that can run a shell command), as an **MCP server** (for harnesses that speak MCP), and through **thin hook adapters** per harness (Claude Code hooks, Codex hooks/notify, Gemini CLI hooks, Cursor rules, OpenCode plugins, plus a CI fallback). The core has zero knowledge of any vendor's prompt format.

```
                 ┌──────────────────────────────────────────────┐
                 │  harness (Claude Code / Codex / Gemini / …)   │
                 └───────┬───────────────┬───────────────┬──────┘
                    hooks│           MCP │           CLI │
                 ┌───────▼───────────────▼───────────────▼──────┐
                 │                    saga                       │
                 │  bench · gate · index · mem · guard · shape   │
                 │              trace · route                    │
                 └───────┬───────────────┬───────────────┬──────┘
                         │ .saga/ (content-addressed local state) │
                         └────────────────────────────────────────┘
```

### 3.1 `saga bench`: the measurement harness (ships first)

Nothing else in Saga is allowed to claim an effect without this.

- Paired A/B: same task, same model, with and without one component; the control arm is prevented from reaching the component (the codegraph discipline, doc 04 §2.4).
- K independent clean-room runs per cell; report **pass^k**, per-task medians, Wilcoxon, tokens, wall time, and cost. Variance is a first-class output.
- Task set: 20–50 tasks per language drawn from real failures, across TypeScript, Python, Go, Rust, Java/Kotlin, Swift, and Dart, each with **hidden** oracles the agent never sees, and a known-broken positive control for every absence check.
- Runs on the user's own repo for under $20 so a team can answer "did this change to my agent config help *here*".
- Publishes negative results in the repo.

### 3.2 `saga gate`: contract before, evidence after

Adopts unlazy's ledger grammar (doc 08 §2.5) and generalises it.

- **Contract**: before work starts, a small machine-readable file: in-scope paths, out-of-scope paths, allowed side effects, and gates (`CHECK` command, `EXPECT` output). Saga helps derive it from the request, but the human or the agent writes it; the checker never infers semantics from prose.
- **Red proof**: a gate is not trusted until it has been observed failing at least once against a known-broken state. This turns unlazy's negative-control rule into a tool and answers "does this check actually check anything".
- **Evidence**: exit 0 and marker match, hashed output, resolved environment, stored under `.saga/`. `ABANDON` is a first-class terminal, non-successful state.
- **Diff guards** (deterministic, run on every edit and at Stop): touched path outside scope; test file deleted or assertion weakened; `skip`/`xfail`/`only` added; expected value hard-coded to match an observed failure; new dependency not in lockfile or registry. Each flags a mandatory, logged justification.
- **Traceability**: every gate carries a pointer to the sentence of the request it discharges; an outcome in the request with no gate is reported as uncovered.
- **Stop enforcement**: harness hook where one exists (Claude Code Stop, Gemini AfterAgent, Codex notify), CI check otherwise. The hook blocks "done" while gates are unmet or uncovered.

### 3.3 `saga index`: the code knowledge layer

Follows the recommended architecture in doc 05 §9, which is the only design with three independent studies behind it.

- Tree-sitter definition/reference graph, per-file fact sets keyed by content hash; BM25 lexical index; AST-aligned chunk embeddings optional and model-swappable; test-impact map from imports and call graph plus last coverage; lazy LSP overlay only for find-references and rename.
- Four agent-facing tools: `search`, `symbol` (depth 1, signatures not bodies), `read`, `impact`. Grep stays available.
- Incremental update in about a second on file change; the whole index versioned by the hash of sorted file hashes; tool results cached under `(tool, args, index_version)` so replays are exact and survive sessions.
- A ≤1k-token repo map lives in the cached prompt prefix and changes only when the index version changes; everything else is injected as tool results so prompt caching keeps its 90–97.5% discount.
- Regime-aware: off by default below a size threshold, because every structural tool in the survey is net negative on small repos (doc 04 §5.5).

### 3.4 `saga mem`: typed memory with targeted injection

- Record types: **environment facts** (build/test commands, toolchain versions), **conventions** (human-written, short), **decisions and constraints** (with the turn that produced them, so they survive compaction verbatim), **procedures** (what worked and what failed in this repo, the only kind of learned memory with positive evidence), and **pitfalls**.
- No narrative summaries of the code. The index is the memory of the code.
- **Targeted injection**: a PreToolUse adapter injects only the two or three records that apply to the path or tool about to be used, and nothing else. This attacks compliance decay directly and is the gap doc 04 §5.2 says nobody has filled.
- Freshness: every record that names a path or command is checked on read; stale records are flagged, not injected.
- Sub-agent preamble: the constraints and decisions are handed to every sub-agent verbatim as a small structured block.

### 3.5 `saga guard`: privacy and capability reduction at the tool boundary

- Deterministic secret and PII masking on every tool result and prompt before it leaves the machine: gitleaks-class rules plus configurable patterns, optional Presidio, reversible placeholders, a coverage benchmark with positive controls (the agent-guard pattern, doc 04 §2.5).
- Package existence check against registry and lockfile before any install or import is accepted. This removes the 19.7% hallucinated-package class entirely at negligible cost.
- Dangerous-command policy (destructive git, recursive delete, credential reads) with an allow-list model, compatible with OS sandboxes rather than replacing them.
- Signed releases, pinned hook scripts with hashes, no self-updating `git pull`, a manifest declaring exactly what executes at which lifecycle event.

### 3.6 `saga shape`: structured tool output and caching

- Parsers for common test runners, compilers, linters, and build tools that return failing test names, first error with file and line, exit code, and a pointer to the full log, instead of the raw stream.
- Head/tail truncation with error-aware tail retention and never mid-line; default caps aligned with the SWE-agent ablations.
- Content-addressed cache of tool results keyed on command, arguments, and the hashes of relevant files.

### 3.7 `saga trace`: the event log, cost attribution, and drift detection

- Append-only event log keyed by content hashes so a run can be replayed or forked without re-calling the model (the "log is the agent" pattern, doc 05 §4.2).
- Per-component token attribution: what each installed piece added to context per session, so the honest "token optimizer" is a profiler.
- Drift monitor: flags trajectories that leave the expected tool-call path (repeated failed edits, repeated identical commands, scope violations) and suggests restart from the last good checkpoint.
- Portable trace format so a session can be resumed in a different harness.

### 3.8 `saga route`: model and effort routing (last, least evidence)

- Policy file mapping task class to model and reasoning effort, with a visible budget. Evidence for routing is weaker than for the other layers and it is model-specific, so it ships after the benchmark can measure it. *(To be refined against doc 06.)*

## 4. What each layer does for the complaints in doc 02

| Complaint | Layers |
|---|---|
| "Said done, does not work" | gate (red proof, evidence, Stop enforcement), shape |
| "Forgot what we agreed" | mem (decisions survive compaction), trace |
| "Ignored CLAUDE.md" | mem (targeted injection), gate (rules become checks) |
| "Touched what I did not ask" | gate (scope contract, diff guards) |
| "Stopped halfway" | gate (uncovered outcomes, ABANDON) |
| "Deleted or weakened my test" | gate (diff guards) |
| "Invented an API or package" | guard (existence check), index (external API cache), shape (type-checker loop) |
| "Same prompt, different result" | gate (collapses branch points), trace (variance measured), index (same facts every run) |
| "Burned tokens reading the repo" | index, shape, cache |
| "Leaked my secrets" | guard |

## 5. Roadmap

| Milestone | Scope | Exit criterion |
|---|---|---|
| **M0 Measure** | `bench` + `trace` core, task set for 3 languages, one adapter (Claude Code) | Bare harness baseline published with pass^k and variance for 2 models |
| **M1 Gate + Guard** | ledger, red proof, diff guards, Stop adapters (Claude Code, Gemini CLI, Codex, CI), secret masking, package existence check | Measured effect on M0 task set; zero false-positive masking on the positive-control suite |
| **M2 Index** | tree-sitter graph, BM25, test-impact map, 4 tools, MCP server, incremental update, regime gating | Localization and resolve delta reproduced on the M0 set with control arm blocked |
| **M3 Memory** | typed records, targeted PreToolUse injection, freshness, sub-agent preamble | Compliance-over-session-length curve measured with and without |
| **M4 Shape** | output parsers for the top runners per language, error-aware truncation, result cache | Token per completed task down with correctness flat or up |
| **M5 Route** | policy file, budget, cheap-model delegation for read-only sub-tasks | Cost down at equal pass^k |
| **M6 Portability** | remaining adapters (Cursor, OpenCode, Windsurf, Cline), portable trace resume | Same install on ≥3 harnesses, no per-harness prompt rewrites |

Order rationale: the strongest evidence and smallest surface are in gates and guards; the index has the largest measured gain but a bigger build; memory has the largest gap between hype and evidence, so it comes after the benchmark can catch a regression.

## 6. Cost versus time

Default order of preference: correctness, then cost, then speed. Concretely: a gate run that costs a minute is always preferred to skipping it; a cheaper model is preferred for read-only exploration; a structural index is preferred to re-reading files. The user can flip any of these per repo in `.saga/config`. Every flip is logged in the trace so the benchmark can show what it cost.

## 7. Open questions carried forward

1. Line-level localization recall (about 15%) is the bottleneck no index fixes; a learned reranker on PR data may be needed. *(doc 05 §10)*
2. Gate quality beyond lexical lint: red proof helps, but an oracle can still measure the wrong thing.
3. Interaction between dynamic knowledge injection and stable prompt prefixes for caching.
4. Which per-model differences justify routing rules. *(pending doc 06)*
5. Which harness-level pain points from issue trackers are unsolved for structural reasons and which are just backlog. *(pending doc 07)*

## 8. Naming and positioning

Saga is "the story humanity keeps telling": the same complaints, every model generation. The project's job is to end the retelling by making the story checkable. Positioning in one line for the README: **Saga makes any coding agent prove its work: a local, model-agnostic layer for repository knowledge, machine-checked completion, memory that survives compaction, and privacy at the tool boundary, with a benchmark that keeps it honest.**
