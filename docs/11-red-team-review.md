# Red-team review: the case against Saga

*2026-09-03. Written to kill the project before code exists. Scope: docs 01, 09 (v0.4), 10, specs/00, specs/REVIEW-LOG, with docs 03 to 07 read for the evidence they are cited for. Nothing was edited. Quoted sentences are verbatim from the document named.*

## 0. Summary verdict

| Question | Answer |
|---|---|
| Is the thesis plausible (harness matters; most reported failures are attackable outside the model)? | Yes. Doc 07 §9 is the strongest part of the package. |
| Is the eight-layer design the right first move? | No. 63k words of specification, 24 cross-spec inconsistencies in one pass, zero measured numbers of its own. |
| What would test the thesis? | One hook, one harness, one number: does a Stop gate with claim verification cut false-done rate beyond the noise floor. Eight weeks, under $3k. |
| Recommendation | Do not ship Saga as scoped. Ship the eight-week experiment under the conditions in §5. |

## 1. Five arguments that Saga fails or is unnecessary

### Argument 1: the labs ship these mechanisms natively, co-trained with the model

**Steelman.** Every layer maps to something a harness already has. Worse, the labs train the model *in* the harness (doc 03 §2.6: RL "in the exact harness"; Cursor: "minimise train/test harness mismatch"). A layer that rewrites tool inputs (`updatedInput`), tool results (shape) and injects context mid-loop (mem) reintroduces the mismatch they are training out.

| Saga layer | Already shipped, per docs 03/06/07 | Left for Saga |
|---|---|---|
| guard | Claude Code auto-mode classifier (0.4% FP), `Read(.env*)` deny rules, `/rewind` file and transcript checkpoints, `sandbox-runtime` (84% fewer prompts); Codex kernel sandbox plus Guardian AI reviewer; Gemini checkpointing | post-expansion classification; undo of untracked files |
| mem | Claude Code auto-memory plus `PreCompact`/`SessionStart`, built from #34556; Codex native compaction; Cursor RL self-summarisation "reduces the error from compaction by 50%" | declared invariants |
| trace watchdog | Gemini loop detector; OpenHands `AgentStuckInLoopError` | output-aware detection |
| shape | Claude Code 25k tool-result cap, `ToolSearch` (77k to 8.7k tokens); Codex "retained reasoning and context compaction raised ... 13.3% to 38.3%" | runner parsers |
| index | Claude Code built-in LSP; Cursor semantic search on agent traces (+12.5%); codegraph, serena, graphify at 28k to 114k stars | regime gating |
| route | OpenHands multi-LLM routing; Claude Code per-subagent model | a table "expected to be wrong within months" (doc 09 §3.8) |
| gate | Kiro spec-driven hooks; unlazy ledger; Claude Code Stop hook | red proof, claim verification |
| bench | skill-creator Benchmark mode; JetBrains Harbor protocol; HAL | per-repo paired A/B |

Native adoption is fast: #73125 fixed "within 2 days"; #34556 became auto-memory; #7328 became `ToolSearch`; #353 became `/rewind`. Anthropic's guidance already says "move enforceable rules into hooks" (doc 03 §1.3).

**Evidence for Saga.** Doc 07 §8: "none of these is provided by a harness" holds for the cost ledger, pinning and canary; codex #28879 was "batch-closed without explanation"; Gemini CLI went closed-source for individuals (doc 06 Part B). Labs have commercial reasons not to ship a cost ledger or a public canary. Post-expansion validation is still a community hook (#28240).

**Falsifier.** If two of {post-expansion classifier, claim verification, cost ledger, canary} ship natively in Claude Code or Codex before M1 ends, the residual value is the bench alone.

**Mitigation.** Build only what has no native equivalent and no visible roadmap: ledger, canary, post-expansion classification, claim verification.

### Argument 2: injected tokens, hook latency and cache invalidation erase the gains

**Steelman.** Contracts §7.3 budgets 3,800 injected tokens per session, a 2,000-token prefix, 600 per compaction, 300 per sub-agent, and echoes up to 1,500 per index or shape call: 6k to 10k tokens of Saga before any layer has done anything. Doc 04 §3 recorded the pattern: caveman's "own rules cost 1-1.5k *input* tokens per turn, which at short sessions can exceed the saving." Doc 03 §1.2 cites the tool-use tax: "the gains from tools often fail to offset the 'tool-use tax'". Doc 05 §1.4: LSP tools cost Sonnet +118% tokens.

| Cost source | Spec figure | On one M-class task (200-turn cap, ~3 tool calls per turn) |
|---|---|---|
| Composed PreToolUse chain with snapshot, 10k files | p95 ≤ 300 ms (guard-spec §11.4) | 600 × 0.3 s = 3 min added at p95 on a 5 to 20 min task |
| Same on 100k-file monorepo | p95 2,000 ms (guard-spec §3.3) | 20 min per run, longer than the task |
| Seven layers per PreToolUse entry | trace, guard, route, mem, shape, gate, trace (contracts §1) | seven process invocations per tool call; no spec describes a long-lived server |
| Mid-conversation `additionalContext` | mem 200 per injection | claude-code #91514: "Warm prompt cache fully re-written ... seconds after ToolSearch / Skill / tool_result"; trace-spec's own example shows cache writes at "61% of cost; 3 full re-writes" |
| Dense tool results resident in context | index ≤ 1.5× control | codegraph measured +80% (doc 04 §2.4) |

Mem and index inject after the cached prefix, so every injection is a candidate cache rewrite at 1h TTL prices. No spec models this; doc 09 §7 item 3 lists it as open.

**Evidence for Saga.** Trace injects zero tokens (M0 exit); the ledger exposes every layer's cost; M2 and M4 exits are cost-bounded; the index is off below 20k lines.

**Falsifier.** M0's overhead bar (trace-spec §11.2, "wall ≤ +1%") applied to the *composed* M1 entry, plus `tokens_per_solved` all-layers versus bare.

**Mitigation.** One long-lived hook process over a socket, not seven CLI invocations. No mid-conversation `additionalContext` until cache-rewrite cost is measured. Snapshot per turn above 10k files.

### Argument 3: the bench is unaffordable at the scale its claims need, and underpowered below it

**Steelman.** The bench is load-bearing (ADR 0001). Its cost lines do not add up.

| Item | Spec | Arithmetic |
|---|---|---|
| `publish` tier | "≥ 2 families, ≥ 1 harness, ladder, ≥ 30/lang, 3 langs, K=10, $500-2,000" (bench-spec §4.4) | 2 models × 3 arms × 90 tasks × 10 = 5,400 runs. At the $2.30 per solve doc 05 §1.2 reports for Opus 4.7 on S/M tasks, about $12,000 before the 1.3× treatment multiplier and L tasks. Spec figure is 6 to 25× low. |
| Canary | "20 tasks daily plus a 60-run burst on any pin change" (doc 09 §5) | Claude Code releases "~daily" (doc 07 §8), so the burst fires most days: ~80 runs/day, roughly $5,000/month per model-harness pair |
| Power | noise floor "3.5 to 4.5 points" (doc 06 A.10) | Binomial SE at n=90, p≈0.5 is 5.3 pp per arm; a paired 5 pp effect needs several hundred tasks. Doc 09 §7 item 8 admits this is open. |
| `user` tier | "≤ 20 tasks, K=3, ≤ $20 ... directionally, report only, no badge" | SE ≈ 11 pp. Twenty dollars buys a coin flip; doc 09 §3.1 sells it as answering "did this change to my agent config help *here*". |
| Task authoring | 30 per language with "6 impossible, 6 hack-bait", hidden oracles, red-proof fixtures, contamination scans, macOS runners for Swift and Dart | JetBrains reused an existing 86-task bench and still spent ~$100 per tool plus a team's time. Authoring 90 to 210 tasks is uncosted. |

**Evidence for Saga.** Bench-spec §1.3 is unusually honest ("'X helps' (unqualified): Never"); interleaved arms and blocked controls are the right protocol.

**Falsifier.** M0 publishes the bare baseline with dollars, runs completed and CI width. Under $1,500 with CIs narrower than 8 pp and this argument is wrong.

**Mitigation.** Two languages for a year. Canary per release, triggered by `doctor` on a pin change, capped at 20 runs. Publish the power calculation before authoring tasks and size the set from it.

### Argument 4: "harness-agnostic" collapses on the hook matrix

**Steelman.** The seam is "the hook surface ... that the three major CLIs now share by design" (doc 01). Contracts §1.2 shows what that sharing is today:

| Capability | Claude Code | Codex CLI | Gemini CLI |
|---|---|---|---|
| Pre-tool add context (mem, index) | "two vendor pages disagree; probe" | "documented" | "none (verified)" |
| Post-tool block or rewrite (guard mask, shape) | "feedback only (verified)" | "assume feedback only (unverified)" | `block` replaces result |
| Pre-tool input rewrite (mask, route) | "documented; probe" | "probe" | verified |
| Stop block | verified, "cap 8 consecutive" | verified, "cap unverified" | verified |
| Post-compaction context | verified | documented | "none" |
| Harness status | daily releases; hook JSON changes unlogged (doc 07 §8) | hooks "beta", outside the sandbox | "ended service for free/AI Pro/Ultra individuals 18 Jun 2026, replaced by closed-source Antigravity CLI" (doc 06 Part B) |

Three hook-bearing harnesses: one with contradictory docs, one with beta hooks and three unverified semantics, one effectively dead for individuals. Mem targeted injection, guard masking on Codex and shape wrapping all sit on unverified cells (REVIEW-LOG risks 2, 3). Cursor, OpenCode, Cline and Kilo are "CLI plus MCP only until M6", so gate Stop and guard PreToolUse do not exist there. Doc 07 §7 predicts the result: "a 'harness-agnostic' layer that ships N adapters inherits N×bugs"; caveman and ponytail, far smaller, already carry Gemini, Codex, Copilot and PowerShell adapter bugs.

**Evidence for Saga.** Contracts is honest about verification state; `doctor` probes rather than assumes; CLI and CI fallbacks exist; one composed entry per event limits adapter code.

**Falsifier.** A one-week sprint verifying `additionalContext` placement on Claude Code and `updatedInput`, PostToolUse and Stop cap on Codex, then four weeks of `doctor` in CI against nightly harness releases with no silent breakage.

**Mitigation.** Claude Code is the only supported harness for the first release; CLI and CI are the portability story. No Codex or Gemini adapter until its cell is verified; remove every spec sentence that depends on an unverified cell.

### Argument 5: the evidence is preprints, vendor posts and blogs, assembled by agents and never read in full

**Steelman.** Doc 10 says both "The research supports the original claim more strongly than expected" and "Research reports 03 to 08 were written by agents and reviewed only through their summaries; spot-check any number before quoting it externally." Both cannot be the project's position. Doc 03 §5 lists the gaps: "No public benchmark isolates the value of a harness-agnostic verification layer"; "Memory-file efficacy ... is essentially unmeasured"; "Sandboxing/permission numbers are lab-internal"; "Many 2026 arXiv papers cited here are preprints".

| Headline in doc 09 §1 | Source grade per the research doc itself |
|---|---|
| "Harness variance 18.5 pp² vs model variance 2.4 pp²" | doc 03 §2.1: "Position paper with a small controlled experiment, directionally strong, N small" |
| "83.4% ... in Codex CLI vs 76.4% in Terminus 2" | doc 03 §2.1: "blogs and leaderboards, not papers, *(secondary, unverified)*" |
| "transfer +5 to +10 pp across model families" | one preprint on SWE-bench Verified, which doc 06 A.0 calls "dead as a discriminator" |
| "3.5 to 4.5 point standard error" | doc 06 A.10: "(MarkTechPost)" |
| "GPT-5.5 lied ~29% of the time" | doc 06 A.2: "Zvi on the system card" |
| "17% false-negative rate" | doc 03 §1.4: "(secondary, not confirmed on the primary page)"; doc 06 Part B states it as fact |
| "95% at turn 1 to 20-60% by turn 6-10" | doc 04 §2.7: "Compliance decay (Khare)", no link or identifier |
| "~40% higher secret-leak rate" | doc 06 C.2: Snyk and a dev.to post; doc 04 §2.5 says "~2× baseline (3.2%)" from GitGuardian |

**Evidence for Saga.** Doc 07 is primary data (334 JSON files, live reaction counts) and depends on no preprint. The bench exists because the team does not trust these numbers (ADR 0001).

**Falsifier.** A human reads docs 03 to 07 end to end and finds fewer than five material errors. Until then doc 09 §1 is a table of hypotheses, which is what charter principle 10 demands of everyone else.

**Mitigation.** Add a source-grade column (primary, preprint, vendor, secondary) to doc 09 §1; nothing secondary-grade reaches the README.

## 2. The eight-layer surface

| Measure | Value |
|---|---|
| Spec words before any code | 63k (doc 10) |
| Inconsistencies found in one pass | 24 (REVIEW-LOG): exit codes meaning different things per layer, four snapshot stores, two placeholder formats |
| Schema ids; trace event types; attribution keys; exit codes | 36; 15; 14; 8 |
| Shell grammars with exact expansion semantics | bash, zsh, PowerShell, cmd; "A resolved argv that differs from the shell's is a P0 bug" (guard-spec) |
| Snapshot back-ends | git-tree, zfs, btrfs, reflink |
| Languages needing graphs, test-impact maps and runner parsers | seven |
| Price-table rows "expected to be wrong within months" | nine (doc 09 §4.1) |
| Comparable single-purpose project | unlazy: "5,700 lines of hardened JS ... a heavy core for a single-maintainer skill" (doc 08 §2.6), for one gate ledger |

Doc 07 §7: "the tooling category's #1 issue class is install/uninstall, not intelligence". Saga wrote the compatibility contract first, which beats ruflo, but wrote it for eight products at once.

## 3. The scope cut

### 3.1 Layer by layer

| Layer | First public release | Reason |
|---|---|---|
| bench | Keep; TypeScript and Python only; `smoke` and `dev` tiers; no badge until power is published | No native equivalent |
| trace | Keep ledger, pins, change detection. Defer canary schedule, replay, portable resume, watchdog actions | Reconciliation is a real M0 exit; the rest is scope |
| gate | Keep Stop block on unmet oracles plus claim verification. Defer red proof, diff guards, traceability, reviewer contract, approvals | Claim verification is the thesis in one mechanism |
| guard | Keep bash/zsh post-expansion classifier on macOS and Linux, git-tree per-turn snapshot, incident deny list. Defer PowerShell, cmd, Windows, zfs/btrfs/reflink, masking, package check, MCP gateway | agent-guard already ships masking (doc 04 §2.5) |
| mem | Defer entirely, state block included | Claude Code has auto-memory plus `PreCompact`; targeted injection has "the largest gap between hype and evidence" (doc 09 §5) and rests on a contradicted harness fact |
| index | Defer a year | codegraph, serena, built-in LSP exist; the leak-audited study is one model, one bench |
| shape | Defer | rtk and context-mode exist; rewriting tool output breaks the train/test match the labs optimise |
| route | Drop from roadmap | "last, least evidence"; defaults expire in months |
| doctor | Keep minimal: hooks fire, fixture fails, uninstall proves itself | Cheapest credibility |
| Harnesses | Claude Code plus CLI for CI | Argument 4 |

### 3.2 The smallest thing that proves the thesis in eight weeks

**Claim.** On Claude Code with one frontier model, a Stop hook that blocks "done" while declared oracles are unmet or the final message contradicts the trace reduces false-done rate by more than the noise floor, without raising tokens per solved task by more than 10%.

| Week | Deliverable | Kill criterion |
|---|---|---|
| 1 | Verify `PreToolUse` `additionalContext`, `Stop` cap, `SessionStart` on compact against the live Claude Code release; commit the table | Stop block unreliable: stop |
| 2 | `saga trace` ledger from transcript JSONL, reconciled with provider usage on 20 sessions | Error > 5%: fix first |
| 3 to 4 | 40 tasks, TypeScript and Python, hidden oracles, 8 impossible, 8 hack-bait; bare baseline K=5 | CI width > 12 pp: add tasks, not layers |
| 5 | Gate: `CHECK`/`EXPECT` contract, Stop block, claim verification (edits exist, runner ran, claim vs diff) | |
| 6 to 7 | Paired bare vs bare+gate, K=5, interleaved, control blocked | Delta inside noise floor: publish the null, stop |
| 8 | Report: pass^5, false-done rate, tokens per solved, hook overhead, dollars; push | |

Budget: 40 × 2 × 5 × ~$2.50 ≈ $1,000 for the ablation; under $3,000 with baseline and reruns. One person. If the gate moves false-done rate, the guard classifier is the second experiment. If not, the product is the bench, which doc 04 §5 says "would differentiate from 100% of the tools above."

## 4. Ten claims that are overstated or under-sourced

| # | Doc | Sentence | Problem | Fix |
|---|---|---|---|---|
| 1 | 01 | "turns 'the agent probably got it right' into 'the agent demonstrably got it right, in one run'" | Doc 09 §7 item 2: "an oracle can still measure the wrong thing"; doc 03 §2.5: hidden oracles let "incompleteness [go] undetected". Saga proves oracle satisfaction, not correctness. | "demonstrably passed its declared oracles, in one run" |
| 2 | 09 §1 | "Prompt-only tools deliver a tenth of what they claim" | caveman 8.5/65 ≈ 13%; ponytail −10.3% cost vs claimed −20% (half), −15.4% code vs −54% (a quarter). Doc 04 says "shrank 4-7×". | "a seventh to a half of what they claim" |
| 3 | 09 §1 | "Structural index: localization 44% → 85%, resolve 41.9% → 50.4% at lower cost" | Doc 05 §1.2 gives the fair comparator, agentic grep, at 45.3% resolve, p=0.087: not significant. One model, one study. | Add "vs agentic grep 45.3%, p=0.087; one model" |
| 4 | 09 §1 | "a mid-run restart monitor gained +8.8 pp" | Doc 03 §2.2: "8.8 pp among intervened runs"; population effect unstated; preprint. | "+8.8 pp among intervened runs (preprint)" |
| 5 | 09 §1 | "The harness is co-equal with the weights on agentic benchmarks" | Its number is "(secondary, unverified)" in doc 03 §2.1; the 81.2 vs 51.9 gap also differs in split and effort. | Label vendor and secondary; drop "co-equal" |
| 6 | 09 §7 item 7 | "Per-turn snapshots are free on APFS and ZFS" | Contracts §5: "never an APFS volume snapshot"; ADR 0006: "APFS or ZFS snapshot where available". Three positions. | Align to contracts §5 |
| 7 | 01 principle 8 | "AI-assisted code has a ~40% higher secret-leak rate" | Doc 06 C.2: Snyk and dev.to; doc 04 §2.5: "~2× baseline (3.2%)" from GitGuardian. Two numbers, no primary in the charter. | Name the report, pick one figure |
| 8 | 01 | "every mechanism Saga proposes has already been built ad hoc by a user inside an issue thread and shipped by no harness across more than one product" | Doc 06 Part B lists `/rewind`, auto-memory, `PreCompact`, the classifier, sandboxes, loop detectors and `ToolSearch` as shipped. "Across more than one product" hides "shipped by at least one". | "each shipped by at most one harness; the ledger, canary and post-expansion validation by none" |
| 9 | 09 §3.1 | "Runs on the user's own repo for under $20 so a team can answer 'did this change to my agent config help *here*'" | bench-spec §4.4 `user` tier: "directionally, report only, no badge", K=3, SE ≈ 11 pp. Cannot answer at the noise floor the same doc cites. | "gives a directional, unbadged reading for under $20" |
| 10 | 09 §1 | "type-checker loop +3.5 to +37% pass@1" | Doc 05 §6 attributes the range to "type-constrained decoding", which needs decoder access and is unavailable on hosted APIs. | Cite only harness-side compile-in-loop results |

Also: doc 09 §3.5 "43% of hallucinations recur" vs doc 03 §2.7 "58% recur across 10 runs" for the same paper; and doc 09 §1 omits the ETH finding that "Claude Code was the one agent where even human files didn't help".

## 5. Ship or do not ship

**Do not ship Saga as specified.** Eight layers, seven languages, four shells, three harnesses and 36 schemas, on evidence the brief itself says to "spot-check ... before quoting", is the shape of the projects doc 07 §7 catalogues, with better documents.

**Ship the eight-week experiment in §3.2**, on these conditions:

| # | Condition |
|---|---|
| 1 | Harness facts verified on live Claude Code before adapter code; contracts §1.2 "probe" and "documented" cells become "verified" or the dependent spec text is removed |
| 2 | M0 publishes real dollars, runs and CI width for the bare baseline; bench-spec §4.4's cost column is corrected from measurement |
| 3 | No spec for index, mem, shape, route or the MCP gateway is touched until the gate ablation has a number; those specs move to `docs/specs/deferred/` |
| 4 | Composed-hook overhead is measured on the M1 entry, not trace alone: p95 per tool call and tokens per solved task in the report |
| 5 | Doc 09 §1 gains a source-grade column and the ten fixes above; nothing secondary-grade appears in the README |
| 6 | Pre-registered kill rule: false-done delta with CI including zero at K=5 ends the gate layer and repositions Saga as bench-only |
| 7 | One human reads docs 03 to 07 end to end before the repository is pushed |

If conditions 1, 2 and 6 hold and the gate clears the noise floor, the project has earned the guard classifier as its second experiment and a public push. If not, the bench is the product, and that is still more than any of the ~60 tools in doc 04 has.
