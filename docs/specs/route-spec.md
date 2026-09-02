# `saga route`: technical specification

*Draft v0.1, 2026-09-03. Implements doc 09 §3.8 and milestone M5 (doc 09 §5), the last layer in the stacking order because it has the least evidence (doc 09 §7 item 4). Consumes trace-spec §3 (ledger), §3.6 (budgets) and §4 (pins); gate-spec §2 (contract scope, gate state); index-spec §1.3 (regime, `tier` tag for the LSP overlay); mem-spec §4.6 (sub-agent preamble); bench-spec §4 and §5 (ablation matrix, pass^k, cost per solved task). Every default in this file is a prior to be replaced by a bench result, and says so in its own row.*

---

## 1. Purpose, non-goals, evidence statement

### 1.1 Purpose

`saga route` decides, before a task or a sub-agent starts, which model tier, reasoning effort, context budget and cost cap apply, writes that decision to the trace, and refuses to decide when the trace says the provider is not serving what was asked for. It exists for three reasons that do not depend on which model is best this month (doc 09 §3.8): silent model and effort changes are the largest "got dumber" class (claude-code #42796, 3,286 reactions, traced to an effort default change; gemini-cli #2208 and #28859, Pro served as Flash; doc 06 A.1, A.3); price per token misleads (Gemini 3.5 Flash emitted 73M tokens across the AA index against a 36M average, doc 06 A.3); and read-only sub-agents on cheaper models are the one delegation pattern that practitioners report as working (Cognition, doc 03 §1.3).

### 1.2 Non-goals

| Not this | Because |
|---|---|
| A proxy that rewrites prompts or swaps models in flight | Saga never sits in the request path except the opt-in trace proxy, which forwards unchanged (trace-spec §9.3). Route acts at task start and sub-agent spawn, through the harness's own selection surface (§6). |
| Switching the primary model mid-task | Forfeits the cached prefix (doc 05 §4.2) and makes the run non-comparable (trace-spec §4.2). Allowed only when `.saga/route.toml` sets `[policy] mid_task_switch = "ask"` and the user answers (§5.4). |
| A leaderboard-driven chooser | Harness variance is 7.8× model variance (doc 03 §2.1); vendor evals carry a 3.5 to 4.5 point standard error per model (doc 06 A.10). Only mechanism-backed differences become rules (doc 09 §7 item 4). |
| An LLM classifier of task difficulty | Every signal in §2 is a count from the contract, the index or the trace. A model's own estimate of its task is the thing #42796 shows cannot be trusted. |
| A savings claim | See §1.3. |

### 1.3 Evidence statement

The ecosystem's routing claims have no baseline: oh-my-claudecode advertises "30 to 50% token savings via model routing" with no published control (doc 04 §2.2); multi-agent orchestration raises total tokens (about 15× a chat session in Anthropic's research system, doc 03 §1.3) and the survey's verdict is that coordination "usually raises total tokens" (doc 04 §2.2). The measured items route can lean on are few:

| Finding | Number | Source | What it licenses |
|---|---|---|---|
| Architect/editor split | o1-preview + DeepSeek 85.0% vs 79.7%; Sonnet + Sonnet 80.5% vs 77.4%; GPT-4o + GPT-4o 75.2% vs 71.4%; o3 + GPT-4.1 83% at lower cost | doc 03 §1.2 (Aider polyglot, 225 Exercism tasks) | An **option**, off by default; small self-contained exercises, edit-format compliance confounded |
| Tool value depends on model strength | LSP tokens-to-success: Haiku −26%, Sonnet +118% | doc 05 §1.4 (preliminary, small N) | Tier-gated tools (index-spec §1.3), and the interaction test in §8 |
| Higher effort is not monotone | "higher reasoning effort reducing accuracy in the majority of runs" | doc 03 §2.1 (HAL, 21,730 rollouts) | Effort is a bench arm, not a free upgrade |
| Effort default change caused the largest regression thread | #42796; fabrication turns had "exactly zero chain-of-thought" | doc 06 A.1 | Effort pinning (§4.1) |
| Sub-agent summaries are cheap to return | 1 to 2k tokens per sub-agent | doc 03 §1.3 | Read-only delegation ladder rung L1 (§3.3) |
| Separate-context reviewer | about 2 bugs per PR, 58% severe | doc 03 §1.3 (Cognition, self-reported) | Reviewer rung L2, never on a cheaper tier for security (§7) |
| Cache read discount | 0.1× reads; 0.025× on Fable 5.1; writes 1.25× (5m) or 2× (1h) | doc 05 §4.2, doc 06 A.1 | Cache-aware ordering (§4.2) |

Saga's own routing claim, if one is ever made, is the bench-spec §1.3 form: "route moved cost per solved task by Δ (CI) at equal pass^k on T with model pair P in H, K = 10". Nothing in this spec permits a README number before §8 has run.

---

## 2. Task classes

### 2.1 Taxonomy

Closed set of eight classes plus `unknown`. A class is a label on a **unit of work**: the active contract (gate-spec §2) for the primary agent, or one sub-agent invocation.

| Class | Definition | Writes? | Typical bench size class (bench-spec §2.3) |
|---|---|---|---|
| `explore` | Read-only exploration: answer a question about the repo, no edit expected | no | S |
| `localize` | Find the files and symbols a change must touch; output is a path list | no | S, M |
| `mechanical` | Edit whose shape is fully specified: rename, move, apply a known pattern, regenerate | yes | S, M |
| `design` | Refactor or feature where the diff shape is not known in advance | yes | M, L, XL |
| `debug` | Root-cause a failing check; the fix is unknown until the cause is found | yes | M, L |
| `test` | Author or extend tests against existing code | yes, tests only | S, M |
| `review` | Read a diff and return findings against a contract (gate-spec reviewer contract) | no | S, M |
| `docs` | Write or update documentation, comments, changelogs | yes, docs only | S |
| `unknown` | Signals conflict or are absent | assume yes | any |

Two flags orthogonal to class: `security` (the contract, path or request matches `[security]` patterns in §3.1) and `impossible_risk` (the contract header `RISK: impossible`, gate-spec §2.3, surfaced as `risk` in `saga.gate.status/1`, or the bench tag `impossible`; `RED: control` is a red-proof mode and no longer implies it, REVIEW-LOG risk 8). Both pin the tier upward (§7).

### 2.2 Signals available at run time

Every signal is deterministic and is recorded in the decision record (§7.3) so `saga route explain` can reproduce the classification bit for bit.

| Signal | Source | Value |
|---|---|---|
| `scope_files` | Count of working-tree files matched by the contract's `IN:` globs, minus `OUT:` (gate-spec §2.3) | integer; `null` with reason if no contract |
| `scope_tests_only` | Every `IN:` glob matches only the repo's test globs (gate-spec §5.2 per-language heuristics) | bool |
| `scope_docs_only` | Every `IN:` glob matches `*.md`, `docs/**`, `CHANGELOG*`, or comment-only paths | bool |
| `impact_size` | `saga index impact` result count over `scope_files` (index-spec §5); `null` when regime is `off` (index-spec §1.3) | integer or `null` |
| `gates_runnable` / `gates_manual` | Counts from `saga gate check --status --json` | integers |
| `gates_failing_at_start` | Runnable gates whose `CHECK` currently fails (a failing gate before any edit is the debug signature) | integer |
| `risk_impossible` | `risk = "impossible"` in `saga gate check --status --json` (the `RISK:` header, gate-spec §2.3); stays true while `risk_removed` is set, so deleting the line never down-tiers (§7.1) | bool |
| `tool_allow` | The tool set the sub-agent is spawned with (Claude Code `tools:` frontmatter or Agent tool input; Codex sandbox policy `ReadOnly`) | set |
| `tool_mix_recent` | Over the last 20 tool calls in the trace: fraction that are read-class (guard-spec §2.4 classification `read`) | 0..1 |
| `subagent_prompt_len` | Bytes of the sub-agent prompt (never its content) | integer |
| `explicit_class` | `saga route plan --class`, or a `[override]` row keyed by contract slug (§3.1) | class or `null` |
| `regime` | index-spec §1.3 `off`, `lite`, `full` | enum |

Route reads no assistant text and no prompt content, the mem-spec §4 rule.

### 2.3 Classification procedure

Ordered rules; the first match wins; the matched rule id is stored.

```
R0  explicit_class != null                                  -> explicit_class
R1  subagent  and  tool_allow ⊆ READ_TOOLS  and  the parent
      is at gate state "all gates met, Stop pending"         -> review
R2  subagent  and  tool_allow ⊆ READ_TOOLS                  -> localize if the parent's contract
                                                                has scope_files == 0, else explore
R3  no contract  and  not subagent                          -> unknown
R4  scope_files == 0  and  gates_runnable == 0              -> explore
R5  scope_files == 0  and  gates_runnable >= 1              -> localize
R6  scope_docs_only                                         -> docs
R7  scope_tests_only                                        -> test
R8  gates_failing_at_start >= 1  and  scope_files <= 4      -> debug
R9  scope_files <= 4  and  impact_size != null
      and  impact_size <= 2 * scope_files                    -> mechanical
R10 scope_files >= 5  or  impact_size == null
      or  impact_size > 2 * scope_files                      -> design
R11 otherwise                                               -> unknown
```

`READ_TOOLS` is the harness-specific set of tools guard classifies as `read` (guard-spec §2.4: Read, Grep, Glob, `saga index *`, `trace_*`, Bash restricted by an allow-list of read patterns; guard-spec §2.5). The thresholds `4`, `5` and `2 ×` are priors aligned to the bench size classes S/M (≤ 4 files) versus L (5 to 12) in bench-spec §2.3; §8.1 measures their precision.

### 2.4 `unknown` handling

`unknown` is a first-class result, never coerced. Policy for it is fixed and not overridable by a repo policy: primary model at the pinned effort, no delegation below the primary tier, session budget only. The decision record carries `class: "unknown", rule: "R3|R11"`, and `saga route explain` prints which signals were `null` and why. The bench reports the `unknown` rate per harness (§8.1); a rate above 20% on the M0 task set fails the M5 exit criterion because a router that cannot classify is a router that cannot save anything measurable.

---

## 3. Policy file

### 3.1 `.saga/route.toml`

Precedence, highest first: hard rules in §7 (never removable) > user global `~/.saga/route.toml` > repo `.saga/route.toml` > shipped defaults. A repo policy may pick cheaper tiers and lower caps; it may **not** add a provider to `allowed_providers`, remove a `[security]` pattern, or change `unknown` handling. Repo policies are honoured only after `saga route policy trust` records their hash, the guard-spec §2.5 mechanism, because a cloned repo must not be able to route a session's code to a provider the user excluded.

```toml
# .saga/route.toml
schema = "saga.route.policy/1"

[tiers]                       # ordered, cheapest last; ids must exist in the trace price table (trace-spec §3.3)
frontier = ["claude-fable-5.1", "claude-opus-5", "gpt-5.6-sol"]
standard = ["claude-sonnet-5", "gpt-5.6-terra", "gemini-3.1-pro"]
small    = ["gpt-5.6-luna", "claude-haiku-4.5", "gemini-3.6-flash"]
local    = ["deepseek-v4-pro-0813", "qwen3.6-27b", "devstral-small-2"]

[primary]
model  = "claude-opus-5"      # the model the user runs; route never changes it without §5.4
effort = "high"               # pinned; trace-spec §4.2 raises pin_change on any drift
tier   = "frontier"           # derived from [tiers]; index-spec §1.3 reads this as the LSP gate

[class.explore]
tier = "standard"; effort = "low";    context_budget_tokens = 60000;  cost_cap_usd = 0.50
source = "doc 06 D.1 row 3, 2026-09-02"; validated = false

[class.localize]
tier = "standard"; effort = "medium"; context_budget_tokens = 80000;  cost_cap_usd = 0.75
source = "doc 06 D.1 row 3; doc 05 §1.4 (grep at 100% success)"; validated = false

[class.mechanical]
tier = "standard"; effort = "low";    context_budget_tokens = 120000; cost_cap_usd = 2.00
source = "doc 06 D.1 row 3 (bulk edits)"; validated = false

[class.design]
tier = "primary"; effort = "primary"; context_budget_tokens = 400000; cost_cap_usd = null
source = "doc 06 D.1 row 1 and 2"; validated = false

[class.debug]
tier = "primary"; effort = "high";    context_budget_tokens = 400000; cost_cap_usd = null
source = "doc 06 D.1 row 1 (root-cause debugging)"; validated = false

[class.test]
tier = "standard"; effort = "medium"; context_budget_tokens = 120000; cost_cap_usd = 2.00
source = "none; prior"; validated = false

[class.review]
tier = "standard"; effort = "medium"; context_budget_tokens = 120000; cost_cap_usd = 1.50
source = "doc 06 D.1 row 3 (PR review); doc 03 §1.3 (separate-context reviewer)"; validated = false

[class.docs]
tier = "small";    effort = "low";    context_budget_tokens = 60000;  cost_cap_usd = 0.50
source = "none; prior"; validated = false

[class.unknown]                # fixed; a repo policy that sets this table is exit 2
tier = "primary"; effort = "primary"; context_budget_tokens = 400000; cost_cap_usd = null

[security]                     # any match pins tier = "primary" and forbids delegation below it (§7)
paths    = ["**/auth/**", "**/crypto/**", "**/*secret*", "**/*token*", "**/permissions/**", ".github/workflows/**"]
classes  = ["review"]          # security review is a class-level pin: doc 06 D.1 row 6, 38/40 blind investigations
request_terms = ["security", "vulnerability", "exploit", "auth", "credential"]

[delegation]
ladder = "L1"                  # L0 | L1 | L2 | L3, see §3.3
readonly_tier = "standard"     # small allowed only when validated = true for that (model, class) cell (§8.3)
reviewer_tier = "standard"     # security reviewer is always primary (§7)
architect_editor = false       # L3; evidence is doc 03 §1.2 only
summary_max_tokens = 2000      # doc 03 §1.3

[privacy]
allowed_providers = ["anthropic", "openai"]      # a model whose vendor is absent is never routed to (§7.2)
deny_consumer_training_plans = true              # doc 06 C.1: consumer plans train by default
deny_free_tier_keys = true                       # doc 06 C.1: AI Studio free tier trains and humans may read

[openweight]
schema_repair = true           # doc 06 D.2 item 11; applied to tier "local" and any model tagged open_weight in the price table

[policy]
mid_task_switch = "never"      # never | ask
on_fallback = "refuse"         # refuse | ask   (trace pin model.served != requested, §4.3)
on_canary_nonpass = "primary_only"

[override."ts-0031-retry-jitter"]     # keyed by contract slug; wins over classification (R0)
class = "debug"
```

### 3.2 Default policy, with provenance

Every default row above is derived from doc 06 D.1 as condensed in doc 09 §4.2 and carries `validated = false` at v0.1; `saga route validate --against bench <manifest>` flips a cell to `true` only when §8.3 says so.

| Class | Tier (v0.1 default) | Model behind it (price in/out per MTok) | Effort | Source | Validated on bench |
|---|---|---|---|---|---|
| explore, localize | standard | Sonnet 5 ($3/$15), Terra ($2.5/$15), Gemini 3.1 Pro ($2/$12) | low / medium | doc 06 A.1, A.2, A.3; D.1 row 3 | no |
| mechanical, test, review | standard | same | low / medium | doc 06 D.1 row 3 | no |
| docs | small | Luna ($1/$6), Haiku 4.5 ($1/$5, 200K), Gemini 3.6 Flash | low | doc 06 A.2, A.1, A.3 | no |
| design | primary | Opus 5 ($5/$25, 89.1% TB 2.1) or Fable 5.1 ($10/$50, cache read $0.25) | primary | doc 06 A.1; D.1 rows 1, 2 | no |
| debug | primary | same | high | doc 06 D.1 row 1 | no |
| security-flagged | primary | Opus 5 | primary | doc 06 D.1 row 6 | no |
| local tier (air-gapped) | local | DeepSeek V4-Pro-0813 ($0.435/$0.87), Qwen3.6-27B, Devstral Small 2 ($0.10/$0.30) | n/a | doc 06 A.4, A.6, A.8; vendor scores unreplicated | no |

Rows expire: the price table is hash-pinned (trace-spec §3.3) and a `saga.route.policy/1` file whose `source` dates are older than the price table's `observed` date by more than 90 days prints `stale defaults` from `policy show`.

### 3.3 Delegation ladder

| Rung | What is delegated | Tier | Evidence | Default |
|---|---|---|---|---|
| L0 | nothing; primary does all | primary | none needed | off |
| L1 | read-only sub-agents (`explore`, `localize`, `review` without the security flag); return ≤ `summary_max_tokens` | `readonly_tier` | doc 03 §1.3 (Cognition read-only sub-agents "mostly resemble tool calls"; summaries 1 to 2k tokens) | **on** |
| L2 | separate-context reviewer at Stop, prompt never contains the author's opinion (gate-spec reviewer contract) | `reviewer_tier`; primary if security | doc 03 §1.3 (about 2 bugs/PR, 58% severe, self-reported) | off until §8 |
| L3 | architect/editor: primary plans, `standard` tier emits the edit | mixed | doc 03 §1.2 only (Aider polyglot numbers in §1.3); no evidence on repo-scale tasks | off |

Writing sub-agents in parallel are not a rung (doc 09 §2: shared-worktree races, claude-code #91513). Every delegated sub-agent receives the mem-spec §4.6 preamble (≤ 1,200 bytes) and a scope no wider than the parent's contract.

---

## 4. Effort and cache discipline

### 4.1 Effort pinning

Effort is part of the plan and, when a contract exists, of the contract's evidence namespace: `saga route plan` writes `effort` into the trace pin record (`pins/current.json`, trace-spec §4.1) with `source: "route"`. Trace-spec §4.2 then raises `pin_change, severity: warn` on any per-call or settings drift, which is the #42796 mechanism made visible on the first turn it happens instead of weeks later.

| Harness | Effort knob route sets | Verified | Fallback |
|---|---|---|---|
| Claude Code | effort setting in settings (the value #42796's post-mortem names as the default that moved to `medium`); per sub-agent: none exposed | settings hash pinned; per-call value *probe* | pin the settings hash; report drift |
| Codex CLI | `model_reasoning_effort` in `config.toml` or a `[profiles.<name>]` (trace-spec §9.3) | config | reasoning quantisation detector (codex #30364) stays on |
| Gemini CLI / successor | thinking budget in settings *probe* | *probe* | settings hash only |
| Anthropic API (Fable 5.1) | none: adaptive thinking always on (doc 06 A.10) | documented | record `effort: "adaptive"`, no pin possible, plan says so |
| `bare` adapter | request field, exact | exact | none needed |

A model whose effort cannot be pinned is never given a `class.*.effort` other than what the API offers; the plan prints `effort: unpinnable` and the bench arm records it.

### 4.2 Cache-aware ordering

Changing the model starts a new cache; changing tool definitions invalidates everything below them (`tools → system → messages`, doc 05 §4.2; claude-code #91514). Route therefore obeys four ordering rules:

1. A routing decision is taken only at a **boundary**: contract start, sub-agent spawn, or a user-answered §5.4 prompt. Never inside a turn.
2. The primary's prefix (harness system prompt, tool definitions, the ≤ 1k-token index map) is never touched by route. Route injects nothing into the primary's context; its only outputs are the harness selection surface (§6) and the trace.
3. A sub-agent is a separate prefix by construction; its cost estimate (§5.2) includes one full cache write of its own prefix at the 5m write price (1.25×) and assumes 1h writes (2×) only when the plan expects the sub-agent to exceed 5 minutes (bench-spec §2.3 `expected_minutes`).
4. A mid-task primary switch, when the user allows it, is priced in the prompt as `context_tokens × cache_write price` from the last ledger row (trace-spec §3.2 `context_tokens`), so the user sees the re-write before agreeing.

### 4.3 Reading trace pins: refusal on silent fallback

Before every plan, route reads `pins/current.json` and the last 20 ledger rows:

| Observation (trace-spec §4.2) | Route action |
|---|---|
| `model.served ≠ model.requested` in any of the last 20 calls, or a `pin_change` with `severity: incident` | `plan` returns `status: "refused", reason: "provider_fallback"`, names the served model, and delegates nothing; `on_fallback = "ask"` turns this into a §5.4 prompt. The run is already `non_comparable` in the bench (ADR 0007). |
| Cache TTL bracket excludes the pinned TTL (#46829 pattern) | Plan proceeds; sub-agent cost estimates switch to 5m write prices; `explain` prints the incident |
| Canary verdict other than `pass` for the requested model (trace-spec §4.4) | `on_canary_nonpass = "primary_only"`: L1 to L3 are suspended for that model until the next `pass`; the plan says which verdict caused it |
| Reasoning quantisation flagged (codex #30364) | Effort ladder for that model is marked `unpinnable` for the session |
| Price table hash differs from the one the policy was validated against | every `validated = true` cell degrades to `stale` for display; behaviour unchanged |

Route never proceeds on an inference it cannot show: every refusal quotes the trace event id.

---

## 5. Budget integration

### 5.1 Scopes

Route adds no budget store of its own; it reads and writes trace-spec §3.6 (`session_usd`, `task_usd`) and adds the per-class `cost_cap_usd` as a third, narrower scope applied to one unit of work. Precedence when several bind: the smallest remaining amount wins, and the report names all three.

### 5.2 Pre-run estimate

`saga route plan` prints an estimate before any model call, in this order of preference, with the source labelled:

| Source | Method | Label |
|---|---|---|
| Bench rows for this `(class, size, model, harness)` cell | median `cost_usd` over `run.json` rows (bench-spec §9.3), and `cost_per_solved` (bench-spec §5.6) | `bench` |
| This repo's own ledgers | median `cum_usd` per contract of the same class over the last 30 days | `ledger` |
| None of the above | `(context_budget_tokens × in) + (context_budget_tokens × 0.15 × out)` from the price table, plus one prefix cache write; printed with a `~` prefix (trace-spec §3.1 convention) | `estimated` |

The estimate is stored in the decision record so §8.1 can measure estimate error against the ledger after the fact.

### 5.3 Stops

Reuses trace-spec §3.6 mechanisms verbatim; route only supplies the third scope and the messages below.

| Crossing | Scope | Action | Mechanism |
|---|---|---|---|
| soft (80%) | class cap, task, session | `budget` event, one line ≤ 60 tokens naming the scope and the remaining amount | PostToolUse feedback (Claude Code, Codex), AfterTool reason (Gemini), stderr (CI) |
| compact (95%) | task, session | trace's own compaction advice | trace-spec §3.6 |
| hard (100%) | class cap | sub-agent: deny its next tool call, return what it has; the parent receives `route: sub-agent <id> hit cost cap <x>/<cap>; partial summary attached` | PreToolUse deny in the sub-agent's session |
| hard (100%) | task, session | trace-spec §3.6 hard action, plus the §5.4 prompt when `hard_action = "ask"` | PreToolUse deny + Stop block |

Budget messages are route's 200-token share of the one per-session injected budget (contracts §7) and are attributed to `route` in the ledger.

### 5.4 The user-facing prompt

Fixed wording, printed by the CLI or the harness's ask channel; an agent cannot answer it (`saga route budget --raise`, `saga route policy trust`, `saga route validate --write` and `saga trace budget --raise` are on the agent-forbidden command list of contracts §8, denied by the composed PreToolUse hook).

```
saga route: task "ts-0031-retry-jitter" (class debug, claude-opus-5 @ high) reached its cap
  spent  $4.12 of $4.00 (task)      session $18.30 of $25.00
  estimate at start: $2.90 (bench, n=10)   actual so far: +42%
  options
    [1] raise task cap to $8.00 and continue on claude-opus-5 (same cache, no re-write)
    [2] continue on claude-sonnet-5           (new cache: ~152k tokens re-written, ~$0.57; bench pass^k for
                                              this cell: not validated; run becomes non-comparable)
    [3] stop and keep the working tree        (saga undo available; gates: 2 of 5 met)
    [4] mark ABANDON                          (gate-spec §2; terminal, non-successful)
  answer 1-4:
```

Option 2 appears only when `mid_task_switch = "ask"`; it is never the default and the prompt blocks until answered (doc 09 §2, "no answer means yes" is not replicated).

---

## 6. Surfaces

### 6.1 What each harness exposes

Verification follows gate-spec §6: cells marked *probe* are confirmed by the adapter conformance suite at install and never assumed.

| Harness | Primary model | Effort | Per-sub-agent model | Route's mechanism | Status |
|---|---|---|---|---|---|
| Claude Code | settings `model`, `/model` | settings effort value | sub-agent frontmatter `model:` in `.claude/agents/*.md`; Agent tool `model` input field | PreToolUse on `Agent`: route's step in the composed hook (contracts §1) sets `updatedInput.model = <tier model>`; mem's step sets `updatedInput.prompt` (mem-spec §4.6); the composed hook merges them field-wise into one `updatedInput` with `permissionDecision: "allow"`, never clobbering either; generated agent files carry `model:` from the policy | `updatedInput` documented (mem-spec §4.6); `model` field acceptance *probe* |
| Codex CLI | `model` in `config.toml` or `[profiles.<name>]`, `--profile` | `model_reasoning_effort` per profile | **none**: codex #31814 (doc 06 A.2) | route writes `[profiles.saga-<class>]` tables and the launcher picks `--profile` per task; sub-agents inherit the primary; ladder capped at L0 for sub-agents, L2 via a separate `codex exec` run | documented for profiles; sub-agent limit is a known gap |
| Gemini CLI / Antigravity CLI | settings `model`, `-m` | *probe* | *probe* | per-session only; fallback banner parsed by trace (trace-spec §9.3) and enforced by §4.3 | enterprise API-key users only (doc 06 Part B) |
| `bare` adapter | request | request | request | exact | exact |
| Cursor, OpenCode, Cline, Kilo | model picker | varies | varies | M6; read-only `route.plan` advice only until an adapter exists | not before M6 |

When a harness cannot honour a tier for a sub-agent, the plan says `applied: false, reason: "harness_no_subagent_model"` and the sub-agent runs on the primary; route never pretends a decision was applied. Trace confirms application by comparing `model_served` on the sub-agent's first call with the plan; a mismatch is a `route_decision` event with `outcome: "unapplied"` (§7.3), counted as `route_unapplied_rate` in §8.1.

### 6.2 CLI

```
saga route plan      [--class c] [--contract <slug>] [--subagent] [--tools t,..] [--harness h] [--json]
saga route explain   [<decision-id> | last] [--json]
saga route policy    show | init | validate [--file f] | trust [--file f] | diff
saga route validate  --against bench <manifest-hash> [--epsilon 0.05] [--write] [--json]
```

Exit codes follow the uniform table of contracts §4: 0 ok, 1 finding (refused plan, stale defaults, unapplied decision), 2 usage or schema (invalid policy, repo policy touching a fixed table), 3 refusal on budget (estimate exceeds the smallest binding cap), 4 trust required (repo policy hash not trusted), 6 environment (no trace, no price table).

`saga route plan --json` emits `saga.route.plan/1`:

```json
{
  "schema": "saga.route.plan/1", "decision_id": "01J7…", "status": "ok",
  "unit": {"kind": "subagent", "contract": "ts-0031-retry-jitter", "parent_turn": 47},
  "class": "localize", "rule": "R2", "flags": {"security": false, "impossible_risk": false},
  "signals": {"scope_files": 0, "impact_size": null, "impact_reason": "regime off",
              "gates_runnable": 3, "gates_failing_at_start": 1, "tool_allow": ["Read","Grep","Glob"],
              "tool_mix_recent": 0.85, "regime": "off"},
  "decision": {"tier": "standard", "model": "claude-sonnet-5", "effort": "medium",
               "context_budget_tokens": 80000, "cost_cap_usd": 0.75, "ladder": "L1",
               "schema_repair": false, "validated": false, "source": "doc 06 D.1 row 3, 2026-09-02"},
  "estimate": {"usd": 0.31, "source": "ledger", "n": 12},
  "pins": {"model_served_ok": true, "effort_pinned": true, "canary": "pass", "price_table": "sha256:…"},
  "applied": {"surface": "claude-code:Agent.updatedInput.model", "expected": true},
  "policy_hash": "sha256:…", "explain": ["R2: subagent and tool_allow ⊆ READ_TOOLS, parent scope_files == 0", "…"]
}
```

`explain` re-derives the same object from the stored signals and prints the rule trace; it never re-reads the working tree, so the explanation is of the decision that was made, not of the tree as it is now.

### 6.3 MCP

`saga route serve` exposes one read-only tool, `route_plan(class?, subagent?, tools?)`, over stdio or a Unix socket (index-spec §7.2 convention), result capped at 2 KiB (trace-spec §9.2). It returns the plan without applying it; application happens only through hooks or the launcher. There is no MCP tool for `policy trust`, `budget`, or `validate --write`, so an agent cannot widen its own tier or providers.

---

## 7. Safety

### 7.1 Never down-tier

| Step | Rule | Source |
|---|---|---|
| `security`-flagged units (paths, classes, request terms in §3.1) | tier = primary, no L1 to L3 delegation below primary; reviewer for a security diff is the primary tier | doc 06 D.1 row 6 (Opus won 38 of 40 blind investigations; Gemini reviews trend sycophantic, gemini-cli #24725) |
| Guard decisions | not routable: guard is deterministic and calls no model (guard-spec §1.1); a classifier may sit in front of it, never instead (doc 09 §2) | |
| Gate approvals, `attest`, `WAIVE:` | human actions (gate-spec §4.2, §5.3); no model is involved and route has no surface here | |
| `impossible_risk` units | tier = primary; the `ABANDON` path must be available; sub-agents on models with documented impossible-task hack rates (Sonnet lineage 12.8%, Haiku 4.5 12.6%, doc 06 A.1; GPT-5.5 ~29% lies, A.2) are not used for these units | doc 06 A.1, A.2 |
| `unknown` class | §2.4, fixed | |

### 7.2 Provider exclusion

A model is routable only if its vendor is in `[privacy] allowed_providers` **and** the trace pins show a key or plan that is not excluded: consumer plans that train by default (Anthropic Free, Pro, Max since 28 Sep 2025), the Gemini AI Studio free tier (trains, humans may read), per doc 06 C.1 and guard-spec §10.4. `saga doctor` already warns on an AI Studio key; route refuses the tier. Route repeats the guard-spec §10.4 limit verbatim in `policy show`: covered models (Fable, Mythos) retain 30 days even under ZDR since 9 Jun 2026; routing away from them changes exposure, not the policy.

### 7.3 Audit

Every decision, refusal and unapplied plan is the trace event `route_decision` (trace-spec §2.2, `component: "route"`), body = the `saga.route.plan/1` object plus `outcome ∈ {applied, unapplied, refused, user_answered}` and, for §5.4, the option chosen with `ack: "user"`. The ledger's `attribution` carries the `route` key (trace-spec §3.2) for budget messages. Nothing route writes is readable by the model except the ≤ 60-token budget lines and the sub-agent's cap notice.

---

## 8. Test plan and bench ablation

### 8.1 Tests for the layer

| Test | Fixture | Pass condition |
|---|---|---|
| Classification determinism | 200 recorded signal sets across three harnesses | identical class and rule on 3 runs and 2 platforms (macOS, Windows) |
| Classification precision | 300 hand-labelled units from the M0 task set traces (bench-spec §2.6), labels by two people, disagreements dropped | per-class precision ≥ 0.85; `unknown` rate ≤ 20% |
| Policy parser fails closed | 40 malformed policies (repo policy touching `[class.unknown]`, adding a provider, unknown model id) | exit 2 every time; no plan emitted |
| Refusal on fallback | trace fixture with `model.served ≠ requested` (gemini-cli #28859 shape) | `status: refused`, event id quoted |
| Application probe per harness | scripted fake harness echoing the sub-agent's first request | served model equals the plan; else `route_decision` with `outcome: unapplied` recorded, never silent |
| Estimate error | 100 completed contracts with ledgers | median absolute error ≤ 30% for `bench` and `ledger` sources; `estimated` printed with `~` |
| Budget prompt blocks | headless run hitting a class cap | no tool call after the cap until a CLI answer; agent attempt to run `--raise` denied and logged |
| Cache ordering | proxy-captured session with one L1 delegation | primary's `cache_write` on the turn after delegation ≤ its median (no prefix re-write caused by route) |

### 8.2 Bench ablation

Runs after M0 to M4 have landed, in stacking order `gate → guard → index → mem → shape → route` (bench-spec §4.3), so route's headline number is against the `shape` rung, never against bare.

**Arms** (paired per task, same seed index, interleaved; bench-spec §3.2):

| Arm | Content |
|---|---|
| C | prefix(5): everything below route; primary model does all work; effort at the harness default, recorded |
| R1 | C + route with ladder L1 only (read-only delegation to `standard`) |
| R1e | R1 + effort pinning at the policy value (isolates effort from delegation) |
| R2 | R1e + L2 reviewer |
| R3 | R1e + L3 architect/editor (funded at `publish` tier only) |

**Model pairs** (primary → delegate), at least three, on two harnesses:

| Pair | Primary | Delegate | Harness | Why this pair |
|---|---|---|---|---|
| P1 | Claude Opus 5 | Claude Sonnet 5 | Claude Code | same family; doc 06 A.1 gap 79.2% vs 63.2% SWE-bench Pro is the largest in-family delta |
| P2 | GPT-5.6 Sol | GPT-5.6 Luna | Codex CLI (L2 via `codex exec`, no sub-agent model: §6.1) | cheapest frontier-lineage delegate (doc 06 A.2) |
| P3 | Claude Fable 5.1 | Claude Opus 5 | Claude Code | tests whether the $0.25 cache read (doc 06 A.1) already makes delegation pointless at the top |
| P4 (optional) | Claude Sonnet 5 | Claude Haiku 4.5 | Claude Code, index `lsp = "on"` and `"off"` crossed | the doc 05 §1.4 interaction (LSP: Haiku −26%, Sonnet +118%) |

**Tasks.** M0 set, three languages, sizes S to L (bench-spec §2.3), ≥ 30 per language, `publish` tier K = 10 (bench-spec §4.4; $500 to $2,000). Cost per cell is bounded by bench-spec §4.5.

**Null hypothesis and decision rule.** Pre-registered per pair and per class:

- H0: `cost_per_solved(R) ≥ cost_per_solved(C)` **or** `pass^10(R) < pass^10(C) − ε`, with ε = 0.05 (TOST equivalence bound, bench-spec §5.5).
- A class default ships as `validated = true` for a `(model, class)` cell only when, on **at least two of the three pairs**, the bootstrap 95% CI of Δ`cost_per_solved` excludes zero in the cheaper direction **and** the CI of Δ`pass^10` lies inside `[−ε, +ε]` **and** Δ`false_done` (bench-spec §5.4) is not positive with CI excluding zero **and** the `component_unused` and `route_unapplied` counts are below 10% of runs.
- One pair passing licenses nothing (bench-spec §1.3 transfer rule). A pair where the CI on Δ`pass^10` crosses −ε ships `validated = false` and the rung stays off for that model; the negative result is published in the report's `negative` array.
- Effort: R1e vs R1 is reported on its own; if pinned effort lowers `pass^10` with CI excluding zero, the class effort default is raised, never the model changed (HAL, doc 03 §2.1, is the prior that says this can happen).
- Detection floor: the bench cannot see below the 3.5 to 4.5 point per-model standard error (doc 06 A.10); a cost delta smaller than the pair's cache-write noise, or a quality delta inside that floor, is reported as "within noise" and does not flip `validated`.

### 8.3 Per-model validation matrix

Maintained in the repo as `docs/specs/route-validation.md`, regenerated by `saga route validate --write` from a bench manifest, and rendered in `policy show`. Initial state:

| Model | explore | localize | mechanical | design | debug | test | review | docs |
|---|---|---|---|---|---|---|---|---|
| claude-opus-5 (primary) | n/a | n/a | n/a | n/a | n/a | n/a | n/a | n/a |
| claude-sonnet-5 | no (P1) | no (P1) | no (P1) | never | never | no (P1) | no (P1, non-security) | no |
| claude-haiku-4.5 | no (P4) | no (P4) | never | never | never | never | never | no |
| gpt-5.6-luna | no (P2) | no (P2) | no (P2) | never | never | no (P2) | no (P2) | no |
| claude-fable-5.1 | n/a | n/a | n/a | primary (P3) | primary (P3) | n/a | primary if security | n/a |
| gemini-3.1-pro | no | no | no | never (TB 2.0 54.2%, doc 06 A.3) | never | no | never (sycophantic review, doc 06 A.3) | no |
| deepseek-v4-pro-0813, qwen3.6-27b | no | no | no | never | never | no | never | no |

Cell values: `yes` (validated, with manifest hash), `no` (prior only, delegation allowed at `standard` but reported as unvalidated in every plan), `never` (§7 or a documented mechanism forbids it), `n/a` (the model is the primary in that pair). A `small`-tier cell is used by L1 only when it reads `yes`.

---

## 9. Open problems and settling experiments

| # | Problem | Settling experiment |
|---|---|---|
| 1 | **Tool value vs model strength.** LSP helped Haiku and hurt Sonnet on a small N (doc 05 §1.4); whether the index's `tier` gate and route's delegation compound or cancel is unmeasured | P4 crossed with `lsp on/off` (§8.2); fit Δ`tokens_per_solved = f(tier, lsp, regime)`; if the interaction term's CI excludes zero, index-spec §1.3's `auto` rule and route's `readonly_tier` are set jointly, otherwise independently |
| 2 | **Classification precision on real work.** §2.3 thresholds are priors from bench size classes | §8.1 labelled corpus, then a `user`-tier field study on three repos counting `override` rows users add per week (each is a misclassification) |
| 3 | **Provider fallback detection coverage.** Claude Code transcripts carry `model`; Codex and Gemini per-call model are *probe* (trace-spec §9.3) | install probe per harness; where the served model is unobservable, route treats the cell as `on_fallback = "refuse"` unreachable and prints `fallback_detection: unavailable` in every plan, and the bench excludes that harness from routing claims |
| 4 | **Does the cache discount already beat delegation?** Fable 5.1 cache reads at $0.25 (doc 06 A.1) may make a `standard`-tier sub-agent's fresh prefix dearer than the primary reading its warm one | P3; report Δ`cost_per_solved` with the sub-agent's cache write itemised from the ledger |
| 5 | **Effort as the stronger variable.** Doc 09 §7 item 4 says effort outranks model choice for Claude models; HAL says higher effort can hurt | R1e vs R1 per class; publish the per-class effort curve before any model default is revised |
| 6 | **Architect/editor at repo scale.** Only Exercism-scale evidence (doc 03 §1.2) | R3 at `publish` tier on L tasks only; ships as a default only under the §8.2 rule on two pairs |
| 7 | **Sub-agent model selection on Codex.** codex #31814 | tracked as an upstream dependency; ladder capped at L0 for Codex sub-agents until the adapter probe passes |
| 8 | **Local-tier validation.** Vendor numbers for DeepSeek, Qwen, Kimi unreplicated (doc 06 A.4 to A.6) | `user`-tier bench on the user's own repo is the only path; `schema_repair` effect measured as a separate arm (guard-spec §12, deferred to M5) |
| 9 | **Estimate drift.** Prices and cache TTLs change server-side (#46829, #46917) | estimate error tracked per session (§8.1); a median error above 30% for 7 days writes a trace `budget` event with `metric: "estimate_error"`, `action: "warn"` and the plan switches to the `ledger` source (the canary verdict set of trace-spec §4.4 stays closed) |
