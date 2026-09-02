# Cross-spec contracts

*v0.1, 2026-09-03. The contracts every layer spec depends on, in one place. Where a layer spec disagrees with this file, this file wins and the spec has a bug (log it in `REVIEW-LOG.md`). Harness facts: **verified** = checked against vendor docs on 2026-09-02 by the gate, mem and trace authors; **documented** = in vendor docs, not exercised; **probe** = confirmed per harness version by the install probe (`saga doctor`, trace-spec §7) and never assumed.*

---

## 1. Hook composition

One binding per harness event: `saga hook <harness> <event>`, written by `saga install --harness <h>`. No layer installs a hook of its own; `saga <layer> install` adds the layer to `.saga/manifest.json` and re-runs the installer. The entry runs the installed layers in this order and merges their outputs into the one envelope the harness accepts.

| Event (Claude Code name) | Order inside the entry | Short-circuit |
|---|---|---|
| `PreToolUse` | trace (record) → guard → route → mem → shape → gate → trace (record decision) | a `deny` from guard or gate ends the chain |
| `PostToolUse` | trace → shape → gate → guard (deps) → mem (capture) → index (`update`, optional) → trace (watchdog, budget) | none; feedback only except on Gemini |
| `Stop` / `AfterAgent` | trace (turn end, checkpoint) → gate (`check --status`) → trace (budget hard stop, watchdog block) → mem (pending line) | `block` wins; gate's reason first |
| `UserPromptSubmit` / `BeforeAgent` | trace (turn) → guard (mask scan) → mem (statement capture; Gemini: state re-inject) | guard `block` ends the chain |
| `PreCompact` / `PreCompress` | mem (`state --write`) → trace | never blocks; exit 0 always |
| `SessionStart` / `PostCompact` | trace (session, pins) → mem (state re-inject on `compact`/`resume`) | |
| `SubagentStart` / `SubagentStop` | trace | |

### 1.1 Merge rules

| Field | Rule |
|---|---|
| decision | `deny` > `block` > `ask` > `allow`; the first denying layer's reason leads |
| `updatedInput` | field-wise union (`command`: the one rewrite below; `prompt`: mem; `model`: route; `content`: guard); two layers on one field is a composition bug, exit 2 |
| Bash rewrite | exactly one: `saga shape run --mask -- <cmd>` when shape is installed (its masker and unmask-in are guard's), else `saga guard exec --mask -- <cmd>` |
| `additionalContext` | concatenated in layer order, each block prefixed `saga <layer>:` |
| `reason` | concatenated in layer order under the gate-spec §9 per-event ceilings (pre-tool 150, post-tool 200, Stop 400 est. tokens) |
| exit code | §4 precedence over the layers' codes; exit-2-with-stderr only when the harness rejects JSON |
| latency | whole entry p95 ≤ 300 ms with a snapshot, ≤ 20 ms without (guard-spec §11.4, trace-spec §11.1) |

### 1.2 Per-harness event matrix

| Capability | Claude Code | Codex CLI | Gemini CLI |
|---|---|---|---|
| Pre-tool block | `PreToolUse` `permissionDecision: deny` (verified) | `PreToolUse` `decision: block` (verified) | `BeforeTool` `deny` (verified) |
| Pre-tool ask | `permissionDecision: ask` (verified) | `PermissionRequest` (documented) | none; `ask` collapses to `deny` naming the allow rule (verified) |
| Pre-tool input rewrite | `updatedInput` (documented; probe) | probe | `tool_input` merge (verified) |
| Pre-tool add context | `additionalContext` (two vendor pages disagree; probe) | `additionalContext` (documented) | none (verified) |
| Post-tool | feedback only (verified) | assume feedback only (unverified) | `block` replaces the result (verified) |
| Post-tool add context | probe | `additionalContext` (documented) | `additionalContext` (verified) |
| Stop block | `Stop` block; harness cap 8 consecutive (verified) | `Stop` block (verified); cap unverified | `AfterAgent` block (verified) |
| Prompt hook | `UserPromptSubmit` block; no rewrite (verified) | `UserPromptSubmit` (documented) | `BeforeAgent` turn-scoped context (verified) |
| Pre-compaction | `PreCompact`: no context; exit 2 blocks compaction (verified) | `PreCompact`: `continue`/`stopReason`/`systemMessage` only (documented) | `PreCompress` advisory (verified) |
| Post-compaction context | `SessionStart` `trigger: compact` (verified); `PostCompact` (documented; probe picks one) | `SessionStart` `source: compact` (documented) | none; `BeforeAgent` next turn (verified) |
| Sub-agent spawn | `PreToolUse` on `Agent`: `updatedInput.prompt` + `.model` (probe) | `SubagentStart` `additionalContext` (documented); no per-sub-agent model (codex #31814) | `BeforeTool` `tool_input` merge (tool name: probe) |
| Transcript read | trace only, read-only | trace only | trace only |
| Hooks vs sandbox | inside Seatbelt | outside the sandbox (verified) | n/a |

Other harnesses (Cursor, OpenCode, Cline, Kilo, CI) are CLI plus MCP only until M6.

---

## 2. `.saga/` layout

`saga init` writes `.saga/.gitignore` (`trace/ index/ shape/ snap/ observed/ audit.jsonl`) and a `.saga/config.toml` skeleton; the repository's own `.gitignore` is never edited.

| Path | Owner | Committed | Notes |
|---|---|---|---|
| `.saga/config.toml` | all | yes | one table per layer: `[gate]`, `[trace.budget]`, `[trace.watchdog]`, `[index]`, `[shape]`, `[mem]`; `[gate]` and `[trace.*]` are read from gate's `BASE:` via `git show`, the rest from the working tree |
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
| 1 | finding | unmet gate, ask, stale record, canary non-pass, runtime budget exhausted, empty result, doctor check failed |
| 2 | usage, parse or schema failure (fail closed) | invalid contract, unknown major, repo policy touching a fixed table |
| 3 | refusal | hard deny, unwaived guard violation, bench estimate over budget, masked write rejected |
| 4 | approval or trust required | gate approval missing, policy hash untrusted, snapshot over budget with `on_budget = "ask"` |
| 5 | integrity or proof missing | red proof absent, hash chain mismatch, strict replay divergence, manifest hash drift, stale index under `--require-fresh`, evicted log |
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
| `turn` | phase, prompt hash and bytes, final message hash, `claimed_done` |
| `model_call` | requested and served model, request id, fingerprint, effort, usage (§7.2), `call_key`, hashes, latency, status, attribution |
| `tool_call` | tool, args hash and ref, `component` (§7.1), cwd, `index_version` |
| `tool_result` | for_seq, exit, error, result hash, bytes, ref, truncated, wall, `served`, optional `shaped` (shape-spec §8) |
| `edit` | path, before and after `sha256:`, hunks, added, removed, by_tool, in_scope |
| `gate` | kind (check, guard_diff, red, stop, claim), ids, states, decision, progress hash |
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
| trace | budget line 60, watchdog warn 80 | 400 | trace-spec §3.6, §5.3 |
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
| 5 | mem state block (M1) | `PreCompact` exit 0; re-inject placement; `Agent` `updatedInput.prompt` reaches the sub-agent |
| 6 | index (M2) | regime; `tools/list` byte-stable; map block hash |
| 7 | mem targeted (M3) | `PreToolUse` vs `PostToolUse` `additionalContext` placement |
| 8 | shape (M4) | Bash rewrite honoured; `PostToolUse` `additionalContext`; harness result cap recorded |
| 9 | route (M5) | `Agent` `updatedInput.model` accepted; effort knob pinned; served model observable |
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

Registry: `saga.trace/1`, `saga.trace.ledger/1`, `saga.trace.report/1`, `saga.trace.prices/1`, `saga.trace.pins/1`, `saga.trace.checkpoint/1`, `saga.trace.bundle/1`, `saga.doctor/1`; `saga.gate.status/1`, `saga.gate.evidence/1`, `saga.gate.red/1`, `saga.gate.approval/1`; `saga.guard.policy/1`, `saga.guard.decision/1`, `saga.guard.mask/1`; `saga.shape.parser/1`, `saga.shape.result/1`, `saga.shape.edit/1`, `saga.shape.window/1`, `saga.shape.cache/1`; `saga.mem.record/1`, `saga.mem.index/1`, `saga.mem.session/1`, `saga.mem.inject-request/1`, `saga.mem.inject/1`; `saga.route.policy/1`, `saga.route.plan/1`; `saga.index.status/1`; `saga.bench.task/1`, `saga.bench.run/1`, `saga.bench.harness/1`, `saga.bench.manifest/1`, `saga.bench.report/1`.
