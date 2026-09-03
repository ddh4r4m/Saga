# `saga mem`: technical specification

*Draft v0.1, 2026-09-02. Implements doc 09 §3.4 (typed memory, targeted injection), the M1 compaction-survival state block and the M3 memory milestone (doc 09 §5). Harness facts verified the same day against code.claude.com/docs/en/hooks, geminicli.com/docs/hooks/reference and learn.chatgpt.com/docs/hooks; where two pages of the same vendor disagree, the disagreement is recorded and the adapter probes at install. Written to be implemented from the text; every number carries its source or is marked `default` (a tunable with no evidence behind it).*

---

## 1. Purpose, non-goals, permitted claims

### 1.1 Purpose

`saga mem` is the typed memory layer. It answers four complaints, each with a mechanism that does not depend on the model reading a long document:

| Complaint | Evidence | Mechanism (section) |
|---|---|---|
| Compaction deletes the "why" and keeps the "what" | doc 02 item 3; doc 07 §6 item 2 (ranked second; claude-code #7530, #34556; codex #4106); doc 06 D.2 item 4 (#21925, #24460 are "pure harness design") | State block written before compaction, re-injected after (§4) |
| Cross-session amnesia, repeated mistakes | doc 02 item 11; doc 07 §6 item 11 (every memory product ships its own silent-loss bug: mem0 #5245, #4956; letta #3388) | Plain-file typed records with freshness checks (§2) |
| Sub-agent handoff loses constraints | doc 02 item 12; ponytail #597 and #502 show the opposite failure, sending everything to every sub-agent (doc 07 §7) | Verbatim constraint preamble, constraints and decisions only (§4.6) |
| Instructions decay with session length | 5.6% lower compliance odds per function written, 20 to 60% by messages 6 to 10 (doc 04 §2.7); nobody does targeted re-injection keyed to the next tool call (doc 04 §5.2 item 2) | PreToolUse-adjacent injection of at most N records (§5) |

The layer is one core (`saga mem` CLI, `mem.*` MCP tools) plus per-harness adapters under 150 lines each, translation only, per ADR 0003.

### 1.2 Non-goals

| Not done | Reason and source |
|---|---|
| Narrative summaries of the codebase | The index is the memory of the code (ADR 0004). LLM-generated context files reduced resolve rate about 3% and raised cost 20 to 23%; human-written ones gained about 4% (doc 05 §3.3, arXiv 2602.11988). |
| LLM-authored facts without provenance and expiry | Retaining an agent's own prior outputs propagates errors (doc 04 §2.4, arXiv 2602.24287). Every LLM-derived record carries a source hash and expires when the source changes (ADR 0004). |
| Session-start dumps | claude-mem v3 auto-injected everything and produced "context pollution"; v4 retreated to an ~800-token index, after which 8,785 observations across 13 projects were "never surfaced unless asked" (doc 04 §2.4). Saga injects nothing at session start except the state block after a compaction or resume (§4). |
| Per-turn unconditional injection | caveman #303: a rule re-injected by SessionStart plus UserPromptSubmit hooks "firing every turn" still drifts mid-session (doc 07 §4 item 5); everything injected every turn is paid every turn (doc 07 §7; doc 09 §2). |
| A vector database | #34556 and Claude Code's own auto-memory converged on plain files (doc 07 §6 item 11); MemDelta finds controlled baselines beat purpose-built memory systems once confounds are removed (doc 05 §3.1). |
| Reading the harness transcript | Adapters never open `transcript_path` (same rule as gate-spec §6). Capture uses hook payload fields only (§3). |
| Enforcing anything | A record is advice adjacent to a tool call. Every rule with a checkable form is a gate or diff guard (gate-spec §5, ADR 0002); mem exists to reduce the violations the gate must catch, not to guarantee zero (doc 09 §3.4). |

### 1.3 What mem cannot promise

Stated once so the README does not exceed it:

1. A prose-only rule with no checker will still drift. gemini-cli #13852 locates the problem "in the model itself"; #303 shows per-turn re-injection is not sufficient (doc 07 §4 item 5, §8 item 3). The honest prior for targeted injection is "less, not zero" (doc 09 §7 item 9). The bench reports the drift rate; it is not hidden.
2. Memory costs tokens on every turn it is injected. The trace ledger attributes those tokens to `mem` per record kind (ADR 0007).
3. Cross-session memory has no published positive result on a coding benchmark (doc 04 §2.4: "storage works, retrieval/injection does not"; doc 05 §3.3: SWE-Bench-CL has "no strong positive result yet"). Procedural memory is the only kind with positive evidence (AWM +24.6% Mind2Web relative, ACE +10.6%, SWE-Exp +7.2% relative on SWE-bench Verified inside an MCTS scaffold; doc 05 §3.3), and that evidence is mostly outside coding.
4. Nothing survives compaction on a harness with neither a pre-compaction nor a post-compaction hook, other than what the agent fetches itself (§4.7).

### 1.4 Evidence map

| Mechanism | Traces to | Status |
|---|---|---|
| Typed records, human conventions | doc 05 §3.3 (human-written +4%), §9.6 | evidence |
| Procedures with outcomes | doc 05 §3.3 (AWM, ACE, SWE-Exp) | evidence, non-coding or confounded |
| Freshness on read | doc 02 item 11 ("does this path still exist?") | design, no measurement |
| State block around compaction | doc 07 §5 (#34556), §9 ("users already did it") | user-validated, unmeasured |
| Targeted injection | doc 04 §5.2 item 2 (gap) | experimental; M3 exit criterion |
| Sub-agent preamble | doc 02 item 12 | design, no measurement |

---

## 2. Records

### 2.1 Kinds

| Kind | Who writes it | Provenance required | Freshness check | Expiry | Injected? |
|---|---|---|---|---|---|
| `env` | `saga mem discover` (derived from lockfiles, `.tool-versions`, `.nvmrc`, `go.mod`, `Package.swift`, CI config) or human | `derived_from` file path and sha256, or `source = "user"` | source hash unchanged; named command resolvable | when `derived_from` hash changes | yes, on matching command |
| `convention` | human only (CLI or file edit) | `source = "user"` | paths and globs still match ≥ 1 file | manual; `stale_days` flag (§2.4) | yes, on matching path or tool |
| `constraint` | user statement via hook (§3.2), human CLI, agent proposal after confirmation | verbatim `quote`, `turn_hash` | paths still exist | closes when its contract closes, or manual | yes; always in state block and preamble |
| `decision` | as `constraint`, plus `alternatives` and `reason` fields | `quote`, `turn_hash` | paths still exist | contract close or manual | state block and preamble; injected on matching path |
| `procedure` | gate evidence (§3.4), shell outcomes (§3.4), human | `evidence_hash` or `source = "user"` | `command` argv[0] resolvable; `cwd` exists | `stale_days` since `last_ok_at` | yes, on matching command family or path |
| `pitfall` | human; agent proposal after confirmation; automatic failure signature (§3.4) | `quote` or `signature` | paths and commands resolvable | `stale_days`; automatic ones expire on first success of the same command | yes, on matching path or command |

Any record whose provenance is `agent` is `pending` until a human confirms it (§3.3). Pending records are never injected and never enter the state block.

### 2.2 Common schema (`saga.mem.record/1`)

One record per file, TOML, because TOML diffs line by line and merges without a parser (§7).

```toml
schema      = "saga.mem.record/1"
id          = "01J9Z6Q0N5F3R8K2VH7T4M1XWE"      # ULID; also the file name
kind        = "constraint"                      # §2.1
status      = "active"                          # active | pending | stale | closed | tombstone
text        = "Never edit files under db/migrations/; add a new migration instead."
                                                 # ≤ 240 bytes UTF-8 (≈ 60 est. tokens, bytes/4 as gate-spec §9)
created_at  = 2026-09-02T14:02:11Z
confirmed_at = 2026-09-02T14:02:11Z             # absent while pending
expires_at  = 2026-12-01T00:00:00Z              # optional
visibility  = "repo"                            # repo | user (§7.3)

[scope]                                         # selector keys (§5.2); at least one for injectable kinds
paths       = ["db/migrations/**"]              # gitignore-style globs, repo-relative
commands    = []                                # argv[0] or "argv0 sub" families, e.g. "pnpm test"
tools       = ["edit"]                          # canonical tool classes: read edit shell search agent mcp:<name>

[source]
origin      = "user"                            # user | agent | derived | gate | shell
quote       = "never touch the migration files, add a new one"   # verbatim, ≤ 240 bytes
turn_hash   = "sha256:9f3c…"                    # sha256(session_id ‖ prompt_id ‖ user_prompt) from the hook payload
harness     = "claude-code/2.1.190"             # informational
session_id  = "…"
derived_from = { path = "pnpm-lock.yaml", sha256 = "…" }   # derived only
evidence_hash = "sha256:…"                      # gate/shell only; gate-spec §4 record hash

[links]
contract    = "vendor-import"                   # gate contract id, optional
gates       = ["vendor-import:G2"]
```

Kind-specific tables (all optional fields absent, never null):

```toml
# decision
[decision]
alternatives = ["approach B: SwiftUI-only"]     # ≤ 3 entries, ≤ 120 bytes each
reason       = "iOS 16 floor"                   # ≤ 120 bytes

# procedure
[procedure]
command       = "pnpm test --filter import"
cwd           = "."
context       = "after editing src/import/**"   # ≤ 120 bytes, human or gate-derived
success_count = 3
fail_count    = 0
last_ok_at    = 2026-09-02T14:02:11Z
last_fail_at  = 2026-08-30T09:11:00Z

# pitfall
[pitfall]
command   = "pnpm test"
signature = "Cannot find module '@app/shared' from 'src/index.ts'"   # masked (§8), ≤ 200 bytes
fix       = "run pnpm build --filter shared first"                   # ≤ 120 bytes, human or confirmed
```

Rules:

| Rule |
|---|
| `text`, `quote`, `reason`, `fix`, `signature` are control-stripped and bidi-stripped on write and on read (gate-spec §8, untrusted ledger text). |
| A record with `source.origin = "agent"` and no `confirmed_at` is `pending`; the checker refuses to write it as `active`. |
| A record with `kind ∈ {constraint, decision}` and no `quote` is rejected (exit 2). |
| Every write masks secrets first (§8); a record whose `text` triggers a mask is rejected, never stored with a placeholder. |
| Total bytes per record file ≤ 2 KiB `default`. |

### 2.3 Storage layout

```
.saga/mem/                       committed
  records/<kind>/<ulid>.toml     one active or stale or closed record per file
  pending/<ulid>.toml            agent proposals awaiting confirmation
  tombstones/<ulid>              empty file; deletion marker that survives merges (§7.2)
  README.md                      12 lines, generated once, explains the directory to humans
.saga/observed/                  gitignored through .saga/.gitignore, written by `saga init` (contracts §2)
  mem-index.json                 derived index, rebuilt when any record hash changes (§2.5)
  mem-session-<id>.json          injection ledger for dedup and caps (§5.5), audit trail (§8)
  state-<session>.md             current state block (§4)
~/.saga/mem/<repo-id>/records/…  per-user records, `visibility = "user"` (§7.3)
```

`repo-id` is the sha256 of the root commit hash, stable across clones and worktrees. No SQLite: ruflo #442 ("SQLite storage always falls back to in-memory due to logic error") and mem0 #5245 are the failure class plain files avoid (doc 07 §6 item 11, §7).

### 2.4 Freshness checks

Run on every read that could inject (`inject`, `state`, `get`, `search`) and by `saga mem check`. A record failing any check is reported `stale` with the reason and is not injected (doc 09 §3.4: "stale records are flagged, not injected").

| Check | Applies to | Method | Failure reason |
|---|---|---|---|
| path exists | any record with `scope.paths` containing no glob metacharacters, `derived_from.path` | `lstat`, no-follow, inside canonical repo root | `path missing` |
| glob matches | `scope.paths` with metacharacters | match against `git ls-files` plus untracked non-ignored, cached per index rebuild | `glob matches nothing` |
| command resolvable | `scope.commands`, `procedure.command`, `pitfall.command` | argv[0] on `PATH`, or a script name in `package.json` `scripts`, `Makefile` targets, `justfile`, `Taskfile`; never executed | `command not found` |
| source unchanged | `derived_from` | sha256 of the file equals the stored hash | `source changed` (record expires, `discover` regenerates) |
| not expired | `expires_at` | clock | `expired` |
| recently useful | `procedure`, `pitfall` | `now - last_ok_at ≤ stale_days` (`default` 90) | `stale: no success in N days` |
| contract open | `constraint`, `decision` with `links.contract` | contract present in `.saga/contract.md` or in `.saga/evidence/` history | `contract closed` → status `closed`, kept for history, not injected |

Cost bound: checks are filesystem metadata calls, no process execution, no network. `saga mem check` prints per-record timing; the adapter budget for a full `inject` call is 200 ms `default`, above which the adapter emits nothing and logs `mem: timeout` (fail open on injection, fail closed on nothing else).

### 2.5 Index

`mem-index.json` is derived and never merged:

```json
{"schema": "saga.mem.index/1", "built_from": "sha256 of sorted record hashes", "records": 137,
 "by_path": [{"glob": "db/migrations/**", "ids": ["01J9…"]}],
 "by_command": {"pnpm test": ["01J9…"], "pnpm": ["01JA…"]},
 "by_tool": {"edit": ["…"], "agent": ["…"]},
 "constraints_open": ["…"], "decisions_open": ["…"],
 "stale": [{"id": "…", "reason": "path missing"}],
 "pending": 2}
```

Rebuild trigger: the sorted list of record file hashes differs from `built_from`. Target rebuild time for 400 records under 50 ms on a laptop (`default`, measured by the test suite, §9.1).

### 2.6 Size caps

| Cap | Value | Basis |
|---|---|---|
| `text` per record | 240 bytes (≈ 60 est. tokens) | one instruction; HumanLayer's 150 to 200 followable instructions and Claude Code's ~50 system-prompt instructions (doc 04 §2.7) say the scarce resource is instruction count, so each record is one |
| Active records per store | soft 400, `check` warns above | `default`; injection never reads more than N (§5.4), so the store cap is a hygiene limit, not a context limit |
| Pending proposals | 20; `mem.propose` returns `pending_full` above | `default`; prevents an agent flooding the queue (letta #3388 poisoning class, doc 07 §6 item 11) |
| Automatic pitfalls per command | 5 | `default` |
| Record file | 2 KiB | `default` |

---

## 3. Capture

### 3.1 Sources

| Source | Hook or command | Writes | Status on write |
|---|---|---|---|
| Explicit user statement | `UserPromptSubmit` (Claude Code, Codex), `BeforeAgent` (Gemini), field `user_prompt` / `prompt` | `constraint`, `decision` | `active` (human-authored) |
| Human CLI | `saga mem add` | any kind | `active` |
| Agent proposal | MCP `mem.propose` | any kind except `env` | `pending` |
| Gate outcome | `saga gate check` evidence records (gate-spec §4) | `procedure` | `active`, `origin = "gate"` |
| Shell outcome | `PostToolUse` / `AfterTool` on the shell tool, fields `tool_input.command`, `tool_response` exit code | `procedure` counters, automatic `pitfall` | `active`, `origin = "shell"` |
| Discovery | `saga mem discover` | `env` | `active`, `origin = "derived"` |

### 3.2 Explicit statement detection

Deterministic, over the user's own words only, so the result is human-authored by construction:

```
input     : user_prompt (hook payload), never the transcript
split     : sentences on [.!?\n]; drop sentences > 240 bytes or inside fenced code
match     : a versioned pattern list, patterns.toml in the saga binary, user-extensible
            constraint: ^(never|always|do not|don't|must not|must|only ever|from now on|going forward)\b
            decision  : ^(we (decided|will go with|are going with|chose)|decision:)\b
scope     : paths = repo-relative path tokens or globs in the sentence that pass the glob check (§2.4)
            commands = argv0 tokens that resolve (§2.4)
            tools    = "edit" if the sentence names edit/write/change/touch, "shell" if run/execute
write     : text = the sentence verbatim; quote = the sentence; turn_hash = sha256(session_id ‖ prompt_id ‖ user_prompt)
notify    : one line to the human via systemMessage (not the model):
            "saga mem: recorded constraint 01J9…: \"…\" (saga mem drop 01J9… to remove)"
```

No model call, no paraphrase. A sentence that matches but yields no scope is stored with `scope.tools = ["edit","shell"]` so it still reaches the state block and preamble; it is never targeted-injected because it matches every call. Precision and recall of the pattern list are measured on a labelled corpus (§9.1) and printed by `saga doctor`; the list ships with positive controls that must match and negative controls ("never mind", "always fails on CI") that must not.

### 3.3 Agent proposals and confirmation

`mem.propose` writes to `pending/`. Nothing in `pending/` is injected, listed in the state block, or handed to sub-agents. Confirmation paths, in order of preference:

1. `saga mem confirm <id>` by a human at the terminal.
2. `saga mem review` interactive list (accept, edit text, reject) at session end.
3. A blocking question raised by the harness's own question tool when the agent calls `mem.propose` with `ask_now = true`; the adapter never answers on timeout ("no answer means yes" is forbidden, doc 09 §2; claude-code #73125).

A proposal must carry `quote` (verbatim user text it rests on) or `basis = "observed"` with an `evidence_hash`; a proposal with neither is accepted into `pending/` but flagged `unsupported` in `review`. Proposals older than 14 days `default` are pruned. The Stop adapter appends, within gate-spec §9's cap, one line `N mem proposals pending: saga mem review` only when N > 0.

### 3.4 Gate and shell outcomes feed procedures

| Event | Rule |
|---|---|
| Gate `met` with runnable `CHECK:` | upsert `procedure` keyed on `(command, cwd)`: `success_count += 1`, `last_ok_at`, `evidence_hash`, `context = "gate <id>: <outcome ≤ 120 bytes>"`, `scope.paths = IN: globs` of the contract |
| Gate `unmet` after `met` for the same command | `fail_count += 1`, `last_fail_at` |
| Shell tool exit 0, argv[0] in the runner family list | `success_count += 1` on the matching procedure if one exists; no new record is created from shell success alone (`default`: creation needs a gate or a human) |
| Shell tool non-zero exit, same masked error signature (first 200 bytes after masking, digits and paths normalised) 3 times `default` in one session | automatic `pitfall` with `signature`, no `fix`, status `active`, `origin = "shell"`; expires on the next exit-0 run of the same command |

Runner family list until `saga shape` (M4) provides parsers: `npm pnpm yarn bun pytest go cargo swift xcodebuild gradle mvn make just dotnet mix bundle rspec phpunit` plus `[mem] runner_families` in config. Procedures never store output; they store the command, the counters and the evidence hash.

### 3.5 Keeping the agent's prose out

| Rule |
|---|
| No hook reads assistant text. `last_assistant_message` (Stop, SubagentStop) is never parsed by mem. |
| The only path for agent-authored text is `mem.propose`, capped at 240 bytes of `text`, landing in `pending/`. |
| `procedure.context` from a gate is the gate's outcome line, which gate-spec §2.3 requires to be an observable outcome, not narration. |
| Automatic pitfalls contain a masked error signature and no free text. |
| `env` facts are file-derived with a hash; a human may edit, an agent may only propose. |
| `search` and `get` results are tool results, so anything the agent writes after reading them is transcript, not memory. |

---

## 4. The compaction-survival state block

### 4.1 Contents

| Line | Source of truth | Cap |
|---|---|---|
| `CONTRACT` | `saga gate status --json`: contract id, hash, gate counts by state | 1 line |
| `TASK` | first sentence of `.saga/request.md` (gate-spec §2.4) or contract title | 200 bytes |
| `CONSTRAINTS` | active constraints, newest last, each `id`, `text`, origin, turn index | never trimmed below the cap; see §4.3 |
| `DECISIONS` | active decisions with `reason` | oldest trimmed first |
| `PROCEDURE` | the procedure with the highest `success_count` whose scope matches the contract's `IN:` globs | 1 line |
| `FILES` | `git diff --name-only <BASE>` from gate's `BASE:` plus untracked non-ignored | 8 paths then `(+N)` |
| `LAST CHECK` | most recent gate evidence per unmet gate: id, state, one-line reason | 3 lines |
| `BUDGET` | trace ledger (ADR 0007): context tokens used of window, USD of budget, turn count | 1 line |
| `PENDING` | count of `pending/` records | 1 line |

Format, exact:

```
<saga-state v1 session=01J9… turn=41 hash=sha256:2be1…>
CONTRACT: vendor-import sha256:4c1e… | met 2 unmet 1 (G3) unproven 1
TASK: Import valid records from the vendor feed and reject malformed ones.
CONSTRAINTS:
  01J9Z6Q0: "Never edit files under db/migrations/; add a new migration instead." [user, turn 7]
  01J9Z7A1: "Keep the iOS 16 floor." [user, turn 12]
DECISIONS:
  01J9Z7B2: "Approach B (SwiftUI-only) rejected: iOS 16 floor." [user, turn 12]
PROCEDURE: pnpm test --filter import (ok 3, fail 0, last ok 14:02)
FILES: src/import/parse.ts src/import/index.ts tests/import/parse.test.ts (+2)
LAST CHECK: G3 unmet 14:02 "assertion failed: expected 4 records, got 3"
BUDGET: ctx 148k/200k | $3.20/$5.00 | turn 41
PENDING: 2 mem proposals (saga mem review)
</saga-state>
```

### 4.2 Ceiling

2,400 bytes, 600 estimated tokens (`ceil(bytes/4)`, gate-spec §9). Basis: claude-mem's retreat settled at an ~800-token index (doc 04 §2.4) and Cursor's RL-trained compaction summaries run ~1,000 tokens against >5,000 for the baseline (doc 03 §1.3); the state block is a strict subset of what a summary must carry, so it sits under both. Trimming order when over the cap: `FILES` to 3 paths, `LAST CHECK` to 1 line, `DECISIONS` oldest first, `PROCEDURE`, then `CONSTRAINTS` oldest first with a final line `+N constraints: saga mem state --full`. The full block is always available from the CLI.

### 4.3 Regeneration

The block is a file, `.saga/observed/state-<session>.md`, rewritten atomically:

| Trigger | Where |
|---|---|
| Any mem record write in this session | `saga mem` core |
| Any gate state change | `saga gate check`, `status` (post-write hook inside the binary) |
| Pre-compaction event | adapter calls `saga mem state --write` |
| At most once per turn otherwise | `PostToolUse`-equivalent adapter, cheap because inputs are cached in the index |

Writing is never skipped for lack of a hook. On a harness with no pre-compaction hook the file is at most one turn old when compaction strikes.

### 4.4 Re-injection adapters

| Harness | Write | Re-inject | Verified fact that shaped it |
|---|---|---|---|
| Claude Code | `PreCompact` (`trigger` ∈ manual, auto): run `saga mem state --write`, exit 0 always | `SessionStart` with `source ∈ {compact, resume, fork}`: `hookSpecificOutput.additionalContext` = block, dedup on `(session, state hash)` | Verified 2026-09-03 (harness-facts C13 to C16): `PreCompact` has no context channel and exit 2 or `decision: block` blocks compaction, which the adapter never does; `SessionStart` accepts `additionalContext`; the input field is `source`, not `trigger`; `PostCompact` "has no decision control" and no context channel, so it is not bound by mem |
| Codex CLI | `PreCompact` writes | `SessionStart` with `source = "compact"` or `"resume"`: `additionalContext` | Verified (harness-facts X15, X16): `PreCompact`/`PostCompact` stdout supports `continue`, `stopReason`, `systemMessage` only; `SessionStart` supports `additionalContext`, and after a mid-turn auto-compaction Codex "delivers the hook's additional context to the immediate continuation instead of waiting for a later user turn" |
| Gemini CLI | `PreCompress` (advisory, "fired asynchronously", "cannot block or modify the compression process", harness-facts G8) writes the block and sets `compressed_pending` in the session file; because the hook races the compression, the block is always regenerated from the file (§4.3), never from hook stdin | `BeforeAgent` on the next user turn: `hookSpecificOutput.additionalContext` (turn-scoped) once, then clears the flag; `SessionStart` `source = "resume"` injects | `SessionStart.source` has no `compact` value; `BeforeModel` could rewrite `llm_request` on every call but that touches the cached prefix, so it is not the default (§5.6) |
| CI, bare loop, any harness without hooks | file written by the core on every write | none automatic; §4.7 | |

The injected block is preceded by one fixed line: `Context was compacted. Saga state (authoritative for constraints and decisions):`. Nothing else is added. Ledger attribution: `component = "mem.state"`.

### 4.5 Resume semantics

On `resume` the block is injected once per new session id even without compaction, because a resumed session has the same amnesia shape (doc 02 item 3). `startup` and `clear` inject nothing: a fresh session has no open contract state worth 600 tokens, and doc 04 §2.4's session-start dump is the failure mode being avoided. If a contract is open at `startup` (`.saga/contract.md` exists with unmet gates), the adapter injects only the `CONTRACT` and `CONSTRAINTS` lines (≤ 200 est. tokens).

### 4.6 Sub-agent preamble

Content: `CONTRACT`, `CONSTRAINTS`, `DECISIONS` lines only, ≤ 1,200 bytes (300 est. tokens). Procedures, files and budget are deliberately excluded: ponytail #502 injected lazy-mode into code reviewers and #597 sent the full SKILL.md to every sub-agent (doc 07 §7); the preamble carries what the sub-agent must not violate, nothing about how to work.

| Harness | Mechanism | Status |
|---|---|---|
| Claude Code | Primary: `SubagentStart` (matcher: agent type, or all) `hookSpecificOutput.additionalContext` = preamble, delivered "at the start of its conversation, before its first prompt". `SubagentStart` fires on 2.1.259 with `agent_type` and `agent_id` (harness-probes P2). Fallback when the probe finds `SubagentStart` missing: `PreToolUse` matcher `Agent` (the hook-input `tool_name` is `Agent` even though the `-p` tool list names it `Task`, P2): `hookSpecificOutput.updatedInput` = the full `tool_input` with `prompt = preamble ‖ "\n\n" ‖ tool_input.prompt`, `permissionDecision = "allow"`; route sets `updatedInput.model` on the same event (route-spec §6.1) and the composed hook merges field-wise (contracts §1) | Verified 2026-09-03 (harness-facts C17, C3): "SubagentStart hooks can't block subagent creation, but they can inject context into the subagent"; `updatedInput` "replaces the entire input object". The primary channel removes the `prompt` field from the merge, so REVIEW-LOG #4 only concerns the fallback. Install probe (§9.1) confirms the preamble reaches the sub-agent; otherwise `subagent_preamble = unavailable`. The fallback's `updatedInput` merge is exercised on 2.1.259: a hook-written `updatedInput.model` beside the original fields is honoured (harness-probes P2), so the field-wise union with route is sound |
| Codex CLI | `SubagentStart`: `additionalContext` "added as extra developer context for the subagent" | verified (harness-facts X17) |
| Gemini CLI | `BeforeTool` matcher = the union of configured sub-agent names (`generalist`, `codebase_investigator`, `cli_help`, `browser_agent`, custom agents' `name`), `hookSpecificOutput.tool_input` merge on the prompt field | verified (harness-facts G10): "Subagents are exposed to the main agent as a tool of the same name"; the installer reads the names from settings |
| No hook | the parent is told nothing; `mem.state --preamble` exists for agents that are instructed to call it | reported as `unavailable` |

The preamble is verbatim record text, so a constraint reads identically in the parent, the sub-agent, and the file. Sub-agent results coming back as structured evidence is gate's job (gate-spec §4), not mem's.

### 4.7 Harnesses without a pre-compaction hook

The file is still written (§4.3). What is lost is automatic re-injection. Fallbacks, in order:

1. Post-compaction hook if one exists (Claude Code and Codex `SessionStart source=compact`; Claude Code's `PostCompact` has no context channel, harness-facts C16).
2. Next-user-turn hook with a pending flag (Gemini `BeforeAgent`), which covers manual and auto compression that fires between turns; compression inside a long agent loop is re-injected only at the next user turn, and the bench measures how much that costs (§9.2, survival rate per harness).
3. No hooks at all: `mem.state` (MCP) and `saga mem state` (CLI). The instruction file gets one line, measured at ≤ 30 est. tokens: `After any context compaction run: saga mem state`. This is prompt text and by ADR 0002 it ships only with its bench number; the expected survival rate in this mode is near the bare baseline and the report says so.

---

## 5. Targeted injection

### 5.1 Selector input

The adapter maps the upcoming tool call to a canonical request:

```json
{"schema": "saga.mem.inject-request/1",
 "session_id": "…", "turn": 41,
 "tool": "edit",                          // read edit shell search agent mcp:<name>
 "path": "src/import/parse.ts",           // editor tools; null otherwise
 "command": "pnpm test --filter import",  // shell tools; raw string
 "argv0": "pnpm", "family": "pnpm test",  // resolved by guard's post-expansion parser (ADR 0006) when installed; else argv[0] of the first segment
 "mcp_tool": null}
```

Tool class mapping per harness lives in the adapter (Claude Code `Edit|Write|NotebookEdit → edit`, `Read → read`, `Bash → shell`, `Grep|Glob → search`, `Agent|Task → agent`; Gemini `write_file|replace → edit`, `run_shell_command → shell`; Codex `apply_patch → edit`, `shell → shell`). Unknown tools map to `mcp:<name>`.

### 5.2 Matching

A record is a candidate when any of its scope keys matches:

| Scope key | Matches when |
|---|---|
| `paths` | `path` matches a glob (gitignore semantics), or, for `shell`, any path token in the command matches |
| `commands` | `family` equals the entry, or `argv0` equals a one-token entry |
| `tools` | equals `tool`; a record whose only scope is `tools = ["edit"]` is a candidate for every edit and is therefore ranked last (§5.3) and counted against the false-injection rate (§9.2) |

Stale, pending, closed and tombstoned records are excluded before ranking.

### 5.3 Ranking

```
score = kind_weight            # constraint 5, pitfall 4, decision 3, convention 2, procedure 2, env 1
      + specificity            # number of literal path segments in the longest matching glob, 0 for tools-only
      + min(success_count, 5)  # procedures only
      - reinjection_penalty    # 3 if injected within the last W turns of this session (§5.5)
ties : newer confirmed_at first; repo store before user store
take : top N (default 3, max 5), stop early when the injection byte cap is reached
```

Weights are `default`; the ordering constraint>pitfall>decision is the only evidence-based part (constraints are the records users call betrayal when lost, doc 02 item 3).

### 5.4 Ceilings

| Ceiling | Value | Basis |
|---|---|---|
| Records per injection | N = 3 (`max 5`) | doc 09 §3.4 ("two or three records") |
| Bytes per injection | 800 (≈ 200 est. tokens) including the 1-line header | 3 × 240-byte records plus header |
| Targeted injection per session, hard cap | 1,200 est. tokens `default` | doc 05 §9.6: "cap at a few hundred tokens injected per task"; a session spans several tasks. Sibling of gate-spec §9's 1,000-token cap |
| State block per injection | 600 est. tokens (§4.2) | counted separately, `mem.state` |
| Preamble per sub-agent | 300 est. tokens (§4.6) | counted per sub-agent, `mem.preamble` |

At the session cap the adapter injects nothing further and writes `mem: cap reached` to the audit file; the bench treats a cap breach as a failed run of the layer (as gate-spec §9). The 1,200-token targeted cap is mem's share of the one per-session injected budget in contracts §7; the state block and the preamble are counted per event there, not against that share. All counts use `ceil(bytes/4)`, kept in the shared counter file `.saga/observed/session-<id>.json` next to gate's; the bench reports the ratio to the harness's actual token counters.

### 5.5 Dedup and re-injection window

A record injected at turn τ₀ is "in context" until either a compaction or resume event resets the session ledger, or `W` turns have passed. Within the window it is not re-injected (penalty 3 removes it from the top N unless nothing else matches; a constraint with `kind_weight 5` still outranks a fresh `env` fact, which is intended). `W` default 8: compliance falls to 20 to 60% by messages 6 to 10 (doc 04 §2.7), so a record that still matches at turn τ₀ + 8 is re-sent once, adjacent to the call, rather than every turn (#303). `W` is swept in §10.

The session ledger `mem-session-<id>.json`:

```json
{"schema": "saga.mem.session/1", "session_id": "…", "compactions": 1,
 "injected": [{"id": "01J9…", "turn": 33, "tool": "edit", "path_sha": "…", "bytes": 231, "placement": "pretool"}],
 "bytes_targeted": 2210, "bytes_state": 1980, "bytes_preamble": 0,
 "cap_reached": false, "state_hash_injected": "sha256:…"}
```

### 5.6 Placement

Injection is a tool-adjacent message, never a system-prompt or tool-list rewrite. It is emitted by mem's step inside the composed `saga hook <harness> <event>` entry (contracts §1), which runs after guard so a denied call gets no injection, and before shape and gate. Basis: Anthropic's cache hierarchy is `tools → system → messages`, reads cost 0.1× (0.025× on Fable 5.1), writes 1.25× to 2× (doc 05 §4.2); claude-code #91514 shows a warm cache fully rewritten seconds after a ToolSearch or Skill result (doc 07 §8). The ledger exposes the cache-read ratio with and without mem (§9.2).

| Harness | Primary placement | Fallback | Fact |
|---|---|---|---|
| Claude Code | `PreToolUse` `hookSpecificOutput.additionalContext` with `permissionDecision` untouched | `PostToolUse` `additionalContext` on `Read|Grep|Glob` whose path matches, so the records arrive with the read that precedes the edit; `PostToolBatch` `additionalContext` (once before the next model call) when a batch touches several matching paths | Verified 2026-09-03 (harness-facts C4, C5, C8, C9): both fields are documented and both land "next to the tool result" as a system reminder. The "debug log only" sentence concerns plain-text stdout, not the JSON field, so the two vendor pages never disagreed. The install probe still sends a fixture per version, and the M3 bench stratifies by placement for effect, not for existence |
| Codex CLI | `PreToolUse` `additionalContext` | `PostToolUse` `additionalContext` | documented |
| Gemini CLI | `AfterTool` `hookSpecificOutput.additionalContext` on read and search tools | `BeforeAgent` (turn-scoped, one bundle for the turn) | `BeforeTool` cannot add context, only merge `tool_input` |

The injected text is exactly:

```
saga mem (3 records for edit src/import/parse.ts):
- constraint 01J9Z6Q0: Never edit files under db/migrations/; add a new migration instead.
- pitfall 01JA0K2P: pnpm test needs `pnpm build --filter shared` first when @app/shared is missing.
- procedure 01JA0M7R: pnpm test --filter import (ok 3/3)
```

No instructions, no "remember", no tone. Ids are present so the agent can `mem.get` the full record and so the audit trail matches injection to record.

### 5.7 The measured claim

M3's exit criterion is a compliance-over-turns curve with and without mem (doc 09 §5), computed by bench-spec §5.9: `c(τ)` binned by run-length decile, AUC, and the decay `β` from `logit P = α + β·τ`. The arms are the ladder rung `base + gate + guard + index` versus that plus `mem` (bench-spec §4.3). The rules under test are stored as mem records **and** listed in the task's `rules.toml` with a bench-side checker, and are deliberately not turned into gates or diff guards in either arm, otherwise gate would absorb the effect and mem would measure nothing.

Pre-registered expectation, stated before the run so it cannot move afterwards: caveman #303 says standing per-turn re-injection drifts; doc 09 §7 item 9 puts the prior for adjacency at "less, not zero". Hypotheses:

| Id | Claim | Test |
|---|---|---|
| H1 | `β_mem > β_control` (slower decay) | Wilcoxon on per-task decay, bootstrap CI over tasks (bench-spec §5.5) |
| H2 | AUC_mem − AUC_control > 0 with CI excluding 0 | same |
| H3 | pass^k does not fall: CI inside the pre-registered equivalence bound ε | TOST (bench-spec §5.5) |
| H4 | `c(τ)` at the last decile is still below 1.0 in the mem arm, i.e. drift is reduced, not removed | reported either way |

A null on H1 and H2 is published in the report's mandatory negative-results section (bench-spec §7.2 item 6) and targeted injection is then cut to the state block only. The minimum effect of interest for H2 is set in the pre-registration file, not here; a badge (bench-spec §7.3) is the only number mem may cite.

---

## 6. Query surface

### 6.1 MCP tools

Registered by the `saga` MCP server alongside index and gate tools (ADR 0003), one-line descriptions (≤ 60 est. tokens each, gate-spec §9), and deferred loading where the harness supports it (doc 07 §5, `ToolSearch`).

```json
{"name": "mem.get", "description": "Fetch one memory record by id.",
 "input": {"type":"object","required":["id"],
           "properties":{"id":{"type":"string","pattern":"^[0-9A-HJKMNP-TV-Z]{26}$"}}},
 "output": {"$ref":"saga.mem.record/1", "plus": {"fresh":"boolean","stale_reason":"string?"}}}

{"name": "mem.search",
 "description": "Search memory by path, command or text. Returns at most 10 fresh records.",
 "input": {"type":"object",
           "properties":{"path":{"type":"string"},"command":{"type":"string"},
                         "text":{"type":"string","maxLength":200},
                         "kind":{"enum":["env","convention","constraint","decision","procedure","pitfall"]},
                         "limit":{"type":"integer","minimum":1,"maximum":10,"default":5}}},
 "output": {"records":[{"id":"…","kind":"…","text":"…","scope":{},"fresh":true}],
            "stale_omitted": 0, "pending_omitted": 0}}

{"name": "mem.propose",
 "description": "Propose a memory record for human confirmation. Never injected until confirmed.",
 "input": {"type":"object","required":["kind","text"],
           "properties":{"kind":{"enum":["convention","constraint","decision","procedure","pitfall"]},
                         "text":{"type":"string","maxLength":240},
                         "quote":{"type":"string","maxLength":240},
                         "basis":{"enum":["user","observed"]},
                         "evidence_hash":{"type":"string"},
                         "scope":{"type":"object","properties":{"paths":{"type":"array","items":{"type":"string"}},
                                  "commands":{"type":"array","items":{"type":"string"}},
                                  "tools":{"type":"array","items":{"type":"string"}}}},
                         "ask_now":{"type":"boolean","default":false}}},
 "output": {"id":"…","status":"pending","unsupported":false}}

{"name": "mem.state",
 "description": "Return the current Saga state block (constraints, decisions, contract, budget).",
 "input": {"type":"object","properties":{"preamble":{"type":"boolean","default":false},"full":{"type":"boolean","default":false}}},
 "output": {"text":"<saga-state …>","bytes":1980,"hash":"sha256:…"}}
```

`mem.search` with `text` is BM25 over `text`, `quote`, `procedure.command`, `pitfall.signature`, built into the index; no embeddings (doc 05 §1.3, §3.1). Tool results are ledger-attributed to `mem.query` and are not counted against the injection cap, because the agent asked.

### 6.2 CLI

```
saga mem add      <kind> --text <s> [--path <glob>]... [--command <s>]... [--tool <class>]...
                  [--quote <s>] [--expires <date>] [--user]
saga mem list     [--kind <k>] [--stale] [--pending] [--path <p>] [--command <s>] [--json]
saga mem get      <id> [--json]
saga mem check    [--fix] [--json]              # freshness over the store; --fix marks stale, prunes expired
saga mem prune    [--older <days>] [--closed] [--pending] [--dry-run]
saga mem inject   --tool <class> [--path <p>] [--command <s>] [--session <id>] [--json]
saga mem state    [--write] [--full] [--preamble] [--session <id>]
saga mem propose  <kind> --text <s> [...]       # same as MCP, writes pending/
saga mem confirm  <id> [--text <s>]             # human only; adapters deny this string from the agent's shell
saga mem drop     <id>                          # writes a tombstone
saga mem review                                 # interactive pending queue
saga mem discover [--write]                     # env facts from lockfiles and tool version files
saga mem install  --harness <claude-code|codex|gemini> [--shared] | uninstall
```

| Subcommand | Executes anything? | Writes |
|---|---|---|
| `add`, `propose`, `confirm`, `drop`, `discover --write` | never | record files, tombstones |
| `check`, `list`, `get`, `inject`, `state` (without `--write`) | never; metadata calls only | nothing (index rebuild is a cache) |
| `state --write` | never | `.saga/observed/state-<session>.md` |
| `install` | never | alias: adds mem to `.saga/manifest.json` and re-runs `saga install --harness <h>`, which writes the composed hook entries (contracts §1, §9) |

Exit codes follow the uniform table of contracts §4: 0 ok; 1 stale or pending records present under `check --strict`; 2 usage or record parse failure; 3 write refused because masking fired (§8); 6 environment refusal (symlinked store, record outside repo root, unreadable). `--json` everywhere; `inject --json`:

```json
{"schema": "saga.mem.inject/1", "session_id": "…", "turn": 41,
 "request": {"tool": "edit", "path": "src/import/parse.ts"},
 "records": [{"id": "01J9Z6Q0", "kind": "constraint", "score": 8, "bytes": 96}],
 "text": "saga mem (3 records for edit src/import/parse.ts):\n- …",
 "bytes": 612, "tokens_est": 153,
 "budget": {"session_bytes": 2822, "session_cap_bytes": 4800, "cap_reached": false},
 "omitted": {"stale": 1, "pending": 2, "in_window": 1, "over_cap": 0}}
```

Every schema is `<name>/<major>`; consumers reject unknown majors.

---

## 7. Portability

### 7.1 File format versioning

`schema = "saga.mem.record/1"` per file. A reader that meets a higher major refuses the file with exit 2 and names it; a lower major is upgraded in memory and rewritten only by `saga mem check --fix`, so a checkout never changes on read. Additive fields within a major are ignored by older readers. The index and session files are derived and carry their own majors; they are deleted and rebuilt on any mismatch.

### 7.2 Branches and worktrees

| Situation | Behaviour |
|---|---|
| Two branches add records | ULID file names never collide; git merges both without conflict; index rebuilds |
| Same record edited on two branches | ordinary one-file conflict, human-resolved; `check` rejects a file that still contains conflict markers (exit 2) |
| Deleted on one branch, edited on another | the tombstone wins: `check` treats a record with a tombstone as `tombstone` regardless of file content, and `prune --closed` removes both |
| Duplicate text after a merge | `check` reports pairs with identical normalised `text` and proposes `drop` of the newer one; never automatic |
| Worktrees | share the committed store through the repository; each worktree has its own `.saga/observed/`, so session ledgers and state blocks never cross |
| Rebase | file-level, no special handling; `turn_hash` and `evidence_hash` are content hashes, not commit hashes, so they survive rewritten history |

### 7.3 Per-repo versus per-user

| Store | Path | Contents | Committed |
|---|---|---|---|
| repo | `.saga/mem/` | conventions, constraints, decisions, procedures, pitfalls, env facts about the repo | yes |
| user | `~/.saga/mem/<repo-id>/` | records added with `--user`: personal procedures ("I run tests inside docker"), personal pitfalls | never |

Both are read by `inject`, `search` and `state`; equal rank, repo first on ties (§5.3). `constraint` and `decision` records are repo-only: a constraint one person cannot see is the doc 02 item 12 failure in another form. The user store follows the same schema and checks. `SAGA_MEM_USER_DIR` is honoured under the same conditions as gate-spec §8's approval store (real, owner-private, outside the repo root, no symlinks; exit 6 otherwise).

### 7.4 Cross-harness

Records contain no harness-specific names: tool scope uses the canonical classes (§5.1) and adapters translate. `source.harness` is informational. A session started in Claude Code and resumed in Codex reads the same store and, given `saga trace`'s portable resume (doc 09 §3.7), the same state block. The bench's `harness.json` disclosure (bench-spec §6.2) records which mem hooks bound, so a cell where the preamble was `unavailable` is compared only with cells of the same shape.

---

## 8. Privacy

| Control | Rule |
|---|---|
| Masking on write | every `text`, `quote`, `signature`, `fix`, `context`, `command` passes `saga guard mask` (gitleaks-class rules plus configured patterns, doc 09 §3.5) before it is written; a hit rejects the write with exit 3 and the reason names the rule, never the matched value |
| No placeholders in records | unlike guard's reversible placeholders on tool results, a record that needed masking is not stored: memory is durable and committed, and a placeholder would be re-expanded by nobody |
| Credential paths | `scope.paths` and `derived_from.path` matching guard's credential-file deny list (`.env*`, `*.pem`, key stores; ADR 0006) are rejected |
| Untrusted text | all string fields control-, line-separator- and bidi-stripped on read and write (gate-spec §8); records are data, never instructions, and the injection header never says otherwise |
| Repo-supplied records | a freshly cloned `.saga/mem/` is untrusted like an inherited contract: `saga mem check` runs before the first injection in a repo, and records whose `text` matches guard's instruction-injection patterns ("ignore previous", "run the following", URLs with credentials) are quarantined as `stale: suspicious` (doc 07 §6 item 12 class; ruflo #1375) |
| Audit trail | `mem-session-<id>.json` records every injection (record id, turn, tool class, sha256 of path or command, bytes, placement) and every capture (record id, origin, turn_hash); `saga trace` copies the per-turn totals into the ledger under `component = mem.state | mem.targeted | mem.preamble | mem.query` |
| Retention caveat | masking reduces what leaves the machine; it does not change provider retention. Covered models (Fable, Mythos) retain 30 days even under ZDR since 9 Jun 2026 (doc 06 C.1). Stated in the README, not softened here |
| Miss rate | pattern masking has a non-zero miss rate (gate-spec §8 gap 7); the positive-control suite (§9.1) measures it and `doctor` prints it |

---

## 9. Test plan and bench ablation

### 9.1 Tests for the layer

| Test | Fixture | Pass condition |
|---|---|---|
| Record round-trip | 6 kinds × valid and invalid files | parse, write, re-parse byte-identical; invalid exit 2 with field named |
| Freshness | repo with a moved file, a removed script, a changed lockfile | each check fires with the documented reason; nothing injected |
| Statement detector | labelled corpus of 500 user prompts `default` (positive and negative controls in-tree) | precision and recall printed; release blocks below precision 0.95 `default`; negatives ("never mind", "always fails on CI", quoted code) produce no record |
| Proposal isolation | `mem.propose` 25 times | 20 stored, 5 `pending_full`; `inject` and `state` never include any |
| Selector | 50 recorded inject requests against a 400-record store | ranking equals golden output; ≤ 50 ms per call; window and cap honoured |
| State block | contract with 4 gates, 12 constraints, 30 changed files | ≤ 2,400 bytes; trimming order as §4.2; `--full` complete |
| Adapter conformance | recorded hook payloads per harness, per event, including the placement probe | output JSON equals golden; never reads `transcript_path`; PreCompact exit is 0 on every input including a broken store |
| Sub-agent preamble probe | scripted fake harness that echoes the rewritten `Agent` prompt | preamble present verbatim; otherwise `unavailable` recorded |
| Masking | positive-control corpus from guard | every secret rejected; zero false rejections on the clean corpus |
| Merge | two branches, add/edit/delete/tombstone combinations | index rebuilds; tombstone wins; conflict markers exit 2 |
| Windows | the full suite on Windows CI | passes (doc 07 §6 item 14; unlazy #30) |

### 9.2 Bench ablation

Run under bench-spec's clean room, paired arms, control-arm blocking (`dir-deny:.saga/mem`, `path-shim:saga`, hooks replaced by logging no-ops, MCP `mem.*` unregistered). Ladder rung: `base+gate+guard+index` vs `+mem`, and three sub-arms so the components of mem are separable:

| Arm | Contents |
|---|---|
| M-state | state block and preamble only (M1 scope) |
| M-target | targeted injection only, store pre-seeded from the task |
| M-full | both |
| M-standing (control for #303) | the same records as a static section in the instruction file, re-sent every turn by a `UserPromptSubmit` hook |

Metrics, all with bench-spec §5.5 statistics (report keys are the mem row of bench-spec §5.11):

| Metric | Definition | Reported for |
|---|---|---|
| Compliance curve | bench-spec §5.9: `c(τ)`, AUC, decay `β`; rules are prose-only in every arm | all arms; headline for M3 |
| Compaction-survival rate | over runs with ≥ 1 compaction (forced by the harness's compaction threshold set low in `harness.json`, `compaction_threshold`): fraction of `(run, constraint-rule)` pairs applicable after the compaction event that are compliant | M-state vs control; per harness, per placement (§4.4, §4.7) |
| Tokens injected per session | ledger sums for `mem.state`, `mem.targeted`, `mem.preamble`, `mem.query`; ratio of `bytes/4` estimate to harness token counters | every arm |
| Cache-read ratio | ledger `cache_read / (cache_read + fresh_input)` per turn | M-target vs control (doc 07 §8, #91514) |
| False-injection rate | injections whose record scope is not touched by the next 5 tool calls `default` (path not read or edited, command family not run) divided by injections | M-target, M-full |
| Preamble effect | sub-agent scope-violation rate (bench-spec §5.7) on tasks that spawn sub-agents | M-state |
| Cost per solved, pass^k, false-done | bench-spec §5.2, §5.4, §5.6 | every arm; H3 equivalence |

Tier: `dev` for go/no-go, `publish` for the badge (bench-spec §4.4). Every cell where `component_used` is false is reported as "no exposure" (bench-spec §4.2).

### 9.3 What the tests do not validate

That a record's text is true. That the pattern list generalises to prompts in other languages (patterns are English `default`; the corpus must say which languages it covers). That the model reads an injected record: only the bench does.

---

## 10. Open problems

| # | Problem | Experiment that settles it |
|---|---|---|
| 1 | Does adjacency drift less than standing injection? (doc 09 §7 item 9) | M-target vs M-standing on the compliance curve, same records, same tokens per session matched by cap; the M3 exit criterion |
| 2 | Pre-tool versus post-read placement on Claude Code, given the doc discrepancy (§5.6) | same records, two placements, AUC and cache-read ratio; the probe result per version is logged in `harness.json` |
| 3 | Re-injection window `W` | sweep `W ∈ {4, 8, 16, never}`; plot AUC against tokens per session; choose the knee |
| 4 | Detector precision on real prompts | 500-prompt labelled corpus, then a `user`-tier field study where `drop` events per recorded constraint are counted (a drop is a false positive) |
| 5 | Cache cost of tool-adjacent injection (doc 09 §7 item 3) | ledger cache-write tokens per turn with and without M-target on the same task set |
| 6 | Do procedures help coding at all, given AWM and ACE are web agents and SWE-Exp is confounded (doc 05 §3.3)? | M-target with procedures only versus with procedures excluded, cost per solved and pass^k |
| 7 | Cross-session transfer | SWE-Bench-CL-style pairs: two tasks in the same repo run sequentially, store carried over versus wiped; pass^k and tokens on the second task |
| 8 | Survival without a post-compaction hook (Gemini `BeforeAgent` fallback; no hooks) | compaction-survival rate per adapter mode; if the fallback is within the noise floor of bare (3.5 to 4.5 points, doc 06 A.10), say so |
| 9 | Store growth and false injection over months | longitudinal `user`-tier runs on three real repos; false-injection rate and stale count against store age |
| 10 | Does `updatedInput` hold across harness versions? | the install probe runs in the canary (ADR 0007); a version that stops honouring it flips `subagent_preamble` to `unavailable` and raises a trace event |
| 11 | Multilingual statement detection | corpus extension; until then non-English constraints enter only via `saga mem add` |

---

## 11. Experimental register

Mechanisms in this spec that carry no evidence and are labelled experimental until §9.2 reports: targeted injection ranking weights (§5.3), the re-injection window (§5.5), automatic pitfalls from failure signatures (§3.4), the Gemini `BeforeAgent` fallback (§4.4), and the one-line instruction-file pointer (§4.7). The state block content list (§4.1) is user-validated by #34556 but its effect on any outcome metric is unmeasured until M1's bench run.
