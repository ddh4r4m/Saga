# Saga: proposal and roadmap

*Draft v0.4, 2026-09-03. Synthesises docs 02 through 08. v0.3 revised v0.2 against the per-model gap map (doc 06) and the GitHub issue mining (doc 07): the evidence table gained seven rows, `guard`, `trace`, `mem` and `route` were re-specified, §4 gained a condensed gap map and a cross-harness problem map, milestones M0, M1 and M3 were re-scoped, and open questions 4 and 5 were resolved. v0.4 reconciles §3 and §5 with the eight layer specs under `docs/specs/` and their shared contracts (`docs/specs/00-cross-spec-contracts.md`); every change is one line in §3 or §5 with its spec section cited, and the review itself is `docs/specs/REVIEW-LOG.md`. Section numbers 4 (complaint table) and 5 (roadmap) are kept stable because README and CONTRIBUTING point at them.*

---

## 1. The thesis, now with evidence

The charter's claim was that most reported agent failures are not model failures. The research supports it more strongly than expected:

| Claim | Evidence | Source | Source grade |
|---|---|---|---|
| The harness matters more than the model, among frontier models | Harness variance 18.5 pp² vs model variance 2.4 pp² (7.8×) on SWE-bench Verified; harness changes transfer +5 to +10 pp across model families | doc 03 §2.1 | primary-measured (arXiv preprints; the variance decomposition is a position paper with a small controlled experiment, N small) |
| Same model, different harness, different score on agentic benchmarks (vendor and secondary numbers; the size of the gap is not established) | Same GPT-5.5: 83.4% on Terminal-Bench 2.1 in Codex CLI vs 76.4% in Terminus 2 (secondary, unverified); vendor vs standardised scaffolds diverge by 20+ points on SWE-bench Pro (Fable 5.1 81.2% on Anthropic's harness, best Claude run 51.9% on Scale's public leaderboard; the two runs also differ in split and effort setting); Anthropic's own numbers carry a 3.5 to 4.5 point standard error per model (vendor figure via MarkTechPost) | doc 06 A.0, A.10, D.2; doc 03 §2.1 marks the Terminal-Bench gap "(secondary, unverified)" | vendor / secondary (Terminal-Bench gap is a blog and leaderboard figure marked unverified; SWE-bench Pro numbers are a vendor card against an aggregator; standard error is a vendor figure via MarkTechPost) |
| Failures are drift, not incapacity | Each off-path tool call raises the chance of the next by 22.7 pp; a mid-run restart monitor gained +8.8 pp among intervened runs, population effect unstated (preprint) | doc 03 §2.2 | primary-measured (arXiv preprint) |
| Agents build to the visible check | With a visible oracle, near-perfect scores masked dead code; with it hidden, incompleteness went undetected | doc 03 §2.5 | primary-measured (arXiv preprint) |
| Prompt text decays | Compliance odds fall 5.6% per function written; 95% at turn 1 to 20-60% by turn 6-10; LLM-generated AGENTS.md reduces resolve rate 3% while adding 20% cost; human-written files about +4%, except in Claude Code, where even human-written files did not help | doc 04 §2.7 | primary-measured (two arXiv preprints) except the 95% to 20-60% decay curve, which is unverified (Khare, no identifier in doc 04) |
| Instruction drift persists even when the rule is re-injected every turn | caveman #303: "drifts back to natural prose mid-session despite SessionStart + UserPromptSubmit hooks firing every turn"; gemini-cli #13852 "GEMINI.md instructions are ignored" is P1 and open since 2025-11; claude-code #77136 (541 reactions) on style rules ignored; #6235 AGENTS.md support took 12 months and 6,558 reactions, and the community shipped the hook workaround first | doc 07 §4 item 5, §6 item 3, §2 | primary-measured (issue-tracker data, doc 07) |
| Prompt-only tools deliver between a seventh and a half of what they claim | caveman −8.5% output tokens (claimed 65%, about a seventh); ponytail −10.3% cost (claimed 20%, about half) and −15.4% code (claimed 54%, about a quarter); no quality change either way | doc 04 §3 | primary-measured (independent paired A/B by JetBrains; the tools' own claims are vendor) |
| Deterministic structure works | Structural index: localization 44% → 85% (p<0.0001), resolve 41.9% → 50.4% at lower cost, but against the fair agentic-grep comparator the resolve gain is 45.3% → 50.4% (p=0.087, not significant); one model, one study; test-impact map −70% regressions (one open-weight model); compiler and type-checker feedback in the loop raised pass@1 in every study reviewed, while the often-quoted +3.5 to +37% pass@1 belongs to type-constrained decoding, which needs decoder access and is unavailable on hosted APIs | doc 05 §1.2, §6, §7 | primary-measured (arXiv preprints; index study is one model, leak-audited; test-impact map is one open-weight model) |
| Destructive actions are a permission-layer bug, not an intent bug | claude-code #10077: `rm -rf` expanded from `~/` on WSL2 with permissions on; Dec 2025 Mac home directory and Keychain wiped via a trailing `~/`; Jul 2026 Opus 5 reset a live Supabase database through Prisma `--shadow-database-url`; Antigravity `rmdir /s /q d:\` from a path truncated at an unquoted space; CVE-2026-25723 piped-command bypass; #28240 permission prompt fires on `cd` instead of the real command in compound statements. Root cause every time: "validators check the raw string, execution happens post-expansion" | doc 06 A.1, A.3, C.3, D.2 item 1; doc 07 §6 items 5 and 6 | primary-measured (issue-tracker data, CVE records) for the tracked incidents; secondary for the Dec 2025 and Jul 2026 incident narratives and the root-cause quote |
| Compaction amnesia is harness design, and users already built the fix | claude-code #21925 "compaction destroys workflow, no CLAUDE.md reload" closed as not planned; #24460 CLAUDE.md lost after `/compact`; #66144 auto-compact does not fire at 100%; #7530 (100 reactions, 134 comments) and #5385 stale-closed "despite recent human comments"; #34556's author survived 59 compactions by building an external state file, later mirrored by auto-memory plus `PreCompact`; codex #4106 "control over auto-compaction parameters" open since 2025-09 | doc 06 A.1, D.2 item 4; doc 07 §2, §4 item 4, §6 item 2 | primary-measured (issue-tracker data, doc 07) |
| Cost and cache behaviour are opaque to the user and change server-side | claude-code #16157 (724 reactions, 1,491 comments): "accumulated tool output dragging hundreds of thousands of tokens on every single turn" found only by a user's own 82-session analysis; #46829 cache TTL silently regressed from 1h to 5m, closed not_planned the same day, later confirmed as two bugs fixed in v2.1.108; #46917 cache_creation inflated ~20K tokens per request server-side keyed on User-Agent; codex #28879 per-token rate-limit cost up 10-20× since 16 Jun, batch-closed without explanation; "no harness exposes a per-turn cost ledger the user can act on" | doc 07 §4 item 1, §6 item 1, §8 | primary-measured (issue-tracker data, doc 07); the closing quote is doc 07's own conclusion |
| Model and harness regressions are indistinguishable to users, and nobody runs a canary | claude-code #42796 (3,286 reactions, 583 comments) traced by post-mortem to harness-side changes (effort defaults, thinking redaction); gemini-cli silently serves Flash for Pro (#2208 not_planned, #28859); claude-code #91369 "silently downgraded mid-session"; codex #30364 reasoning tokens quantised at 516/1034/1552; no project in the 32-repo set runs a public scheduled regression suite | doc 06 A.1, D.2 item 15; doc 07 §5, §6 item 8, §8 | primary-measured (issue-tracker data, doc 07); the #42796 post-mortem attribution is vendor |
| Trackers mask the problems: stale bots close the worst incidents | gemini-cli #22141 (1+ hour stalls) stale-closed at 219 comments; #26856 (tens of thousands of files deleted) stale-closed; opencode #12661 and #5887 stale-closed; claude-code #16497 (238 reactions) on bot closures, #60705 "auto-closure overrides human triage" | doc 07 §1, §6 item 20 | primary-measured (issue-tracker data, doc 07) |
| Some failures are model-only and the harness can only detect and contain them | GPT-5.5 lied about completing an impossible task ~29% of the time; Fable 5.1 shows ~24% hidden grader awareness and ~6% exploitation in high-risk coding RL environments (system card); Kimi K3 hallucination rate rose 39% → 51% while accuracy rose; every frontier model resolves 3.4-5.2% of MobileDev-Bench with file-level recall 14-20% | doc 06 A.1, A.2, A.5, A.9, D.2 (model-only list) | vendor / secondary (GPT-5.5 figure is a system card read via Zvi; Fable 5.1 is a system card; Kimi K3 is a blog on an index); MobileDev-Bench is primary-measured (arXiv preprint) |
| Memory storage is solved, injection is not | 8,785 stored observations never surfaced; self-memory underperformed plain retrieval; only procedural "what worked" memory shows gains | doc 04 §2.4, doc 05 §3 | primary-measured (arXiv preprints) for the ETH and procedural-memory results; secondary for the 8,785-observation user report |
| Model-level determinism is impossible on hosted APIs | 80 unique outputs in 1,000 temperature-0 runs; cause is batch-variant kernels and MoE routing; Fable 5.1 has adaptive thinking always on, removing another knob | doc 05 §4.1, doc 06 A.10 | vendor (Thinking Machines lab post with its own measurement; Anthropic docs) |
| Verification is gameable by prompting alone | One anti-hack line cut reward hacking 52% → 18%, but Opus 4.6 fabricated content where "a prompt to not do it did not make this go away"; EvilGenie saw explicit test-file editing by Claude Code and Codex, and withholding tests barely helped | doc 03 §3, doc 06 A.1 | vendor (Anthropic system cards, partly read via Zvi); EvilGenie is primary-measured (early preprint) |
| Nobody measures | Of ~60 tools, two have an independent paired A/B, both from one JetBrains team; ponytail closed "impact on model performance?" as completed without a benchmark | doc 04 §5.1, doc 07 §7 | primary-measured (doc 04's own count of ~60 tools; issue-tracker data, doc 07) |

Source grade (added 2026-09-03 after the red-team review, from the research docs' own labels): **primary-measured** is a paper, preprint, benchmark or issue-tracker dataset with its own measurement; **vendor** is a lab or product's own number; **secondary** is a blog, aggregator or write-up quoting someone else's number; **unverified** is a figure with no traceable source. Preprints are primary-measured but not peer-reviewed. Nothing graded secondary or unverified may appear in the README.

The gap that training will not close in the next one to two years is therefore concrete: **repository-specific structure, machine-checked completion, drift detection, targeted rule injection, command validation after shell expansion, cost and version visibility, and privacy at the tool boundary.** Every one of those lives outside the model, and the labs' own harnesses only partially cover them. Doc 07 §9 adds the decisive observation: for every item on that list, users have already built the mechanism ad hoc inside an issue thread, and each has since been shipped natively by at most one harness, the cost ledger, canary and post-expansion validation by none (doc 06 Part B).

## 2. What Saga will not do, restated with reasons

- **No prompt-only skills as the product.** Evidence: ~10% cost effect, 0% quality effect, decays within a session even with per-turn re-injection (caveman #303). Saga ships prompt text only as thin adapters over checked mechanisms, each with a measured token cost.
- **No unconditional per-turn injection.** Evidence: superpowers #743 "slowness since using the skill", spec-kit #1401 "commands consume a significant portion of the context window in every session", ponytail #597 and #502 (full SKILL.md sent to every sub-agent including reviewers), anthropics/skills #1486. Everything injected on every turn is paid on every turn (doc 07 §7).
- **No LLM-extracted knowledge graph of the code.** Evidence: 10⁷-10⁸ tokens to build, relation errors, and no advantage over deterministic AST graphs for code. Code already has a schema.
- **No LLM-written narrative memory injected as fact.** Evidence: net negative on coding outcomes; every memory product in doc 07 §6 item 11 has its own silent-loss or poisoning bug (mem0 #5245, #4956; letta #3388).
- **No LLM classifier as the only permission check.** Evidence: Anthropic's auto-mode classifier has a 17% false-negative rate on real over-eager actions, is "intermittently unavailable" (claude-code #91517), and blocked the restore call after letting the destructive one through (#91506). Saga's command policy is deterministic; a classifier may sit in front of it, never instead of it.
- **No multi-agent write parallelism by default.** Evidence: raises total tokens, fragments decisions, and shared-worktree races exist even in the vendor's own implementation (claude-code #91513 `git commit --amend` rewriting a teammate's commit); read-only sub-agents and a separate-context reviewer are the parts that work.
- **No unsigned auto-executing hooks, and no SessionStart code path that executes repo-supplied scripts.** Evidence: the two largest "enhancer" repos shipped a CVSS 10.0 RCE and a malware clone; BMAD #2624 and #2625 execute and read arbitrary project files; Gemini CLI CVE-2026-12537 was `.gemini/.env` command injection before the sandbox (doc 06 C.3, doc 07 §7).
- **No "no answer means yes."** Evidence: claude-code #73125 (414 reactions, fixed in two days), codex #28969 (still open, users patch `extension.js` every update), GSD #803. A question Saga's adapters ask blocks until answered or the run is marked `ABANDON`.

## 3. Architecture

One local binary, `saga`, that exposes the same capabilities three ways: as a **CLI** (works with anything that can run a shell command), as an **MCP server** (for harnesses that speak MCP), and through **thin hook adapters** per harness (Claude Code hooks, Codex hooks, Gemini CLI hooks, Cursor rules, OpenCode plugins, plus a CI fallback). The core has zero knowledge of any vendor's prompt format. The seam Saga relies on is the hook surface that Claude Code, Codex and Gemini CLI now share by design (PreToolUse, PostToolUse, Stop, SessionStart, PreCompact; gemini-cli #2779 was built "to make very easy to migrate hooks from Claude Code", doc 07 §5). Each harness event is bound exactly once, to `saga hook <harness> <event>`, which runs the installed layers in a fixed order and merges their outputs into the one envelope the harness accepts; no layer installs a hook of its own (specs/00-cross-spec-contracts.md §1, shape-spec §7).

```
                 ┌──────────────────────────────────────────────┐
                 │  harness (Claude Code / Codex / Gemini / …)   │
                 └───────┬───────────────┬───────────────┬──────┘
                    hooks│           MCP │           CLI │
                 ┌───────▼───────────────▼───────────────▼──────┐
                 │                    saga                       │
                 │  bench · gate · index · mem · guard · shape   │
                 │           trace · route · doctor              │
                 └───────┬───────────────┬───────────────┬──────┘
                         │ .saga/ (content-addressed local state) │
                         └────────────────────────────────────────┘
```

### 3.1 `saga bench`: the measurement harness (ships first)

Nothing else in Saga is allowed to claim an effect without this.

- Paired A/B: same task, same model, with and without one component; the control arm is prevented from reaching the component (the codegraph discipline, doc 04 §2.4).
- K independent clean-room runs per cell; report **pass^k**, per-task medians, Wilcoxon, tokens, wall time, and cost. Variance is a first-class output. The per-model standard error on vendor agentic evals is 3.5 to 4.5 points (doc 06 A.10); any claimed effect smaller than that is reported as "within noise".
- Task set: 20-50 tasks per language drawn from real failures, across TypeScript, Python, Go, Rust, Java/Kotlin, Swift, and Dart, each with **hidden** oracles the agent never sees, and a known-broken positive control for every absence check. SWE-bench Verified is not used as a discriminator (59.4% of audited o3 failures were test flaws; frontier models reproduce gold patches verbatim, doc 06 A.0).
- Runs on the user's own repo for under $20 and gives a directional, unbadged reading of "did this change to my agent config help *here*" (bench-spec §4.4 `user` tier: K=3, at most 20 tasks, standard error about 11 pp, so it cannot resolve an effect at the noise floor cited above).
- Doubles as the **canary** (§3.7): a 10 to 20 task subset runs on a schedule and on any harness or model version change.
- Publishes negative results in the repo.

### 3.2 `saga gate`: contract before, evidence after

Adopts unlazy's ledger grammar (doc 08 §2.5) and generalises it.

- **Contract**: before work starts, a small machine-readable file: in-scope paths, out-of-scope paths, and gates (`CHECK` command, `EXPECT` output). There is no side-effect field, because a diff cannot observe one and ADR 0002 forbids unenforced text (gate-spec §1.3); network and out-of-repo effects are guard policy (§3.5). Saga helps derive it from the request, but the human or the agent writes it; the checker never infers semantics from prose.
- **Red proof**: a gate is not trusted until it has been observed failing at least once against a known-broken state. This turns unlazy's negative-control rule into a tool and answers "does this check actually check anything". It also closes unlazy's own UL001 to UL003 (gates passing when their `CHECK` never observes the thing they are about; doc 07 §7): gates fail closed, and the gate runner ships with a fixture that must fail.
- **Evidence**: exit 0 and marker match, hashed output, resolved environment, stored under `.saga/`. `ABANDON` is a first-class terminal, non-successful state. This is the harness-side answer to impossible-task deception (GPT-5.5 ~29%, doc 06 A.2): the agent has a legitimate way to stop that is not "done".
- **Claim verification** (new in v0.3, doc 07 §6 item 4 and §9): after every turn, Saga diffs the working tree and compares it with what the agent said it did. "Agent claimed success with zero edits", "said tests pass but no test runner executed this turn", and "said it read the file but no read occurred" are flagged from the trace, never from the model's self-report. cline #4384's commenter: "the agent often reports success either way, so the trace is the only ground truth". Owner: trace computes the three deterministic checks (claim detection, evidence reconciliation, claim-versus-diff consistency; trace-spec §5.5 to §5.9) and gate's Stop step cites the verdict (gate-spec §6): `contradicted` blocks, `unverified` warns in minimal mode and blocks in full mode. REVIEW-LOG risk 1 resolved 2026-09-03.
- **Diff guards** (deterministic, run on every edit and at Stop): touched path outside scope; test file deleted or assertion weakened; `skip`/`xfail`/`only` added; expected value hard-coded to match an observed failure; new dependency not in lockfile or registry; generated comment bloat above a per-file threshold (claude-code #65961, doc 06 A.1). Each flags a mandatory, logged justification. Test directories can be declared read-only in the contract (doc 06 D.2 item 3).
- **Traceability**: every gate carries a pointer to the sentence of the request it discharges; an outcome in the request with no gate is reported as uncovered.
- **Reviewer contract**: when a separate-context reviewer is used, its prompt never contains the author's opinion, and it must return findings against the contract, not a verdict (doc 06 D.2 item 14; both Claude Code and Codex tilt analyses toward what the user wants, Asher et al.).
- **Stop enforcement**: harness hook where one exists (Claude Code Stop, Gemini AfterAgent, Codex Stop; verified in specs/harness-facts.md), CI check otherwise. The hook blocks "done" while gates are unmet or uncovered, which is what claude-code-action #599 (stopped after 5 of 10 todos, skipped lint and typecheck) asked for.

### 3.3 `saga index`: the code knowledge layer

Follows the recommended architecture in doc 05 §9, which is the only design with three independent studies behind it.

- Tree-sitter definition/reference graph, per-file fact sets keyed by content hash; BM25 lexical index; AST-aligned chunk embeddings optional and model-swappable; test-impact map from imports and call graph plus last coverage; lazy LSP overlay only for find-references and rename.
- Four agent-facing tools: `search`, `symbol` (depth 1, signatures not bodies), `read`, `impact`. Grep stays available.
- Incremental update in about a second on file change; the whole index versioned by the hash of sorted file hashes; tool results cached under `(tool, args, index_version)` so replays are exact and survive sessions.
- A ≤1k-token repo map lives in the cached prompt prefix and changes only when the index version changes; everything else is injected as tool results so prompt caching keeps its 90-97.5% discount.
- Regime-aware: off by default below a size threshold, because every structural tool in the survey is net negative on small repos (doc 04 §5.5). Richer tools (LSP) are enabled by model tier: they helped Haiku and hurt Sonnet (doc 05 §10 item 6); concretely, on for the route policy's `small` and `local` tiers and off for `frontier` and `standard` (index-spec §1.3, route-spec §3.1). Regime thresholds are 20k lines or 200 files for `lite` and 80k lines or 800 files for `full`, priors until the bench revises them (index-spec §1.3).
- **External API cache** for fast-moving SDKs (Swift 6, iOS 26, doc 06 A.9): version-pinned docs fetched on demand and gated by the import actually present, because ungated retrieval hurts (doc 05 §10 item 7).
- Honest limit: file-level localization on large mobile repos is 14-20% recall for every frontier model (doc 06 A.9) and line-level recall about 15% (doc 05 §10). The index raises the file-level number; it does not fix the line-level one.

### 3.4 `saga mem`: typed memory with targeted injection

- Record types: **environment facts** (build/test commands, toolchain versions), **conventions** (human-written, short), **decisions and constraints** (with the turn that produced them, so they survive compaction verbatim), **procedures** (what worked and what failed in this repo, the only kind of learned memory with positive evidence), and **pitfalls**.
- No narrative summaries of the code. The index is the memory of the code.
- Storage is plain files under `.saga/` in the repository, not a vector database: that is what claude-code #34556 and Claude Code's own auto-memory converged on, and every memory product in doc 07 §6 item 11 ships its own silent-loss bug.
- **Compaction survival** (moved forward to M1 in v0.3): a `PreCompact`/`SessionStart` adapter pair writes and re-injects a structured state block (goals, open gates, decisions, constraints, file list, last test status). This is exactly the mechanism users built by hand in #34556 and is portable across the three hook-bearing harnesses (doc 07 §6 item 2, §9).
- **Targeted injection**: a PreToolUse adapter injects only the two or three records that apply to the path or tool about to be used, and nothing else. This attacks compliance decay directly and is the gap doc 04 §5.2 says nobody has filled.
- Freshness: every record that names a path or command is checked on read; stale records are flagged, not injected.
- Sub-agent preamble: the constraints and decisions are handed to every sub-agent verbatim as a small structured block.

**What doc 07 forces us to say about drift.** caveman #303 shows a rule re-injected on every turn still drifts mid-session, and gemini-cli #13852's maintainer-adjacent comment locates the problem "in the model itself". Saga's position:

1. It does not rely on injection for any rule that can be checked. Every rule with a checkable form becomes a diff guard or gate ("verification, not exhortation", doc 07 §6 item 3). Injection exists to reduce the number of violations the gate has to catch, not to guarantee zero.
2. Targeted injection differs from #303's mechanism in placement and size: two or three records adjacent to the tool call, not a standing document at the top of the prompt re-sent every turn. Whether that placement holds up better is a measured claim (M3 exit criterion), not an assumption.
3. What Saga cannot promise: a prose-only rule with no checker (comment verbosity, tone, "do not narrate", claude-code #77136) will still drift, and Saga will report the drift rate on the bench rather than claim to have removed it. Memory records also cost tokens on every turn they are injected; the ledger (§3.7) shows that cost per record type.

### 3.5 `saga guard`: command validation, capability reduction, and privacy at the tool boundary

Re-specified in v0.3 around the finding that most 2025-26 data-loss incidents were "substrate execution gaps (shell quoting, path expansion) rather than model hallucination" (doc 06 D.2 item 1). Details in ADR 0006.

- **Post-expansion command classification.** A PreToolUse adapter parses the proposed shell command with a real shell grammar (bash, zsh, PowerShell, cmd), resolves tilde, variable, glob and quoting exactly as the target shell would, splits compound statements (`&&`, `;`, `|`, `||`, subshells, `xargs`, `find -exec`, `sh -c`) into segments, and classifies **each segment** on its resolved argv: read-only, mutating-in-scope, mutating-out-of-scope, destructive. Policy is matched on the resolved form, never the raw string. Anything the parser cannot resolve (unquoted expansions of unknown variables, `eval`, here-documents piped to a shell) is classified as unresolvable and requires confirmation. This is the community workaround in claude-code #28240 and #16561 turned into a tool, and it addresses CVE-2026-25723 (piped-command bypass), the 50-subcommand deny-rule truncation, #10077, and the Antigravity unquoted-path deletion.
- **Hard denies on the resolved form**: recursive delete at or above `$HOME`, the repo root, or a drive root; `git reset --hard`, `git push --force` to a protected branch, `git clean -fdx` outside scope; framework-level data wipes (`migrate:fresh`, Prisma `--shadow-database-url`, `db:drop`); credential file reads outside an allow-list. Denies are lists in `.saga/policy` that the user extends; the defaults come from the incident ledger in doc 06 A.1, A.3 and C.3.
- **Per-turn filesystem snapshots.** Before any Bash, Write or Edit tool call, Saga snapshots the working tree, including untracked files and intra-turn overwrites, through the one `saga snapshot` primitive (guard-spec §3.1, contracts §5): ZFS or btrfs snapshots where the repo is its own dataset, a git tree written through a temporary index under `refs/saga/snap/` everywhere else including APFS and NTFS (volume snapshots need admin), clonefile reflinks for large ignored binaries. Trace checkpoints, shape's edit pre-images and gate's red-proof scratch trees all use the same snapshot ids. `saga undo <turn>` restores files independently of the harness's checkpoint. This is what codex #9203 (464 reactions, `/undo` removed) and #11626 asked for, and what Claude Code's `/rewind` covers only for its own edits (doc 07 §4 item 3, §6 item 6).
- **Allow-list model.** Read-only segments run without a prompt; mutating segments inside contract scope run without a prompt when the contract says so; everything else asks. Fewer prompts *and* fewer bypasses, which is the pair no harness currently delivers (doc 07 §6 item 5).
- **Ambient credential reduction.** Adapters run with short-lived scoped tokens where the platform allows; repo-supplied content (issue bodies, PR titles, AGENTS.md, `.env`) is treated as data, never as instructions; no Saga code path executes a repo-supplied script at SessionStart (doc 06 C.3, D.2 item 8).
- **MCP gateway** (M6): lazy schema loading, per-agent allow-lists, and scanning of tool descriptions for injected instructions before they reach the model (MCPTox: 36.5% average tool-poisoning success; ruflo #1375; doc 07 §6 item 12).
- **Secret and PII masking** on every tool result and prompt before it leaves the machine: gitleaks-class rules plus configurable patterns, optional Presidio, reversible placeholders, a coverage benchmark with positive controls (the agent-guard pattern, doc 04 §2.5). Motivation: GitGuardian's State of Secrets Sprawl 2026 measured secrets in Claude-Code-co-authored commits at about 2× the baseline rate (the 3.2% figure quoted in doc 04 §2.5). Honest limit: covered models (Fable, Mythos) retain inputs 30 days even under zero-data-retention since 9 Jun 2026 (doc 06 C.1); masking reduces what is retained, it does not change the retention policy.
- **Package existence and age check** against registry and lockfile before any install or import is accepted; blocks packages younger than N days by default. Removes the hallucinated-package class (open-weight models 21.7%, commercial 5.2%; 58% of hallucinated names recur more than once across 10 runs and 43% recur on all 10, doc 03 §2.7 and doc 06 C.4).
- **Tool-call schema validation and repair** for open-weight and local models, which loop on tool syntax in frontier-tuned harnesses (cline #7262, Roo #10322, doc 06 D.2 item 11).
- Signed releases, pinned hook scripts with hashes, no self-updating `git pull`, a manifest declaring exactly what executes at which lifecycle event, and a tested Windows path (doc 07 §6 item 14: "an external layer must be tested on Windows or it will be the next entry in this list").
- What guard is not: it is not an OS sandbox and does not replace one. It runs in front of Codex's Seatbelt/Landlock or a container, and cannot fix sandbox leaks through the network, temp directories or the sandbox's own config file (doc 07 §6 item 17).

### 3.6 `saga shape`: structured tool output and caching

- Parsers for common test runners, compilers, linters, and build tools that return failing test names, first error with file and line, exit code, and a pointer to the full log, instead of the raw stream.
- Head/tail truncation with error-aware tail retention and never mid-line; default caps aligned with the SWE-agent ablations. Accumulated tool output is the hidden cost driver in claude-code #16157; shape's per-turn output bytes are reported in the ledger.
- Content-addressed cache of tool results keyed on command, arguments, and the hashes of relevant files.
- Post-edit formatter hook that strips generated comment bloat where the contract asks for it (doc 06 D.2 item 13).

### 3.7 `saga trace`: the event log, cost ledger, version pinning, and drift detection

Re-specified in v0.3 around doc 07 §8: the only determinism levers available outside the harness are pinning, recording, verifying, and canary runs. Details in ADR 0007.

- **Append-only event log** keyed by content hashes so a run can be replayed or forked without re-calling the model (the "log is the agent" pattern, doc 05 §4.2). Portable format so a session can be resumed in a different harness.
- **Per-turn cost ledger** with cache-hit accounting: for every model call, fresh input tokens, cache-read tokens, cache-write tokens, output tokens, reasoning tokens, the observed cache TTL, tool-output bytes added this turn (raw and after shaping), and the component that added them, from the closed key set of trace-spec §3.2 (harness, user, tool results, index, mem by sub-component, shape, gate, guard, trace, route). Source is the harness transcript JSONL where available and a local proxy capture otherwise. This is the "82-session analysis" in #16157 and the proxy capture in #46917 shipped as a command, and it makes the honest "token optimizer" a profiler: users see which installed piece is eating tokens (doc 04 §5.8).
- **Budget enforcement**: warn, force a compaction, or stop at a declared token or dollar budget per session and per task, with the decision logged.
- **Version pinning and change detection**: every session records harness version, model id as returned by the API, effort or thinking setting, provider request ids and fingerprints where offered, and the cache TTL actually observed. Any change mid-session or between consecutive sessions is surfaced as an event, not buried. A vendor fallback (Pro served as Flash, gemini-cli #28859; "unintended model switching", claude-code #91522) is treated as an incident and the run is marked non-comparable in the bench.
- **Canary evals**: a fixed 10 to 20 task subset of the bench runs on a schedule and on any detected version change, reporting pass^k, tokens per task, and cache-hit ratio against the pinned baseline. This is how #42796, #46829, #46917 and #30364 get caught in hours instead of weeks (doc 06 D.2 item 15). Detection power is bounded by the 3.5 to 4.5 point standard error; the canary reports confidence, not a verdict.
- **Loop and stall watchdog**: repeated identical tool calls, repeated identical failing commands, no tool call or token for N seconds, and context growth without progress each raise an event and, past a threshold, pause for the user (OpenHands #7183 ships this; claude-code #26224 with 151 reactions and gemini-cli #22141 with 219 comments do not have it). The watchdog compares outputs, not just commands, to avoid the over-aggressive detection that halts legitimate repetitive work (gemini-cli #5761, #8237).
- **Drift monitor**: flags trajectories that leave the expected tool-call path (scope violations, off-path calls) and suggests restart from the last good checkpoint (doc 03 §2.2, +8.8 pp among intervened runs).

### 3.8 `saga route`: model and effort routing (last, least evidence)

Re-specified against doc 06 Part D. Routing ships after the bench can measure it, because the evidence is weakest and most perishable here.

- **Policy file** mapping task class to model and effort, with a visible budget; defaults are the "when to use which model" table in §4.2 and are expected to be wrong within months, so every default carries its source date.
- **Rules that do not depend on which model is best this month**: never route silently (the ledger shows the model id on every turn); compare on total tokens per completed task, not price per token (Gemini 3.5 Flash emitted 73M tokens across the AA index vs a 36M average, doc 06 A.3); read-only sub-agents default to the cheapest tier that passes the bench for that task class; open-weight and local models get schema validation and repair turned on; security review and impossible-task-heavy work never route down-tier.
- **Effort pinning**: effort or thinking level is part of the contract, because the largest single "got dumber" episode (#42796) was an effort default change.

### 3.9 `saga doctor`: install hygiene and self-test

New in v0.3 because the tooling trackers say the category's number-one issue class is install and uninstall, not intelligence (doc 07 §7).

- Install-time smoke test that each adapter hook actually fires in the target harness and that gates fail on the shipped known-bad fixture (anthropics/skills #556 found a 0% trigger rate from four independent causes, each sufficient; mattpocock/skills #908 six skills unreachable from a frontmatter quirk).
- `saga uninstall` removes everything it installed and proves it (ruflo #670/#710, caveman #400).
- Reports the per-component token cost of the current install from the last N sessions' ledgers.
- Optional linting of the user's own skills and hooks for the silent-failure patterns catalogued in doc 07 §7.

### 3.10 What Saga cannot do

Stated once so that no README claim exceeds it (doc 07 §9, doc 06 D.2 model-only list):

- Fix a harness's edit tool, terminal rendering, sandbox leaks, vendor model fallback, subscription lock-in, or the 60-second auto-answer. Saga detects the first four and refuses to replicate the last.
- Make a model obey a prose rule. Saga can only reject the output.
- Remove evaluation awareness, deception on impossible tasks, calibration failures, or raw localization ability. Saga contains them with hidden oracles, `ABANDON`, red proof, and claim verification.
- Make sampling deterministic. Saga makes outcomes checkable and runs replayable.
- Change provider retention or training policy. Masking reduces exposure; it does not create zero-data-retention.

## 4. What each layer does for the complaints in doc 02

| Complaint | Layers |
|---|---|
| "Said done, does not work" | gate (red proof, evidence, claim verification, Stop enforcement), shape |
| "Forgot what we agreed" | mem (decisions survive compaction, PreCompact state block), trace |
| "Ignored CLAUDE.md" | mem (targeted injection), gate (rules become checks) |
| "Touched what I did not ask" | gate (scope contract, diff guards), guard (post-expansion classification, snapshots) |
| "Stopped halfway" | gate (uncovered outcomes, ABANDON) |
| "Deleted or weakened my test" | gate (diff guards, read-only test paths) |
| "Invented an API or package" | guard (existence and age check), index (external API cache), shape (type-checker loop) |
| "Same prompt, different result" | gate (collapses branch points), trace (variance measured, version pinning), index (same facts every run) |
| "Burned tokens reading the repo" | index, shape, cache, trace (ledger says which component) |
| "Leaked my secrets" | guard |
| "It worked yesterday, today it is dumber" | trace (version pinning, canary), route (effort pinning) |
| "It deleted my home directory" | guard (post-expansion denies, per-turn snapshot, `saga undo`) |

### 4.1 Per-model gap map, condensed

Sources are doc 06 Parts A and D unless stated; vendor numbers are labelled there and every one is as of 2 Sep 2026. Issue numbers are `anthropics/claude-code` unless prefixed.

| Model (price in/out per MTok) | Top documented weaknesses | Saga layer that mitigates |
|---|---|---|
| **Claude Fable 5.1** ($10/$50, cache read $0.25) | Hidden grader awareness ~24% and exploitation ~6% in high-risk coding RL environments, rising over training (Fable 5.1 / Mythos 5.1 system card); premature "done" lineage (#42796, 3,286 reactions; HN on 5.1: "I said I would do it and I did not do it and I said that I had"); parallel tool calling "more variable", narrates less at low effort; comment bloat on every method despite instructions (#65961, 186); redirects exploit-dev and pentest tasks to Opus | gate (hidden oracles, red proof, claim verification, Stop); shape (comment stripper); route (effort pinning); trace (effort recorded per turn) |
| **Claude Opus 5** ($5/$25) | Reset a live Supabase database via Prisma `--shadow-database-url` in Claude Code, Jul 2026 (adversa incident list); explore/thinking loops in the Opus 4.6 lineage (#24585); thinking on by default so effort is not a knob | guard (post-expansion denies, snapshot); trace (loop watchdog); route (best price/performance for TB-class work, 89.1% TB 2.1) |
| **Claude Sonnet 5** ($3/$15) | SWE-bench Pro 63.2% vs Opus 5 79.2% (system card); Sonnet lineage reward-hacked 12.8% on impossible tasks (Opus 4.5 card); "wet blanket" over-discouraging responses up slightly | route (bulk, sub-agents, review); gate (`ABANDON` for impossible tasks) |
| **Claude Haiku 4.5** ($1/$5, 200K) | SWE-bench Verified 73.3% [unverified]; 12.6% impossible-task hack rate; needs richer tools (LSP helped Haiku, hurt Sonnet, doc 05 §10) | route (read-only sub-agents only); index (LSP overlay on by tier); guard (schema validation) |
| **GPT-5.6 Sol / Terra / Luna** ($5/$30, $2.5/$15, $1/$6) | GPT-5.5 lied ~29% about completing impossible tasks, evaluation awareness 22% (system card via Zvi, OpenAI deployment hub); reasoning tokens quantised at 516/1034/1552 (codex #30364, 429); "rarely parallelizes programmatic tool calls, multiplying cost" (codex #32503); cannot set sub-agent models (codex #31814); cyber guard false positives on defensive work (codex #32468); terse to the point of incoherent handoffs (HN); rate-limit cost 10-20× (codex #28879, 560) | gate (`ABANDON`, hidden oracles); trace (ledger, reasoning-token accounting, canary); route (Luna for read-only bulk); mem (sub-agent preamble) |
| **Gemini 3.1 Pro** ($2/$12; $4/$18 over 200K) | Multi-step agentic weakness (TB 2.0 54.2% vs GPT 77.3%); long-session self-contradiction "by round eight"; sycophantic code review (gemini-cli #24725, #4556); tool-call avoidance (edits via PowerShell, corrupting comments); misread a failed `mkdir` as success and overwrote files; silent Pro→Flash fallback (gemini-cli #2208 not_planned, #28859); loop detector halts legitimate work (#5761, #8237) | gate (claim verification, reviewer contract); trace (model id per turn, fallback as incident, output-aware watchdog); route (avoid for long runs; use for LiveCodeBench-class and frontend one-shots); shape (tokens per task) |
| **DeepSeek V4-Pro-0813** ($0.435/$0.87) | Vendor SWE-bench Verified 96.4% and TB 2.1 87.9% unreplicated by any third party; "ranks last of seven on agentic coding" in one comparative [unverified method]; open-weight package-hallucination class (21.7%); no vision | bench (run your own eval before trusting); guard (package gate, schema repair); route (self-hosted tier) |
| **Kimi K3** ($3/$15, cache hit $0.30) | AA-Omniscience hallucination rate rose 39% → 51% from K2.6 to K3 while accuracy rose (Kili); bespoke non-OSI licence; vendor TB 88.3 unreplicated | guard (package and API existence); index (external API cache); gate (compile-in-loop); route (frontend one-shots where it leads Frontend Code Arena) |
| **Qwen3-Coder-Next / Qwen3.6-27B** (open weights) | MobileDev-Bench best-of-four 5.21% on real mobile repos; small MoE variants degrade on tool-call formatting in Claude-Code-style harnesses [community, unverified]; Qwen3.6-27B is the best single-24GB-GPU coder (77.2% Verified) | guard (schema validation and repair); index (docs injection, test-impact); shape; route (air-gapped tier) |

Cross-model: every frontier model resolves 3.4-5.2% of MobileDev-Bench (React Native 0%) and the bottleneck is file-level localization (14-20% recall), so for Swift, Kotlin and Dart the harness that feeds current API docs, keeps diffs small, and compiles in the loop matters more than the model (doc 06 A.9, D.1).

### 4.2 When to use which model (from doc 06 D.1, with cost)

Defaults for the `route` policy file; each row expires when the bench says so.

| Task class | Default | Alternative | Why, and what it costs |
|---|---|---|---|
| Multi-hour autonomous refactor, root-cause debugging | Claude Fable 5.1 | Claude Opus 5 when budget-bound | Longest measured horizon (METR: Mythos "likely at least 16 hours"); best SWE-bench Pro on the vendor harness (81.2%); "avoids shortcuts". $10/$50, but cache reads at $0.25 make agentic loops ~45% cheaper than Fable 5. Pin effort; watch parallel-tool-call variance. |
| Everyday agentic coding, CI automation | Claude Opus 5 in Claude Code | GPT-5.6 Sol or Terra in Codex CLI | Opus 5: 89.1% TB 2.1 at $5/$25. Sol: 88.8-91.9% TB 2.1, about half the output tokens, kernel sandbox, $5/$30. Choose by harness need: hooks and sub-agents favour Claude Code; sandbox and Windows favour Codex. |
| Read-only sub-agents, bulk edits, PR review | Claude Sonnet 5 ($3/$15) | GPT-5.6 Luna ($1/$6), Gemini 3.6 Flash | Sonnet 5 beats Opus 4.8 on TB 2.1; Luna is the cheapest frontier-lineage option; Flash is verbose, so compare total tokens per task, not price per token. |
| Algorithmic snippets, frontend one-shots | Gemini 3.1 Pro ($2/$12) | Kimi K3 | Gemini leads LiveCodeBench Pro Elo and WebDev Arena; K3 leads Frontend Code Arena. Do not use Gemini for long multi-step agent runs (TB 2.0 54%). |
| Self-hosted or air-gapped | DeepSeek V4-Pro-0813 (API, ~1/36th the cost of Fable) | Qwen3.6-27B (single 24GB GPU), Devstral Small 2 (laptop, $0.10/$0.30) | Open weights; vendor scores unreplicated, so run the bench first; enable schema repair. |
| Security code review | Claude Opus 5 (or 4.6+) | none | Opus 4.6 won 38 of 40 blind security investigations against Gemini; Gemini reviews trend sycophantic; Fable still redirects exploit-dev to Opus. |
| Mobile (Swift, Kotlin, Dart) | Any frontier model **plus** docs injection, small diffs, compile in loop | none | All models 3-5% on MobileDev-Bench; the harness feeding current API docs is the lever. |
| Statistical or data-analysis code | Any, with an adversarial reviewer under the reviewer contract | none | Both Claude Code and Codex p-hack toward user expectation (Asher et al.). |

### 4.3 Top cross-harness problems from issue trackers, mapped to Saga

Doc 07 §6's ranked list of 20, with the layer that owns each and an honest verdict: **external** (Saga can solve it from outside the harness, because users already did ad hoc), **partial** (Saga detects or reduces it, the harness must finish it), or **not ours** (structurally outside an external layer).

| # | Problem (doc 07 §6) | Representative issues | Saga layer | Verdict |
|---|---|---|---|---|
| 1 | Opaque cost and quota burn | claude-code #16157, #38335, #46829, #46917; codex #14593, #28879 | trace (ledger, budget) | external |
| 2 | Context loss on compaction, no control over what survives | claude-code #7530, #5385, #34556; codex #4106; sdk #23, #772 | mem (PreCompact state block), gate (open gates survive) | external |
| 3 | Instruction files ignored, drift | claude-code #6235, #77136, #60705; gemini-cli #13852; caveman #303 | gate (rules become checks), mem (targeted injection) | partial: checkable rules external, prose rules not ours |
| 4 | Edit-tool failures and false success | cline #4384; gemini-cli #1028; serena #576, #1744 | gate (claim verification), guard (snapshot) | partial: detection external, the tool is the harness's |
| 5 | Permission fatigue and bypass | claude-code #28240, #91517, #91506; kilocode #10068; goose #11399 | guard (post-expansion classifier, allow-list) | external |
| 6 | Destructive actions and data loss | gemini-cli #26856; claude-code #91506, #10077; codex #9203 | guard (denies, per-turn snapshot, `saga undo`) | external |
| 7 | Hangs, stalls, silent failures | claude-code #26224, #46987; gemini-cli #22141; cline #4356 | trace (watchdog) | partial: detect and pause, cannot fix the stream |
| 8 | Model regressions indistinguishable from harness regressions | claude-code #42796, #91516; codex #30364; gemini-cli #2208, #28859 | trace (pinning, canary), bench | external for detection |
| 9 | Looping and repeated tool calls | claude-code loop cluster (892 open by body search); Roo #11071; OpenHands #7183 | trace (output-aware loop detector, turn cap) | external |
| 10 | 60-second auto-answer | claude-code #73125 (fixed); codex #28969; GSD #803 | adapters must never replicate it | not ours |
| 11 | Memory across sessions | claude-code #34556, #2511; mem0 #5245, #4956; letta #3388 | mem (plain files, typed, fresh-checked) | external |
| 12 | MCP bloat, breakage, security | claude-code #6915, #7328; servers #3537, #754; ruflo #1375 | guard (MCP gateway, M6) | partial: scoping already shipped by harnesses; scanning and logging external |
| 13 | Multi-agent coordination without audit trail | codex #28058, #31814; claude-code #91513; opencode #12661 | mem (sub-agent preamble), trace (per-agent ledger) | partial: shared-worktree correctness needs the harness |
| 14 | Windows, WSL, path issues | claude-code #4928; kilocode #9440; unlazy #30; caveman #366 | doctor (Windows in CI from M1) | ours to not repeat |
| 15 | Local and OpenAI-compatible model tool calling | cline #7262, #1418; Roo #10322; kilocode #3967 | guard (schema validation and repair) | external |
| 16 | Subscription lock-in | claude-code #17118; opencode #7410; Roo #10566 | none | not ours |
| 17 | Sandbox correctness | opencode #2242; claude-code #91512; copilot-chat #5098; codex #2847 | guard runs in front of a sandbox, never replaces it | not ours |
| 18 | Skill and plugin loading fails silently | anthropics/skills #556; mattpocock #908; vercel-labs/skills #693, #1138 | doctor (load-time smoke test) | external |
| 19 | Terminal and TUI usability | claude-code #826, #769, #1913 | none | not ours |
| 20 | Triage itself: users cannot tell known from fixed from wontfix | claude-code #16497, #60705; stale-closed incidents | docs 06 and 07 as a maintained failure-mode registry; bench results per release | partial |

Count: 10 external, 7 partial, 3 not ours. The three "not ours" items and the partial halves are listed in §3.10 so they never appear as claims.

## 5. Roadmap

| Milestone | Scope | Exit criterion |
|---|---|---|
| **M0 Measure** | `bench` core; `trace` core including per-turn cost ledger with cache accounting, version pinning, change detection, and loop/stall watchdog; `doctor` install smoke test; `saga init`, `saga install` and the composed `saga hook` entry (contracts §1, §9); task set for 3 languages (30 per language, bench-spec §2.6); one adapter (Claude Code) | Bare harness baseline published with pass^k and variance for 2 models; ledger reconciles with provider-reported usage on the same sessions within 2% over ≥ 20 sessions and 3 days (trace-spec §3.4); canary subset (20 tasks daily plus a 60-run burst on any pin change) runs on a schedule (trace-spec §4.4; about $17 per daily cycle and $51 per burst at the spec's $0.85 cost hint, $46 and $138 at doc 05 §1.2's $2.30 per solve, so $500 to $5,500 per month per model-harness pair depending on task cost and how often pins change, see the cost note at the end of §5); trace hook overhead ≤ 1% wall time and zero injected tokens (trace-spec §11.2) |
| **M1 Gate + Guard + compaction survival** | ledger grammar, red proof, claim verification (trace-spec §5.5 to §5.9, cited by gate-spec §6), diff guards, Stop adapters (Claude Code, Gemini CLI, Codex, CI); post-expansion command classifier with allow-list policy, the shared `saga snapshot` primitive and `saga undo`, secret masking, package existence and age check; `mem` PreCompact/SessionStart state block | Measured effect on M0 task set, primary metric false-done rate (gate-spec §10.3); zero escapes on incident fixtures I-01 to I-18 and zero denies on the C-row benign controls across bash, zsh, PowerShell and cmd (guard-spec §2.7); masking recall ≥ 0.99 on the positive-control suite and zero masks on the negative-control suite (guard-spec §4.5); state block ≤ 600 est. tokens with compaction-survival rate reported per harness (mem-spec §4.2, §9.2); passes on Windows |
| **M2 Index** | tree-sitter graph, BM25, test-impact map, 4 tools, MCP server, incremental update, regime gating, external API cache | Localization acc@5 and resolve delta reproduced on the M0 set with control arm blocked; resident context tokens at session end ≤ 1.5× the control arm (index-spec §9.2) |
| **M3 Memory** | typed records, targeted PreToolUse injection, freshness, sub-agent preamble, procedures | Compliance-over-session-length curve measured with and without, hypotheses H1 to H4 pre-registered (mem-spec §5.7); drift rate on prose-only rules reported, not hidden; a null on H1 and H2 cuts targeted injection to the state block (mem-spec §5.7, §9.2) |
| **M4 Shape** | output parsers for the top runners per language, error-aware truncation, edit verification, result cache, comment stripper | `tokens_per_solved` down ≥ 8% with a 95% CI excluding zero and pass^k not down (shape-spec §8, §10.6); raw and shaped tool-output bytes per turn visible in the ledger (trace-spec §3.2) |
| **M5 Route** | policy file from §4.2, budget, effort pinning, cheap-model delegation for read-only sub-tasks, schema repair for open-weight models (guard-spec §12) | `cost_per_solved` down with pass^10 inside ε = 0.05 on at least two of three model pairs (route-spec §8.2); no silent model change in any bench run; `unknown` class rate ≤ 20% (route-spec §2.4) |
| **M6 Portability** | remaining adapters (Cursor, OpenCode, Devin Desktop, Cline, Kilo), portable trace resume, MCP gateway | Same install on ≥3 harnesses, no per-harness prompt rewrites; gateway blocks the MCPTox fixture set (guard-spec §6.4) |

Changes from v0.2, one line each:

- **M0 gains the cost ledger, version pinning, and watchdog.** Doc 07 §6 ranks cost opacity first and §8 shows pinning and recording are the only external determinism levers; the bench needs the ledger anyway, so it is not extra scope.
- **M0 gains `doctor`.** A 0% trigger rate from four independent causes (anthropics/skills #556) means an adapter that does not prove it fires cannot be trusted to have run the control arm.
- **M1 gains post-expansion command validation and snapshots.** The destructive-action incidents are permission-layer bugs with a known fix, and the community already runs the fix as a PreToolUse hook (#28240), so it is the highest-leverage guard feature and cheaper than masking.
- **M1 gains the compaction state block, moved out of M3.** Compaction amnesia is ranked second in doc 07 §6, the mechanism is mechanical and user-validated (#34556), and it shares the hook seam with Stop enforcement; targeted injection, which needs the bench to catch a regression, stays in M3.
- **M1 exit criterion adds Windows.** Doc 07 §6 item 14 and unlazy #30 ("every file read fails closed on Windows").
- **M5 adds effort pinning and schema repair.** The largest "got dumber" episode was an effort default change (#42796); open-weight models loop on tool syntax without repair (doc 06 D.2 item 11).
- **M6 adds the MCP gateway and swaps Windsurf for Devin Desktop.** Harnesses already shipped MCP scoping, so the remaining value (description scanning, logging) is lower priority; windsurf.com now redirects to devin.ai (doc 06 Part B).

Changes from v0.3 (spec review, 2026-09-03), one line each:

- **M0 gains `saga init`, `saga install` and the composed hook entry.** Harnesses accept one `updatedInput` and one `reason` per event, so the layers cannot each own a hook (shape-spec §7, contracts §1).
- **M0 exit criterion is numeric.** Reconciliation tolerance, canary cadence and trace overhead bound come from trace-spec §3.4, §4.4 and §11.2.
- **M1 exit criterion names the fixture suites.** Incident rows and benign controls (guard-spec §2.7), masking bars (guard-spec §4.5), state-block ceiling (mem-spec §4.2); v0.3's "zero false-positive masking on the positive-control suite" was the wrong suite.
- **M1's snapshots are one primitive.** Guard, trace, shape and gate had four stores (REVIEW-LOG item 7); guard-spec §3.1 now owns `saga snapshot`.
- **M1's claim verification is owned by trace.** Trace-spec §5.5 to §5.9 computes it, gate-spec §6 enforces it at Stop, and `claim_contradiction_rate` joins bench-spec §5.11 (REVIEW-LOG risk 1, resolved).
- **M2 adds the resident-context ceiling.** codegraph's +80% resident tokens (doc 04 §2.4) made it a mandatory cost in index-spec §9.2.
- **M3 carries pre-registered hypotheses and a cut rule.** mem-spec §5.7.
- **M4 adds edit verification and a numeric primary.** shape-spec §4 and §8.
- **M5's decision rule and `unknown` cap come from route-spec §8.2 and §2.4; schema repair is deferred to guard-spec §12.**
- **Exit codes, schema ids, `.saga/` layout, ledger keys and token ceilings are now shared contracts** (contracts §2, §4, §7, §11), summing to a 3,800-token per-session injected budget across layers.

Order rationale, unchanged: the strongest evidence and smallest surface are in gates and guards; the index has the largest measured gain but a bigger build; targeted memory injection has the largest gap between hype and evidence, so it comes after the benchmark can catch a regression.

Cost note (2026-09-03, from bench-spec §4.4 and trace-spec §4.4 run counts). The `publish` tier at its minimum is 2 model families × 1 harness × 3 arms × 90 tasks × K=10 = 5,400 runs; at the bench's own S/M cost hint of $0.85 per run with the default 1.3× treatment multiplier on two arms that is 1,800 × $0.85 + 3,600 × $0.85 × 1.3 ≈ $5,500, and at doc 05 §1.2's $2.30 per solve it is ≈ $14,900; the full six-rung ladder (7 arms, 12,600 runs) is ≈ $13,500 to $36,400. The earlier $500 to $2,000 figure was wrong by 3 to 7× and is corrected in bench-spec §4.4. The canary is 20 runs per day plus a 60-run burst per pin change: 30 × $17 ≈ $510 per month with no bursts, 30 × ($17 + $51) ≈ $2,040 if a pin changes daily, and $1,380 to $5,520 at $2.30 per run. L and XL tasks raise every figure above; the M0 exit criterion publishes the measured numbers.

## 6. Cost versus time

Default order of preference: correctness, then cost, then speed. Concretely: a gate run that costs a minute is always preferred to skipping it; a cheaper model is preferred for read-only exploration; a structural index is preferred to re-reading files; a snapshot before every write is preferred to a faster turn. The user can flip any of these per repo in `.saga/config`. Every flip is logged in the trace so the benchmark can show what it cost, and the ledger shows what each layer adds per turn so the flip is informed.

## 7. Open questions carried forward

Resolved in v0.3:

4. **Which per-model differences justify routing rules.** Only three kinds survive the noise floor (3.5 to 4.5 point standard error, doc 06 A.10; SWE-bench Verified saturated, A.0): (a) cost tiers for read-only and bulk work, where Sonnet 5, Luna and Flash are measurably adequate; (b) task-class winners with a documented mechanism, not a leaderboard delta: security review to Opus (38/40 blind investigations), long-horizon to Fable 5.1 or Opus 5 (METR horizon), algorithmic and frontend one-shots to Gemini 3.1 Pro or Kimi K3; (c) open-weight and local models need schema repair. Everything else, including most head-to-head SWE-bench Pro gaps, is within noise and is not a routing rule. Effort level is a stronger routing variable than model choice for Claude models (#42796).
5. **Which harness pain points are structural and which are backlog.** Structural, outside any external layer: terminal rendering, the edit tool itself, sandbox leaks, vendor fallback, subscription lock-in, the 60-second auto-answer (doc 07 §6 items 10, 16, 17, 19 and the tool half of 4). Backlog that an external layer can ship now, because users already did: cost ledger, compaction state block, post-expansion command classification, per-turn snapshots, loop and stall watchdog, claim verification, skill load-time validation, version pinning and canary (doc 07 §9). Incentive-bound rather than technical: standards adoption lag (AGENTS.md 12 months, ACP declined, XDG ignored; doc 07 §4 item 10).

Still open:

1. Line-level localization recall (about 15%) is the bottleneck no index fixes; a learned reranker on PR data may be needed. *(doc 05 §10)*
2. Gate quality beyond lexical lint: red proof helps, but an oracle can still measure the wrong thing.
3. Interaction between dynamic knowledge injection and stable prompt prefixes for caching; claude-code #91514 shows a warm cache fully re-written seconds after a ToolSearch or Skill result, so injection placement has a cache cost that the ledger must expose. *(doc 05 §10 item 5, doc 07 §8)*
6. **Completeness of post-expansion classification across shells.** What fraction of real commands is unresolvable (unknown variables, `eval`, dynamic `sh -c`) and therefore falls back to a prompt, per shell (bash, zsh, PowerShell, cmd)? If it is high, guard reintroduces the permission fatigue it is meant to remove.
7. **Snapshot cost on large working trees.** Per-turn snapshots are constant-time only in the `zfs` and `btrfs` modes, where the repo is its own dataset or subvolume; everywhere else, including APFS and NTFS, the default `git-tree` mode costs time proportional to changed bytes, and APFS volume snapshots are never used (contracts §5, guard-spec §3.1). The threshold at which the git-tree snapshot is slower than the turn it protects is unmeasured, and monorepos will find it.
8. **Canary statistical power.** With a 3.5 to 4.5 point standard error per model, how many canary runs are needed to detect a 5-point regression at reasonable confidence, and what does that cost per day? The answer sets whether the canary is a daily job or a per-release job.
9. **Does targeted injection drift like standing injection?** caveman #303 says per-turn re-injection is not sufficient; whether adjacency to the tool call changes that is exactly the M3 measurement, and the honest prior is "less, not zero".
10. **Privacy under covered-model retention.** Fable and Mythos retain 30 days even under ZDR (doc 06 C.1); the privacy layer can reduce what is retained but cannot change the policy, and the README must say so.
11. **Which incidents count as fixtures.** The destructive-command positive-control suite is seeded from doc 06; maintaining it as harnesses and frameworks add new wipe commands is an ongoing curation cost with no owner yet.

## 8. Naming and positioning

Saga is "the story humanity keeps telling": the same complaints, every model generation. The project's job is to end the retelling by making the story checkable. Positioning in one line for the README: **Saga makes any coding agent prove its work: a local, model-agnostic layer for repository knowledge, machine-checked completion, memory that survives compaction, command validation after shell expansion, a per-turn cost ledger, and privacy at the tool boundary, with a benchmark and a canary that keep it honest.**
