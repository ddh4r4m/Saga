# Implementation status

*M0 step 1, 2026-09-03. What the Go module at the repository root implements against each spec section, with the gaps. Updated with every milestone step; the bench decides what is stable (ADR 0001).*

Module `github.com/ddh4r4m/saga`, Go 1.26, `CGO_ENABLED=0`. Dependencies (ADR 0008 allow-list): `github.com/BurntSushi/toml`, `github.com/zeebo/blake3`. Build with `make build`; `go vet ./...` and `go test ./...` (also under `-race`) pass.

## Layout (ADR 0008 section 3)

| Path | Status |
|---|---|
| `cmd/saga/main.go` | version stamp via `-X main.version`, calls `internal/cli/cmd` |
| `internal/cli/exit.go` | the one exit-code table and precedence 6, 7, 2, 3, 4, 5, 1 |
| `internal/cli/cmd/` | subcommand tree on the standard `flag` package (cobra not needed yet) |
| `internal/hook/` | composed entry: deadline, layer chain, merge rules, one JSON object on stdout |
| `internal/hookio/` | harness-neutral input, output, merge and layer interface shared by adapters and layers |
| `internal/harness/claude/` | Claude Code stdin to neutral input and neutral output to stdout, 150 lines |
| `adapters/claude-code/` | settings fragment, `saga install`, read-only registration check for doctor |
| `internal/trace/` | events, chain, rotation, masker, prices, usage, ledger, transcript normaliser, pins, budget, recorder layer, doctor |
| `internal/canon/`, `internal/store/`, `internal/schema/` | canonical JSON and hashes; `.saga/` layout, config, observed state, hostile-shape check; embedded schemas and strict validator |
| `schema/trace/1/`, `schema/doctor/1/` | envelope, 16 body schemas, ledger, pins, doctor |
| `fixtures/trace/` | Claude Code transcript fixture for the ledger test |
| `scripts/bench-hook.sh`, `Makefile` | build, test, lint (gofmt and vet; golangci-lint is not on the allow-list), cold-start measurement |

## Cross-spec contracts

| Section | Implemented | Gaps |
|---|---|---|
| 1 Hook composition | one binding per event via `saga hook claude-code <event>`; chain runs installed layers in order, deny or block short-circuits; PreCompact never blocks; SessionEnd bound for trace in addition to the table | only trace is installed, so the order table is exercised with one layer; the manifest's `layers` list is read but no other layer exists |
| 1.1 Merge rules | decision precedence, reason concatenation, `additionalContext` prefixed `saga <layer>:`, `updatedInput` field-wise union over the original `tool_input`, two layers on one field is exit 2 with a deny; entry-side deadline from `[hook] deadline_ms` (default 5,000) emits `deny` or `block` with `saga: deadline` on PreToolUse and Stop; malformed stdin fails closed the same way with exit 2; exactly one JSON object on stdout | per-event token ceilings are enforced only for trace's own 400-token share; the 10,000-character harness cap is not checked; UserPromptSubmit overrun allows (blocking would erase the user's prompt, C26) |
| 2 `.saga/` layout | `saga init` writes `.saga/.gitignore` (`trace/ index/ shape/ snap/ observed/ audit.jsonl`), `config.toml` skeleton, `trace/{sessions,pins,prices}`, `observed/`; idempotent; hostile-shape check (symlink, FIFO, device, hard link) before every open | `~/.saga/` is not created |
| 3 Identity and encoding | canonical JSON, `sha256:` ids, `blake3:` helper, ULID fallback session id, one turn counter in `observed/session-<id>.json`, `ceil(bytes/4)`, repo-relative paths with the `«outside-repo»/<hash>` rule, control and bidi stripping | |
| 4 Exit codes | uniform table in `internal/cli/exit.go`, used by every command and by the hook entry | |
| 6 Event catalogue | all 16 types have body schemas; the hook writes `session`, `turn`, `tool_call`, `tool_result`, `model_call`, `budget`, `compaction`, `subagent` | `edit`, `gate`, `guard`, `mem_inject`, `drift`, `checkpoint`, `canary`, `route_decision` are never emitted yet |
| 7 Ledger attribution | closed key set, every key present at 0.0; usage object per 7.2; trace's 400-token session share counted in `observed` | attribution is the section 3.5 estimate over prompt, tool-result and trace bytes only; no calibration |
| 8 Agent-forbidden commands | string match on `Bash` and `PowerShell` commands in PreToolUse, deny with exit 3; edits to the protected `.saga/` and `.claude/settings*.json` files denied | post-expansion classification is guard (M1) |
| 9 Install and probe order | step 1 (`init`) and step 2 (`install`, `doctor`) | `hooks fire` probe not run (needs the bench runner's `-p` harness) |
| 11 Schema naming | ids and majors per the registry; unknown field is a validation failure; every event and ledger row is validated on write | `saga.trace.report/1`, `saga.trace.checkpoint/1`, `saga.trace.bundle/1`, `saga.trace.claims/1` have no schema file yet |

## trace-spec

| Section | Implemented | Gaps |
|---|---|---|
| 2.1 to 2.4 Envelope, chain, ordering | envelope per 2.8, `prev` chain from `sha256:genesis`, hash of canonical JSON with `hash` removed; single writer per session under `flock` on `LOCK`; head recovered from the last line on every spawn; `mono_ns` from the first event's wall clock | `merged_into` for late transcript events unused; Windows lock is an exclusive-create spin |
| 2.5 Size caps | 4 KiB inline, blobs under `blobs/<sha256>`, head plus tail above 8 MiB with `truncated: true`, 16 KiB line cap replaced by an `event_oversize` tool_result, rotation at 64 MiB with the chain continuing | tool results are truncated by bytes, not line-aware |
| 2.6 Redaction | built-in rules: private key blocks, Anthropic, OpenAI, GitHub, AWS, Google, Slack keys, JWTs, `*_SECRET|TOKEN|PASSWORD|API_KEY=` assignments; `SAGA_MASK_<TYPE>_<8 hex>` stable per session; `masked_count` per event; hashes computed after masking | IPv4 and email classes, the positive and negative corpora of 11.1 |
| 2.7 Storage | `sessions/<id>/events.NNNNNN.jsonl`, `blobs/`, `ledger.jsonl`, `LOCK`, `pins/current.json`, `prices/<sha256>.toml` | `checkpoints/`, `canary/`, `index.sqlite`, `saga trace rebuild` |
| 3.1 to 3.3 Usage, ledger row, price table | Anthropic, OpenAI, Google mappings with `null` plus `_reason`; 5m/1h split read from the nested `cache_creation` object (harness-probes P8), unsplit totals go to the pinned TTL with `split_reason`; ledger row per 3.2 with `context_delta`, `cache_hit_ratio`, `cum_usd`; `saga.trace.prices/1` TOML seeded from doc 06 A.1 to A.4 with source and date, persisted under its hash, unpriced model is `null` never 0, long-context tier applied | `saga trace prices update`; `mixed_price_tables` flag is in the schema but not set; `ttl_inferred` always null |
| 3.4 Reconciliation | | not started |
| 3.5 Attribution | estimate scaled to `context_delta`, remainder to `harness` | `bare` adapter calibration |
| 3.6 Budgets | `[trace.budget]` parsed; soft, compact and hard crossings write a `budget` event once each and one feedback line (60 est. tokens or fewer); hard with `hard_action = "stop"` denies the next tool call and blocks Stop (at most 6 blocks); `saga trace budget --session-usd x` edits the config line | `task_usd`, `turn_context_tokens`, `--raise`, `ack` |
| 3.7 Report | `saga trace ledger [session] [--json]` prints the token, usd and share table, cache hit ratio, per-call context and the estimated attribution | `--by` pivots, top tool results by bytes, `saga.trace.report/1` schema file |
| 4.1 to 4.3 Pins | `saga.trace.pins/1` written at every `session` event with settings and hooks hashes, effort from hook input, price table, saga version and components; `changed` keys recorded | harness version and binary hash (doctor prints the version but the hook does not exec `claude`); change-detection rules and advice table |
| 4.4 Canary | | M0 step 2 with the bench |
| 5 Watchdog and claims | `claimed_done` is `null` with a reason on every turn end | all signals and claim checks (M1) |
| 6 Replay, fork, checkpoint | | not started; checkpoints wait for `saga snapshot` (guard) |
| 7 Doctor | `saga doctor [--json]` in `saga.doctor/1`: Go version, binary sha256, harness on PATH with version, `.saga` presence, config parse and mode, hook registration per event across `.claude/settings.local.json`, `.claude/settings.json`, `~/.claude/settings.json` (read-only), usage source over the last 5 sessions, pins, price table, retention count; exit 0, 1 or 6 | hooks fire, gate fixture, cache, drift, canary, incidents, uninstall proof, `--fix` |
| 9.1 CLI | `init`, `install --harness claude-code [--dry-run] [--shared]`, `hook`, `trace tail|ledger|budget|verify|prices|doctor`, `doctor`, `version` | `pin`, `canary`, `replay`, `fork`, `ack`, `claims`, `export`, `import`, `rebuild`, `prune`, `uninstall`, `serve`, `proxy`, `ci` |
| 9.3 Claude Code hook points | tool call and result from `PreToolUse`/`PostToolUse` stdin, turn boundaries, `last_assistant_message` at Stop and SubagentStop, usage from `transcript_path` deduplicated on `message.id`, sub-agent usage from `agent_transcript_path` at SubagentStop (C33), compaction from `PreCompact`/`PostCompact`, resume input extras (`context_tokens`, `prompt_cache_likely_expired`, `seconds_since_last_response`, `estimated_cache_write_usd`, harness-probes P7) recorded on the `session` event | Codex and Gemini adapters (M1); stream-json and proxy sources |
| 11.1 Tests | chain and tamper (`verify` exits 5 naming the seq), rotation across segments with recovery, oversize replacement, masker positive and negative, usage mappings, ledger arithmetic against `fixtures/trace/claude-transcript.jsonl` (5m and 1h split, dedupe, unpriced model, cumulative), long-context tier, budget crossings, pins diff, hook deadline and single-JSON stdout and fail-closed stdin, merge conflict, adapter install idempotence and foreign-hook preservation, end-to-end session through the CLI | 500-session synthetic suite, determinism across hosts, normaliser goldens, corpora |

## Hook cold start (trace-spec 11.1, ADR 0008)

Measured with `make bench-hook` on this machine (Apple silicon, macOS, Go 1.26.3, `CGO_ENABLED=0`, `-trimpath -ldflags "-s -w"`, binary 3.3 MB). Each spawn is a `PreToolUse` for `Bash` in a scratch repository with `.saga/` initialised: the entry reads stdin, takes the session lock, recovers the chain head, masks and hashes the arguments, validates against the schema, appends one event and rewrites `observed/session-<id>.json`. Timings include Python's `subprocess` overhead.

| Run | p50 | p95 | min | max |
|---|---|---|---|---|
| 50 spawns, run 1 | 6.9 ms | 10.5 ms | 6.3 ms | 12.8 ms |
| 50 spawns, run 2 | 6.8 ms | 7.5 ms | 6.2 ms | 9.0 ms |
| 50 spawns, run 3 | 6.8 ms | 8.7 ms | 5.8 ms | 9.3 ms |
| 50 spawns, after race-fix rebuild | 6.9 ms | 8.7 ms | 6.1 ms | 8.8 ms |
| 200 spawns | 6.9 ms | 8.8 ms | 6.0 ms | 12.0 ms |
| baseline `saga version` (spawn only), 50 | 5.4 ms | 6.2 ms | | |

Trace's own work per invocation is therefore about 1.5 ms at p50 and about 2.5 ms at p95, inside the section 11.1 budget of p50 5 ms and p99 20 ms added per hook. The whole-process figure is 2 ms above the ADR 0008 stub measurement (4.7 ms), which is the cost of the lock, the observed-state rewrite and schema validation. The 10,000-call run on the 50k-file repository and the p99 figure are the step 2 measurement with the bench runner.

## Open items for M0 step 2

1. Bench runner and archive (bench-spec 3, 8); canary task set; `hooks fire` probe.
2. `saga uninstall --dry-run` listing exactly what `install` wrote; `--fix` in doctor.
3. `saga trace rebuild`, `prune`, `index.sqlite`; retention defaults.
4. Pins: harness version and binary hash at session start without executing the harness (read the npm package metadata), change-detection rules of 4.2.
5. Probe whether Claude Code honours a deny JSON object on a non-zero exit code (contracts 1.1 says JSON plus the uniform code; C25 says JSON is read on every exit code). Until probed, the entry exits 3 or 1 with the JSON; if the probe fails, switch to exit 2 with the reason on stderr.
6. Schema files for `saga.trace.report/1`, `saga.trace.checkpoint/1`, `saga.trace.bundle/1`, `saga.trace.claims/1`.
