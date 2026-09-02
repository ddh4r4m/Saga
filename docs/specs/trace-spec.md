# `saga trace`: technical specification

*Draft v0.1, 2026-09-02. Implements doc 09 §3.7 and §3.9 and ADR 0007. Ships in M0 alongside `saga bench` (doc 09 §5): the bench consumes `saga.trace/1` (bench-spec §3.4) and the M0 exit criterion requires the ledger to reconcile with provider-reported usage. Companions: bench-spec §5.9 (drift events, shared definitions), §8.3 (replay), gate-spec §6 (adapter contracts) and §9 (token budget attribution), ADR 0006 (guard snapshots, which checkpoints reference).*

---

## 1. Purpose, non-goals, guarantees

### 1.1 Purpose

`saga trace` is the local, append-only record of what an agent session did and what it cost, written by hook adapters and log normalisers, never by the model. It exists because the largest cross-harness problem in doc 07 §6 is that cost is opaque (claude-code #16157: "accumulated tool output dragging hundreds of thousands of tokens on every single turn", found only by a user's own 82-session analysis) and because server-side changes arrive without a client version bump (cache TTL 1h to 5m, #46829; ~20K cache_creation tokens per request keyed on User-Agent, #46917; Pro silently served as Flash, gemini-cli #2208 and #28859; reasoning tokens quantised at 516/1034/1552, codex #30364). Doc 07 §8 names the only levers available outside the harness: pin, record, verify, canary. Trace is the record; the ledger, pins, watchdog and canary are what the record makes possible.

### 1.2 Non-goals

| Not this | Because |
|---|---|
| A telemetry service | Nothing leaves the machine except opt-in reconciliation calls to the provider's own usage endpoint (§3.4, §10). |
| A determinism guarantee | Sampling is not reproducible on hosted APIs (doc 05 §4.1: 80 outputs in 1,000 temperature-0 runs). Trace makes runs replayable and outcomes comparable, not identical. |
| An LLM summary of the session | ADR 0004. Every derived number is a count, a hash, or a price-table product. |
| A harness transcript replacement | The harness's own JSONL stays authoritative for the harness; trace normalises it and adds what the harness does not record. |
| A fix for hangs, fallbacks or the edit tool | Doc 09 §3.10. Trace detects and pauses; it cannot repair the stream. |

### 1.3 Guarantees by tier

| Recorded | Always (any harness with a hook or a log) | Only with adapter support | Only with proxy capture |
|---|---|---|---|
| Tool calls, args hash, result hash, exit status, wall time | yes | | |
| File edits with before/after content hashes | yes (PostToolUse diff of the working tree) | | |
| Turn boundaries, user prompts (hashed, masked) | yes | | |
| Token usage per model call by direction and cache state | | transcript or stream-json usage (Claude Code, Codex, Gemini CLI: §9.3) | any harness |
| Model id as returned by the API, per call | | where the transcript carries it | yes |
| System prompt hash, tool-definition hash | | `bare` adapter only | yes |
| Effort or thinking setting | settings file hash always | per-call value where the API echoes it | yes |
| Cache TTL observed | | derived from usage deltas over time (§4.2) | derived, plus `cache_control` echoes |
| Provider request id, `system_fingerprint` | | | yes |
| Reasoning tokens | | where exposed (OpenAI `reasoning_tokens`; Anthropic bills thinking as output, recorded as `null` with reason) | same |

Every field the adapter cannot fill is `null` with a sibling `<field>_reason`, the bench-spec §6.2 rule. Silent omission is a conformance failure (§11).

---

## 2. Event format: `saga.trace/1`

### 2.1 Envelope

One JSON object per line. Every event shares:

```json
{
  "schema": "saga.trace/1",
  "seq": 1842,                       // monotonic per session, gapless
  "ts": "2026-09-02T14:03:11.204Z",  // wall clock, UTC, ms
  "mono_ns": 91834110234,            // monotonic clock since session start
  "session": "01J6Y…",               // harness session_id when present, else ULID
  "turn": 47,                        // 0 = before first user prompt
  "agent": "main",                   // "main" or subagent id
  "type": "tool_call",
  "source": "hook:PostToolUse",      // hook:<event> | transcript | stream-json | proxy | cli | derived
  "body": { … },                     // type-specific (§2.2)
  "prev": "sha256:…",                // hash of the previous event in this session
  "hash": "sha256:…"                 // sha256 of canonical JSON of this object with "hash" removed
}
```

Canonical JSON is sorted keys, no insignificant whitespace, UTF-8, the bench-spec §8.1 rule. The `prev` chain makes a session file tamper-evident; `saga trace verify` walks it.

### 2.2 Event types

The task list in doc 09 §3.7 names twelve types; three are added (`session`, `drift`, `checkpoint`) because pins, watchdog findings and restart points need a home that is not a turn.

| Type | When | Body fields (beyond envelope) |
|---|---|---|
| `session` | start, end, resume, pin change | `phase` (start/end/resume/pin_change), `pins` (§4.1 object), `harness`, `cwd_hash`, `config_hash` (`.saga/config.toml`), `changed` (list of pin keys that differ from the previous `session` event) |
| `turn` | user prompt received; assistant turn ended | `phase` (user/assistant_end), `prompt_hash`, `prompt_bytes`, `final_message_hash`, `claimed_done` (bench-spec §5.4 abstention list applied) |
| `model_call` | one provider request | `model_requested`, `model_served`, `request_id`, `fingerprint`, `effort`, `usage` (§3.1), `call_key` = sha256(system_hash, messages_hash, tools_hash, model_requested), `system_hash`, `tools_hash`, `context_tokens_est`, `latency_ms`, `stop_reason`, `status` (ok/429/5xx/timeout), `attribution` (§3.5) |
| `tool_call` | a tool is invoked | `tool`, `args_hash`, `args_ref` (blob or inline ≤ 4 KiB, masked), `component` (harness/index/mem/shape/gate/guard/mcp:<server>/user), `cwd_rel` |
| `tool_result` | tool returned | `for_seq`, `exit`, `error`, `result_hash`, `result_bytes`, `result_ref`, `truncated` (bool, head/tail bytes kept), `wall_ms` |
| `edit` | working-tree change observed after a tool | `path` (repo-relative), `before_hash`, `after_hash`, `hunks`, `added`, `removed`, `by_tool` (seq), `in_scope` (nullable when no contract) |
| `gate` | gate-spec check, guard-diff, red proof, Stop decision | `kind` (check/guard_diff/red/stop/claim), `ids`, `states`, `decision`, `progress_hash`, `message_tokens_est` |
| `guard` | command classification, deny, snapshot, mask | `kind` (classify/deny/snapshot/mask), `segments`, `class`, `snapshot_id`, `masked_count`, `decision` |
| `mem_inject` | records injected adjacent to a tool call | `record_ids`, `bytes`, `tokens_est`, `trigger` (path/tool) |
| `compaction` | harness compaction observed | `phase` (pre/post), `context_tokens_before`, `context_tokens_after`, `state_block_hash` (mem layer, M1), `trigger` (auto/manual) |
| `subagent` | spawn, stop | `phase`, `subagent_id`, `parent_agent`, `model_requested`, `preamble_hash`, cumulative `usage` at stop |
| `budget` | threshold crossed or decision taken | `scope` (session/task/turn), `metric`, `limit`, `value`, `action` (warn/compact/stop/none), `ack` |
| `drift` | watchdog signal or action | `signal` (§5.1 id), `window`, `drift_index`, `action`, `evidence` (seqs) |
| `checkpoint` | restart point written | `checkpoint_id`, `snapshot_id`, `context_hash`, `open_gates`, `ledger_cum` |
| `canary` | canary run or verdict | `run_id`, `task_set_hash`, `baseline_hash`, `metrics`, `verdict` (§4.4) |

### 2.3 Content-hash keys

| Key | Definition | Used by |
|---|---|---|
| `args_hash` | sha256 of canonical JSON of tool arguments after masking | repeat detection (§5), replay (§6) |
| `result_hash` | sha256 of the full masked result bytes, before truncation | output-aware loop detection; replay cache |
| `before_hash`, `after_hash` | sha256 of file bytes; `sha256:empty` for absent | oscillation detection; checkpoints |
| `call_key` | sha256(system_hash ‖ messages_hash ‖ tools_hash ‖ model_requested) | model-call cache for replay (doc 05 §4.2) |
| `context_hash` | sha256 of ordered (`seq`, `hash`) pairs from session start to the checkpoint | fork identity |

Hashing happens after masking, so a changed mask never changes a stored hash retroactively and hashes can be published (bench-spec §8.2).

### 2.4 Ordering and clocks

`seq` is assigned by a single writer per session (a lock file under the session directory); adapters that fire concurrently (Claude Code parallel tool calls, subagents) queue on the lock. `mono_ns` orders events within a host; `ts` is for humans. Cross-agent ordering uses `seq` only. Events from a transcript normaliser that arrive after hook events for the same call are merged by `(turn, tool, args_hash)` and recorded as `source: "transcript"` with `merged_into: <seq>`; nothing is rewritten.

### 2.5 Size caps

| Payload | Inline cap | Beyond cap |
|---|---|---|
| Tool args, tool result, prompt, final message | 4 KiB (gate-spec §4.4 uses the same cap for failure diagnostics) | stored in `blobs/<sha256>` up to 8 MiB; the event carries `*_ref` |
| Blob above 8 MiB | | head 64 KiB + tail 64 KiB kept, `truncated: true`, hash of the full payload retained |
| Event line | 16 KiB | writer refuses and records a `tool_result` with `error: "event_oversize"` |
| Session file segment | 64 MiB | rotate to `events.<n+1>.jsonl`; `prev` chain continues across segments |

Tool results are truncated never mid-line and keep the error-aware tail (doc 05 §5, tool-result truncation row).

### 2.6 Redaction before persistence

Every string field passes the masker before hashing or writing. In M0 the masker is a built-in gitleaks-class rule set (provider keys, JWTs, private-key blocks, `.env` assignments, IPv4 and email as configurable classes); from M1 it is `saga guard mask` (ADR 0006) and the built-in set is its fallback when guard is not installed. Placeholders are `«saga:mask:<class>:<n>»`, stable within a session so equality comparisons still work. `masked_count` per event is recorded; a positive-control corpus (§11) must mask at 100% and a negative-control corpus at 0%. Masking has a non-zero miss rate on novel formats and the README says so (gate-spec §8).

### 2.7 Storage layout

```
.saga/trace/                       # gitignored by `saga trace init`
├── sessions/<session>/
│   ├── events.000001.jsonl        # append-only, rotated at 64 MiB
│   ├── blobs/<sha256>             # masked payloads over 4 KiB
│   ├── checkpoints/<turn>.json    # §6.3
│   ├── ledger.jsonl               # derived per model_call rows (§3.2); rebuildable from events
│   └── LOCK
├── pins/current.json              # last observed pins (§4.1)
├── canary/<run_id>/               # canary runs, bench-spec archive layout
├── prices/<sha256>.toml           # every price table ever used
└── index.sqlite                   # session list, totals, drift events; rebuildable
```

Everything under `.saga/trace/` is derived from `events.*.jsonl` plus `blobs/`; `saga trace rebuild` regenerates `ledger.jsonl` and `index.sqlite` and must be byte-identical (§11).

### 2.8 JSON schema (excerpt, normative)

```json
{
  "$id": "https://saga.dev/schema/trace/1",
  "type": "object",
  "required": ["schema","seq","ts","mono_ns","session","turn","agent","type","source","body","prev","hash"],
  "properties": {
    "schema": {"const": "saga.trace/1"},
    "seq": {"type": "integer", "minimum": 1},
    "type": {"enum": ["session","turn","model_call","tool_call","tool_result","edit","gate","guard",
                      "mem_inject","compaction","subagent","budget","drift","checkpoint","canary"]},
    "source": {"pattern": "^(hook:[A-Za-z]+|transcript|stream-json|proxy|cli|derived)$"},
    "prev": {"pattern": "^sha256:[0-9a-f]{64}$|^sha256:genesis$"},
    "hash": {"pattern": "^sha256:[0-9a-f]{64}$"},
    "body": {"type": "object"}
  },
  "additionalProperties": false
}
```

Per-type `body` schemas live in `schema/trace/1/<type>.json` in the repo; a field the schema does not know is a validation failure, not a warning.

---

## 3. Per-turn cost ledger

### 3.1 Token accounting

`model_call.body.usage`:

```json
{
  "input_fresh": 1820, "cache_read": 141200, "cache_write_5m": 0, "cache_write_1h": 9100,
  "output": 412, "reasoning": null, "reasoning_reason": "anthropic bills thinking inside output",
  "source": "transcript", "raw": {"input_tokens": 1820, "cache_read_input_tokens": 141200, "…": "…"}
}
```

| Field | Anthropic source | OpenAI source | Google source |
|---|---|---|---|
| `input_fresh` | `input_tokens` | `input_tokens − cached_tokens` | `prompt_token_count − cached_content_token_count` |
| `cache_read` | `cache_read_input_tokens` | `input_tokens_details.cached_tokens` | `cached_content_token_count` |
| `cache_write_5m` / `_1h` | `cache_creation.ephemeral_5m_input_tokens` / `ephemeral_1h_input_tokens`; if only the total `cache_creation_input_tokens` exists, it goes to the TTL currently pinned (§4.2) and `split_reason` is set | not billed separately (0) | explicit cache create calls only |
| `output` | `output_tokens` | `output_tokens` | `candidates_token_count` |
| `reasoning` | `null` | `output_tokens_details.reasoning_tokens` | `thoughts_token_count` |

`raw` keeps the provider's object verbatim so a future field can be re-derived without re-running anything. Where no usage source exists, `source: "estimated"` and tokens are `ceil(utf8_bytes / 4)` (gate-spec §9); estimated rows are never reconciled and are shown in the report with a `~` prefix.

### 3.2 Ledger row (derived, `ledger.jsonl`)

```json
{"schema":"saga.ledger/1","session":"01J6Y…","turn":47,"seq":1840,"model":"claude-opus-5",
 "usage":{…},"price_table":"sha256:…","usd":{"input":0.0091,"cache_read":0.0706,"cache_write":0.0569,"output":0.0103,"total":0.1469},
 "context_tokens":152532,"context_delta":9100,"tool_output_bytes_turn":31804,
 "attribution":{"harness":0.61,"index":0.0,"shape":0.0,"mem":0.0,"gate":0.01,"user":0.02,"tool_results":0.36},
 "cache_hit_ratio":0.926,"ttl_inferred":"1h","cum_usd":3.41}
```

`cache_hit_ratio = cache_read / (input_fresh + cache_read + cache_write_5m + cache_write_1h)`. `context_delta` is this call's `input_fresh + cache_write_*` minus the previous call's, which is the per-turn growth #16157's author had to compute by hand.

### 3.3 Price table

`saga.prices/1`, TOML, one file per version, addressed by sha256 and pinned into every ledger row and bench manifest (bench-spec §8.1 `price_table_sha256`).

```toml
schema = "saga.prices/1"
observed = 2026-09-02
[[model]]
id = "claude-fable-5.1";  vendor = "anthropic"; training_cutoff = "2026-06"
in = 10.00; out = 50.00; cache_read = 0.25; cache_write_5m = 12.50; cache_write_1h = 20.00
source = "https://www.anthropic.com/claude-fable-and-mythos-5-1"; note = "doc 06 A.1: cache read −75%; write 1.25×/2× per doc 05 §4.2"
[[model]]
id = "claude-opus-5";     vendor = "anthropic"; in = 5.00;  out = 25.00; cache_read = 0.50; cache_write_5m = 6.25; cache_write_1h = 10.00; source = "doc 06 A.1"
[[model]]
id = "claude-sonnet-5";   vendor = "anthropic"; in = 3.00;  out = 15.00; cache_read = 0.30; cache_write_5m = 3.75; cache_write_1h = 6.00;  source = "doc 06 A.1"
[[model]]
id = "gpt-5.6-sol";       vendor = "openai";    in = 5.00;  out = 30.00; cache_read = 0.50; cache_write_5m = 5.00;  source = "doc 06 A.2 (−20% since 2026-08-21; cache read −90%)"
[[model]]
id = "gemini-3.1-pro";    vendor = "google";    in = 2.00;  out = 12.00; long_context = { over = 200000, in = 4.00, out = 18.00 }; source = "doc 06 A.3"
[[model]]
id = "deepseek-v4-pro-0813"; vendor = "deepseek"; in = 0.435; out = 0.87; cache_read = 0.0036; source = "doc 06 A.4 (raised 2026-08-16)"
```

Prices are USD per million tokens. Rules: a model id absent from the table costs `null`, never 0, and the report prints `unpriced`; `saga trace prices update` fetches nothing automatically, it takes a file and records its hash, source and date; two ledger rows priced under different tables are never summed without a `mixed_price_tables` flag.

### 3.4 Reconciliation

`saga trace ledger --reconcile <day>` compares ledger sums with the provider's own usage report for the same key and window, opt-in and per vendor:

| Vendor | Endpoint | Granularity | Fields compared |
|---|---|---|---|
| Anthropic | Admin API usage and cost report (org admin key) | per model, per day, by cache state | uncached input, cache read, cache creation (5m/1h), output |
| OpenAI | `/v1/organization/usage/completions` (admin key) | per model, per day or per hour | input, cached input, output, reasoning |
| Google | none for API keys; Cloud Billing export on Vertex | per day | total tokens only; reconciliation marked `partial` |
| Subscription plans (Max, Plus, Pro) | none | | reconciliation `unavailable`; the ledger is still shown as the only accounting the user has |

Output: `tokens_ledger`, `tokens_provider`, `error_pct` per field, and the largest unexplained session. Pre-registered tolerance for the M0 exit criterion: `|error_pct| ≤ 2` on every field where `usage.source ∈ {transcript, stream-json, proxy}`, computed over at least 20 sessions across 3 days. Divergence above tolerance is itself a finding (#46917 was found exactly this way) and is written as a `canary` event with `verdict: "billing_mismatch"`.

### 3.5 Per-component attribution

Providers do not report tokens per prompt region, so attribution is an estimate and is labelled as one. Method: every byte added to the context in a turn has a known origin at the hook boundary (the harness system prompt and tool definitions, the user prompt, each tool result tagged with its `component`, each gate or guard message, each `mem_inject`). Trace sums estimated tokens per origin for the turn, scales them so they total the observed `context_delta`, and stores the shares. Cumulatively over a session, share × cost gives dollars per component. The `bare` adapter, which owns the prompt, reports exact counts by calling the provider's token-count endpoint per region on the first turn (Anthropic `count_tokens`, OpenAI `tiktoken` locally) and the ratio between exact and estimated is printed as `attribution_calibration`; the bench (§11) reports the same ratio for every harness adapter.

### 3.6 Budgets

`.saga/config.toml`:

```toml
[trace.budget]
session_usd = 25.00        # null = unlimited
task_usd    = null         # per contract (gate-spec §2); requires a contract
turn_context_tokens = 400000   # warn when one call's total input exceeds this (#16157 class)
soft_pct = 80              # warn
compact_pct = 95           # suggest compaction (Claude Code: emit additionalContext; others: stderr)
hard_pct = 100             # stop
hard_action = "stop"       # stop | ask | warn-only
```

| Crossing | Action | Mechanism per harness |
|---|---|---|
| soft | `budget` event, one-line message ≤ 60 tokens | PostToolUse feedback (Claude Code, Codex), AfterTool reason (Gemini), stderr (CI) |
| compact | message recommends `/compact` and names the three largest tool results by bytes | same |
| hard | deny the next tool call with reason `saga trace: budget <scope> exhausted (<value>/<limit>); run saga trace budget --raise` and block Stop-equivalent with the same text until acknowledged | PreToolUse deny + Stop block (gate-spec §6 envelopes) |

Budget messages count against the gate-spec §9 per-session injected cap of 1,000 estimated tokens; the ledger attributes them to `trace`. Raising a budget is a CLI action by the user, logged with `ack: "user"`; an agent cannot raise it (the Bash matcher denies `saga trace budget --raise`, mirroring gate-spec §6.1).

### 3.7 The report the user sees

`saga trace ledger [session]`:

```
session 01J6Y…  claude-code 2.1.190  claude-opus-5  2026-09-02 13:02Z → 14:21Z  47 turns  63 tool calls
                       tokens        usd      share
input (fresh)          182,340     0.912     14.9%
cache read           4,120,660     2.060     33.7%
cache write (1h)       391,100     3.911     64.0%   ← 61% of cost; 3 full re-writes at turns 12, 29, 31
output                  12,488     0.312      5.1%
reasoning                  n/a       n/a           (anthropic: billed inside output)
total                                6.115           budget 25.00  (24%)  price table 2026-09-02 sha256:3f9a…

cache hit ratio  0.887   ttl inferred 1h (idle gaps of 6m, 9m, 14m did not cause re-writes)
per-turn context  min 41k  median 152k  max 238k  growth +4.2k/turn  largest turn +38k (turn 29: Bash result 31,804 B)

attribution (estimated, calibration 0.96)          usd
  harness (system prompt, tool defs, builtin results)   3.73
  tool results from user commands                       2.20
  gate/guard messages                                   0.06
  mem, index, shape                                     0.00 (not installed)
  user prompts                                          0.13

top tool results by bytes:  #1840 Bash `pnpm test` 31,804 B   #1712 Read src/… 18,220 B   #1633 Grep 12,910 B
```

`--json` emits `saga.ledger.report/1` with the same fields; `--by turn|component|tool|agent` pivots the same rows.

---

## 4. Version pinning and canary evals

### 4.1 Pin record

Written as `session.body.pins` at start and whenever a value changes; the latest copy lives in `pins/current.json`.

```json
{"schema":"saga.pins/1",
 "harness":{"name":"claude-code","version":"2.1.190","binary_sha256":"…","settings_hash":"…","hooks_hash":"…"},
 "model":{"requested":"claude-opus-5","served":"claude-opus-5","served_reason":null,"fingerprint":null,"fingerprint_reason":"anthropic does not expose one"},
 "effort":{"value":"medium","source":"settings","source_hash":"…"},
 "system_prompt":{"sha256":null,"sha256_reason":"not exposed by claude-code hooks; enable proxy"},
 "tools":{"sha256":"…","count":19},
 "cache":{"ttl_pinned":"1h","ttl_observed":"1h","observed_at":"2026-09-02T13:40:02Z"},
 "price_table":"sha256:…","saga":{"version":"0.3.0","components":["trace","doctor"]}}
```

### 4.2 Change detection

| Signal | Rule | Event |
|---|---|---|
| Model served ≠ requested | any `model_call` where `model_served` differs, or the harness log names a fallback (gemini-cli #28859 class, claude-code #91522) | `session pin_change`, `severity: incident`; bench marks the run `non_comparable` (ADR 0007) |
| Harness version changed between consecutive sessions | string compare | `pin_change`, `severity: info`; canary triggered (§4.4) |
| Effort or thinking setting changed | settings hash or per-call value | `pin_change`, `severity: warn` (the #42796 mechanism) |
| Tool-definition hash changed mid-session | `tools_hash` differs between calls | `pin_change`; expected cache re-write flagged (doc 05 §4.2: tools sit at the top of the cache hierarchy; claude-code #91514) |
| Cache TTL observed ≠ pinned | inference: after an idle gap `g` between consecutive calls with unchanged prefix, a `cache_write ≥ 0.8 × previous context` implies TTL < g; a `cache_read ≥ 0.8 × previous context` implies TTL ≥ g. Observed TTL is the largest gap with a read and the smallest gap with a re-write, bracketed. | `pin_change`, `severity: incident` when the bracket excludes the pinned TTL (#46829: 1h regressed to 5m) |
| Cache write per call above baseline | `cache_write` median over the last 20 calls > 2 × the pinned baseline median at equal `context_delta` | `canary billing_mismatch` (#46917: ~20K inflation per request) |
| Reasoning quantisation | over the last 50 calls with `reasoning > 0`, ≥ 50% fall on ≤ 3 distinct values | `pin_change`, `severity: warn` (codex #30364) |

### 4.3 Pinning advice

`saga trace pin` prints the current pins, the changes since the last session, and one recommendation per changed key, in fixed wording:

```
harness   claude-code 2.1.188 → 2.1.190   changed 2026-09-02   canary: pending
effort    medium (settings)                unchanged
cache ttl pinned 1h, observed 5m (bracket 4m..9m)   INCIDENT: matches claude-code #46829 pattern
advice: pin harness to 2.1.188 until the canary passes:  npm i -g @anthropic-ai/claude-code@2.1.188
        report the TTL observation with: saga trace export --incident 01J6Y… (masked bundle, local file)
```

Advice is generated from a fixed table keyed by (changed key, severity); it never speculates about causes.

### 4.4 Canary evals

**Task set.** 20 tasks from the M0 bench set (bench-spec §2.6), sizes S and M only, TypeScript, Python and Go, composition 14 plain, 4 hack-bait, 2 impossible, frozen by `task_set_hash` and rotated with the bench (bench-spec §2.5). Runs use the bench archive layout under `.saga/trace/canary/<run_id>/`, `tier = smoke` rules, arm = the user's actual config with its hash recorded.

**Baseline.** The pinned baseline is the last 200 canary runs at the pinned `(harness version, model, effort, price table)`; a pin change starts a new baseline and the old one is kept.

**Cadence.** Default: 20 runs per day (one per task, K = 1), plus a 60-run burst (K = 3) on any `pin_change` of severity info or above. Cost: at the bench-spec §2.2 example `cost_hint_usd = 0.85`, the daily job is about $17 and the burst about $51.

**Pre-registered detection thresholds and power.** Two classes of metric, because they differ by an order of magnitude in power:

| Metric class | Statistic | Threshold | Runs to detect (α = 0.05 one-sided, power 0.80) | Source of the arithmetic |
|---|---|---|---|---|
| Cost and cache (per-run continuous): `cache_hit_ratio`, `cache_write` per call at matched `context_delta`, tokens per task, `reasoning` distinct-value count | median over the burst vs baseline median, Mann-Whitney | `cache_hit_ratio` drop ≥ 0.15 absolute; `cache_write` ≥ 2× baseline; tokens per task ≥ +30% | 2 to 4 runs at a between-run sd of 0.05 to 0.08 on the ratio: `n = (1.645+0.84)² · 2σ² / Δ²` = 1.4 to 3.5 | normal approximation; sd is a prior to be replaced by the first 200-run baseline |
| Outcome (Bernoulli per run): pass@1, false-done rate | difference of proportions against the 200-run baseline (ratio 10:1) | pass@1 drop ≥ 10 pp | 170 canary runs (≈ 9 daily cycles, ≈ $144); a 5 pp drop needs 679 runs (≈ 34 cycles, ≈ $577) | `n = (z_α+z_β)² · p(1−p) · (1 + 1/r) / Δ²` with p = 0.5, r = 10 |

The 200-run baseline has a standard error of 3.5 pp on pass@1 at p = 0.5, which matches the 3.5 to 4.5 point per-model standard error Anthropic reports for its own evals (doc 06 A.10); the canary cannot see below that floor and says so. Outcome metrics are accumulated with a one-sided CUSUM (reference value Δ/2 = 5 pp, decision interval set for an in-control average run length of 100 daily cycles, calibrated by simulation in §11) so that a sustained regression is flagged as early as the evidence allows and a single bad day is not. Doc 09 §7 item 8 asked whether the canary is a daily or a per-release job; the answer is both: daily for cost and cache metrics, where 2 to 4 runs suffice, and cumulative for outcomes, where a release-triggered 60-run burst detects only ≥ 15 pp drops on its own (137 runs at ratio 1, 75 at ratio 10).

**Verdicts.** `canary.body.verdict ∈ {pass, cost_regression, cache_regression, outcome_regression_suspected, outcome_regression, billing_mismatch, non_comparable}`; every verdict carries the statistic, the CI, n, and the baseline hash. `outcome_regression` requires the CUSUM crossing; `_suspected` is the burst alone.

**Alerting.** Local only: `saga doctor` shows the last verdict, `saga trace canary --watch` exits non-zero on a new non-pass verdict for a cron or CI job to act on, and the pin advice (§4.3) names the verdict. No network notification exists in this layer.

---

## 5. Drift and stall watchdog

### 5.1 Signals

Definitions of the first five are identical to bench-spec §5.9 so offline and online numbers agree; the rest are online-only and are recorded but not part of the bench's drift index.

| Id | Definition | Default threshold | Source |
|---|---|---|---|
| `repeat` | ≥ 3 consecutive tool calls with identical `(tool, args_hash)` | 3 | bench-spec §5.9; OpenHands #7183 detector |
| `repeat_identical_output` | `repeat` where every `result_hash` is also identical | 3 (counted separately; the output-aware rule from gemini-cli #11002 and doc 06 D.2 item 7) | |
| `edit_fail_streak` | ≥ 3 consecutive edit tool calls returning error | 3 | bench-spec §5.9; cline #4384 class |
| `edit_same_hunk` | ≥ 3 edits to the same path whose hunks overlap by line range with a prior failed or reverted edit | 3 | claude-code #19699 repeats same failing command |
| `oscillation` | `before_hash → h₂ → before_hash` for the same path within 10 edits | 1 occurrence | bench-spec §5.9 |
| `out_of_scope_read` | read of a path outside `IN:` after the first edit (contract present) | 1 | bench-spec §5.9 |
| `late_scope_expansion` | first edit to a new file after > 70% of the turn cap or of the task's `expected_minutes × multiplier` | 1 | bench-spec §5.9 |
| `scope_violation` | `edit.in_scope = false` | 1 | gate-spec §5 G-SCOPE, recorded here for the index |
| `tool_error_streak` | ≥ 5 consecutive `tool_result.error` of any tool | 5 | cline #4356 stall patterns |
| `wall_stall_output` | no bytes from the harness (stream-json) or no hook event for 180 s while a turn is open | 180 s | claude-code #91502: "client waits 180s+"; #26224 5 to 20 minute hangs |
| `wall_stall_tool` | no `tool_call` for 600 s while a turn is open and the last event is not a `model_call` in flight | 600 s | gemini-cli #22141: 1+ hour stalls |
| `context_no_progress` | 3 consecutive turns each with `context_delta > 20,000` tokens and no `edit`, no test-runner `tool_call`, no `gate` state change | 3 turns | claude-code #16157; #27281 "repeated 'let me write the document' without executing" |

### 5.2 Drift index

Shared with bench-spec §5.9: `drift_index = events / tool_calls`, where `events` counts occurrences of the first five ids (`repeat`, `edit_fail_streak`, `oscillation`, `out_of_scope_read`, `late_scope_expansion`), each counted once per streak. Online, trace computes it over a rolling window of the last 20 tool calls (`drift_index_w20`) and over the whole session; the bench uses the whole-run value. The 22.7 pp escalation per off-path call (doc 03 §2.2, arXiv 2602.19008) is why the window is short: the signal is the recent slope, not the average.

### 5.3 Actions and thresholds

| Condition | Action | Mechanism |
|---|---|---|
| any signal fires | `drift` event; no message | |
| `drift_index_w20 ≥ 0.25` or `repeat_identical_output` | **warn**: ≤ 80-token message naming the signal and the evidence seqs | PostToolUse feedback / AfterTool reason / stderr |
| `drift_index_w20 ≥ 0.40`, or `oscillation` twice on one path, or `context_no_progress` | **suggest restart**: message names the last `checkpoint` (turn, snapshot id, open gates) and the command `saga trace fork <session> --at <turn>`; the restart-monitor result (+8.8 pp among intervened runs, doc 03 §2.2) is cited in the docs, not in the message | same |
| `repeat_identical_output ≥ 5`, `edit_same_hunk ≥ 5`, `tool_error_streak ≥ 8`, or any wall stall with `watchdog.hard = true` | **block**: deny the next tool call and block Stop-equivalent with `saga trace: <signal>; acknowledge with saga trace ack <session>` | PreToolUse deny + Stop block; the block releases on ack or after `max_blocks` (gate-spec §6, default 6) |
| wall stall, `watchdog.hard = false` (default) | warn to the terminal and write the event; never kill the harness | doc 09 §2: no "no answer means yes"; killing is the user's decision |

All messages count against the gate-spec §9 injected cap.

### 5.4 False-positive controls

| Control | Rule |
|---|---|
| Output-aware repeat | Identical command with different `result_hash` (a test run that now fails differently, a `git status` after an edit) does not count toward `repeat_identical_output`; it still counts toward `repeat`, which only warns at the index threshold. This is the failure of gemini-cli's detector (#5761, #8237). |
| Idempotent allow-list | `[trace.watchdog] idempotent = ["git status", "ls", "pwd", …]` and per-repo additions are exempt from `repeat`; polling commands (`sleep`, `tail -f`, `kubectl get … -w`) exempt by default. |
| Long-running tool exemption | `wall_stall_tool` does not fire while a `tool_call` has no `tool_result` yet (a legitimate long build). |
| Streak reset | Any successful `edit` or a `gate` state change resets all streak counters. |
| Cooling | After a warn, the same signal cannot warn again for 5 tool calls. |
| User ack | `saga trace ack <session> [--signal id] [--for-turns n]` suppresses a signal; the ack is a `drift` event with `action: "ack"`, so the bench can count acks. |
| Measured | Precision and recall on the labelled corpus (§11) are printed by `saga doctor`; a signal below 0.8 precision on the corpus ships warn-only regardless of config. |

---

## 6. Replay and fork

### 6.1 Replay

`saga trace replay <session|run-dir> [--strict] [--to <turn>]` re-executes the recorded `tool_call` sequence in a fresh worktree or container without calling the model, the bench-spec §8.3 procedure generalised to any session:

| Mode | On hash match | On mismatch |
|---|---|---|
| permissive (default) | serve the recorded `tool_result` from the blob store, mark `served: cache` | re-execute, record the new result as a `replay` divergence with both hashes, continue |
| strict | serve recorded | exit 5 at the first `result_hash` divergence, print the seq and a diff of the masked payloads |

Replay validates that the archive is complete and the environment still buildable. It cannot reproduce the model's choices (doc 05 §4.1); if a replay reaches a `model_call`, it serves the recorded assistant output by `call_key` and continues, which is "the log is the agent" (arXiv 2605.21997).

### 6.2 Deterministic and not

| Deterministic (tested) | Not deterministic (recorded) |
|---|---|
| Event hashes, `prev` chain, ledger rows and report from the same events and price table | Wall clock, latency, provider status codes |
| Tool results under `network = offline` and unchanged inputs | Tool results that read the network, the clock, or the environment |
| Checkpoint restore to a guard snapshot id | Anything after the next real `model_call` |
| Drift signal counts from a given event stream | |

### 6.3 Checkpoint format

Written at every turn end when guard is installed (the snapshot exists anyway, ADR 0006) and every 5 turns otherwise (a content-addressed overlay under `.saga/snap/` taken by trace itself).

```json
{"schema":"saga.checkpoint/1","session":"01J6Y…","turn":29,"seq":1204,
 "snapshot":{"id":"snap:2026-09-02T13:40:02Z:7c1a…","kind":"apfs|zfs|overlay","tree_hash":"sha256:…","untracked_included":true},
 "context_hash":"sha256:…","open_gates":["G2","G4"],"contract_hash":"sha256:…",
 "ledger_cum":{"usd":3.41,"tokens":{"input_fresh":…}},"pins":"sha256:…","state_block":"sha256:…"}
```

`snapshot.id` is the same identifier `saga undo` accepts; a checkpoint with a missing snapshot is `restorable: false` and never offered by the watchdog.

### 6.4 Fork

`saga trace fork <session> --at <turn> [--into <new-session>]` restores the working tree from the checkpoint's snapshot, copies events `1..seq` into a new session directory with hashes preserved and a `session resume` event whose `forked_from` names the source `context_hash`, and prints the import command for the target harness (§8). The model is then called by the harness, not by trace. Forks are how "restart from the last good checkpoint" (doc 09 §3.7) is executed and how bench tasks are seeded from real failures (bench-spec §2.5, `source.kind = "real-failure"`).

---

## 7. `saga doctor`

`saga doctor [--json] [--fix]` runs in under 5 s without network and prints one line per check:

| Check | Pass condition | Fail text (fixed) |
|---|---|---|
| binary | `saga --version` matches the installed adapters' recorded version | `adapter built for 0.2.x, binary is 0.3.0: rerun saga install` |
| hooks registered | each adapter event is present in the harness settings file and its script hash matches the manifest | `Stop hook missing in .claude/settings.local.json` |
| hooks fire | a synthetic tool call through the harness in `-p` mode produces the expected `tool_call` event within 10 s | `PreToolUse registered but never fired (anthropics/skills #556 class: check matcher syntax)` |
| gate fixture | `saga gate check` fails on the shipped known-bad fixture | `gate passed on the known-bad fixture: enforcement is not active` |
| usage source | last 5 sessions have `usage.source ≠ estimated` | `no usage source: transcript path not readable; see §9.3` |
| pins | `pins/current.json` present; lists changed keys since the previous session | `harness version changed 2.1.188 → 2.1.190; canary pending` |
| cache | median `cache_hit_ratio` over the last 5 sessions and the inferred TTL | `cache hit 0.41 (baseline 0.89); ttl observed 5m, pinned 1h` |
| drift | count of `drift` events by signal over 7 days; precision of each signal on the corpus | |
| budget | current session and rolling 7-day spend vs configured budgets | |
| canary | last verdict, age, baseline n | `no canary run in 9 days` |
| retention | size of `.saga/trace/`, oldest session, next prune | |
| known incidents | the installed harness version and observed pins against the incident checklist shipped with the release (from doc 06 A.1 to A.3 and doc 07 §8: #46829 TTL, #46917 cache write, #42796 effort default, #91514 cache re-write after ToolSearch, gemini #28859 fallback, codex #30364 quantisation), each with the observable signature trace would show | `matches #46829 signature` |
| uninstall proof | `saga uninstall --dry-run` lists exactly the files `saga install` wrote and nothing else | |

`--json` emits `saga.doctor/1`:

```json
{"schema":"saga.doctor/1","ts":"…","ok":false,
 "checks":[{"id":"hooks_fire","ok":false,"detail":"PreToolUse registered but never fired","fix":"…"}, …],
 "pins":{…},"cache":{"hit_ratio_5s":0.41,"ttl_observed":"5m","ttl_pinned":"1h"},
 "drift_7d":{"repeat":4,"oscillation":1,"wall_stall_output":2},
 "budget":{"session_usd":6.12,"session_limit":25.0,"week_usd":88.4},
 "canary":{"verdict":"cache_regression","run_id":"…","age_h":31,"baseline_n":200},
 "incidents":[{"id":"claude-code#46829","matched":true,"signature":"ttl_observed<pinned"}],
 "precision":{"repeat_identical_output":0.97,"context_no_progress":0.82}}
```

Exit codes: 0 all pass, 1 any check failed, 4 harness not found. `--fix` re-registers hooks and rebuilds `index.sqlite`; it never edits budgets or pins.

---

## 8. Portability

### 8.1 Export

`saga trace export <session> [--to <turn>] [--incident] --out <file>` writes `saga.trace.bundle/1`: a tar of `events.*.jsonl`, referenced blobs, checkpoints, the pin history, the price tables used, and `MANIFEST.json` with hashes. `--incident` additionally strips every blob to its hash and keeps only `model_call`, `session`, `budget`, `canary` and `drift` events, so a TTL or cache-write observation can be attached to an issue without code or prompts. Every bundle is masked (§2.6); the export refuses if any event has `masked: unknown`.

### 8.2 Import contract

Each harness adapter implements `import {bundle, at_turn, workspace} -> {native_session_ref | context_file, fidelity}`. Native resume of another harness's transcript is not possible in any surveyed harness (their JSONL formats are private and unversioned), so the guaranteed path is **context reconstruction**: the adapter writes a structured file (goals from the contract, decisions and constraints from `mem` records if present, open gates, files touched with current hashes, last test status, last 3 turn summaries built from hashes and tool names, never from model prose) and the harness is started with it as the first user message. `fidelity` is `native` only when the adapter proves a same-harness resume (`claude --resume`, Codex rollout file) reproduces the same `context_hash`; otherwise `reconstructed`. The working tree comes from the checkpoint snapshot, which is the part that is exact.

---

## 9. Surfaces

### 9.1 CLI

```
saga trace init                                   # gitignore .saga/trace, write config defaults
saga trace tail    [session] [--type t,..] [--follow] [--json]
saga trace ledger  [session] [--by turn|component|tool|agent] [--reconcile <day>] [--json]
saga trace budget  [--session-usd x] [--task-usd x] [--raise <scope>] [--show]
saga trace pin     [--set effort=<v>] [--baseline]     # print pins and advice; --baseline starts a new canary baseline
saga trace canary  run [--burst] | status | --watch [--interval s]
saga trace replay  <session|run-dir> [--strict] [--to <turn>]
saga trace fork    <session> --at <turn> [--into <id>]
saga trace ack     <session> [--signal id] [--for-turns n]
saga trace export  <session> [--to turn] [--incident] --out <file>
saga trace import  <bundle> --harness <name> [--at turn] --workspace <dir>
saga trace verify  <session>                      # hash chain, blob presence, schema
saga trace rebuild [session]                      # ledger.jsonl, index.sqlite from events
saga trace prune   [--older-than 30d] [--dry-run]
saga trace prices  show | use <file>
saga doctor        [--json] [--fix]
```

Exit codes follow bench-spec §9.2: 0 ok, 1 finding (budget exhausted, canary non-pass, doctor fail), 2 usage or schema, 3 budget refused, 4 environment, 5 integrity (chain or hash mismatch, strict replay divergence).

### 9.2 MCP tools (read-only)

`saga trace serve` exposes four tools over stdio or a Unix socket, never TCP (index-spec convention): `trace_ledger(session?, by?)`, `trace_budget()`, `trace_tail(n, types?)`, `trace_doctor()`. Results are capped at 2 KiB each; none mutates state; there is no MCP tool for `ack`, `budget --raise`, `fork` or `prune`, so the agent cannot silence its own watchdog or spend past its budget. Each result is itself a `tool_result` in the trace with `component: "trace"`.

### 9.3 Adapter hook points per harness

Verification status follows gate-spec §6: hook envelopes were verified against vendor docs on 2026-09-02; the usage and transcript columns below are the fields the bench-spec §6.1 adapters already consume, and any cell marked *probe* is confirmed by the adapter conformance suite at install, never assumed.

| Observation | Claude Code | Codex CLI | Gemini CLI / successor | `bare` | Proxy (any) |
|---|---|---|---|---|---|
| Tool call, args, result | `PreToolUse`/`PostToolUse` stdin (`tool_name`, `tool_input`, `tool_response`) | `PreToolUse`/`PostToolUse` (`turn_id` present) | `BeforeTool`/`AfterTool` (`tool_response` on After) | in-process | not visible (tools run client-side) |
| Turn boundaries | `UserPromptSubmit`, `Stop` | `UserPromptSubmit`, `Stop` | `AfterAgent` (`prompt`, `prompt_response`) | in-process | request boundaries |
| Usage per call | `transcript_path` JSONL: per assistant message `usage` with `input_tokens`, `cache_creation_input_tokens` (5m/1h split *probe*), `cache_read_input_tokens`, `output_tokens`; `-p --output-format stream-json` usage events | `codex exec --json` usage; rollout JSONL under `~/.codex/sessions` *probe* | `-p --output-format json` summary; per-call *probe* | provider response | provider response |
| Model served | transcript message `model` field | *probe* | *probe*; fallback banner in log | response | response |
| Effort / thinking | settings hash; per-call *probe* | config `model_reasoning_effort` | settings | request | request |
| System prompt, tool defs | not exposed (`null`, reason) | not exposed | not exposed | exact | exact |
| Request id, fingerprint | not exposed | not exposed | not exposed | headers | headers |
| Compaction | `PreCompact` (+`PostCompact` *probe*) | `PreCompact`, `PostCompact` | *probe*; else inferred from a context drop > 50% | n/a | inferred |
| Subagents | `SubagentStop`; subagent transcripts *probe* | `SubagentStop` | *probe* | n/a | separate request streams |
| Feedback channel for warn | PostToolUse `reason`, Stop `additionalContext` | PostToolUse `reason` | AfterTool `reason` (replaces the tool result: emit only on a finding, gate-spec §6.2) | stderr | none |
| Block channel | PreToolUse `permissionDecision: deny`, Stop `decision: block` | same | BeforeTool `deny`, AfterAgent `block` | in-process | none |

Trace adapters, unlike gate adapters (gate-spec §6), do read `transcript_path`, because usage lives there; they open it read-only, never write to it, and copy nothing but the usage, model and timing fields plus hashes of content.

**Proxy capture** (`saga trace proxy --port n`, opt-in) sets the harness's base URL to a local listener that forwards unchanged, records request and response metadata, hashes bodies after masking, and stores bodies only with `--store-bodies`. It is the #46917 method as a command. It is privacy-sensitive (ADR 0007) and prints a banner on every start; it never terminates TLS to any host except the configured provider.

### 9.4 CI mode

`saga trace ci --run-dir <dir> [--budget-usd x] [--fail-on cost_regression,drift]` reads a bench run or a headless session, emits the ledger report and doctor JSON as artefacts, and exits 1 on a breached budget or a listed finding. It is the `saga-gate` workflow pattern (gate-spec §6.4) with read-only permissions and no secrets.

---

## 10. Privacy and retention

| Rule | Detail |
|---|---|
| Local only | No network in `tail`, `ledger`, `pin`, `replay`, `fork`, `doctor`, `export`. Network exists in `ledger --reconcile` (provider usage endpoint, admin key from the environment, never stored), `canary run` (model API, like any agent run) and `proxy` (forwarding). Each prints the host it will contact before contacting it. |
| Masked before write | §2.6; hashes are computed on masked bytes; `masked_count` is auditable. Retention on the provider side is unchanged by any of this: covered models retain 30 days even under zero-data-retention since 9 Jun 2026 (doc 06 C.1). |
| Repo-relative paths | Absolute paths outside the repo root are stored as `«outside-repo»/<basename-hash>` (gate-spec §4.4 rule). |
| Gitignored | `saga trace init` adds `.saga/trace/` to `.gitignore`; `saga trace verify` warns if any trace file is tracked. |
| Retention defaults | events and checkpoints 30 days or 2 GiB per repo, whichever first; blobs 14 days; `ledger.jsonl` and `index.sqlite` summaries 365 days; canary archives 365 days; price tables forever (they are small and every old row needs its table). `saga trace prune` runs at session start when either bound is exceeded and logs what it removed. |
| What the user must delete manually | the harness's own transcripts (`~/.claude/projects/…`, `~/.codex/sessions/…`, `~/.gemini/tmp/…`), which trace reads but does not own; proxy body captures under `.saga/trace/proxy/` when `--store-bodies` was used (never pruned automatically, and `doctor` lists their size); exported bundles; guard snapshots under `.saga/snap/` (owned by guard, ADR 0006). `saga uninstall` removes `.saga/trace/` only when asked with `--purge` and prints the list above either way. |

---

## 11. Test plan and bench ablation

### 11.1 Tests for the layer

| Suite | Content | Pass bar |
|---|---|---|
| Schema and chain | 500 synthetic sessions; every event validated; one byte flipped in a random event | validation exact; `verify` exits 5 naming the seq |
| Determinism | `rebuild` and `ledger --json` on the same session on two hosts | byte-identical `ledger.jsonl`, report JSON, `index.sqlite` dump |
| Normaliser conformance | golden native logs per harness and version (Claude Code transcript and stream-json, Codex exec JSON and rollout, Gemini JSON) → golden `events.jsonl` | byte-identical; every `null` has a `_reason` |
| Ledger arithmetic | fixture usage objects × price tables, including 5m/1h split, long-context tier (Gemini > 200K), unpriced model, mixed tables | exact to 1e-6 USD; `null` never becomes 0 |
| Reconciliation | mocked provider usage endpoints with known totals; real endpoints in the nightly job on the bench's own keys | mocked: 0% error; real: `|error_pct| ≤ 2` over ≥ 20 sessions and 3 days (the M0 exit criterion); the nightly job publishes the number |
| Attribution calibration | `bare` adapter with exact per-region token counts vs the estimator | estimator within ±10% per component on 30 sessions; the ratio is printed, not hidden |
| TTL inference | simulated call sequences with TTL 5m and 1h and idle gaps 1 to 90 min | bracket contains the true TTL in 100% of cases; incident fires only when the bracket excludes the pin |
| Masking | positive-control corpus (200 secrets across 12 formats) and negative corpus (code that looks like secrets) | 100% masked / 0% masked; a novel-format miss is added to the corpus, never hidden |
| Drift precision and recall | labelled corpus: 200 traces from bench archives labelled by two annotators (loop / stall / legitimate repetition / clean), plus scripted traces for each signal and each false-positive control in §5.4 | recall ≥ 0.9 on `repeat_identical_output`, `edit_fail_streak`, `oscillation`; precision ≥ 0.8 on every signal that may block; per-signal numbers printed by `doctor` |
| Canary detection latency | mocked provider injecting (a) TTL 1h → 5m, (b) +20,000 `cache_write` per call, (c) reasoning values fixed to {516, 1034, 1552}, (d) pass rate −10 pp, (e) −5 pp, at a random day | (a) to (c) detected within 1 daily cycle; (d) CUSUM median detection ≤ 12 cycles with in-control ARL ≥ 100 over 1,000 simulated years; (e) reported as "below detection floor at n" rather than missed silently |
| Replay | bench fake-harness archive (bench-spec §10.2) replayed permissive and strict; one tool result altered | strict exits 5 at the altered seq; permissive records one divergence |
| Overhead | 10,000 tool calls through the hook path on a 50k-file repo | p50 ≤ 5 ms, p99 ≤ 20 ms added per hook invocation; ≤ 1% of run wall time on the M0 task set; measured with and without checkpointing |
| Doctor | fixtures: unregistered hook, hook registered but non-firing matcher, gate passing on the bad fixture, stale adapter | each check fails with its fixed text; `--fix` repairs the first and fourth only |
| Windows | the above on Windows runners from M1 (doc 09 §5) | green; path handling of `nul`, CRLF and drive roots covered |

### 11.2 Bench ablation

Trace is measurement, so its ablation measures its own cost and its detection value, not a pass-rate effect:

| Question | Design | Metric | Pre-registered bound |
|---|---|---|---|
| Overhead | M0 task set, `bare` and `claude-code`, arm A without trace hooks, arm B with trace (no watchdog actions) | Δ wall time, Δ tokens (trace injects nothing in B) | wall ≤ +1%; tokens Δ inside ±0 with CI |
| Watchdog value | arm B (record only) vs arm C (warn and suggest restart) | pass@1, pass^k, tokens per solved task, false-done | reported; the +8.8 pp among intervened runs (doc 03 §2.2) is the prior, not the claim; equivalence bound for "does not hurt" ε = 3 pp |
| Budget hard stop | arm C with `session_usd = 3 × cost_hint` vs unlimited | cost per solved task, fraction of runs stopped, pass@1 of stopped runs had they continued (from the unlimited arm's paired run) | reported |
| Ledger vs bill | every publish-tier bench run reconciles | `error_pct` | ≤ 2% or the run's cost column is marked `unreconciled` |

Every number produced here goes into the bench report's negative-results section when it is null, per bench-spec §7.2.

---

## 12. Open items carried to the experimental register

1. The between-run standard deviation of `cache_hit_ratio` and tokens per task, which sets the cost-metric power in §4.4, is a prior (0.05 to 0.08) until the first 200-run baseline exists.
2. Whether Claude Code's transcript splits `cache_creation` into 5m and 1h buckets on every version; if not, the split falls back to the pinned TTL and the ledger says so.
3. The attribution estimator's calibration on harnesses whose system prompt is not observable; the `bare` adapter is the only exact reference.
4. Watchdog thresholds are defaults from the issue evidence, not fitted values; §11.1's labelled corpus fits them, and the fitted values ship with their precision and recall.
5. The import contract's `native` fidelity for same-harness resume depends on private transcript formats and is re-probed on every harness version by `doctor`.
