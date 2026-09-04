# ADR 0009: Cut scope to an eight-week experiment on one hook, one harness, one number

**Status:** accepted, 2026-09-05

## Context

The red-team review (docs/11, §3 and §5) argues that the eight-layer design is the wrong first move: 63k words of specification, 24 cross-spec inconsistencies in one pass, and no measured number of its own. Its five arguments, in one line each:

| # | Argument | Evidence cited |
|---|---|---|
| 1 | The labs ship most of these mechanisms natively and co-train the model in the harness; only the cost ledger, canary, post-expansion classification and claim verification have no native equivalent | docs/03 §2.6, docs/06 Part B, docs/07 §8 |
| 2 | Injected tokens, hook latency and cache invalidation can erase the gains; nothing in the specs models the composed cost | contracts §7.3, guard-spec §11.4, claude-code #91514 |
| 3 | The bench is unaffordable at the scale the `publish` tier needs ($5.5k to $15k by its own run counts) and underpowered below it | bench-spec §4.4 as corrected, docs/06 A.10 |
| 4 | "Harness-agnostic" collapses on the hook matrix: one harness with contradictory docs, one in beta, one effectively closed | contracts §1.2, docs/06 Part B |
| 5 | The headline evidence is preprints, vendor posts and blogs assembled by agents and not read in full | docs/03 §5, docs/10 caveats |

The review's recommendation (§3.2) is the smallest thing that tests the thesis: on Claude Code with one frontier model, does a Stop hook that blocks "done" while declared oracles are unmet or the final message contradicts the trace reduce the false-done rate by more than the noise floor, without raising tokens per solved task by more than 10%. Eight weeks, under $3,000, one person.

The project owner accepted the recommendation on 2026-09-05. This ADR records the cut; docs/12-experiment-protocol.md is the pre-registration.

What already exists against the cut scope (docs/specs/IMPLEMENTATION-STATUS.md, 2026-09-03): trace events, chain, ledger and pins; the composed hook with the Claude Code adapter; gate contract grammar, oracle execution, evidence records, diff guards and the Stop step; the guard command classifier with the incident fixtures; the git-tree snapshot primitive; the bench runner, metrics and report; 20 verified tasks in TypeScript and Python. Not yet: claim verification, control-arm blocking, two-arm interleaving, containers, the guard hook wiring.

## Decision

### Kept, deferred, dropped

| Layer or surface | Decision | Scope kept | Red-team reason (docs/11 §3.1) |
|---|---|---|---|
| bench | Keep | TypeScript and Python only; `smoke`, `dev` and the experiment's own tiers; no badge until the power calculation is published from measured data | No native equivalent; "would differentiate from 100% of the tools" in docs/04 |
| trace | Keep | Event ledger, per-turn cost ledger, pins and change detection, claim verification (trace-spec §5.5 to §5.9) | Reconciliation is a real M0 exit; the ledger, canary and claim checks are the mechanisms no harness ships |
| trace, rest | Defer | Canary schedule, replay, fork, portable resume, watchdog actions | Scope, not thesis; the canary alone is $500 to $5,500 per month at the spec's cadence |
| gate | Keep | `CHECK`/`EXPECT` contract, Stop block on unmet oracles, Stop block on a contradicted claim (gate-spec §6 citing trace-spec §5.9) | "Claim verification is the thesis in one mechanism" |
| gate, rest | Defer | Red proof as a requirement (`require_red` off in the experiment; `RED: none` and `baseline` records stay), mutation operators, diff guards as blockers, traceability, reviewer contract, approvals workflow | Each is a separate experiment; none is needed to measure false-done |
| guard | Keep | bash and zsh post-expansion classifier on macOS and Linux, hard-deny list D1 to D11, incident fixtures, git-tree per-turn snapshot, `saga undo` | Community already runs the fix as a hook (#28240); it is the second experiment if the gate clears the floor |
| guard, rest | Defer | PowerShell, cmd, Windows, zfs, btrfs and reflink snapshot modes, secret masking, package existence check, MCP gateway, audit log | agent-guard already ships masking (docs/04 §2.5); Windows and the extra shells are adapter surface with no measured value |
| mem | Defer entirely | Nothing, the PreCompact state block included | Claude Code has auto-memory and `PreCompact`; targeted injection has "the largest gap between hype and evidence" (docs/09 §5) |
| index | Defer, revisit in a year | Nothing | codegraph, serena and built-in LSP exist; the leak-audited study is one model on one bench, p = 0.087 against agentic grep |
| shape | Defer | Nothing | rtk and context-mode exist; rewriting tool output reintroduces the train/test mismatch the labs optimise out |
| route | Drop from the roadmap | Nothing; the spec stays in the tree as a record | "Last, least evidence"; the price table is "expected to be wrong within months" |
| doctor | Keep minimal | Hooks registered, hooks fire, gate fixture fails when it should, `uninstall --dry-run` lists exactly what `install` wrote | Cheapest credibility; docs/07 §7: the tooling category's first issue class is install and uninstall |
| Harnesses | Claude Code only, plus the CLI for CI | One adapter, verified against the live release (docs/specs/harness-facts.md) | Argument 4; Codex and Gemini adapters wait for verified hook cells and a positive result |
| Surfaces | CLI and thin Claude Code hooks | No MCP server in this period | The MCP gateway and the index and mem tools are the only MCP consumers, all deferred |
| Languages | TypeScript and Python | 40 tasks total, 20 existing plus 20 to author | Go leaves M0; Swift and Dart wait for the M2-class corpus |

### Rules during the experiment

| # | Rule |
|---|---|
| 1 | No spec for index, mem, shape, route or the MCP gateway is edited until the experiment reports. The files stay at their paths (other specs and ADRs link to them); docs/README.md marks them deferred. The red team's proposed move to `docs/specs/deferred/` is not done, to keep links stable. |
| 2 | No code lands under `internal/index`, `internal/mem`, `internal/shape` or `internal/route`. |
| 3 | The pre-registration in docs/12 is frozen (content hash recorded in the bench manifest) before the first paid run; changes after that are appended as dated amendments, never edited in place. |
| 4 | Every number quoted about Saga in the README comes from a bench manifest linked beside it. Nothing from docs/03 to 07 that is graded secondary appears in the README (docs/11 §5 condition 5). |
| 5 | The kill rule in docs/12 is binding: a pilot delta inside the noise floor ends the project as scoped, and the bench is the product. |
| 6 | Composed hook overhead is measured on the experiment's arm B entry (trace plus gate), not on trace alone (docs/11 §5 condition 4). |

### Reversal condition

This ADR is reopened only by a positive, pre-registered result: the docs/12 primary hypothesis H1 supported at the full run (40 tasks, K = 5), with the cost condition H2 and the non-inferiority condition H3 holding, reported by `saga bench report` from a linked manifest. On that result:

| Order | Reopened | By |
|---|---|---|
| 1 | guard classifier as the second experiment (its own pre-registration, same protocol shape) | an amendment to docs/12 |
| 2 | mem state block, index, shape, in the order the guard result and the ledger's cost data justify | one new ADR per layer, each with a pre-registered ablation on the bench |
| 3 | route | a new ADR only, with a measured cost gap it would close |
| 4 | Codex and Gemini adapters, MCP surface, Windows | after two harness cells are verified per contracts §1.2 and a second harness is funded |

Two events close the reversal path regardless of the result: claim verification or post-expansion command classification ships natively in Claude Code before the report (docs/11 Argument 1 falsifier), in which case the residual product is the bench; or the owner withdraws the budget.

A null result does not reopen this ADR. It repositions Saga as bench-only (docs/11 §5 condition 6), and the deferred specs stay deferred.

## Consequences

Easier: one adapter to keep green against daily Claude Code releases; one composed hook (trace then gate) whose overhead can be measured honestly; a task corpus in two languages the owner can verify by hand; a budget that fits one person's card; a result, positive or null, inside eight weeks.

Harder: the README's positioning ("repository knowledge, memory that survives compaction, privacy at the tool boundary") now describes deferred work and is rewritten to what is measured; ADR 0008's layout keeps empty package directories for deferred layers; the single-harness choice means every result is a Claude Code result until a second harness is funded; the guard classifier exists but ships unmeasured, so it may not be cited.

Given up for this period: the eight-layer surface, three harnesses, seven languages, four shells, the canary schedule, the `publish` tier and its badge. Each returns only through the reversal condition above.
