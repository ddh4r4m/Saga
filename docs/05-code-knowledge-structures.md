> **Provenance.** Research report produced 2026-09-02 by a Claude (Fable 5.1) research agent for the Saga project, ~110 web searches/fetches of papers and vendor posts. Vendor claims are labelled; independent measurements are cited with URLs. Treat every number as of that date.

# Code Knowledge Layers for Coding Agents: Indexes, Graphs, Memory, Caches — What the Evidence Says

**Date:** 2026-09-02 · **Method:** ~110 web searches/fetches of primary papers, vendor posts, and independent replications. Vendor-run numbers are flagged `[vendor]`; peer-reviewed or independently replicated numbers are flagged `[indep]`.

---

## 0. Executive summary (the 8 findings that matter)

1. **Structural indexes (def/ref graphs from tree-sitter) reliably improve localization, and modestly improve resolve rate.** RepoGraph +2–2.7 pts on SWE-bench Lite across four scaffolds (2024); LocAgent function-level Acc@10 77% vs 59% for Agentless (ACL 2025); the only leak-audited causal ablation of a *shipped* index (2026) shows localization acc@5 44%→85% and resolve 41.9%→50.4% (p=0.003) at *lower* cost per solve. Structure beats embeddings on "what should change" tasks; embeddings collapse there (CORE-Bench: 71.7→20.3 NDCG@10).
2. **More index ≠ better.** RepoGraph's 2-hop context underperformed 1-hop; CodeCompass (2026) documents a "navigation paradox"; an LSP token study shows semantic tools *cost* +6–118% tokens on localization for strong models and only save tokens for weak ones. Grep remains the backbone of every major agent for good reasons (zero setup, all file types, benign failure mode).
3. **LLM-extracted knowledge graphs are the wrong tool for code.** Deterministic AST-derived graphs are cheaper, hallucination-free, and at least as accurate; GraphRAG/LightRAG-style LLM indexing costs ~80M tokens per corpus and is built for prose "global sensemaking," not symbol resolution.
4. **Agent-memory vendor benchmarks do not survive scrutiny.** Mem0's "26%" is vs. OpenAI's memory feature, not vs. full context — full context (72.9%) beat Mem0 (66.9%) in Mem0's own table; Zep and Mem0 each claim the other misconfigured them; LoCoMo fits in 16–26k tokens. Independent 2026 work (MemDelta, "Does Memory Need Graphs?") finds controlled baselines match or beat memory products. For *coding*, evidence that persistent memory helps is thin: LLM-generated AGENTS.md files *hurt* (−3%, +20–23% cost); human-written help +4%; procedural/experience memory (AWM, ACE, SWE-Exp) shows real but scaffold-specific gains.
5. **Temperature 0 is not deterministic** because kernels aren't batch-invariant (Thinking Machines, Sep 2025: 80 unique outputs in 1000 runs of Qwen3-235B) and because MoE routing is batch-level. Fixes exist (SGLang deterministic mode, ~34% overhead; batch-invariant vLLM kernels, 1.6–2×) but only for self-hosted dense models. A harness must therefore get determinism from *structure outside the model*: content-addressed tool caches, replayable logs, verification gates.
6. **Prompt caching is now the dominant token-cost lever**: reads at 0.1× (Anthropic; 0.025× on newest Claude models), writes 1.25×/2×; OpenAI 0.1× with 24-h retention by default since 2026-05-29. This makes *stable prefix ordering* (tools → system → messages) an architectural requirement, and it penalizes any knowledge layer that mutates the prefix per turn.
7. **Cheap verification loops beat cheap "knowledge."** Type-constrained decoding +37% pass@1 on repair; mutation-guided test generation 53%→89.5% mutation score; TDAD test-impact map cut agent regressions 70% and lifted resolve 24%→32%; docs retrieval cuts API hallucination for rare APIs but *hurts* frequent ones unless gated (CloudAPIBench: −39 pts with a poor retriever). Agent-written tests, by contrast, barely move resolve rate (83% of outcomes unchanged) while eating 33–49% of tokens.
8. **Edit format matters more than compression.** Aider's udiff took GPT-4 Turbo 20%→61%; search/replace remains best for modern models; OpenAI trained GPT-4.1+ on a specific V4A patch format. General prompt compression (LLMLingua-2) has no positive evidence on code; task-aware pruning (SWE-Pruner) does: 23–54% fewer tokens with no loss.

---

## 1. Repository maps, symbol indexes, and structural vs. embedding retrieval

### 1.1 What exists

| System | Representation | Language coverage | Status (2026) |
|---|---|---|---|
| **aider repo map** ([2023-10-22](https://aider.chat/2023/10/22/repomap.html)) | tree-sitter tags → file/symbol graph → personalized PageRank, ~1k-token budget default | 130+ tree-sitter grammars | Shipped; no controlled on/off benchmark ever published |
| **Sourcegraph SCIP** ([overview](https://sourcegraph.com/resources/context-compare)) | Compiler-grade defs/refs, cross-repo; SCIP moved to open governance Mar 2026 | Per-language indexers (Go, TS, Java, Python, Rust, C++…) | Active; powers Cody/agents; requires build-aware indexers |
| **GitHub stack-graphs** | Incremental name-binding rules, no build needed | Python, TS, JS, Java, Ruby | **Archived 2025-09-09** ("no longer supported") — [repo](https://github.com/github/stack-graphs) |
| **Glean (Meta)** ([2024-12-19](https://engineering.fb.com/2024/12/19/developer-tools/glean-open-source-code-indexing/)) | Language-agnostic fact DB, incremental, Angle query language | Hack, C++, Python, Java, Rust… | Active at Meta; heavy to operate |
| **Kythe (Google)** | Semantic graph over monorepo | C++, Java, Go… | US team laid off Apr 2024; still used in Google's LLM migrations ([2025](https://arxiv.org/abs/2504.09691)) |
| **CodeQL databases** | Relational DB over AST+dataflow | ~10 languages | QLCoder (ICLR 2026) synthesizes queries via MCP + LSP + vector DB on CWE-Bench-Java |
| **Joern CPG** | Code property graph (AST+CFG+PDG) | C/C++, Java, JS, Python… | LLMxCPG (USENIX Sec 2025); codebadger MCP |
| **LSP-driven agents (Serena)** ([repo](https://github.com/oraios/serena)) | Live language-server symbols; find_symbol/replace_symbol_body | 30+ via language servers | Vendor claims "up to ~70%" token savings — unverified |

### 1.2 Evidence: structural graphs help localization

- **RepoGraph** ([arXiv 2410.14684](https://arxiv.org/abs/2410.14684), Oct 2024) `[indep]`: tree-sitter line-level def/ref graph (avg 1,419 nodes / 26,392 edges per repo). Plugged into four scaffolds on SWE-bench Lite: Agentless +2.34 abs (→29.67%), SWE-agent +2.00, AutoCodeRover +2.33, RAG +2.66. Token overhead modest (Agentless 42,376→47,323). **Ablation:** 1-hop flattened context 29.67% vs 2-hop 26.00% — more graph context *hurt*. CrossCodeEval code-match EM 10.8→28.5%.
- **LocAgent** ([ACL 2025](https://aclanthology.org/2025.acl-long.426/), [arXiv 2503.09089](https://arxiv.org/abs/2503.09089)) `[indep]`: heterogeneous graph (dir/file/class/function; contain/import/invoke/inherit) + sparse hierarchical index (entity IDs, name dictionary, BM25 inverted index). Three agent tools: `SearchEntity`, `TraverseGraph`, `RetrieveEntity`. SWE-bench Lite function-level Acc@10: LocAgent-Claude-3.5 77.4%, LocAgent-Qwen-32B-ft 77.0%, Agentless 58.8%, CodeRankEmbed 58.8%. Cost: $0.66/instance (Claude 3.5) vs $0.09 (fine-tuned Qwen-32B, ~86% cheaper). Later: SWE-Debate 81.67% file-level ([arXiv 2507.23348](https://arxiv.org/pdf/2507.23348)).
- **Agentless** ([arXiv 2407.01489](https://arxiv.org/abs/2407.01489)) `[indep]` baseline without graph: file 69.7% / function 52.0% / line 35.3% localization; 32% resolve at $0.70.
- **"Code Isn't Memory"** ([arXiv 2606.22417](https://arxiv.org/html/2606.22417v1), Jun 2026) `[indep, leak-audited]`: the most rigorous ablation to date of a shipped structural index (vector + def/call graph + BM25; tools `codebase_search`, `codebase_graph`; Merkle-tree incremental updates). Claude Opus 4.7, SWE-PolyBench Verified + SWE-bench Pro: localization acc@5 **84.5% vs 44.3%** (p<0.0001); resolve **50.4% vs 41.9%** (p=0.003); vs agentic-grep comparator 45.3% (p=0.087); cost/solve $2.30 vs $2.84 vs $2.92; mean tokens 10.1k vs 11.1k vs 14.0k.
- **Codebase-Memory** ([arXiv 2603.27277](https://arxiv.org/html/2603.27277), Mar 2026) `[indep, small]`: tree-sitter KG served via MCP (Cypher). Vs file-exploration agent across 31 languages: quality 0.83 vs 0.92, but 2.1× fewer tool calls, ~10× fewer tokens (~1k vs ~10k/question). Index Django in ~6 s, Linux kernel (2.1M nodes) ~3 min; incremental re-index ~1.2 s via XXH3 content hashes. Weak on macro-heavy C (0.58 vs 1.00).
- **GraphCoder** ([ASE 2024](https://arxiv.org/abs/2406.07003)), **CodexGraph** ([arXiv 2408.03910](https://arxiv.org/abs/2408.03910)): statement-level control/data-dependence graphs; +6 EM on completion; graph-DB queries beat similarity retrieval on CrossCodeEval/SWE-bench/EvoCodeBench.

### 1.3 Evidence: embeddings alone are weak for "what should change"

- **CodeRAG-Bench** ([NAACL Findings 2025](https://aclanthology.org/2025.findings-naacl.176/)) `[indep]`: RAG helps weak models on basic tasks (+15.6–17.8 pts MBPP for StarCoder2-7B) but retrieval quality is "limited" on DS-1000/ODEX/SWE-bench.
- **CORE-Bench** ([arXiv 2606.11864](https://arxiv.org/html/2606.11864v2), Jun 2026) `[indep]`: 632 repos, 180k queries. Best embedder (Qwen3-8B) drops from 71.7 NDCG@10 on snippet retrieval to **20.3** on issue-to-edit localization; SFT on PR data recovers to 32.8. Graph/LSP/agentic-grep were *not* evaluated.
- **SWE-Explore** ([arXiv 2606.07297](https://arxiv.org/html/2606.07297v1)) `[indep]`: BM25/TF-IDF near-random; dense retrieval marginally better; agentic exploration substantially better; agents' file-level hit ~0.65 but line-level recall ~0.15 — **line-level recall is the bottleneck.**
- **cAST** ([EMNLP Findings 2025](https://arxiv.org/abs/2506.15655)) `[indep]`: AST-aligned chunking alone gives +4.3 Recall@5 on RepoEval and +2.67 pass@1 on SWE-bench generation. Cheap, language-agnostic win.
- **Cursor semantic search** ([2025-11-06](https://cursor.com/blog/semsearch)) `[vendor]`: custom embedder trained from agent traces; +12.5% QA accuracy (6.5–23.5% by model), +2.6% code retention on 1,000+-file repos. Notably: "the combination of grep and semantic search leads to the best outcomes."

### 1.4 Evidence: LSP and "richer" tools can be a token tax

- **"Does a Language Server Save Tokens?"** ([arXiv 2608.13568](https://arxiv.org/html/2608.13568), Aug 2026) `[indep, preliminary]`: tokens-to-success metric. Localization: grep 920 tokens, LSP 971 (+6%) at equal 100% success; Sonnet +118%; Haiku −26%. Find-references: LSP precision 1.00 vs grep 0.76 (24% of grep hits are false positives), F1 0.778 vs 0.706, +19% tokens. Agents pick grep for localization (0–6% semantic use) and LSP ~45–57% for references unprompted. Small N; pylsp→pyright swap changed results.
- **CodeCompass** ([arXiv 2602.20048](https://arxiv.org/pdf/2602.20048)): "navigation paradox" — richer indexes can raise steps/tokens.
- **Why grep persists** ([yage.ai, 2026-03-27](https://yage.ai/share/why-coding-agents-still-use-grep-en-20260327.html)): zero startup, all file types (YAML/Dockerfile/Markdown), false positives (filterable) vs LSP false negatives/crashes, shell composability. Qualitative, but consistent across Claude Code, Codex CLI, Cursor, Aider.
- **Long-context alternative** ([arXiv 2505.08120](https://arxiv.org/abs/2505.08120)): dumping the whole repo into Gemini-1.5-Pro hits 38% SWE-bench Verified with no scaffold, but at $2.6/task vs $0.25 Agentless — and **context rot** ([Chroma, 2025](https://www.trychroma.com/research/context-rot)) shows all 18 tested frontier models degrade with every added increment of input.

**Bottom line for §1:** deterministic def/ref graphs from tree-sitter (no build required) + BM25 + AST-chunk embeddings, exposed through *few* agent tools, with grep left in place. Depth-1 neighborhoods; never dump 2-hop.

---

## 2. Knowledge graphs and GraphRAG

- **Microsoft GraphRAG** ([arXiv 2404.16130](https://arxiv.org/abs/2404.16130)): LLM-extracted entity graph + community summaries; wins on "global sensemaking" over prose corpora; indexing is "astronomical" in tokens.
- **LightRAG** ([EMNLP 2025, arXiv 2410.05779](https://arxiv.org/abs/2410.05779)): cheaper than GraphRAG (1 LLM call/chunk vs 5) but still ~15.8k tokens/query; on GraphRAG-Bench, index cost ~84M tokens (LightRAG) vs ~80M (GraphRAG) for one corpus ([arXiv 2506.02404](https://arxiv.org/pdf/2506.02404)).
- **RAG vs GraphRAG systematic eval** ([arXiv 2502.11371](https://arxiv.org/abs/2502.11371)) `[indep]`: neither dominates; hybrids win; graph pays off for multi-hop/relational queries only.
- **For code specifically** — [arXiv 2601.08773](https://arxiv.org/pdf/2601.08773) (OpenMRS, ThingsBoard): AST-derived graphs are deterministic, cheaper, and more reliable than LLM-extracted KGs, which introduce relation errors. This is the decisive point: *code already has a schema; don't pay an LLM to guess one.*
- **Temporal KGs for memory** — Zep/Graphiti ([arXiv 2501.13956](https://arxiv.org/abs/2501.13956)) claims up to +18.5% on LongMemEval with 90% lower latency `[vendor]`; Cognee's HotpotQA head-to-head (0.85 vs Mem0 0.54) ran Cognee tuned vs competitors on defaults `[vendor]` ([cognee blog](https://www.cognee.ai/blog/deep-dives/knowledge-graph-memory-benchmarks)). **"Does Memory Need Graphs?"** ([arXiv 2601.01280](https://arxiv.org/pdf/2601.01280)) `[indep]`: well-designed non-graph memory matches graph memory; retrieval quality matters more than structure.
- **Code property graphs (Joern)**: valuable for security (LLMxCPG, USENIX Sec 2025) and dataflow slicing; heavy build cost; C/C++-centric; not a general "knowledge layer."

**Cost/latency:** LLM-built graphs cost 10⁷–10⁸ tokens per corpus and must be rebuilt on drift; AST graphs cost seconds (Django ~6 s) and update incrementally in ~1 s.

---

## 3. Agent memory systems

### 3.1 The vendor benchmark wars (verify before citing)
- **Mem0** ([arXiv 2504.19413](https://arxiv.org/html/2504.19413)) `[vendor]`: LoCoMo J-score Mem0 66.88, Mem0-graph 68.44, **full-context 72.90**, Zep 65.99, LangMem 58.10, OpenAI memory 52.90, A-Mem 48.38. The "26%" is relative to *OpenAI's memory feature*; "90% fewer tokens" is 7k vs 26k per conversation; "91% lower p95 latency" is 1.44 s vs 17.1 s. Full context beat Mem0 on accuracy.
- **Zep's rebuttal** ([blog](https://blog.getzep.com/lies-damn-lies-statistics-is-mem0-really-sota-in-agent-memory/)): Mem0 misconfigured Zep (single-user model, non-standard timestamps, sequential searches); corrected Zep 75.14%. Mem0's counter ([issue #5](https://github.com/getzep/zep-papers/issues/5)): Zep's original 84% included excluded adversarial categories → 58.44%. Both agree LoCoMo is tiny (16–26k tokens), has label errors, and lacks knowledge-update tests.
- **LongMemEval** ([ICLR 2025](https://github.com/xiaowu0162/longmemeval)): Mem0 now claims 94.4 at ~6.9k tokens/query `[vendor]`; Supermemory 86% at ~190 tokens `[vendor]`. **MemDelta** ([arXiv 2606.29914](https://arxiv.org/pdf/2606.29914)) `[indep]` finds controlled baselines (full context, BM25, naive RAG) frequently beat purpose-built memory systems once confounds are removed.

### 3.2 Harness-level memory (what actually ships)
- **Anthropic memory tool + context editing** ([2025-09-29](https://claude.com/blog/context-management)) `[vendor]`: file-based memory directory; on an internal agentic-search eval +39% (combined) / +29% (context editing alone); 84% token reduction on a 100-turn web-search eval. Not a coding benchmark.
- **Claude Code auto-memory** (v2.1.59, Feb 2026, [docs](https://code.claude.com/docs/en/memory)): `~/.claude/projects/<project>/memory/MEMORY.md` index (first 200 lines/25 KB loaded) + topic files on demand; per-repo, not synced. No published eval.
- **Cline Memory Bank** ([docs](https://docs.cline.bot/features/memory-bank)): six mandated markdown files read at every session start. No eval.
- **Letta Code** ([blog](https://www.letta.com/blog/letta-code/)): git-backed memory filesystem; #1 model-agnostic harness on Terminal-Bench `[vendor]` — but that measures the harness, not memory's contribution.

### 3.3 Does persistent memory help *coding*?
- **AGENTS.md study** ([arXiv 2602.11988](https://arxiv.org/html/2602.11988v1), Feb 2026) `[indep]`: LLM-generated context files **reduce** resolve rate ~3% and raise cost 20–23% (+2.5–3.9 steps); human-written files +4%. Recommendation: keep context files minimal (tooling only). This is the closest thing to a controlled test of "memory bank" patterns, and it is negative for auto-generated notes.
- **Procedural/experience memory** `[indep]`: AWM ([OpenReview](https://openreview.net/forum?id=NTAhi2JEEE)) +24.6% Mind2Web / +51.1% WebArena relative; ACE ([ICLR 2026, arXiv 2510.04618](https://arxiv.org/abs/2510.04618)) +10.6% agents via evolving playbooks; SWE-Exp ([arXiv 2507.23361](https://arxiv.org/abs/2507.23361)) experience bank +7.2% relative on SWE-bench Verified (DeepSeek-V3), 73.0% with Claude 4 Sonnet — but inside an MCTS scaffold, so attribution is muddy. SWE-MeM ([arXiv 2606.28434](https://arxiv.org/pdf/2606.28434)) learns what to retain within long trajectories. SWE-Bench-CL ([arXiv 2507.00014](https://arxiv.org/abs/2507.00014)) exists to measure cross-task transfer; no strong positive result yet.
- Reflexion/ExpeL-style reflective memory helps within a task; cross-project transfer remains unproven.

**Reading:** for coding, the reliable memory is (a) *derived* deterministically from the repo (index), (b) *small* human-curated rules, (c) *procedural* traces of what worked in *this* repo. Free-form LLM-written summaries are net negative on current evidence.

---

## 4. Deterministic vs. probabilistic

### 4.1 Why temperature 0 isn't deterministic
- **Thinking Machines, "Defeating Nondeterminism in LLM Inference"** ([2025-09-10](https://thinkingmachines.ai/blog/defeating-nondeterminism-in-llm-inference/)): floating-point non-associativity is necessary but not sufficient; the "concurrency + atomics" folk theory is wrong (most forward passes have no atomic adds). Root cause: **kernels are not batch-invariant** — reduction order in RMSNorm/matmul/attention changes with batch size, and batch size depends on other users' load. Qwen3-235B, 1,000 identical temp-0 runs → **80 unique outputs**, first divergence at token 103. Batch-invariant kernels → identical outputs at ~2× (unoptimized) to 1.6× cost.
- **MoE routing**: expert capacity buffers are filled per batch, so a sequence's routing depends on its batch-mates ([152334H, 2023](https://152334h.github.io/blog/non-determinism-in-gpt-4/)). SGLang's deterministic mode ([2025-09-22](https://www.lmsys.org/blog/2025-09-22-sglang-deterministic/)) still excludes MoE and TP>2; overhead ~34%.
- **Implication:** on any hosted API you cannot get bitwise-reproducible outputs. Determinism must come from the harness.

### 4.2 Where determinism *can* live
- **Content-addressed tool caches / replay logs**: key tool results on `hash(tool, args, relevant file hashes)`; key model calls on `hash(system, messages, tools, model)`; replay permissive (serve hits, re-call misses) or strict (fail on divergence) — pattern formalized in "The Log is the Agent" ([arXiv 2605.21997](https://arxiv.org/pdf/2605.21997)). Pest 5's TIA replays cached test results with the same idea (19k tests: 3 min → 5 s).
- **Prompt caching economics** ([Anthropic docs](https://platform.claude.com/docs/en/build-with-claude/prompt-caching), 2026): writes 1.25× (5-min) / 2× (1-h); reads 0.1× (0.025× on Claude Fable 5.1 / Mythos 5.1); minimum 512–4,096 tokens by model; cache hierarchy `tools → system → messages` — changing tool definitions invalidates everything. OpenAI: 0.1× reads on GPT-5 series, **24-h retention default since 2026-05-29** ([effloow measurement](https://effloow.com/articles/openai-prompt-cache-retention-24h-cost-proof-2026)). Consequence: a knowledge layer that rewrites the system prompt or tool list per turn forfeits a 90–97.5% discount; it should inject knowledge as *late messages* or tool results and keep the index summary stable.
- **Semantic caching** (GPTCache etc.): hit rates 60–80% on FAQ-style workloads, but false hits are real (GPTCache 54 vs MeanCache 3 on contextual queries, [arXiv 2403.02694](https://arxiv.org/pdf/2403.02694)); not appropriate for code edits where "similar" ≠ "same."

---

## 5. Token-efficiency techniques

| Technique | Evidence | Verdict |
|---|---|---|
| **General prompt compression (LLMLingua-2)** ([arXiv 2403.12968](https://arxiv.org/abs/2403.12968)) | Task-agnostic; degrades on reasoning/exact-structure tasks; no positive code result found; "Prompt Compression in the Wild" ([arXiv 2604.02985](https://arxiv.org/pdf/2604.02985)) shows rate-adherence and quality variance | Avoid for code |
| **Task-aware code pruning (SWE-Pruner)** ([arXiv 2601.16746](https://arxiv.org/abs/2601.16746)) | 0.6B skimmer; 23–54% fewer tokens on SWE-bench Verified with equal/better success; 14.8× on LongCodeQA | Promising `[indep]` |
| **Edit formats** ([aider udiff](https://aider.chat/docs/unified-diffs.html), Dec 2023; [notes](https://aider.chat/docs/leaderboards/notes.html)) | GPT-4 Turbo 20%→61% with udiff, 3× less "lazy"; flexible hunk matching cut edit errors 9×; modern models do best with search/replace; whole-file is token-hungry and size-capped; OpenAI trains GPT-4.1+ on V4A `apply_patch` ([guide](https://developers.openai.com/cookbook/examples/gpt4-1_prompting_guide)); architect/editor split hit 85% on aider's bench | Use search/replace (or model-native patch format) + fuzzy apply |
| **Tool-result truncation** | Codex: 256 lines / 10 KiB head+tail; Pydantic harness: head/tail/head_tail default; error-aware tail retention ([OpenClaw](https://github.com/openclaw/openclaw/blob/main/src/agents/pi-embedded-runner/tool-result-truncation.ts)) | Standard; keep tails for build/test output, heads for schemas; never cut mid-line |
| **Context editing** ([Anthropic 2025-09-29](https://claude.com/blog/context-management)) | Auto-clear stale tool results: −84% tokens on 100-turn eval, +29% task perf | Strong `[vendor]` |
| **Just-in-time context / progressive disclosure** ([Anthropic 2025-09-29](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents); [advanced tool use 2025-11-24](https://www.anthropic.com/engineering/advanced-tool-use)) | Tool Search Tool: 77k→8.7k tokens (−85%), MCP-eval accuracy Opus 4 49→74%; programmatic tool calling −37% tokens | Strong `[vendor]`, mechanism plausible |
| **Sub-agent isolation** ([Anthropic multi-agent](https://claude.com/blog/building-multi-agent-systems-when-and-how-to-use-them)) | +90.2% on research eval but ~15× tokens; worthwhile when subtask output >1k tokens and mostly irrelevant to parent | Use for retrieval-heavy exploration, return ≤2k-token summaries |
| **Hybrid text+visual repo graphs** ([arXiv 2606.14061](https://arxiv.org/abs/2606.14061)) | −26% input tokens at equal accuracy for multimodal agents | Niche |

---

## 6. External API / library knowledge

- **Scale of the problem** (USENIX Sec 2025, "We Have a Package for You!"): 2.23M samples, **19.7%** contained ≥1 hallucinated package (commercial models 5.2%, open 21.7%; GPT-4 Turbo 3.59%); 43% of hallucinated names recur ≥10× → slopsquatting ([Socket](https://socket.dev/blog/slopsquatting-targets-across-frontier-llms)). Frontier-cohort re-evaluation 2026 ([arXiv 2605.17062](https://arxiv.org/pdf/2605.17062)) says range shrank, threat remains.
- **Docs retrieval helps — conditionally** `[indep]`: DocPrompting ([ICLR 2023](https://arxiv.org/abs/2207.05987)) established gains for unseen libraries. Amazon's CloudAPIBench ([arXiv 2407.09726](https://arxiv.org/abs/2407.09726)): GPT-4o valid low-frequency API calls 38.6%→47.9% with doc augmentation, **but −39 pts on high-frequency APIs with a sub-optimal retriever**; gating retrieval on an API index / model confidence gives +8.2 abs overall. De-Hallucinator ([arXiv 2401.01701](https://arxiv.org/pdf/2401.01701)) iteratively grounds against project API references. Hierarchical-dependency-aware mitigation ([FSE 2025](https://dl.acm.org/doi/pdf/10.1145/3696630.3728569)) finds *project context* outperforms generic doc RAG.
- **Context7 / DevDocs MCPs** ([Context7](https://github.com/upstash/context7)): plausible mechanism (version-pinned docs, 5k-token default), wide adoption — **no independent accuracy evaluation found**. Treat as a gated retrieval source, not a guarantee.
- **llms.txt**: 10.1% adoption across 300k domains, 51.8% among dev-focused sites (Aug 2026); Google states it does not affect AI search; crawlers barely fetch it. Useful only as a curated pointer list for an agent you control.
- **What actually reduces API hallucination, in order of evidence:** (1) compiler/type-checker in the loop — type-constrained decoding +3.5%/+5%/+37% pass@1 on synthesis/translation/repair ([arXiv 2504.09246](https://arxiv.org/pdf/2504.09246)); on-the-fly compiler feedback beats post-hoc ([arXiv 2607.13921](https://arxiv.org/pdf/2607.13921)); (2) existence check against the lockfile/registry before any `import` is accepted; (3) *gated* docs retrieval keyed on resolved package@version; (4) project-local usage examples over generic docs.

---

## 7. Verification-oriented structures

- **Test-impact maps (TDAD)** ([arXiv 2603.17973](https://arxiv.org/abs/2603.17973v2), Mar 2026) `[indep]`: AST code→test graph with weighted impact; delivered as a static skill file. Regressions 6.08%→1.82% (−70%) on SWE-bench Verified with Qwen3-Coder-30B; resolve 24%→32% in a second harness. TDD *prompting* alone increased regressions (9.94%) — context beats procedure for small models.
- **Agent-written tests** ([arXiv 2602.07900](https://arxiv.org/html/2602.07900v1)) `[indep]`: suppressing tests changed outcomes in only 16.8% of tasks (Δ ≤2.6 pts) while cutting input tokens 33–49%; assertions shallow (exact-value 34–43%, relational 3–8%). Tests as *probes* are cheap; tests as *oracles* need mutation pressure.
- **Mutation testing**: Meta ACH ([arXiv 2501.12862](https://arxiv.org/abs/2501.12862)) 9,095 mutants → 571 tests, 73% accepted; ~half add no line coverage yet catch faults; mutation-feedback loop raises mutation score 53%→89.5% on HumanEval-Java ([ScienceDirect](https://www.sciencedirect.com/science/article/abs/pii/S0950584924000739)).
- **Property-based tests**: LLM PBT and EBT each 68.75% bug detection, combined 81.25% (AIware 2025). **Differential testing**: DiffSpec ([arXiv 2410.04249](https://arxiv.org/abs/2410.04249)) 1,901 differentiating tests, ≥4 confirmed eBPF bugs.
- **Formal verification**: DafnyBench 68% (Opus 3, 2024) → 89% (Opus 4.1) → 96% union; error-message retry loops are the main lever; VerusBench (150 Rust/Verus tasks); vericoding benchmark 12k problems across Dafny/Verus/Lean ([arXiv 2509.22908](https://arxiv.org/pdf/2509.22908)). Practical today for small kernels, not whole services.
- **Compiler-in-the-loop generally**: LLMloop ([arXiv 2603.23613](https://arxiv.org/pdf/2603.23613)) orders loops compile → test → static analysis; every study reviewed reports pass@1 gains from compiler feedback; the *type checker* is the cheapest oracle with the highest signal-to-token ratio.

---

## 8. Comparison table

| Approach | Accuracy gain (evidence) | Token cost | Latency | Language coverage | Maintenance | Evidence quality |
|---|---|---|---|---|---|---|
| Grep/ripgrep (baseline) | — | Low per call; many calls | ms | All files | None | Universal practice |
| Tree-sitter def/ref graph + BM25 (RepoGraph/LocAgent/"Code Isn't Memory") | +2–8 pts resolve; +20–40 pts localization | Neutral to −10% | Index secs–mins; query <1 ms | 130+ grammars; weak on macros/dynamic dispatch | Low (incremental by content hash) | **High** (3 indep. studies, 1 leak-audited) |
| AST-aligned chunk embeddings (cAST, Cursor) | +2.7 pass@1; +12.5% QA `[vendor]` | Embedding compute; retrieval cheap | Index minutes | Language-agnostic | Medium (re-embed on change) | Medium |
| Generic embeddings (no structure) | Collapses on edit localization (20 NDCG@10) | — | — | Any | Medium | High (negative) |
| LSP (Serena etc.) | Precision 1.00 refs; +6–118% tokens on localization | + for strong models, − for weak | Server startup 10s+ | Per-language servers, config-fragile | High | Medium (small N) |
| SCIP/Glean/Kythe precise indexes | Compiler-grade | Neutral | Build-integrated, minutes–hours | Per-language indexers | High | Industrial, no agent RCTs |
| CodeQL / Joern CPG | Strong for security/dataflow | High to build | Minutes–hours | ~10 langs | High | High in security domain only |
| LLM-extracted KG (GraphRAG/LightRAG) | Prose multi-hop wins; no code advantage | 10⁷–10⁸ tokens/index | Hours | N/A | Very high (rebuild on drift) | Medium; negative for code |
| Conversation memory products (Mem0/Zep) | Below full-context on LoCoMo in Mem0's own table | −70–90% vs full history | <1 s | N/A | Medium | Low (vendor disputes) |
| Human-written repo rules (CLAUDE.md/AGENTS.md) | +4% | +20% cost | — | — | Human | Medium |
| LLM-generated repo notes | **−3%** | +20–23% | — | — | Auto | Medium (negative) |
| Procedural/experience memory (AWM/ACE/SWE-Exp) | +7–50% relative (task-dependent) | Small | — | — | Auto | Medium |
| Context editing / tool-result clearing | +29%, −84% tokens | Large savings | — | — | None | Vendor |
| Search/replace or native patch edit format | 20→61% (old GPT-4T); best for modern models | Large savings vs whole-file | — | — | None | High |
| Prompt caching (stable prefix) | — | −90–97.5% on cached prefix | Lower TTFT | — | Design discipline | High (pricing fact) |
| Test-impact map (TDAD) | −70% regressions; +8 pts resolve | Tiny (static file) | Seconds | AST-based | Low | Medium (2 models) |
| Type-checker / compiler loop | +3.5–37% pass@1 | Small | Seconds | Typed langs | None | High |
| Mutation-guided tests | Mutation score 53→89.5% | Moderate | Minutes | Any with mutator | Medium | High |
| Docs retrieval (gated) | +8 abs (gated) / −39 (ungated, bad retriever) | 2–5k tokens/call | ms–s | Any | Doc freshness | High |

---

## 9. Recommended architecture: a language-agnostic "code knowledge layer"

Design principle: **deterministic structure in, few tools out, stable prefix always.**

### 9.1 Indexes (all derived, all content-addressed)
1. **Symbol graph** — tree-sitter tags per file → nodes {file, class, function, method, variable}, edges {contains, imports, calls/references (name-resolved heuristically), inherits}. Store per-file fact sets keyed by `blake3(file bytes)`; the graph is the union. This is RepoGraph/LocAgent/Codebase-Memory's shape and is the only component with strong causal evidence.
2. **Lexical index** — trigram/BM25 over identifiers and tokens (ripgrep stays available as the un-indexed fallback for non-code files).
3. **AST-chunk embeddings** — cAST-style chunks (function/class units, merged siblings under a size cap); optional, model-swappable; used only for natural-language → code queries.
4. **Test-impact map** — code→test edges from imports/call graph + last-run coverage where available (TDAD).
5. **Precision overlay (optional, lazy)** — an LSP client started only when a task needs *find-references* or rename; never for localization.
6. **External API cache** — `package@version → {symbols, signatures, doc snippets}` populated from the lockfile-resolved installed package (docstrings/type stubs first, Context7/DevDocs second), consulted only when a symbol is not found locally or the type checker complains.

### 9.2 Update strategy
- Watch the working tree; on change, re-parse only files whose content hash changed (Codebase-Memory: ~1.2 s incremental; Merkle diff in "Code Isn't Memory"). Rebuild edges only for touched files and their reverse-dependents.
- Version the whole index by `hash(sorted file hashes)`; tool results cached under `(tool, args, index_version)` so replays are exact and cache hits survive across sessions.
- Never write LLM-generated summaries into the index as facts. If summaries exist, mark them as `derived:llm` with the source hash and expire them when the source changes.

### 9.3 Agent-facing query API (keep it to ~4 tools)
```
search(query, mode=auto|lexical|semantic|symbol, k=10) -> [{path, line_range, symbol, score, snippet≤N lines}]
symbol(name|id, depth=1, edges=[callers|callees|imports|inherits|tests]) -> compact adjacency + signatures (no bodies)
read(path|symbol_id, lines?) -> exact source (the only tool that returns bodies)
impact(diff|paths) -> {affected_symbols, affected_tests, suggested_test_command}
```
Rules from the evidence: depth defaults to 1 (RepoGraph ablation); `symbol` returns signatures not bodies (10× token reduction in Codebase-Memory); `search` auto-mode routes symbol-looking queries to lexical/symbol and prose to semantic (agents already do this, per the LSP study); every result carries `path:line` so the model can escalate to `read`.

### 9.4 Prompt/cache discipline
- Put tool schemas + system rules + a ≤1k-token repo map (aider-style PageRank top symbols) in the cached prefix; regenerate the map only on index-version change and pin it with a 1-h cache.
- Inject retrieved knowledge as tool results (post-prefix), and use context editing to clear stale ones.
- Keep human-written repo rules minimal (tooling, commands, constraints); do not auto-generate AGENTS.md content.

### 9.5 Verification gates (where determinism actually comes from)
- After every edit: parse check (tree-sitter) → type-check/compile (if available) → `impact()` → run affected tests → optionally mutation-check any *new* test the agent wrote.
- Import guard: reject any new dependency not present in the lockfile/registry.
- Record everything in an event log keyed by content hashes so a run can be replayed or forked without re-calling the model.

### 9.6 Procedural memory (small, scoped, evidence-based)
- Store per-repo *procedures* (commands that worked, failure→fix pairs, test invocation quirks) as short structured entries with provenance; retrieve by task similarity (AWM/ACE-style). Cap at a few hundred tokens injected per task. Do not store narrative summaries of the codebase — the index is the memory.

---

## 10. Open problems

1. **Line-level recall.** Agents find files (~65% hit) but not lines (~15% recall) — SWE-Explore. No index yet fixes this; it may require learned rerankers trained on PR data (CORE-Bench SFT +61%).
2. **Name resolution without a build.** Tree-sitter graphs mis-resolve dynamic dispatch, macros, duck typing; precise indexers need builds. A hybrid (heuristic graph + lazy LSP confirmation) is the pragmatic answer, but nobody has measured its cost/benefit at scale.
3. **Benchmark contamination and vendor bias.** Memory benchmarks (LoCoMo) are small and disputed; index vendors run their own ablations. Only one leak-audited index ablation exists; it needs replication across harnesses and models.
4. **Memory that helps rather than hurts.** LLM-generated context files are net negative today. What granularity, provenance, and expiry make cross-session memory positive for coding remains open; SWE-Bench-CL is the right instrument but lacks a convincing positive result.
5. **Determinism on hosted APIs.** Batch-invariant inference exists only for self-hosted dense models. Harness-side determinism (caching, replay, verification) is the only option, and its interaction with prompt caching (prefix stability vs dynamic knowledge) is under-studied.
6. **Cost accounting of "richer" tools.** The LSP study, CodeCompass, and RepoGraph's 2-hop result all show diminishing or negative returns. We lack a principled model of when a tool's precision pays for its tokens across model strengths (LSP helped Haiku, hurt Sonnet).
7. **External API grounding.** Gated docs retrieval helps; ungated retrieval hurts. There is no independent evaluation of Context7-style MCPs, and no standard for version-pinned, machine-readable API surfaces that agents actually consume (llms.txt has not filled that role).
8. **Test oracles.** Agent-written tests are probes, not oracles. Mutation- and property-guided generation works but is expensive; the sweet spot for an always-on harness is unknown.

---

### Key sources (chronological)
- aider repo map (2023-10-22) https://aider.chat/2023/10/22/repomap.html · aider udiff https://aider.chat/docs/unified-diffs.html
- 152334H, MoE nondeterminism (2023) https://152334h.github.io/blog/non-determinism-in-gpt-4/
- DocPrompting (ICLR 2023) https://arxiv.org/abs/2207.05987
- GraphRAG (2024) https://arxiv.org/abs/2404.16130 · LightRAG https://arxiv.org/abs/2410.05779
- Agentless (2024) https://arxiv.org/abs/2407.01489 · CloudAPIBench https://arxiv.org/abs/2407.09726 · CodexGraph https://arxiv.org/abs/2408.03910 · RepoGraph https://arxiv.org/abs/2410.14684
- Glean at Meta (2024-12-19) https://engineering.fb.com/2024/12/19/developer-tools/glean-open-source-code-indexing/
- Zep (2025-01) https://arxiv.org/abs/2501.13956 · Meta ACH https://arxiv.org/abs/2501.12862 · A-MEM https://arxiv.org/abs/2502.12110 · RAG vs GraphRAG https://arxiv.org/abs/2502.11371
- LocAgent (ACL 2025) https://arxiv.org/abs/2503.09089 · Type-constrained decoding https://arxiv.org/pdf/2504.09246 · Mem0 https://arxiv.org/abs/2504.19413 · Zep rebuttal https://blog.getzep.com/lies-damn-lies-statistics-is-mem0-really-sota-in-agent-memory/
- LCLM agents https://arxiv.org/abs/2505.08120 · cAST https://arxiv.org/abs/2506.15655 · SWE-Exp https://arxiv.org/abs/2507.23361 · Chroma context rot https://www.trychroma.com/research/context-rot
- Thinking Machines (2025-09-10) https://thinkingmachines.ai/blog/defeating-nondeterminism-in-llm-inference/ · SGLang deterministic (2025-09-22) https://www.lmsys.org/blog/2025-09-22-sglang-deterministic/
- Anthropic context management (2025-09-29) https://claude.com/blog/context-management · context engineering https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents · advanced tool use (2025-11-24) https://www.anthropic.com/engineering/advanced-tool-use
- ACE (ICLR 2026) https://arxiv.org/abs/2510.04618 · Cursor semsearch (2025-11-06) https://cursor.com/blog/semsearch
- Does Memory Need Graphs? https://arxiv.org/pdf/2601.01280 · AST vs LLM KGs for code https://arxiv.org/pdf/2601.08773 · SWE-Pruner https://arxiv.org/abs/2601.16746
- AGENTS.md eval https://arxiv.org/html/2602.11988v1 · Agent-generated tests https://arxiv.org/html/2602.07900v1 · CodeCompass https://arxiv.org/pdf/2602.20048
- TDAD https://arxiv.org/abs/2603.17973v2 · Codebase-Memory https://arxiv.org/html/2603.27277
- Prompt compression in the wild https://arxiv.org/pdf/2604.02985 · TS indexing https://arxiv.org/abs/2604.18413
- CORE-Bench https://arxiv.org/html/2606.11864v2 · SWE-Explore https://arxiv.org/html/2606.07297v1 · Code Isn't Memory https://arxiv.org/html/2606.22417v1 · MemDelta https://arxiv.org/pdf/2606.29914 · SWE-MeM https://arxiv.org/pdf/2606.28434
- LSP token study (2026-08) https://arxiv.org/html/2608.13568 · Anthropic prompt caching docs https://platform.claude.com/docs/en/build-with-claude/prompt-caching · Claude Code memory docs https://code.claude.com/docs/en/memory · stack-graphs (archived 2025-09-09) https://github.com/github/stack-graphs
