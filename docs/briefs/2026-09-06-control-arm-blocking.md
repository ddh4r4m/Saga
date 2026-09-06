# Brief: finish control-arm blocking (docs/12 readiness row 3) and the contract leak scan (row 12)

Date: 2026-09-06. Owner of the decision: saga (Fable). Implementer: saga-opus. Two commits, one per part.

## Part 1: control-arm blocking

### 1. Problem

bench-spec §4.2 requires the bare arm to be blocked at every surface the component lives at, with each block instrumented, and §10.2 requires a blocking test: a fake agent tries every reach path in the control arm, every attempt is logged, none succeeds, and `blocked_reach_attempts` equals the attempt count. Today (`internal/bench/adapter/claude.go` `stageShim`, `run.ControlBlocks`) only the `PATH` shim exists, `blocked_reach_attempts` counts shim hits alone, and there is no blocking test. docs/12 row 3 is open and must close before the pilot.

### 2. Decisions (taken)

Per §4.2 surface, on the worktree substitute of row 5 (no container, macOS host):

| surface | block in the bare arm | instrumentation |
|---|---|---|
| CLI binary | `PATH` shim, as today | shim log line per invocation (exists) |
| MCP server | not registered in the generated settings and the private config dir; nothing to add, but assert it in the blocking test | none |
| hooks | absent from the generated settings; the test asserts the settings carry no `hooks` key in the bare arm | none |
| files (`.saga/`) | a sentinel directory `.saga` created in the workspace with mode `0o000` before the agent starts, so a read or write attempt fails with EACCES instead of silently creating the layout; removed (mode restored, then deleted) after Collect and before Diff and grading | the sentinel cannot count opens on macOS (no audit log); record `sentinel: "present"` and `sentinel_open_count: null` with reason `no audit log on host` in the disclosure's `blocks` detail |
| prompt text | the bare arm's prompt is prompt.md plus the protocol sentence only; `harness.json.prompt_hash` already records it | none |

`blocked_reach_attempts` stays the shim count; the disclosure gains a `blocks_detail` block naming each surface, its block and its instrumentation status, so the report can say what was blocked rather than imply it. `ControlBlocks` in `run.go` lists every surface applied: `path-shim:saga`, `settings:no-hooks`, `settings:no-mcp`, `sentinel:.saga`.

The `Diff` in `internal/bench/task/stage.go` already excludes `.saga`, and grading applies the diff to a clean checkout, so the sentinel never reaches the oracle. Still remove it before Diff, because `git add -A` walks the tree and an unreadable directory can make git print warnings on some versions; the test proves the diff of a run with the sentinel is byte-identical to one without.

Under `acceptEdits`, Claude Code's own `Write` to `.saga/x` fails on the sentinel with a permission error the model sees; that is the intended signal. Do not try to intercept it further.

### 3. Changes

- `claude.go` `Prepare`, bare path: after `stageShim`, create the sentinel; a new `Cleanup`-style step, or the existing post-Collect point in `run.go`, restores mode `0o700` and removes it before `task.Diff`. Put the removal where a panic or context cancel still runs it (defer in the run function).
- `adapter.go` `Disclosure`: `blocks_detail` block as above; validate against `schema/bench/1/harness.json` and extend that schema if the block is not allowed by it (say so in the commit message either way).
- `run.go` `ControlBlocks`: the four entries.
- `bench/results` and older archives are not migrated.

### 4. Tests

1. Unit, adapter: bare `Prepare` creates `.saga` with mode `0o000`; treatment `Prepare` does not; cleanup restores and removes it; running cleanup twice is harmless.
2. The §10.2 blocking test, in `internal/bench/run` with the replay adapter or a fake adapter: a fake agent script executed in the prepared bare workspace with the prepared env tries, in order: `saga gate check`, `saga --version`, `cat .saga/contract.md`, `mkdir -p .saga && echo x > .saga/probe`, `ls .saga`, and reads the generated settings for a `hooks` or `mcpServers` key. Expect: shim log has exactly 2 lines, every file attempt fails (non-zero exit, nothing created under `.saga`), settings have neither key, `blocked_reach_attempts` = 2, `ControlBlocks` all present in `harness.json.blocks`, and the archive's `workspace.diff` equals the diff of the same workspace prepared without the sentinel.
3. Treatment arm control: the same fake agent in an arm B workspace succeeds on `saga --version` and can read `.saga/contract.md` (so the test proves the block is arm-specific, not an environment accident).

### 5. Docs, same commit

- `docs/specs/bench-spec.md` §4.2: add a "host substitute" note under the table: the sentinel replaces the sandbox audit count with `present` and a null count, disclosed per run.
- `docs/12-experiment-protocol.md` §10 row 3: mark closed 2026-09-06 with the commit hash and the substitute wording.
- `docs/specs/IMPLEMENTATION-STATUS.md`: the bench 4.2 line.

## Part 2: contract leak scan (row 12)

### 1. Problem

`internal/bench/task/verify.go` runs the §2.4 leak scan (no `gold.patch` line of 20 or more non-whitespace characters, no oracle file name) over `prompt.md` only. The staged contract is also agent-visible in arm B, so a gold line in `contract.md` leaks the same way. docs/12 row 12 is open.

### 2. Decision

The leak scan runs over `prompt.md` and `contract.md` with the same two rules; a hit in either exits 7 with the file named in the reason. Contract lines that legitimately quote the request (`FROM: R<n> "<span>"`) quote `prompt.md`, not `gold.patch`, so they cannot trip the gold rule; the oracle-name rule applies to them unchanged.

### 3. Tests

Extend `TestVerifyDefective` with "gold line in contract" and "oracle name in contract" cases expecting `cli.ExitContamination` and a reason containing `contract.md`; add the valid twin (a contract with a `FROM:` span quoting prompt.md) that passes. Run `verify-task` over all 30 corpus tasks (`go test ./internal/bench/task` covers `TestVerifyCorpus`) and report the result; if any existing contract trips the new rule, stop and message saga with the task id and the line, do not edit the task.

### 4. Docs, same commit

bench-spec §2.4 leak-scan row: "in `prompt.md` or `contract.md`". docs/12 row 12: closed with the hash. IMPLEMENTATION-STATUS one line.

## Out of scope

Containers, hook no-op scripts (there are no hooks to replace in a private config), `hook-path pins`, tasks 31 to 40.

## Report

Both hashes, the blocking test's observed counts, the corpus verify result, anything left out. Under 40 lines.
