# Cross-spec contracts

*v0.2, 2026-09-03 (v0.1 earlier the same day). The contracts every layer spec depends on, in one place. Where a layer spec disagrees with this file, this file wins and the spec has a bug (log it in `REVIEW-LOG.md`). Harness facts: **verified** = quoted from vendor docs or default-branch source in `harness-facts.md` (sprint of 2026-09-03; Claude Code 2.1.258, Codex rust-v0.152.1, Gemini CLI v0.58.0, OpenCode v1.18.26); **documented** = in vendor docs, not exercised; **probe** = confirmed per harness version by the install probe (`saga doctor`, trace-spec §7) and never assumed. Fact ids (C7, X11, G4, ...) refer to `harness-facts.md`.*

---

## 1. Hook composition

One binding per harness event: `saga hook <harness> <event>`, written by `saga install --harness <h>`. No layer installs a hook of its own; `saga <layer> install` adds the layer to `.saga/manifest.json` and re-runs the installer. The entry runs the installed layers in this order and merges their outputs into the one envelope the harness accepts.

| Event (Claude Code name) | Order inside the entry | Short-circuit |
|---|---|---|
| `PreToolUse` | trace (record) → guard → route → mem → shape → gate → trace (record decision) | a `deny` from guard or gate ends the chain |
| `PostToolUse` | trace → shape → gate → guard (deps) → mem (capture) → index (`update`, optional) → trace (watchdog, budget) | none. Every harness can replace the result (Claude Code `updatedToolOutput`, shape-validated; Codex `decision: block`; Gemini `deny`), and Codex and Gemini hide the original when they do (C7, X11, G4), so the entry emits a replacement only when a layer changed the bytes and otherwise feedback via `additionalContext` |
| `Stop` / `AfterAgent` | trace (turn end, checkpoint) → gate (`check --status`) → trace (claim verdict, trace-spec §5.9; budget hard stop; watchdog block) → mem (pending line) | `block` wins; gate's reason first, trace's claim line second |
| `UserPromptSubmit` / `BeforeAgent` | trace (turn) → guard (mask scan) → mem (statement capture; Gemini: state re-inject) | guard `block` ends the chain |
| `PreCompact` / `PreCompress` | mem (`state --write`) → trace | never blocks; exit 0 always |
| `SessionStart` | trace (session, pins) → mem (state re-inject on `source ∈ {compact, resume, fork}`) | `PostCompact` is bound for trace only: it has no context channel on Claude Code (C16) or Codex (X16) and carries `compact_summary` |
| `SubagentStart` / `SubagentStop` | trace | |

### 1.1 Merge rules

| Field | Rule |
|---|---|
| decision | `deny` > `block` > `ask` > `allow`; the first denying layer's reason leads |
| `updatedInput` | field-wise union over the full original `tool_input`, because Claude Code replaces the entire input object with what the hook returns (C3) and Codex requires `permissionDecision: "allow"` beside it (X8); fields: `command` (the one rewrite below), `prompt` (mem, fallback only; the preamble's primary channel is `SubagentStart`, C17), `model` (route), `content` (guard); two layers on one field is a composition bug, exit 2 |
| Bash rewrite | exactly one: `saga shape run --mask -- <cmd>` when shape is installed (its masker and unmask-in are guard's), else `saga guard exec --mask -- <cmd>` |
| `additionalContext` | concatenated in layer order, each block prefixed `saga <layer>:` |
| `reason` | concatenated in layer order under the gate-spec §9 per-event ceilings (pre-tool 150, post-tool 200, Stop 400 est. tokens); on Stop, trace's claim line (≤ 120, trace-spec §5.9) follows gate's text and is charged to trace's share |
| exit code | §4 precedence over the layers' codes; exit-2-with-stderr only when the harness rejects JSON |
| latency | whole entry p95 ≤ 300 ms with a snapshot, ≤ 20 ms without (guard-spec §11.4, trace-spec §11.1). Harness timeouts: Claude Code 600 s (30 s on `UserPromptSubmit`), Codex 600 s, Gemini 60 s (C22, X5, G12). A timed-out `PreToolUse` entry **allows** on Claude Code (C23) and any non-JSON stdout **allows** on Gemini (G11), so the entry runs its own deadline (`hook.deadline_ms`, default 5,000) and emits `deny` with reason `saga: deadline` when a deciding layer overruns; stdout is the JSON object and nothing else |
| output size | Claude Code caps hook strings at 10,000 characters and spills to a file (C24); Codex caps model-visible hook output at about 2,500 tokens and spills (X20). The §7.3 ceilings are below both |

### 1.2 Per-harness event matrix

| Capability | Claude Code | Codex CLI | Gemini CLI |
|---|---|---|---|
| Pre-tool block | `PreToolUse` `permissionDecision: deny` (verified, C2) | `PreToolUse` `permissionDecision: deny` or `decision: block` (verified, X7) | `BeforeTool` `deny` (verified, G2) |
| Pre-tool ask | `permissionDecision: ask` (verified, C2) | **none**: `ask` is parsed but unsupported and the tool call proceeds (verified, X10), so `ask` collapses to `deny` naming the allow rule; `PermissionRequest` fires only when Codex would prompt anyway | undocumented; present in source (G3); collapses to `deny` naming the allow rule until the install probe passes |
| Pre-tool input rewrite | `updatedInput`, replaces the whole object (verified, C3) | `updatedInput` with `permissionDecision: allow`; `command` must be a string for `Bash` and `apply_patch` (verified, X8) | `tool_input` merge (verified, G2) |
| Pre-tool add context | `additionalContext`, lands next to the tool result (verified, C4, C5) | `additionalContext` (verified, X9) | none (verified, G2) |
| Post-tool replace result | `updatedToolOutput`, must match the tool's output shape; `Bash` shape documented, `Read`/`Grep`/`Glob` shapes probe (C7) | `decision: block` or `continue: false` replaces the result with the reason (verified, X11) | `deny` replaces the result with `reason` (verified, G4) |
| Post-tool add context | `additionalContext`; original result stays visible under `decision: block` (verified, C6, C8); `PostToolBatch` `additionalContext` once per batch (documented, C9) | `additionalContext` (verified, X12) | `additionalContext` appended (verified, G4) |
| Stop block | `Stop` `decision: block`, `reason` required; harness cap 8 consecutive (verified, C10, C11) | `Stop` `decision: block` creates a continuation prompt (verified, X13); no harness cap, verified in source (X14); `max_blocks` is the only cap | `AfterAgent` `deny` retries with `reason` as the new prompt (verified, G5); no dedicated cap, loop bound 100 turns (G6) |
| Prompt hook | `UserPromptSubmit` block erases the prompt; no rewrite; `reason` goes to the user, `additionalContext` to the model (verified, C26) | `UserPromptSubmit` block and `additionalContext` (verified, X24) | `BeforeAgent` turn-scoped context; `deny` discards the message (verified, G7) |
| Pre-compaction | `PreCompact`: no context channel; exit 2 or `decision: block` blocks compaction (verified, C15) | `PreCompact`: common fields only; `continue: false` stops compaction (verified, X16) | `PreCompress` advisory and asynchronous (verified, G8) |
| Post-compaction context | `SessionStart` `source: compact` (verified, C13, C14); `PostCompact` has no context channel (verified, C16) | `SessionStart` `source: compact`, delivered to the immediate continuation mid-turn (verified, X15) | none; `BeforeAgent` next turn (verified, G9) |
| Sub-agent context | `SubagentStart` `additionalContext` (verified, C17); fallback `PreToolUse` on `Agent` `updatedInput.prompt` | `SubagentStart` `additionalContext` (verified, X17); `spawn_agent` matches `Agent` on `PreToolUse` (X18); no per-sub-agent model (codex #31814, not re-checked) | `BeforeTool` on the sub-agent's own tool name, `tool_input` merge (verified, G10) |
| Sub-agent model | `PreToolUse` on `Agent` `updatedInput.model`; `model` is a documented input (C19); hook-written value honoured: probe; `resolvedModel` in `PostToolUse` `tool_response` is the check | none | `BeforeModel` `llm_request.model` per call (documented, G15); not default |
| Final message | `last_assistant_message` on `Stop` and `SubagentStop`; `transcript_path` may lag (verified, C12) | `last_assistant_message` (verified, X6) | `AfterAgent` `prompt_response` (verified, G5) |
| Transcript read | trace only, read-only | trace only; format "isn't a stable interface" (X6) | trace only |
| Hooks vs sandbox | outside the sandbox (verified, C27); the rewritten Bash command runs inside it | outside the sandbox with a cleared environment, verified in source (X21); entry must be an absolute path | n/a |
| Install trust | none | project `.codex/` layer must be trusted and each hook trusted via `/hooks` by hash (verified, X3); `saga doctor` fires a canary | project hooks fingerprinted; a changed command re-prompts (verified, G14) |
| Coverage caveat | `@`-referenced files bypass `PreToolUse` `Read` | hosted tools not hooked; "not a complete enforcement boundary" (X19) | |

Other harnesses (Cursor, OpenCode, Cline, Kilo, CI) are CLI plus MCP only until M6. OpenCode's plugin surface (in-process TypeScript: `tool.execute.before` rewrites args or throws to block, `tool.execute.after` replaces `output.output`, `permission.ask`, `experimental.session.compacting` injects pre-compaction context; no blocking stop equivalent) is recorded in `harness-facts.md` §4 for the M6 adapter.

---

## 2. `.saga/` layout

`saga init` writes `.saga/.gitignore` (`trace/ index/ shape/ snap/ observed/ audit.jsonl`) and a `.saga/config.toml` skeleton; the repository's own `.gitignore` is never edited.

| Path | Owner | Committed | Notes |
|---|---|---|---|
| `.saga/config.toml` | all | yes | one table per layer: `[gate]`, `[trace.budget]`, `[trace.watchdog]`, `[trace.claims]`, `[index]`, `[shape]`, `[mem]`; `[gate]` and `[trace.*]` are read from gate's `BASE:` via `git show`, the rest from the working tree |
| `.saga/policy.toml` | guard | yes | `saga.guard.policy/1`; honoured after `saga guard policy trust` |
| `.saga/route.toml` | route | yes | `saga.route.policy/1`; honoured after `saga route policy trust` |
| `.saga/manifest.json` | install | yes | installed layers; every auto-executing entry (guard-spec §7) |
| `.saga/contract.md`, `request.md`, `evidence/`, `red/` | gate | yes | records are a cache; CI `reverify` recomputes |
| `.saga/mem/` | mem | yes | records, pending, tombstones |
| `.saga/observed/` | all | no | `session-<id>.json` (turn, per-layer token counters, block counter), `mem-*.json`, `state-<session>.md` |
| `.saga/trace/` | trace | no | sessions, blobs, checkpoints, pins, canary, prices, `index.sqlite` |
| `.saga/snap/`, `refs/saga/snap/` | snapshot (§5) | no | never pushed |
| `.saga/index/`, `.saga/shape/`, `.saga/audit.jsonl` | index, shape, guard | no | |
| `~/.saga/` | per user | never | `approved/`, `policy.toml`, `route.toml`, `vault/`, `deps-cache/`, `mem/<repo-id>/`, `audit.jsonl` |

Hostile-shape rules (symlink, hard link, FIFO, outside the canonical repo root, not owner-private) are gate-spec §8 and apply to every path above; violation is exit 6.

---

## 3. Identity and encoding

| Item | Rule |
|---|---|
| Session id | harness `session_id` when present, else a ULID (trace-spec §2.1) |
| Turn | one counter in `.saga/observed/session-<id>.json`, advanced by trace on the user-prompt event; harness `turn_id` recorded alongside |
| Hashes | `sha256:<64 hex>` for every cross-layer field; `blake3:` only as opaque ids inside index and shape's cache; `tree:<oid>` for git tree ids (§5) |
| Canonical JSON | sorted keys, no insignificant whitespace, UTF-8 |
| Masking | `SAGA_MASK_<TYPE>_<8 hex>` (guard-spec §4.3); every hash is computed after masking |
| Token estimate | `ceil(utf8_bytes / 4)`; the bench reports the ratio to harness counters |
| Paths | repo-relative, `/` separated; absolute paths outside the repo become `«outside-repo»/<basename-hash>` |
| Text fields | control-, line-separator- and bidi-stripped; data, never instructions |

---

## 4. Exit codes

One table for every subcommand; `saga shape run` alone propagates the child's code.

| Code | Meaning | Examples |
|---|---|---|
| 0 | ok, allow, all met | |
| 1 | finding | unmet gate, ask, stale record, unverified claim, canary non-pass, runtime budget exhausted, empty result, doctor check failed |
| 2 | usage, parse or schema failure (fail closed) | invalid contract, unknown major, repo policy touching a fixed table |
| 3 | refusal | hard deny, unwaived guard violation, bench estimate over budget, masked write rejected |
| 4 | approval or trust required | gate approval missing, policy hash untrusted, snapshot over budget with `on_budget = "ask"` |
| 5 | integrity or proof missing | red proof absent, contradicted claim (the record refutes the final message), hash chain mismatch, strict replay divergence, manifest hash drift, stale index under `--require-fresh`, evicted log |
| 6 | environment refusal | hostile file shape, unreadable store, harness or runtime missing, vault unavailable |
| 7 | contamination (bench only) | leak scan, canary GUID, post-cutoff check |

Precedence when several apply: **6, 7, 2, 3, 4, 5, 1**.

---

## 5. Snapshot mechanism

One primitive, `saga snapshot`, implemented in guard (guard-spec §3); no layer keeps a second copy of the tree.

| Item | Contract |
|---|---|
| Take | `saga snapshot take --reason guard\|trace\|shape\|gate\|user` → `{id, tree_hash, kind, session, turn, taken}`; an unchanged tree returns the previous snapshot with `taken = false` |
| Id | `snap:<session>:<turn>:<tree_hash[0:12]>` |
| `tree_hash` | `tree:<oid>`: git tree id over tracked plus untracked-not-ignored plus `snapshot.include_ignored`, via a temporary index; refs under `refs/saga/snap/<session>/<turn>` |
| Modes | `git-tree` (default, every filesystem including APFS and NTFS), `zfs`, `btrfs`, `reflink` for large ignored binaries; never an APFS volume snapshot |
| Read, restore | `saga snapshot show <id> [-- <path>]`; `saga undo <id>` (undo snapshots first) |
| guard | takes it in `PreToolUse` before every mutating tool |
| trace | checkpoint at every turn end references the latest id and `tree_hash` (trace-spec §6.3) |
| shape | edit pre-image is `snapshot show <id> -- <path>` (shape-spec §4.1) |
| gate | evidence `worktree_hash` is the `tree_hash`; `RED: mutation` materialises the snapshot into a scratch worktree (gate-spec §3.2, §4.3) |
| Retention | `[snapshot] retain = {turns, days, bytes}`; `saga snapshot gc`; refs never pushed |

---

## 6. Trace event catalogue (`saga.trace/1`, amended)

Envelope per trace-spec §2.1; trace-spec §2.2 is normative for bodies.

| Type | Body |
|---|---|
| `session` | phase, pins (`saga.trace.pins/1`), harness, config hash, changed pin keys |
| `turn` | phase, prompt hash and bytes, final message hash and ref, `claimed_done` (trace-spec §5.6) |
| `model_call` | requested and served model, request id, fingerprint, effort, usage (§7.2), `call_key`, hashes, latency, status, attribution |
| `tool_call` | tool, args hash and ref, `component` (§7.1), cwd, `index_version` |
| `tool_result` | for_seq, exit, error, result hash, bytes, ref, truncated, wall, `served`, optional `shaped` (shape-spec §8) |
| `edit` | path, before and after `sha256:`, hunks, added, removed, by_tool, in_scope |
| `gate` | kind (check, guard_diff, red, stop, claim), ids, states, decision, progress hash; for `claim` (written by trace, last blocking step of the Stop chain): `for_turn`, `claims[]`, `verdict` (verified, unverified, contradicted), `counts`, `tree_hash`, `mode`, `decision`, `exit` (trace-spec §5.9) |
| `guard` | kind (classify, deny, snapshot, mask, deps, mcp, undo), segments, class, snapshot id, masked count, decision |
| `mem_inject` | record ids, bytes, tokens, trigger |
| `compaction` | phase, context tokens before and after, state block hash, trigger |
| `subagent` | phase, ids, requested model, preamble hash, cumulative usage |
| `budget` | scope, metric, limit, value, action, ack |
| `drift` | signal id (trace-spec §5.1, shared with bench-spec §5.9, underscored), window, index, action, evidence |
| `checkpoint` | checkpoint id, snapshot id (§5), context hash, open gates, cumulative ledger |
| `canary` | run id, task set hash, baseline hash, metrics, verdict (closed set, trace-spec §4.4) |
| `route_decision` | `saga.route.plan/1`, outcome (applied, unapplied, refused, user_answered), option, ack |

---

## 7. Ledger attribution and token ceilings

### 7.1 Attribution keys

Closed set, identical in `tool_call.component`, `model_call.attribution` and `ledger.attribution`; an uninstalled layer's share is `0.0`, never absent: `harness`, `user`, `tool_results`, `index`, `mem.state`, `mem.targeted`, `mem.preamble`, `mem.query`, `shape`, `gate`, `guard`, `trace`, `route`, `mcp:<server>`. The ledger row (`saga.trace.ledger/1`) also carries `tool_output_bytes_turn`, `tool_output_bytes_raw_turn` and `resident_tokens.index`.

### 7.2 Usage object

`{input_fresh, cache_read, cache_write_5m, cache_write_1h, output, reasoning}` (trace-spec §3.1) in trace events, ledger rows and bench `run.json` alike; `null` carries a `_reason`.

### 7.3 Per-session injected budget

Est. tokens, counted per layer in `.saga/observed/session-<id>.json`. At its share a layer keeps its decision and collapses its message to a fixed one-liner; the bench treats a breach as a failed run of that layer.

| Layer | Per-event ceiling | Per-session share | Spec |
|---|---|---|---|
| gate | pre-tool 150, post-tool 200, Stop 400 | 1,000 | gate-spec §9 |
| guard | reason 150 | 400 | guard-spec §2.6 |
| shape | feedback 120 inside the post-tool 200 | 600 | shape-spec §4.5 |
| trace | budget line 60, watchdog warn 80, claim block line 120 (only on `block`) | 400 | trace-spec §3.6, §5.3, §5.9 |
| route | budget line 60 | 200 | route-spec §5.3 |
| mem (targeted) | 200 per injection, ≤ 3 records | 1,200 | mem-spec §5.4 |
| **Injected total** | | **3,800** | |
| mem state block | 600 per compaction or resume | per event | mem-spec §4.2 |
| mem preamble | 300 per sub-agent | per sub-agent | mem-spec §4.6 |
| index results, shape results and echoes | index-spec §5.1 ceilings; echo ≤ 1,500 | tool output, attributed to the layer | index-spec §5.1, shape-spec §4.3 |
| Prefix (cached) | map ≤ 1,000 (`full`) or 500 (`lite`); all MCP tool descriptions ≤ 1,000 (index 550, mem 240, rest ≤ 210) | ≤ 2,000, fixed per session | index-spec §5.6, §6.1 |

---

## 8. Agent-forbidden commands

Denied to the agent's shell by guard D11 (post-expansion) when guard is installed, by string match in the composed hook otherwise; never exposed over MCP:

`saga gate approve|attest|check --approve` · `saga trace budget --raise|ack|pin --set|prices use|prune` · `saga route policy trust|validate --write|budget --raise` · `saga mem confirm|review|prune` · `saga guard policy trust|set` · `saga snapshot gc|prune` · `saga install` · `saga uninstall` · edits to `.saga/policy.toml`, `.saga/route.toml`, `.saga/manifest.json`, `.saga/.gitignore`, hook settings, `refs/saga/`.

---

## 9. Install and probe order

| Step | Layers | Probe (`saga doctor`) |
|---|---|---|
| 1 | `saga init` | `.saga/.gitignore` and config skeleton present |
| 2 | trace, doctor (M0) | hooks registered and fire within 10 s; usage source ≠ estimated; pins written |
| 3 | gate (M1) | known-bad fixture fails; Stop-equivalent bound or CI fallback named; `approve`/`attest` denied |
| 4 | guard (M1) | incident suite 0 escapes; `updatedInput` and `ask` support; snapshot mode and cost; vault reachable |
| 5 | mem state block (M1) | `PreCompact` exit 0; `SessionStart source=compact` re-injects once; `SubagentStart` `additionalContext` reaches the sub-agent |
| 6 | index (M2) | regime; `tools/list` byte-stable; map block hash |
| 7 | mem targeted (M3) | placement measured (`PreToolUse` vs `PostToolUse` vs `PostToolBatch`); both fields verified (C4, C8), the probe now picks the better-performing one |
| 8 | shape (M4) | Bash rewrite honoured; `updatedToolOutput` shape accepted for `Bash`; `Read`/`Grep`/`Glob` output shapes discovered from a fixture; harness result caps recorded (C28) |
| 9 | route (M5) | hook-written `Agent` `updatedInput.model` honoured (checked via `resolvedModel`, C19); effort knob pinned (`effort.level` in hook input, C30); served model observable |
| 10 | MCP gateway (M6) | MCPTox fixture set blocked |

Uninstall is the reverse; `saga uninstall --dry-run` lists exactly what `saga install` wrote.

---

## 10. Regime thresholds

| Gate | Rule | Spec |
|---|---|---|
| Index `off` | `source_lines < 20,000` and `source_files < 200`; no tools, no map | index-spec §1.3 |
| `lite` | otherwise, and `source_lines < 80,000`; map ≤ 500, embeddings off | |
| `full` | `source_lines ≥ 80,000` or `source_files ≥ 800`; map ≤ 1,000 | |
| Hysteresis | leave `off` at +10%, re-enter at −10% | |
| LSP overlay `auto` | on for route tiers `small` and `local`, off for `frontier` and `standard` | index-spec §1.3, route-spec §3.1 |
| Route classes | `scope_files ≤ 4` and `impact ≤ 2×` → mechanical; `≥ 5` or null impact → design; `unknown` rate ≤ 20% | route-spec §2.3, §2.4 |
| Bench claims | K ≥ 5 (10 for `publish`); noise floor 3.5 to 4.5 pp; `user` tier ≤ $20 | bench-spec §1.3, §4.4 |

All thresholds are priors, revised only from a bench manifest.

---

## 11. Schema naming and versioning

| Rule |
|---|
| Ids are `saga.<layer>.<thing>/<major>`; bare `saga.<layer>/<major>` only for a layer's primary record (`saga.trace/1`, `saga.doctor/1`). |
| An unknown major is exit 2 naming the file; additive fields within a major are ignored by older readers. |
| Body schemas live in `schema/<layer>/<major>/`; an unknown field is a validation failure. |
| Adding a field, enum value or event type is minor; renaming or removing is a new major. |

Registry: `saga.trace/1`, `saga.trace.ledger/1`, `saga.trace.report/1`, `saga.trace.prices/1`, `saga.trace.pins/1`, `saga.trace.checkpoint/1`, `saga.trace.bundle/1`, `saga.trace.claims/1`, `saga.doctor/1`; `saga.gate.status/1`, `saga.gate.evidence/1`, `saga.gate.red/1`, `saga.gate.approval/1`; `saga.guard.policy/1`, `saga.guard.decision/1`, `saga.guard.mask/1`; `saga.shape.parser/1`, `saga.shape.result/1`, `saga.shape.edit/1`, `saga.shape.window/1`, `saga.shape.cache/1`; `saga.mem.record/1`, `saga.mem.index/1`, `saga.mem.session/1`, `saga.mem.inject-request/1`, `saga.mem.inject/1`; `saga.route.policy/1`, `saga.route.plan/1`; `saga.index.status/1`; `saga.bench.task/1`, `saga.bench.run/1`, `saga.bench.harness/1`, `saga.bench.manifest/1`, `saga.bench.report/1`.
