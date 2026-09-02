> **Provenance.** Research report produced 2026-09-02 by a Claude (Fable 5.1) research agent for the Saga project, ~55 web searches/fetches plus live GitHub API metadata. Vendor claims are labelled; independent measurements are cited with URLs. Treat every number as of that date.

# Do "coding-agent enhancers" actually work? A critical survey of the 2025-2026 open-source ecosystem

*Prepared 2026-09-02. Star counts pulled live via `gh api` on that date. ~55 web searches/fetches plus GitHub API queries. Where a number is a vendor claim it is labelled as such; where it is an independent measurement the source is named.*

---

## 0. TL;DR

1. **The ecosystem is enormous and mostly unmeasured.** Eleven repos in this space now exceed 100k stars (superpowers 281k, ECC 246k, mattpocock/skills 245k, karpathy-skills 210k, deepseek-harness 209k, opencode 203k, anthropics/skills 173k, spec-kit 133k, gstack 131k, ponytail 121k, graphify 114k, caveman 102k). Stars track virality (a single `CLAUDE.md` hit 200k), not measured benefit. Only a handful of tools have ever been tested by someone other than their author.
2. **Where independent measurement exists, prompt-only tools deliver ~10% of what they advertise.** JetBrains' paired A/B runs on SkillsBench found caveman saves 8.5% of output tokens (advertised 65%) and ponytail cuts code 15% (advertised 54%), with no detectable quality change either way. A 7-word YAGNI line captured a third of ponytail's gain.
3. **The categories with real, reproducible gains are deterministic ones**: hooks that block/redact at the tool boundary, tool-output compression (rtk, context-mode, Squeez), LSP/AST retrieval on *large* codebases (serena, codegraph), sandboxes, and curated *domain* skills (SkillsBench: +16.6pp average, but only +4.5pp for software engineering). Harness engineering is where the big numbers live (LangChain 52.8→66.5 on Terminal-Bench by changing only the harness; CORE-Bench 42% vs 78% same model, different scaffold).
4. **Process frameworks (BMAD, spec-kit, GSD, superpowers) are "bloated, not wrong."** They help junior/novice users and large greenfield builds; they impose 20-500% overhead on routine tasks; and they govern by convention, not enforcement, the model can ignore the spec and often does.
5. **Memory tools have solved storage and not retrieval.** claude-mem retreated from auto-injection after context pollution; one user's 8,785 stored observations were never surfaced unprompted. LOCOMO-saturated memory systems fail in agentic settings (MemoryArena).
6. **Some of the most-starred tools are actively harmful**: ruflo/claude-flow (70k★) was audited as ~290/300 MCP tools being stubs, with hard-coded "token savings" counters, and shipped a CVSS 10.0 unauthenticated RCE. ECC installs 28 hooks across 7 lifecycle events globally with unsigned auto-updates.
7. **The unsolved problems are structural**: instruction compliance decays inside a session (~5.6% lower odds per function written; 95%→20-60% by message 6-10); no tool can verify that the agent did what it said; no one has a cheap, model-agnostic way to A/B a change to the harness; and "skills" have become a supply-chain attack surface with no signing or review.

---

## 1. Method and caveats

- Sources: GitHub API for stars/dates; READMEs; independent test reports (JetBrains AI blog, ManoMano, roman-rr audit, joergmichno audit); arXiv papers (SkillsBench 2602.12670, SkillReducer 2603.29919, SlopCodeBench 2603.24755, McMillan 2605.10039, ETH AGENTS.md 2602.11988, Squeez 2604.04979, "Do LLMs benefit from their own words" 2602.24287, "Control under compression" 2608.01056); issue trackers; HN/dev.to/Medium reviews.
- Bias warning: most "reviews" of these tools are SEO content from companies selling adjacent products (Augment, MindStudio, Firecrawl, mcp.directory). I have leaned on paired/controlled measurements wherever they exist and flagged everything else as anecdote.
- Verdict scale: **works** (independent evidence of gain), **partially** (gain under specific conditions or far below claim), **bloated claims** (claim materially contradicted by evidence), **unknown** (no evidence either way).

---

## 2. Tool-by-tool records

### 2.1 Skill collections (prompt-only, model-invoked)

**obra/superpowers**, https://github.com/obra/superpowers, 281k★
- Claims: "agentic skills framework & software development methodology that works": brainstorm → plan → TDD → subagent review.
- Mechanism: prompt text only (SKILL.md files + a bootstrap that injects ~1.4k tokens of skill index each session), distributed as a Claude Code plugin.
- Evidence: One controlled comparison cited by joanmedia.dev: 9% cheaper / 14% fewer tokens on *non-trivial* tasks; costs *more* on simple tasks. The project's own changelog is the most honest evidence: v5.0.6 removed the subagent review loop after finding ~25 minutes of overhead with no quality gain; v6.0.0 merged redundant reviewers ("up to 60% cheaper"); v6.1.0 compressed the bootstrap.
- Criticism: HN ~60/40 favourable; "burned through my whole Max plan on a straightforward task"; rigid for exploratory work; over-engineered for capable models that plan unprompted.
- Verdict: **partially**. The methodology is sound; the token cost is real; nobody has shown a correctness gain on a benchmark.

**mattpocock/skills**, https://github.com/mattpocock/skills, 245k★ (created 2026-02)
- Claims: "Skills for Real Engineers", /grill-me, /grill-with-docs, tdd, domain-modeling, code-review, handoff.
- Mechanism: prompt-only markdown; explicitly positioned against GSD/BMAD/spec-kit as "they take away your control".
- Evidence: none quantified; author-credibility driven. Verdict: **unknown** (well-written, cheap, unmeasured).

**anthropics/skills**, https://github.com/anthropics/skills, 173k★
- Claims: official reference skills (docx/pptx/xlsx/pdf handling, skill-creator).
- Mechanism: prompt + bundled scripts; progressive disclosure is the design principle. skill-creator now ships **Eval and Benchmark modes** (with-skill vs. without-skill side-by-side), the only first-party attempt to make skill efficacy measurable.
- Evidence: SkillsBench-style paired evals baked in. Verdict: **works** for document skills (they encode procedural knowledge the model lacks); the *methodology* (benchmark mode) is the more important contribution.

**vercel-labs/skills (skills.sh)**, https://github.com/vercel-labs/skills, 30k★
- Claims: `npx skills add` package manager for 77+ agents.
- Mechanism: installer + directory. **No evaluation, no security scanning, no signing.** Verdict: **works as a distribution channel; does nothing for effectiveness**, and it is now the primary vector for unreviewed prompt text landing in people's agents.

**Leonxlnx/unlazy**, https://github.com/Leonxlnx/unlazy, 3k★
- Claims: anti-laziness via "Depth Tree" (split task N layers deep, give every leaf the full budget) plus **executable verification gates** and an optional Stop hook that blocks incomplete work.
- Mechanism: prompt + shell gate-check script + hook. Notable for admitting its own internal benchmarks "lack reproducible artifacts, treat as historical design input, not a guarantee."
- Evidence: cites SlopCodeBench (agents pass only 14.8% of checkpoints while claiming completion) as motivation, not proof. Verdict: **unknown, but honest**; the gate mechanism is the right shape.

**multica-ai/andrej-karpathy-skills**, 210k★, a single CLAUDE.md with four principles (think before coding, simplicity, surgical changes, goal-driven execution). No evidence. See §2.7 on CLAUDE.md research: presence of a file moves compliance from 0% to ~68% on a given rule, but structure/length don't matter and compliance decays within a session. Verdict: **partially**, cheap, does something, wildly over-starred.

**Indexes**: awesome-claude-code (53k★, curated, has an "excused" label for disputed listings), ComposioHQ/awesome-claude-skills (74k★, 1,383 open issues, effectively unmoderated), VoltAgent/awesome-agent-skills (34k★), travisvn/awesome-claude-skills (15k★, stale since April). ~3,500 new skills/week are being published (ClaudSkills tracker). Verdict: **noise amplifiers**.

### 2.2 Process frameworks (structured workflow, mostly prompt-only)

**gsd-build/get-shit-done**, https://github.com/gsd-build/get-shit-done, 65k★, *archived 2026-06-26*, continues as "Open GSD"/GSD Core.
- Mechanism: slash commands + phase directories + fresh-context subagents per phase ("context engineering"). Evidence: none controlled; large adoption. Criticism: heavy; the archive/fork churn hurt trust. Verdict: **partially** (context-isolation per phase is a genuinely good mechanism; the rest is process).

**bmad-code-org/BMAD-METHOD**, https://github.com/bmad-code-org/BMAD-METHOD, 53k★
- Mechanism: agile role personas (PM, architect, dev, QA) producing PRD/architecture docs.
- Evidence against: ~31.7k tokens per workflow run; $800-2,000+/month/dev reported; a CRM-dashboard comparison: OpenSpec 12 min, spec-kit 90 min, **BMAD 5.5 h**. Issue #1332: review workflow forces a minimum of 3 issues per review → endless nitpick cycles. Issue #1235: excessive token usage.
- Verdict: **bloated claims** for anything below a multi-week build.

**github/spec-kit**, https://github.com/github/spec-kit, 133k★
- Mechanism: /specify → /plan → /tasks → /implement templates.
- Evidence: Scott Logic hands-on: "a sea of markdown, long agent run-times, unexpected friction", >2,000 lines of markdown from one plan phase; discussion #1784 "creates the illusion of work"; practitioners: "spend hours correcting specs because the LLM makes mistakes forming them." Key structural point (codemyspec): the documents govern by convention, not enforcement, nothing checks the code against the spec.
- Verdict: **partially**, useful on greenfield with a team that already wants specs; net-negative for small changes.

**buildermethods/agent-os**, 5.4k★, standards extraction + spec shaping. No evidence. **unknown**.

**ruvnet/ruflo (claude-flow)**, https://github.com/ruvnet/ruflo, 70k★
- Claims: "agent meta-harness", swarms, "30-50% token reduction", "352× faster", neural learning.
- Independent audit (roman-rr gist, v3.5.51, April 2026): ~290 of 300+ MCP tools are JSON state stubs with no execution backend; `totalTokensSaved += 100` hard-coded per cache hit; "352× faster" benchmarked against a `sleep(352)`; quantization reports 3.92× without converting anything; adds 15-25k tokens/session of routing noise. CVE-2026-59726 (CVSS 10.0) unauthenticated RCE + memory poisoning. Issue #1425 "this codebase is so highly cursed"; awesome-claude-code issue #1338 asked for a disclaimer (closed "excused").
- Verdict: **bloated claims / harmful**. The most important cautionary tale in the ecosystem.

**Yeachan-Heo/oh-my-claudecode**, 39k★, team-plan → prd → exec → verify → fix pipelines, 19 agents, tmux workers, "30-50% token savings via model routing" (no baseline published). 4,500 commits, heavy tmux/Linux dependence. Verdict: **unknown** (plausible model-routing savings; multi-agent coordination usually *raises* total tokens).

**SuperClaude_Framework**, 24k★, commands + 11 personas + modes. Issue #286: the ~8k-token framework can be cut to ~3.2k with 95% of function retained (concepts repeated 3-5×). Verdict: **partially / bloated**.

**garrytan/gstack**, 131k★ (created 2026-03), 23 role prompts (CEO, EM, QA…) plus a real Chromium `/browse` skill and JSONL learning ledgers. Evidence offered: the author's own "810× more LOC/day than 2013", self-reported LOC, with a whole doc defending it. Verdict: **unknown**; the browser tool and ledgers are the real substance, the persona prompts are marketing.

**davila7/claude-code-templates**, 30k★, installer + analytics dashboard. Distribution, not effectiveness. **works as tooling; no effectiveness claim to test.**

**affaan-m/ECC (Everything Claude Code)**, https://github.com/affaan-m/ECC, 246k★
- Mechanism: 286 skills, 68 agents, 103 rules, 28 hooks across 7 lifecycle events installed *globally*, Memory Vault, "AgentShield". No benchmarks.
- Audit (joergmichno, dev.to): 513 auto-loadable instruction files; 49/64 agents grant Bash; two wildcard PreToolUse hooks run before *every* tool call; auto-update is an unsigned `git pull`; a malicious re-upload (`arabicapp/everything-claude-code`) shipped a LuaJIT infostealer. The README itself warns about install-method stacking and provides `ECC_DISABLED_HOOKS`.
- Verdict: **bloated claims** + supply-chain risk. "Optimize the context window" is the slogan; the artifact is the largest context surface in the ecosystem.

### 2.3 Token / verbosity tools

**JuliusBrussee/caveman**, https://github.com/JuliusBrussee/caveman, 102k★, see §3.
**DietrichGebert/ponytail**, https://github.com/DietrichGebert/ponytail, 121k★, see §3.

**rtk-ai/rtk**, https://github.com/rtk-ai/rtk, 78k★ (created 2026-01)
- Mechanism: single Rust binary; PreToolUse hook rewrites `git status` → `rtk git status`; filters/groups/dedupes/truncates command output for 100+ commands.
- Claims vs. honesty: "60-90%" is of *bash output bytes* (estimated bytes/4), and the README explicitly says the reduction "dilutes at every step" because bash output is a fraction of input tokens. Built-in Read/Grep/Glob bypass it.
- Verdict: **works** for what it does, deterministic, lossless-ish, cheap, but expect single-digit to low-double-digit bill savings, not 90%.

**mksglu/context-mode**, ~2.5k★, MCP + hooks (PreToolUse on Bash/Read/WebFetch/Grep/Task, PostToolUse, PreCompact, SessionStart). Runs tools in a subprocess, only stdout summaries enter context; SQLite/FTS5 knowledge base. "98%" = raw bytes vs summary bytes, no independent validation; ~60% without full hook support; known better-sqlite3 SIGSEGV issues. Verdict: **partially**, the mechanism (sandboxed execution, summary-only return) is exactly right; the number is a marketing artefact.

**microsoft/LLMLingua**, 6.6k★, last push 2026-04. Perplexity-based token dropping. Research on tool-using agents ("Control under compression", 2608.01056): fine at 75% retained context (92.7% vs 93.8% success) but collapses below 50% (generic rewriting 19.9% success at 35%). Structure-aware compression of tool schemas beats LLMLingua-2 (74.8% vs 50.8% savings at equal accuracy). Verdict: **works in narrow band; dangerous applied to agent control context**.

**Squeez** (arXiv 2604.04979), fine-tuned 2B pruner returns "smallest verbatim evidence block" from tool output; 0.86 recall at 92% token removal on SWE-bench-derived data. Research, not a product yet, but this is the direction rtk/context-mode are groping toward.

**Meta-stacks**: `oratelecom/tokenwar` (caveman + rtk + context-mode + claude-mem + ponytail + pxpipe), `FlorianBruniaux/flow-lean`, `OmniRoute` (60k★ gateway advertising "RTK+Caveman compression saves 15-95%"). Stacking prompt tools whose *own* rules cost 1-1.5k tokens each per turn is self-defeating; nobody has measured the stack.

### 2.4 Context / knowledge tools

**upstash/context7**, 61.5k★, docs MCP. Real problem (stale API knowledge). Criticisms: free tier cut ~85% in Jan 2026; community-contributed docs unverified ("ContextCrush" vuln); large responses eat context; indexing lag; cloud dependency. Verdict: **works** when you actually need a specific library version; otherwise a context tax.

**oraios/serena**, 28.7k★, LSP-backed MCP (find_symbol, references, symbolic edit). ManoMano's 36K-line Java benchmark: simple business-rule lookup **4× the cost and 60% slower** with serena; function-usage search ≈ equal; complex multi-file modification succeeded ($27.30, 45 min, all tests green). Anthropic's own built-in LSP was reported hallucinating same-named methods. Verdict: **works on large codebases, net negative on small ones**, the cleanest "it depends" in the survey.

**colbymchenry/codegraph**, 69k★, Rust/tree-sitter pre-index in SQLite, one MCP tool `codegraph_explore`. Their own 7-repo benchmark (Opus 4.8, median of 4, CLI blocked in both arms, a rare methodological care): 88% fewer tool calls, 62% fewer tokens processed, 44% lower cost, **but 80% more tokens resident in context at end of session** (67k vs 18k) because dense answers stay in view, and 74-100% coverage depending on language (DI, reflection, runtime dispatch invisible). Verdict: **works** (self-reported but carefully) with a real trade-off nobody else discloses.

**Graphify-Labs/graphify**, 114k★, tree-sitter AST → graph.html/json, PreToolUse nudge hook, `EXTRACTED`/`INFERRED` confidence tags. Odd evidence: it reports LOCOMO/LongMemEval (conversational-memory benchmarks) rather than any code benchmark. Reviewers: on a 40-file project the graph took longer to build than the bug took to find; update ~10s+ on monorepos, too slow for a per-turn hook. Verdict: **partially**, great visualisation, "71× fewer tokens" is unsupported for coding tasks.

**Egonex-AI/Understand-Anything** (81k★), **potpie** (5.7k★, "context graph for SDLC"), Augment Context Engine (spun out as MCP Feb 2026), Greptile (PR review), Sourcegraph Cody (still shipping, index-first). No independent head-to-head; Zylos' survey concludes hybrid "graph + on-demand retrieval" has the best architectural coverage. **unknown/partially.**

**repomix** (28k★), **gitingest** (15k★), **code2prompt** (7.6k★), aider repo-map, deterministic packers. They do exactly what they say. The failure mode is the user's: dumping 300k tokens of repo into a model whose effective attention is a fraction of that. **works (as tools); frequently misused.**

**Memory layer**, thedotmack/claude-mem (93k★), mem0 (65k★), letta (25k★), graphiti (30.5k★), cognee (30k★):
- claude-mem: hooks capture everything, an LLM compresses, injects on SessionStart. v3 auto-injected everything → "context pollution"; v4 retreated to an ~800-token index that must be searched. User report: 8,785 observations across 13 projects, **never surfaced unless asked**. Issue #1464: every spawned teammate inherits the full injection → cost scales linearly with agent count.
- mem0/Zep: public LOCOMO dispute (Zep claims 75.1 vs Mem0's reported 66.0 for Zep); mem0 reports 92.5 LOCOMO at ~6.9k tokens/query. MemoryArena: LOCOMO-saturated systems fail in agentic settings. "Do LLMs benefit from their own words" (2602.24287): replacing prior assistant turns with summaries gives equal quality at 8× less context; retaining your own past outputs propagates errors.
- graphiti has no native MCP/hooks for Claude Code; cognee has a code-graph pipeline + 14 MCP tools; letta is a stateful-agent platform more than a coding-agent add-on.
- Verdict: **storage works, retrieval/injection does not** (alexandrekhoury's framing, corroborated by claude-mem's own version history). Nobody has shown a coding-task success improvement from cross-session memory.

### 2.5 Verification / guardrail tools

**Hooks** (Claude Code PreToolUse/PostToolUse/Stop/SessionStart; also in Codex/Gemini CLI/DSH):
- disler/claude-code-hooks-mastery (3.9k★), parcadei/Continuous-Claude-v3 (3.9k★, ledgers + handoffs), karanb192/claude-code-hooks (safety/cost plugins). Every serious write-up (zarar.dev, ranthebuilder, bitbytebit, paddo.dev) converges on the same sentence: *"Don't rely on instructions; hooks are deterministic."*
- Verdict: **works**, this is the one primitive with no counter-evidence, because it doesn't depend on the model listening.

**Secret/PII**: gitleaks (29k★) in pre-commit; **JeongJaeSoon/agent-guard** (25★, brand new) wraps gitleaks at the *tool boundary* (blocks `.env` Read, `printenv`, masks PostToolUse output, scans UserPromptSubmit) and ships a deterministic coverage benchmark. l-mb/claude-code-redaction-hooks, noirdoc (Presidio + GLiNER ensemble PreToolUse). Motivation is real: GitGuardian 2026, Claude-Code-co-authored commits leaked secrets at ~2× baseline (3.2%); 24,008 secrets in public MCP config files. Anthropic issue #29434 requests native redaction. Limitations (agent-guard's own): gitignored files unscanned, fixed blocklists, user shell escapes bypass hooks. Verdict: **works** for the pattern class it targets; a strong candidate for "should be in the harness, not a plugin".

**protectai/llm-guard** (3.2k★), **presidio** (10.7k★): general LLM I/O scanners, usually deployed as a proxy (LiteLLM tutorial). Nobody has shown them integrated with a coding agent without breaking code (false positives on high-entropy strings). **partially.**

**Prompt-injection defences**: academic (CaMeL: 77% task completion vs 84% undefended; CASCADE; ClawGuard), none shipped as a Claude Code plugin. Reality check: JHU (April 2026) hijacked Claude Code, Gemini CLI and Copilot via PR *titles* and exfiltrated Actions secrets; Rehberger filed CVEs against six agents in one month; "SANDWORM_MODE" typosquatted MCP servers on npm; Adversa found a deny-rule bypass after the March 2026 source leak. OWASP Agentic Top-10 ranks goal hijacking #1. Verdict: **unsolved**; the only things that work are capability reduction (allow-lists, scoped tokens, sandboxes), not detection.

**Sandboxes**: `@anthropic-ai/sandbox-runtime` (Seatbelt/bubblewrap around the whole process incl. hooks and MCP; beta), Docker's Claude Code sandbox template (note: launches with `--dangerously-skip-permissions` by default, trading prompts for isolation), E2B Firecracker microVMs (GA Jan 2026). Verdict: **works**, isolation is the only defence that doesn't depend on detection.

### 2.6 Multi-agent orchestration

- **Anthropic Agent Teams** (Feb 2026, Opus 4.6; `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`): official, still experimental (no resumption, no nesting).
- **smtg-ai/claude-squad** (8.4k★): tmux + worktree manager. Does what it says. **works (utility)**.
- **BloopAI/vibe-kanban** (28k★): kanban UI; **539 open issues, no push since 2026-04-24**, effectively abandoned.
- **paperclipai/paperclip** (80k★, created 2026-03): "agent company" control plane, heartbeats, atomic task checkout, worktrees, **per-agent monthly budgets that hard-stop**, approvals, telemetry on by default. 5,337 open issues. Budget enforcement is the one genuinely new mechanism. **unknown** on outcomes.
- **nwiizo/ccswarm** (149★): niche.
- **Ralph loops** (snarktank/ralph 21.7k★ but dormant since Feb; frankbria/ralph-claude-code 9.6k★ adds exit detection/rate limits; open-ralph-wiggum, ralphex, continuous-claude, vercel ralph-loop-agent, gemini-cli ralph extension): run until a completion criterion is met. Evidence: works spectacularly when the criterion is objective (tests pass), The Register's "vibe-clone commercial software for $10/hour"; fails or runs forever when it isn't; 50-iteration loops $50-100+; memory is "git diffs tell you what, not why." **works conditionally**, and it is really a verification-gate pattern wearing a loop costume.
- Structural fact everyone agrees on: multi-agent *raises* total tokens (per-agent system prompts, coordination payloads, duplicated injections). Ruflo's "swarm saves tokens" claim is the inverse of reality.

### 2.7 What we know about CLAUDE.md / AGENTS.md itself (the substrate every prompt tool sits on)

- McMillan (arXiv 2605.10039, 1,650 sessions, 16,050 functions): file length (25-500 lines), rule position, single-vs-nested architecture, and self-contradiction had **no detectable effect** on compliance. File *presence* moved one rule from 0/524 to 67.7%. New-code tasks 71.3% compliance vs. editing existing code 45.1%. **Each additional function written lowers compliance odds by 5.6%** (OR 0.944, p≈1e-49).
- ETH Zurich AGENTS.md study (2602.11988, 300 SWE-bench tasks): context files "do not generally improve task success while increasing cost >20%"; LLM-generated (`/init`) files *reduced* success; human-written files ~+4%; Claude Code was the one agent where even human files didn't help; stripping repo docs made context files help (+2.7%), i.e. they are largely redundant with existing docs.
- Compliance decay (Khare): 95%+ at messages 1-2 → 20-60% by messages 6-10. HumanLayer: models follow ~150-200 instructions; Claude Code's system prompt already has ~50.
- SkillReducer (2603.29919): across published skills, 26.4% lack routing descriptions, >60% of body text is non-actionable; compressing 39-48% *improved* quality 2.8%.

This is the quantitative backdrop for every "just add this markdown" tool: it works a bit, at the start of a session, for new code, and then stops.

---

## 3. Deep dive: caveman and ponytail

### caveman (102k★)
- **Claim**: "cuts 65% of tokens by talking like caveman" (1,214 → 294 tokens/reply across 10 chat-style tasks). Prompt-only SKILL.md with lite/full/ultra/Wenyan variants; companion proxy claims 33% input reduction; a browser-compression mode claims "129.8× smaller".
- **Independent test** (JetBrains AI, July 2026; Harbor 0.17, SkillsBench 86 tasks, Sonnet 5, ~240 paired trials, $106; skill *forced on* every turn = best case): **−8.5% output tokens** on the full run (542k vs 592k; a 10-task pilot showed −29.5% with high variance). Quality: 8 improved / 10 worsened / 64 tied, sign-test p=0.82, mean Δ −0.015. Verdict quote: "safe but oversold." Real users activate it inconsistently, so 8.5% is a ceiling.
- **Why**: agentic output is code, diffs and tool calls, which caveman promises not to touch; only the thin narration between tool calls is compressed. The skill's own rules cost 1-1.5k *input* tokens per turn, which at short sessions can exceed the saving. Rushis.com: nobody has run CAVEMAN-Claude on SWE-bench/HumanEval. Credit where due: the README now links `HONEST-NUMBERS.md` and states "the skill only shortens output… whole-session savings land lower than the chart."
- **Verdict: bloated claims, harmless mechanism.** ~10% cost reduction at best, no quality effect detectable, and a 100k-star lesson that "tokens per reply in a chat demo" ≠ "tokens per task in an agent".

### ponytail (121k★, created 2026-06-12, 44k stars in 9 days)
- **Claim**: "lazy senior dev" ladder (does it need to exist → reuse → stdlib → native platform → installed dep → one line → minimum code). Own benchmark (12 FastAPI/React tasks, Haiku 4.5, n=4): −54% code, −22% tokens, −20% cost, −27% time; canonical demo is `<input type="date">` instead of a 404-line date-picker (−94%). Mechanism: AGENTS.md ruleset + Node lifecycle hooks (SessionStart injection) + `/ponytail` command; also ships as plain rule files for 20+ hosts.
- **Independent test** (JetBrains, ponytail v4.8.4, Harbor 0.18, 80 paired SkillsBench tasks, Sonnet 5 medium, Wilcoxon on per-task medians): **−15.4% code** (median), **−10.3% cost (p=0.004**, the strongest effect in JetBrains' whole series), −11% time, quality: 65/80 identical, no detectable difference (explicitly a null result, not equivalence). Bigger builds −31% code; already-minimal tasks ≈0.
- **Criticisms** (dev.to/yashddesai, JetBrains): (1) installed passively, ponytail **self-activated zero times**, every gain depends on forced injection; the suggested `ponytail:` code comments appeared in 1/80 runs. (2) The vendor baseline was unusually verbose; a **7-word YAGNI instruction captured ~33% of ponytail's gain**. (3) Design-system blindness: the ladder ranks "native platform" above "installed dependency", so it may emit a bare `<input>` in a shadcn/MUI codebase; the benchmark repo had no design system. (4) On llama3.2-3B results are near random, it needs a frontier instruction-follower. (5) "Raises the probability of simplicity; cannot replace review."
- **Verdict: partially.** It is the best-evidenced prompt-only tool in the ecosystem, a real, statistically significant ~10% cost cut on non-trivial tasks, and still roughly a third of what the README advertises. `ponytail-lite` (one AGENTS.md, no plugin) exists precisely because the plugin machinery adds nothing measurable.

### Shared lesson
Both tools are *style* constraints. Style constraints get ~10% because (a) most agentic tokens aren't prose, (b) the constraint itself costs tokens every turn, and (c) compliance decays with session length (§2.7). JetBrains' paired-Harbor protocol is the first time anyone in this space did the obvious experiment, and both projects' numbers shrank 4-7× under it. Expect the same for every untested skill.

---

## 4. Taxonomy of mechanisms, and which have evidence

| Mechanism | Examples | Independent evidence of gain? | Typical failure |
|---|---|---|---|
| **A. Prompt-only (style/behaviour rules)** | caveman, ponytail, karpathy-skills, SuperClaude personas, most of superpowers/mattpocock/gstack | Weak: ~10% cost on non-trivial tasks (ponytail, p=0.004); 0 quality gain detected; CLAUDE.md presence 0→68% compliance on a rule but decays −5.6%/function | Compliance decay; rules cost tokens every turn; contradicts existing docs; no enforcement |
| **B. Prompt-only (procedural/domain knowledge)** | anthropics/skills doc skills, scientific-agent-skills, K-Dense | **Strong**: SkillsBench +16.6pp average (33.9→50.5) over 18 model-harness configs; but **+4.5pp for software engineering** vs +51.9pp healthcare; ≤3-module skills beat bundles; self-generated skills ≈0 or negative | Bloat (>60% non-actionable body text); poor routing descriptions; the model already knows mainstream SWE |
| **C. Structured process (phases, specs, personas)** | BMAD, spec-kit, GSD, superpowers workflow, agent-os | Anecdotal positives on large greenfield; measured costs: +20-500% time, $800-2k/mo; superpowers removed its own review loop for zero gain | Governs by convention; "illusion of work"; ceremony on small tasks; reviewer forced to invent issues |
| **D. Deterministic tools (compression, retrieval, index)** | rtk, context-mode, Squeez, serena, codegraph, repomix, context7 | **Moderate-strong, conditional**: rtk lossless output filtering; serena wins on 36k-line refactor, loses 4× on lookups; codegraph −88% tool calls/−44% cost self-measured with a blocked-CLI control; Squeez 92% pruning at 0.86 recall | Small-codebase overhead; index staleness; static-analysis blind spots (DI/reflection); resident-context bloat (codegraph +80%) |
| **E. External memory (cross-session)** | claude-mem, mem0, cognee, graphiti, letta, Memory Vault | **None for coding outcomes.** Storage benchmarks (LOCOMO 92.5) don't transfer (MemoryArena); claude-mem's own retreat from auto-inject; 8,785 memories never surfaced | Injection is the unsolved half; pollution; self-generated notes propagate errors; N× cost under agent teams |
| **F. Verification gate / hook** | PreToolUse/PostToolUse/Stop hooks, agent-guard, gitleaks, unlazy gates, Ralph exit criteria, paperclip budgets | **Strong by construction**, doesn't depend on model compliance; GitGuardian 2× leak rate is the motivation; agent-guard ships a coverage benchmark | Fixed blocklists; shell escapes bypass; hooks themselves are an unsigned auto-exec surface (ECC: 28 global hooks) |
| **G. Sandbox / capability reduction** | sandbox-runtime, Docker template, E2B, tool allow-lists | **Strong**: Vercel −80% tools → 80→100% success; CaMeL 77% vs 84% with provable guarantees; only defence that survives prompt injection | Skipping permissions inside the box; network egress leaks; DX friction |
| **H. Harness / scaffold engineering** | opencode, pi, deepseek-harness, LangChain deepagents, Anthropic's own loop | **Strongest numbers anywhere**: 52.8→66.5 Terminal-Bench harness-only; 42% vs 78% same model different scaffold (CORE-Bench) | Mostly proprietary or per-vendor; community "harnesses" (ruflo, ECC) are usually A+C+E stacked on top of the real harness |

**Reading of the table**: evidence of real gain concentrates in B (domain knowledge the model lacks), D (deterministic context reduction on large inputs), F/G (things that don't ask the model's permission), and H. Category A, the one with 600k+ combined stars, has the weakest evidence. Category E has the largest gap between stars and evidence. Category C is the one where the tools' own maintainers keep deleting features because they cost time for no gain.

---

## 5. Recurring gaps nobody solves well

1. **Nobody measures.** Of ~60 tools surveyed, exactly two (caveman, ponytail) have an independent paired A/B, and both came from a single JetBrains team spending ~$100 on Harbor. codegraph is the only vendor that blocked its own CLI in the control arm. Anthropic's skill-creator Benchmark mode is new and per-skill. There is no shared, cheap, model-agnostic harness for "did this change to my agent config help on *my* repo." Consequence: the market selects on README numbers, and README numbers are 4-7× inflated.

2. **Compliance decay is unaddressed by design.** Every prompt tool assumes the model keeps reading the rules. The data says odds of compliance drop 5.6% per function and hit 20-60% by message 6-10. Tools that re-inject rules (claude-mem, ECC hooks, ponytail) either re-inject *everything* (pollution) or nothing (decay). No tool does *targeted* re-injection keyed to what the agent is about to do, the PreToolUse hook exists, but nobody uses it to say "you're about to Edit `auth/`; here are the two rules that apply."

3. **Verification of the agent's own claims.** SlopCodeBench: no model finishes any problem end-to-end; agents pass 14.8% of checkpoints while reporting completion; agent code is 2.2× more verbose and degrades faster than human code. spec-kit produces specs nothing checks against; BMAD's reviewer must invent 3 issues; superpowers' review subagent added 25 min and no signal. Unlazy's runnable gates and Ralph's objective exit criteria are the right shape but are hand-authored per task. Nobody has generalised "derive a machine-checkable acceptance ledger from the request, then block Stop until it passes."

4. **Memory injection.** Storage is a solved commodity (SQLite+FTS5, vectors, graphs). The unsolved part is *when* and *what* to inject, and the evidence (context-pollution research, claude-mem's retreat, self-generated-note error propagation) suggests the answer is "much less than everyone ships, and never the agent's own prose."

5. **Small-vs-large regime detection.** serena, graphify, codegraph, BMAD, spec-kit all flip from net-negative to net-positive somewhere around "codebase too big to grep" / "task longer than a day". None of them detects which regime you're in; every README pretends there is one regime.

6. **Supply chain.** `npx skills add` and plugin marketplaces install unsigned prompt text and hook scripts that auto-execute globally. ECC: 513 auto-loadable files, 49 agents with Bash, unsigned `git pull` auto-update, confirmed malware clone in the wild. ruflo: CVSS 10.0. SkillSafetyBench exists as a paper; nothing in the install path uses it. skill-doctor lints, it doesn't attest.

7. **Prompt injection through the work itself.** PR titles, issue bodies, tool descriptions, docs fetched by context7, all are instruction channels. Detection-based defences (llm-guard, CASCADE) trade task success for partial coverage; only capability reduction works, and every "make the agent better" tool in this survey *adds* capability (more tools, more hooks, more MCP servers, more auto-fetched context).

8. **Cost attribution.** Users can't see which installed enhancer is eating tokens. Multi-agent, memory injection and skill bootstraps stack invisibly (tokenwar bundles six of them). paperclip's per-agent hard budgets and davila7's analytics are the only attempts at cost *governance*; nobody attributes cost to *config components*.

---

## 6. What a new project must do differently to not be "yet another skills repo"

Derived from where the evidence is, not from what's popular:

1. **Ship a measurement harness first, features second.** A paired, per-repo, per-task A/B runner (JetBrains-style: same task, with/without the change, deterministic verifier, per-task medians, Wilcoxon) that any user can run for <$20 against *their own* codebase and model. Every feature the project ships must be accompanied by its own ablation on that harness, with the control arm prevented from accessing the feature (codegraph-style). Publish the negative results. This alone would differentiate from 100% of the tools above.

2. **Prefer mechanisms from rows D/F/G over row A.** Enforcement through hooks, sandboxing and deterministic output shaping, things that hold when the model stops reading. Any behavioural rule that matters should have a hook that checks it, not a paragraph that requests it. Treat prompt text as a *last resort with a measured cost* (every rule costs tokens on every turn).

3. **Make acceptance machine-checkable and gate completion on it.** Turn the request into an executable ledger (commands + expected outputs, tests, lint, secret scan, diff-scope limits) *before* work starts, and block Stop until it passes, Unlazy's gate + Ralph's exit criterion + agent-guard's tool-boundary checks, generalised and automatic. This attacks gap #3, the one every process framework papers over with markdown.

4. **Targeted, tool-scoped re-injection instead of session-start dumps.** Use PreToolUse to inject only the 2-3 rules relevant to the file/tool about to be touched, and nothing else; never re-inject the agent's own previous prose (error propagation). Measure compliance over session length as a first-class metric, since that is the known failure curve.

5. **Regime-aware defaults.** Detect codebase size, task size and session length and switch deterministic helpers (LSP/graph retrieval, spec phases, output compression) on or off accordingly. Say "off" by default for the small regime; that is where most of the anecdotal "it made things worse" comes from.

6. **Subtract capability by default.** Minimal tool set, allow-listed commands, scoped tokens, secret/PII masking at the tool boundary, sandbox-runtime compatible. Every added MCP server or hook is an injection surface, the project's value should be *fewer* things in context and reach, not more.

7. **Attestable distribution.** Signed releases, pinned hook scripts with hashes, no `git pull` auto-update, a skill/hook manifest that declares exactly what auto-executes and at which lifecycle event, and a lint that refuses skills lacking a routing description or exceeding a token budget (SkillReducer's findings turned into a gate).

8. **Per-component cost attribution.** Show the user, per session, how many tokens each installed component consumed and what it changed. If a component can't show a measured effect after N sessions, the tool should suggest removing it. The honest version of "token optimizer" is a profiler, not a compressor.

9. **Stay model- and harness-agnostic at the mechanism level** (hooks/spec-compatible across Claude Code, Codex, Gemini CLI, OpenCode, DSH) but *validate per model*, because ponytail's near-random results on small models and Claude Code's own null result in the ETH study show effects are model-specific.

10. **Say small numbers.** The credible, measured effects in this ecosystem are 5-15% cost and 0% detectable quality change for prompt tools, larger for deterministic retrieval on big repos, and "prevents an incident" for guardrails. A project that advertises 10% and delivers 10% will be the only one that does.

---

## 7. Source list (selected)

- JetBrains AI: caveman test, https://blog.jetbrains.com/ai/2026/07/speak-to-ai-agents-like-cavemen-tosave-tokens/ ; ponytail test, https://blog.jetbrains.com/ai/2026/07/ponytail-skill-claude-tested/
- dev.to ponytail critique, https://dev.to/yashddesai/ponytail-the-ai-coding-skill-taking-github-by-storm-and-the-one-question-nobodys-answered-yet-46mc
- SkillsBench, https://arxiv.org/abs/2602.12670 ; SkillReducer, https://arxiv.org/abs/2603.29919 ; SlopCodeBench, https://arxiv.org/abs/2603.24755 ; McMillan instruction adherence, https://arxiv.org/abs/2605.10039 ; ETH AGENTS.md, arXiv 2602.11988 ; Squeez, https://arxiv.org/abs/2604.04979 ; Do LLMs benefit from their own words, https://arxiv.org/abs/2602.24287 ; Control under compression, https://arxiv.org/html/2608.01056
- Ruflo audit, https://gist.github.com/roman-rr/ed603b676af019b8740423d2bb8e4bf6 ; CVE-2026-59726, https://thehackernews.com/2026/07/ruflo-mcp-flaw-lets-unauthenticated.html ; awesome-claude-code #1338
- ECC audit, https://dev.to/joergmichno/we-audited-the-viral-213k-star-everything-claude-code-repo-and-found-a-malware-clone-in-the-wild-14hb
- Superpowers tradeoffs, https://www.joanmedia.dev/ai-blog/the-honest-tradeoffs-of-superpowers-token-costs-overkill-and-the-alternatives
- BMAD issues #1332, #1235; Reenbit BMAD vs Spec Kit vs OpenSpec; Scott Logic spec-kit review; spec-kit discussion #1784
- claude-mem issue #1464; alexandrekhoury "storage is solved, injection isn't"; joecotellese memory post
- ManoMano serena benchmark (Medium, manomano-tech); serena issue #802
- codegraph README benchmark; graphify README; Kevin Kinnett graphify review
- GitGuardian State of Secrets Sprawl 2026; agent-guard README; anthropics/claude-code issue #29434
- CLAUDE.md evidence roundups, alexdunlop.com, thomas-wiegold.com
- Harness engineering numbers, winder.ai harness comparison; firecrawl best-ai-coding-agents; asermax gist
- Anthropic skill-creator evals, https://claude.com/blog/improving-skill-creator-test-measure-and-refine-agent-skills
- GitHub API star/metadata queries, 2026-09-02.
