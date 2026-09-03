# `saga shape`: technical specification

*v0.1, 2026-09-03. Implements doc 09 §3.6 and milestone M4 (doc 09 §5). Seeded from the agent's own account of noisy tool output, edit retry loops and slow feedback loops (doc 02 items 6, 7, 14), the SWE-agent ACI ablations and the Claude Code 25k-token result cap (doc 03 §1.2), the context-rot evidence (doc 03 §2.3), the truncation, context-editing and edit-format rows of doc 05 §5, the content-addressed cache pattern of doc 05 §4.2, the honest numbers for rtk, context-mode and Squeez in doc 04 §2.3, and the edit-tool false-success cluster in doc 07 (cline #4384). Reuses the adapter contracts of `gate-spec.md` §6, the event format and ledger of `trace-spec.md` §2 to §3, the masking pipeline of `guard-spec.md` §4, and the content addressing and `impact` tool of `index-spec.md` §2 and §5.*

---

## 1. Purpose, non-goals, evidence

### 1.1 Purpose

Shape sits between a tool's raw output and the model's context. It does four deterministic things: parse known runner, compiler and linter output into a fixed schema; truncate what it cannot parse without cutting a line or dropping the error; verify every edit against the working tree and echo the real diff; serve a content-addressed cache for repeatable commands. Every byte it removes stays retrievable by hash (`shape.more`), so the layer is lossless at the store and lossy only in what enters context.

### 1.2 Non-goals

| Not in shape | Reason |
|---|---|
| Lossy prose compression of code or logs (LLMLingua class) | Fine at 75% retained context, collapses below 50% (19.9% success at 35%), doc 04 §2.3; no positive code result, doc 05 §5 |
| Pruning of control context (system prompt, tool schemas, contract, state block) | "Dangerous applied to agent control context", doc 04 §2.3; structure-aware handling belongs to the harness |
| Learned pruners (SWE-Pruner, Squeez) as the default path | Promising (23 to 54% fewer tokens, doc 05 §5; 0.86 recall at 92% removal, doc 04 §2.3) but research, not products; listed in §11 as a future opt-in behind the bench |
| Semantic or fuzzy result caching | "Similar" is not "same" for code; GPTCache 54 false hits vs 3, doc 05 §4.2 |
| Replacing the harness's own `Read`/`Grep`/`Edit` tools | Hooks cannot; rtk's stated limit is that built-in tools bypass it, doc 04 §2.3. Shape shapes what hooks let it see and offers CLI equivalents |
| Being a build cache (ccache, Gradle, Turborepo) or test runner | Those cache artefacts; shape caches results of whole commands and never replaces the toolchain |
| Fixing the edit tool | Structural, harness-only (doc 09 §3.10); shape detects and reports |

### 1.3 Evidence map

| Mechanism | Failure mode | Source |
|---|---|---|
| Output parsers, first error with `file:line` | "40k characters, I skim, I miss the one line and fix the wrong thing" | doc 02 item 6; Anthropic "return only high-signal information", doc 03 §1.2 |
| Head/tail truncation, never mid-line, error-aware tail | Truncated signal; harness caps are blind | Codex 256 lines / 10 KiB head+tail, OpenClaw error-aware tail, doc 05 §5; Claude Code: MCP results capped at 25,000 tokens, Bash results inline to about 30,000 characters then a file path plus preview (harness-facts C28) |
| 100-line read windows, ≤50 search hits, grouping | Full-file reads and iterative search lose accuracy | SWE-agent: 100-line viewer 18.0% vs full file 12.7%; ≤50 summarised hits 18.0% vs iterative 12.0%, doc 03 §1.2 |
| Bounded per-turn output, ledger visibility | Accumulated tool output is the hidden cost driver | claude-code #16157 (724 reactions, 1,491 comments), doc 07 §6 item 1; context rot, doc 03 §2.3 |
| Edit verification: parse check and diff echo | "old_string not found", partial applies, success reported either way | doc 02 item 7; cline #4384 ("the agent often reports success either way, so the trace is the only ground truth"), doc 07 §2 and §6 item 4; gemini-cli #5251, #6766, #2553; SWE-agent lint-on-edit +3.0 pp, doc 03 §1.2 |
| Content-addressed result cache | Minute-long feedback loops in Swift, Kotlin, C++ make the agent "reason instead of run" | doc 02 item 14; `hash(tool, args, relevant file hashes)` pattern and Pest TIA 3 min to 5 s, doc 05 §4.2 |
| Comment stripper (post-edit) | Comment bloat on every method despite instructions | doc 06 D.2 item 13; **experimental**, §11 |
| Token accounting per call | "Token optimizer" claims without a profiler | rtk's own "dilutes at every step", doc 04 §2.3; ledger attribution, trace-spec §3.5 |

---

## 2. Output parsers

### 2.1 Pipeline

```
child stdout+stderr ──► guard masker (line-buffered, 4 KB lookback; trace-spec §2.6 fallback set when guard absent)
                    ──► ANSI strip, CRLF→LF, tee to log store as blake3/sha256 of masked bytes
                    ──► registry: signature → candidates → fingerprint → parser
                    ──► saga.shape.result/1 ──► renderer (text) ──► truncation (§3) ──► adapter (§7)
```

Masking precedes hashing and storage, so log hashes are publishable and no unmasked log exists on disk (trace-spec §2.3 rule; guard-spec §4.4). The full masked log is written to `.saga/trace/sessions/<s>/blobs/<sha256>` when trace is installed and to `.saga/shape/logs/<sha256>` otherwise; both are gitignored. Parsers operate on the ANSI-stripped view and every `log_range` refers to its line numbers.

### 2.2 Registry

Entries live in `core/shape/parsers/<family>.toml`, versioned; a parser version is part of the result and of the golden corpus (§10.1).

```toml
schema = "saga.shape.parser/1"
family = "pytest"; version = 3
signature = ["pytest", "py.test", "python -m pytest", "uv run pytest", "poetry run pytest", "tox"]
fingerprint.head = ['^=+ test session starts =+$', '^platform \S+ -- Python 3\.\d+']
fingerprint.tail = ['^=+ .*(passed|failed|error).* in [\d.]+s .*=+$', '^FAILED ', '^ERROR ']
fingerprint.min_hits = 2
summary = '^=+ (?:(?P<failed>\d+) failed)?,? ?(?:(?P<passed>\d+) passed)?,? ?(?:(?P<skipped>\d+) skipped)?,? ?(?:(?P<errors>\d+) errors?)?.* in (?P<secs>[\d.]+)s'
failure.start = '^_{3,} (?P<id>.+?) _{3,}$'
failure.location = '^(?P<path>[^:\n]+):(?P<line>\d+): (?P<assertion>\w*(?:Error|Exception|Failure).*)$'
failure.short = '^FAILED (?P<id>\S+)(?: - (?P<assertion>.*))?$'
error.line = '^E\s+(?P<message>.+)$'
```

Matching is two-stage. **Signature**: argv[0] after stripping wrappers (`npx`, `pnpm`, `yarn`, `bun`, `uv run`, `poetry run`, `python -m`, `./gradlew`, `xcrun`, `mise exec`, `nix develop -c`, `timeout`, `env`, `nice`) and shell prefixes (`cd x &&`, variable assignments). **Fingerprint**: regexes over the first and last 64 lines; `min_hits` distinct patterns must match. A signature hit with a fingerprint hit is `confidence: exact`; a fingerprint hit without a signature hit (`make test` running pytest) is `heuristic`; a signature hit whose fingerprint fails (a runner version the parser has not seen) is `heuristic` with only the summary and exit code trusted; no hit at all is `none` and §3 generic truncation applies. Fingerprints are exclusive by test: on the full corpus no log fingerprints as a second family (§10.1).

### 2.3 Families (M4 scope)

| Family | Signatures | Fingerprint anchors | Extracts |
|---|---|---|---|
| `jest` | jest, `npm test` when `package.json` test script is jest | `^(PASS\|FAIL) `, `^Tests:\s+\d+`, `^Test Suites:` | failing suites and test names (`● describe › it`), first `expect(...)` line with `at file:line:col`, summary counts, `Time:` |
| `vitest` | vitest | `^ (✓\|×\|❯) `, `^ Test Files `, `^ Tests `, `^ FAIL ` | as jest; `AssertionError` first line; `Duration` |
| `mocha` | mocha | `^\s+\d+ (passing\|failing\|pending)`, `^\s+\d+\) ` | numbered failures with title path, first `AssertionError` or `Error:` |
| `pytest` | see §2.2 | see §2.2 | node ids, `path:line: AssertionError`, `E ` lines, collection errors |
| `unittest` | `python -m unittest`, `manage.py test` | `^(FAIL\|ERROR): test_\w+ \(`, `^Ran \d+ tests? in`, `^(OK\|FAILED)` | test ids, traceback last frame `File "…", line N`, exception line |
| `gotest` | `go test`, `go test -json` | `^(--- \|=== )?(FAIL\|PASS\|ok\|FAIL\s)`, `^\s+\S+_test\.go:\d+:`, `^panic:` | `--- FAIL: TestX`, first `_test.go:line: msg`, panics with goroutine frame, build failures (`# pkg`), `ok pkg 1.2s` |
| `cargo-test` | `cargo test`, `cargo nextest` | `^running \d+ tests?$`, `^test .* \.\.\. (ok\|FAILED\|ignored)$`, `^test result:` | failed test names, `panicked at src/x.rs:12:5`, `assertion \`left == right\` failed` and the left/right lines, compile errors delegated to `rustc` |
| `gradle-test` | gradle, `./gradlew test`, maven, `mvn test` | `^> Task :\S+:test`, `^\S+ > \S+ FAILED$`, `^Tests run: \d+, Failures:`, `^BUILD (SUCCESSFUL\|FAILED)` | `Class > method FAILED`, the `AssertionError at File.java:NN` line, JUnit XML under `build/test-results/**` or `target/surefire-reports/**` when present (preferred: exact) |
| `xctest` | xcodebuild test, `swift test`, `xcodebuild build` | `^Test Case '-\[\S+ \S+\]' (passed\|failed)`, `^Test Suite '`, `^\*\* TEST (SUCCEEDED\|FAILED) \*\*`, `^Executed \d+ tests?` | test case ids, `path:line: error: -[Suite test] : XCTAssertEqual failed: …`, Swift Testing `✘ Test "x" recorded an issue at file:line`, `.xcresult` path when `-resultBundlePath` is set (exact via `xcrun xcresulttool get --format json`) |
| `dart-test` | `flutter test`, `dart test` | `^\d{2}:\d{2} \+\d+( -\d+)?: `, `^Some tests failed\.$`, `^All tests passed!$` | failing test names, `package:test` expect block (`Expected:`/`Actual:`), `test/x_test.dart 41:7`, `--reporter json` when present |
| `tsc` | tsc, `vue-tsc`, `tsc --noEmit` | `^\S+\(\d+,\d+\): error TS\d+:`, `^Found \d+ errors?` | `path(line,col): error TSnnnn: message`, `Found N errors` |
| `mypy` / `pyright` | mypy, pyright, `basedpyright` | `^\S+:\d+:(\d+:)? (error\|warning\|note):`, `^Found \d+ errors? in`, `^\d+ errors?, \d+ warnings?` | `path:line:col: error: message [code]` |
| `gobuild` | `go build`, `go vet` | `^# \S+$`, `^\S+\.go:\d+:\d+: ` | `path:line:col: message` grouped by package |
| `rustc` / `clippy` | `cargo build`, `cargo check`, `cargo clippy` | `^(error\|warning)(\[E\d{4}\])?: `, `^\s+--> \S+:\d+:\d+`, `^error: could not compile` | `E0xxx` code, message, `--> path:line:col`, primary label; `warning: N warnings emitted` |
| `javac` / `kotlinc` | javac, kotlinc, gradle `compileJava`/`compileKotlin` | `^\S+\.java:\d+: error: `, `^e: file://\S+:\d+:\d+ `, `^\d+ errors?$` | `path:line: error: message`; Kotlin `e: path:line:col message` |
| `swiftc` | swiftc, `swift build`, xcodebuild build phase | `^\S+\.swift:\d+:\d+: (error\|warning): ` | `path:line:col: error: message` and the caret line dropped |
| `dart-analyze` | `dart analyze`, `flutter analyze` | `^\s+(error\|warning\|info) • `, `^\d+ issues? found\.$`, `^No issues found!$` | `severity • message • path:line:col • rule` |
| `eslint` / `ruff` / `golangci-lint` / `swiftlint` / `ktlint` / `detekt` / `biome` | by binary | per-tool stylish/compact formats; `--format json` when present | findings grouped by rule with counts, first 20 with `path:line:col`, fixable count |
| `npm` / `pnpm` / `yarn` / `pip` / `uv` / `cargo-fetch` / `go-mod` / `pod` / `pub` / `gradle-deps` | install, add, sync, resolve, `pod install`, `flutter pub get` | `^npm (ERR!\|error) `, `^ERROR: `, `^error: `, `^added \d+ packages`, `^Resolved \d+ packages` | status, added/removed/changed counts, `ERESOLVE`/`peer dep` conflicts, first error block, warnings count, audit summary line |
| `grep` (search shaping, §6) | rg, grep, `git grep`, ag | `^\S+:\d+:` | hits grouped by file, capped |

Machine-readable reporters (`go test -json`, `dart test --reporter json`, `mocha --reporter json`, `vitest --reporter=json`, JUnit XML, `.xcresult`) are used when the agent's own command produced them; shape never appends reporter flags to the agent's command by default (`shape.prefer_structured = false`, §11), because a changed command is a changed observation for the trace.

### 2.4 Result schema `saga.shape.result/1`

```json
{
  "schema": "saga.shape.result/1",
  "command": "pytest -q tests/auth", "cwd_rel": ".", "shell": "bash",
  "family": "pytest", "parser_version": 3, "confidence": "exact",
  "status": "fail", "exit": 1, "signal": null, "duration_ms": 4210, "timed_out": false,
  "summary": {"total": 123, "passed": 118, "failed": 3, "skipped": 2, "errors": 0},
  "failures": [
    {"id": "tests/auth/test_session.py::test_refresh_expired",
     "path": "tests/auth/test_session.py", "line": 41, "col": null,
     "assertion": "AssertionError: assert 401 == 200", "log_range": [812, 861]}
  ],
  "failures_total": 3,
  "errors": [
    {"path": "src/api/session.ts", "line": 12, "col": 5, "code": "TS2345", "severity": "error",
     "message": "Argument of type 'string' is not assignable to parameter of type 'Token'.", "log_range": [3, 3]}
  ],
  "errors_total": 0, "warnings_total": 0,
  "log": {"hash": "sha256:3f9a12cd7b04…", "bytes": 38412, "lines": 1260, "stdout_bytes": 38102, "stderr_bytes": 310},
  "kept": {"head_lines": 0, "tail_lines": 30, "promoted_lines": [412, 519], "bytes": 1840},
  "tokens": {"raw_est": 9603, "shaped_est": 460},
  "cache": {"key": "sha256:…", "hit": false, "served_from": null, "inputs": 214, "index_version": "blake3:…"},
  "masked_count": 0
}
```

| Rule |
|---|
| `status` ∈ `pass`, `fail`, `error` (runner could not run: collection error, compile error inside the test build, missing module), `timeout`, `killed`, `unknown` (confidence `none`; derived from exit code only). |
| `failures` holds at most `shape.max_failures` (default 10); `failures_total` is the parsed count. Same for `errors` (default 20, aligned below the ≤50-hit ablation of doc 03 §1.2). |
| Paths are repo-relative with forward slashes when under the repository root, else verbatim. `line`/`col` are 1-based; `null` when the parser found none. |
| `assertion` is the first line that names an assertion or exception in the failure block, control- and bidi-stripped, capped at 300 bytes. |
| `log_range` is `[first_line, last_line]` in the stored log so `shape.more` can expand exactly that block. |
| `tokens.*_est = ceil(utf8_bytes / 4)` (gate-spec §9); the bench reports the ratio to measured tokens. |
| Parsed counts must reconcile with the summary line (`failed == failures_total`); a mismatch demotes `confidence` to `heuristic` and keeps the raw tail. |

### 2.5 Text rendering (what the model sees)

```
saga shape: FAIL  pytest  exit 1  4.2s  3 failed, 118 passed, 2 skipped  log 3f9a12cd7b04 (38 KB, 1260 lines)
FAIL tests/auth/test_session.py::test_refresh_expired
     tests/auth/test_session.py:41  AssertionError: assert 401 == 200
FAIL tests/auth/test_session.py::test_refresh_revoked
     tests/auth/test_session.py:58  AssertionError: assert None is not None
FAIL tests/auth/test_store.py::test_gc
     tests/auth/test_store.py:120  KeyError: 'expires'
--- tail 30 of 1260 lines ---
…
expand: saga shape more 3f9a12cd7b04 error:1 | L800-870 | tail:120 | grep:'KeyError'
```

The first line is fixed-format so a second parser (mem-spec §3.4 outcome capture, gate `CHECK:` evidence) can read status without the JSON. Rendering ceilings are in §3.3.

---

## 3. Truncation policy

### 3.1 Invariants

| Invariant | Test (§10.2) |
|---|---|
| Output lines are a subsequence of input lines; no line is rewritten | property |
| No line is cut, with one explicit exception: a single line longer than the byte cap is cut at the cap and marked `…[+N bytes, L<n>]` | property |
| The tail is kept in preference to the head for build and test output; the head for schemas, help text, listings (doc 05 §5) | per-family table |
| Lines matching the error regex inside the dropped middle are promoted, up to 20, with their line numbers, so an error above the tail is never lost (OpenClaw error-aware retention, doc 05 §5) | property |
| Every drop is announced once: `--- dropped L<a>-<b> (N lines, M KB); saga shape more <hash> L<a>-<b> ---` | golden |
| Shaping is idempotent and the union of `more` ranges reconstructs the log byte-exactly | property |

Error regex (generic, used when no parser matched and for promotion): `(?i)^(.*\b(error|fatal|panic|traceback|exception|failed|failure|assert(ion)?|unhandled|segmentation fault|E[0-9]{4}|TS[0-9]{4})\b.*)$` minus lines that only mention the words inside a path or a URL, tested on the negative corpus (§10.2).

### 3.2 Per-family defaults

Token figures are `bytes/4` estimates. The hard caps are below Claude Code's result caps (MCP 25,000 tokens via `MAX_MCP_OUTPUT_TOKENS`; Bash about 30,000 characters inline, 10,000 on a failing command, then a file path plus preview; harness-facts C28) so shape, not the harness, decides what is dropped.

| Family class | Default kept | Default ceiling | Hard ceiling |
|---|---|---|---|
| Test runners (parsed) | status line, ≤10 failures with first assertion, tail 30 lines | 2,000 | 6,000 |
| Compilers, type-checkers (parsed) | status line, first 20 errors, warnings count, tail 10 lines | 2,000 | 6,000 |
| Linters (parsed) | rule histogram, first 20 findings | 1,500 | 4,000 |
| Package managers (parsed) | status line, change counts, first error block, warnings count, tail 20 lines | 800 | 3,000 |
| Search (§6) | ≤20 hits grouped by file, ≤5 per file | 1,200 | 3,000 (≤50 hits, doc 03 §1.2) |
| Listings, help, schemas (`ls`, `--help`, `cat *.json`) | head 150 lines | 2,000 | 6,000 |
| Unknown (`confidence: none`) | head 50 lines + tail 150 lines + ≤20 promoted error lines; 8 KiB total (Codex uses 256 lines / 10 KiB, doc 05 §5) | 2,000 | 6,000 |
| File windows (§6) | 100 lines (SWE-agent viewer, doc 03 §1.2) | 2,000 | 12,000 (600 lines, index-spec §5.1) |

`[shape.ceilings]` in `.saga/config.toml` overrides per family; `scale = 0.5` halves every ceiling for harnesses without context editing (index-spec §6.3 uses the same knob for Codex). A ceiling can never exceed the harness cap recorded by `saga doctor`.

### 3.3 `shape.more`

`saga shape more <hash-prefix> <range>` reads the stored log and returns a window. Prefix is ≥12 hex (`3f9a12cd7b04`), ambiguity is exit 2.

| Range | Meaning |
|---|---|
| `L<a>-<b>` | lines a to b inclusive |
| `head:<n>`, `tail:<n>` | first or last n lines |
| `error:<i>` | the `log_range` of failure or error i in the result |
| `grep:<pattern>` | lines matching the RE2 pattern, ±2 context, ≤50 hits |
| `all` | refused above 12,000 est. tokens; the reply names the line count and suggests ranges |

Each window is capped at 300 lines / 4,000 est. tokens; the reply carries `next: L<b+1>-…` so the agent can page. Logs are retained 7 days or 200 MiB LRU, whichever first; a `more` on an evicted hash is exit 5 with the eviction time.

---

## 4. Edit-result verification

### 4.1 Mechanism

Two hook points per editor tool, plus a working-tree diff after any Bash call (Claude Code `PostToolUse` on `Edit|Write` does not fire when a Bash command rewrites the file, gate-spec §6.1).

| Step | Hook | Action |
|---|---|---|
| pre | `PreToolUse` / `Edit\|Write\|NotebookEdit\|MultiEdit\|apply_patch` | record `before_hash = sha256(bytes)` and the id returned by `saga snapshot take --reason shape` (the shared snapshot primitive, contracts §5, guard-spec §3.1); the pre-image is read back with `saga snapshot show <id> -- <path>`. Shape keeps no copy of its own; there is no `.saga/shape/edit/` directory |
| post | `PostToolUse` same matcher | read `tool_input` and `tool_response`; compute `after_hash`; compute the **expected** post-image from the pre-image and the tool's arguments; diff expected vs actual and pre vs actual; parse the actual with tree-sitter; emit `saga.shape.edit/1` |
| post | `PostToolUse` / `Bash` | `git diff --name-only` plus untracked scan against the turn's pre-image set; every changed file gets the parse check and a diff echo; no expected-image check (arguments unknown) |

Expected post-image per tool argument shape:

| Tool arguments | Expected |
|---|---|
| `old_string`, `new_string`, `replace_all` (Claude Code `Edit`) | exactly one occurrence replaced (or all when `replace_all`); 0 occurrences → `not_applied`; >1 without `replace_all` → `ambiguous` |
| `edits[]` (MultiEdit) | sequential application; the first failing edit names the index |
| unified diff / V4A patch (`apply_patch`, Codex) | hunks applied with zero fuzz; any fuzz used is reported per hunk |
| `content` (Write) | file equals `content` byte for byte after the harness's newline policy |
| `old_string`, `new_string`, `expected_replacements` (Gemini `replace`) | as the Claude Code row, with `expected_replacements` as the occurrence count and the tool's own normalisation recorded |

### 4.2 Verdicts

| Verdict | Condition | Adapter feedback |
|---|---|---|
| `applied` | actual == expected | none (silence is the default; ADR 0002 injects nothing unconditionally) |
| `applied_normalised` | actual == expected modulo whitespace, CRLF, trailing newline, or BOM (gemini-cli #2553 `�` class) | one line: what the harness normalised |
| `not_applied` | before_hash == after_hash and the tool did not report an error | **feedback**: `edit reported success but the file is unchanged: old_string not found (0 occurrences)`; this is the cline #4384 class |
| `partial` | some hunks or edits applied, others not | feedback with the applied and missing indices and the real diff |
| `ambiguous` | more than one occurrence, harness picked one | feedback naming the line chosen and the other candidates |
| `divergent` | file changed but not to expected and not a normalisation | feedback with the real diff; typical cause is a concurrent write or a fuzzy match |
| `parse_broken` | tree-sitter error or missing nodes increased against the pre-image | feedback: first new error `path:line:col`, the offending line; the edit stays (shape never reverts) |
| `unverified` | file over 8 MiB, binary, unsupported language, or pre-image missing | one line |

Parse check runs for the seven M1 languages of index-spec §3.1 (JS/TS, Python, Go, Rust, Java/Kotlin, Swift, Dart) plus JSON, YAML, TOML and Markdown front matter; other languages get the diff echo only. The comparison is against the pre-image's error count, so a file that was already unparsable is not blamed on the edit.

### 4.3 Diff echo

The echo is the exact `pre → actual` unified diff, 0 context lines, at most 40 changed lines / 1,500 est. tokens, then `… +N lines; saga shape more <hash> all` where the hash is the diff's own blob. It is emitted on every verdict except `applied` when `shape.echo = "on-mismatch"` (default) and on every edit when `shape.echo = "always"` (the bench arm that measures whether an always-on echo reduces repeated reads, doc 07 §6 item 4 Roo #11071).

### 4.4 Schema `saga.shape.edit/1`

```json
{"schema": "saga.shape.edit/1", "tool": "Edit", "path": "src/api/session.ts",
 "before_hash": "sha256:…", "after_hash": "sha256:…", "snapshot_id": "snap:01J6Y…:41:7c1a12cd7b04", "harness_claimed": "success",
 "verdict": "not_applied", "occurrences": {"expected": 1, "found": 0},
 "hunks": [], "added": 0, "removed": 0,
 "parse": {"lang": "typescript", "ok": true, "errors_before": 0, "errors_after": 0, "first_error": null},
 "diff_hash": null, "message_tokens_est": 38}
```

### 4.5 Interaction with gate and trace

Shape's post step runs before gate's `guard-diff --incremental` on the same event (contracts §1 PostToolUse order: trace, shape, gate, guard, mem, trace) and writes the trace `edit` event (trace-spec §2.2: `path`, `before_hash`, `after_hash`, `hunks`, `added`, `removed`, `by_tool`, all hashes `sha256:` per contracts §3); gate reads those hunks instead of re-diffing. The two feedback messages are merged by the adapter into one `reason` under gate-spec §9 (post-tool ceiling 200 est. tokens; shape's share is capped at 120 and the diff echo moves to `additionalContext` where the harness has it, §7). Gate's claim verification and G-LEDGER are unchanged; shape adds `false_success` counts to the trace so the Stop-time claim check can cite "N edits reported success with no change".

### 4.6 Comment stripper (experimental)

Runs after a verdict of `applied` or `applied_normalised` when `[shape] strip_comments = "added"` (default `off`) or when the contract says so. Rules: only comment nodes on lines the edit added; never doc comments in the language's doc convention (`///`, `/** */`, `"""` docstrings, `#` above `def` in Python when preceded by a blank line is left alone), license headers, pragmas (`eslint-disable`, `noqa`, `type:`, `@ts-ignore`, `nolint`, `swiftlint:`, `ignore:`, `pylint:`), `TODO`/`FIXME`/`NOTE`, or a comment that contains a URL or an issue id. The file is re-parsed after stripping; a parse regression reverts the strip, never the edit. The echo shows the stripped lines. Graduates from §11 when the bench shows no correctness change and a comment-density drop.

---

## 5. Content-addressed result cache

### 5.1 Key

```
key = sha256(canonical_json({
  "argv": [...], "shell": "bash|pwsh|cmd|none", "cwd_rel": ".",
  "env_fp": sha256(sorted selected env: PATH, HOME, LANG, LC_*, TZ, CI, NODE_ENV, PYTHONPATH, VIRTUAL_ENV,
                    GOFLAGS, GOOS, GOARCH, RUSTFLAGS, CARGO_*, JAVA_HOME, GRADLE_OPTS, DEVELOPER_DIR,
                    FLUTTER_ROOT, plus [shape.cache.env] additions),
  "toolchain_fp": sha256(versions of the family's binaries, probed once per session: node, python, go, rustc, java, swift, dart, xcodebuild),
  "platform": "darwin-arm64",
  "inputs_hash": sha256(sorted (path ‖ 0x00 ‖ blake3(bytes)) over the read set)
}))
```

### 5.2 Read set

Precision descends; the first available source is used and recorded as `inputs.via`.

| `via` | Read set | Available |
|---|---|---|
| `declared` | `[shape.cache.inputs]` globs for the command pattern, e.g. `"pytest*" = ["tests/**", "src/**", "conftest.py", "pyproject.toml", "pytest.ini"]` | when configured |
| `index` | for test commands: the selected test files (parsed from argv: `pytest path::name`, `vitest -t`, `go test -run`, `cargo test name`, `gradle --tests`, `swift test --filter`, `flutter test path --name`, the runner table of index-spec §5.5) plus their transitive `imports` closure from the graph, plus the family's config and lock files; for compilers: the package's source closure; for linters: the listed paths or the repo's lint globs | index regime `on` |
| `observed` | files the child actually opened (`strace -f -e trace=openat` on Linux; `fs_usage` needs root on macOS and is not used) | Linux, **experimental** |
| `tree` | every tracked and untracked non-ignored file (the index-spec §2.1 `index_version` rule, generated and vendored excluded) | always |

`tree` is coarse but correct: any edit invalidates. It still hits in the case doc 02 item 14 describes, an agent re-running the same slow build after reading, not editing. Unresolvable argv (globs the shell would expand, `$VAR`, `eval`) forces `tree`.

### 5.3 Hit and miss semantics

| Situation | Behaviour |
|---|---|
| `mode = serve` (default for allow-listed families), key present | return the stored `saga.shape.result/1` and rendering with `cache: hit (<age>, originally <duration>)` appended to the status line; child not executed; trace `tool_result` carries `served: cache` (trace-spec §6.1 marker) and `result_hash` of the original |
| `mode = advisory` | execute; compare with the stored result; append `cache: would-hit, identical` or `would-hit, DIFFERENT` (a stale-hit finding, §10.3) |
| `mode = off` | execute, store nothing |
| Second identical invocation in one session, inputs unchanged | serve, plus `inputs unchanged since <n>s ago; force: saga shape run --no-cache` |
| Third identical invocation, inputs unchanged | execute anyway and compare; a difference marks the entry `nondeterministic`, evicts it, and excludes the family from `serve` for the session (output-aware repeat detection, doc 09 §3.7) |
| Index `extractor_version` or `[index]` config change | all `via = index` entries dropped (index-spec §4.4) |
| Index version change | the read set is re-inferred at lookup; a changed read set is a miss; an unchanged set with unchanged hashes is a hit. Nothing is dropped on the version change itself |

Entries live in `.saga/shape/cache/<key[0:2]>/<key>.json` with the log blob by hash; LRU 500 MiB or 14 days. Failing results are cached like passing ones (they are just as deterministic given the inputs), which is what makes the third-invocation rule necessary.

### 5.4 Never cached

| Class | Detection |
|---|---|
| Network: `curl`, `wget`, `git fetch/pull/push/clone`, `npm install/publish`, `pip install` without `--no-index`, `docker pull/push`, `gh`, `aws`, `gcloud`, `kubectl` | guard's post-expansion classifier (guard-spec §2.4) classes `network_fetch`, `mutate_out_of_scope` (which includes the `ssh`/`docker exec`/`kubectl exec` remote wrappers of §2.3) and `destructive` |
| Time and state dependent: `date`, `uptime`, `ps`, `top`, `df`, `du`, `lsof`, `netstat`, `printenv`, `env`, `whoami`, anything reading `/dev/random`, `$RANDOM`, `--seed` absent on a randomised runner | shape's own `volatile` signature list (`core/shape/volatile.toml`); guard has no such class |
| Interactive or stdin-consuming commands, TTY-requiring commands | stdin not a pipe to shape, or the child opened `/dev/tty` |
| Side effects outside build and test output directories: writes to paths not under `[shape.cache.scratch]` (defaults: `node_modules/.cache`, `.pytest_cache`, `target/`, `build/`, `.dart_tool/`, `DerivedData/`, `__pycache__/`) | post-run diff of the working tree; a command that changed a tracked file is stored with `cacheable: false` |
| Commands the agent marked `--no-cache`, and any command containing `saga` | literal |

A command the classifier cannot resolve is not cached. Cache decisions are recorded in the result (`cache.reason` on miss) and in the trace.

### 5.5 Replay

Entries share `result_hash` with trace-spec §2.3, so `saga trace replay` (permissive) serves from the shape cache when the blob store lacks the payload, and a live cache hit is a replayable event: the recorded `tool_result` carries the original `seq` it was served from. Strict replay treats a served hit exactly as the original execution.

---

## 6. Search and read shaping

Search shaping applies to `rg`, `grep`, `git grep`, `ag`, `find`, `ls -R` output in wrapper mode and to the harness's `Grep`/`Glob` result when a hook exposes it (Gemini `AfterTool`; Claude Code `PostToolUse` `updatedToolOutput` once the `Grep`/`Glob` output shape is known, §7.1).

```
saga shape: grep  47 hits in 12 files (showing 20 in 6 files; ≤50 cap)  log 9c02be117a5d
src/auth/session.py  (9 hits)
   41: def refresh(self, token: str) -> Session:
   58:     raise RevokedToken(token)
  120:     store.gc(now)
   … 6 more: saga shape more 9c02be117a5d grep:'session.py'
src/auth/store.py  (5 hits)
   …
narrow: add a path prefix or a longer literal; symbol lookup: saga index search refresh
```

| Rule | Source |
|---|---|
| Hard cap 50 hits, default 20, grouped by file, ≤5 lines per file, sorted by hit count then path | ≤50 summarised hits 18.0% vs iterative 12.0%, doc 03 §1.2 |
| Hits inside generated, vendored, lockfile and minified files are counted but listed last and collapsed to one line per file | index-spec §4.6 classes |
| When the index is on and the query is a single identifier, the footer offers `saga index search`, which returns symbols not lines | index-spec §5.2 |

`saga shape read <path> [--lines a-b | --symbol path::qname] [--context n]` returns numbered lines with `file_hash` in the JSON form (index-spec §5.4 contract) so an edit can detect staleness. Default window 100 lines from the requested start; a symbol window when the index resolves the id, bounded by the symbol's range plus `context`. A window that would exceed 600 lines is refused with the line count and a range hint. Windows are not cached (they are already a hash lookup) but every window is a trace `tool_result` with `component: shape`.

---

## 7. Harness adapters

Adapters follow gate-spec §6: translation only, under 150 lines, recorded-fixture tested, no shaping logic. Every layer binds the same events and a harness accepts one `updatedInput` and one `reason` per event, so the installer writes a single `saga hook <harness> <event>` command per event. The normative order and merge rules are contracts §1: on PreToolUse **trace, guard, route, mem, shape, gate, trace** (deny short-circuits; `updatedInput` merged field-wise, one command rewrite; `additionalContext` and `reason` concatenated under the gate-spec §9 ceilings); on PostToolUse **trace, shape, gate, guard, mem, trace**. Shape's rows below are its contribution to that merged output, and `saga shape install` is an alias that adds shape to the manifest and re-runs `saga install`.

### 7.1 Claude Code (hook facts from gate-spec §6.1 and guard-spec §8.1, verified 2026-09-03 against `harness-facts.md` §1; the `Read`/`Grep`/`Glob` output shapes were captured empirically on 2.1.259, `harness-probes.md` P1)

| Event / matcher | Shape call | Output |
|---|---|---|
| `PreToolUse` / `Bash\|PowerShell` | none | `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","updatedInput":{...full tool_input..., "command":"saga shape run --mask --harness claude -- <original>"}}}` (`updatedInput` verified, C3; it replaces the whole input object, so `description`, `timeout` and `run_in_background` are echoed); the masker inside `shape run` is guard's (§2.1), so the guard-spec §4.4 rewrite and this one are the same rewrite |
| `PostToolUse` / `Bash\|PowerShell` (result path) | `parse --json` on `tool_response` | used only for a `Bash` call that ran unwrapped (rewrite refused or absent): `hookSpecificOutput.updatedToolOutput = {"stdout": <rendering>, "stderr": "", "interrupted": false, "isImage": false}`, the documented `Bash` output shape (verified, C7); emitted only when shaping changed the bytes; recorded as `replaced_via: updatedToolOutput` |
| `PreToolUse` / `Edit\|Write\|MultiEdit\|NotebookEdit` | `edit pre --path` | `{}` (records the pre-image; never blocks) |
| `PostToolUse` / `Edit\|Write\|MultiEdit\|NotebookEdit` | `edit post --json` with `tool_input` and `tool_response` on stdin | on a non-`applied` verdict: `{"decision":"block","reason":"<≤120 tok>"}`. **Feedback**: PostToolUse cannot undo the edit, and `decision: block` leaves the original result visible ("Claude still sees the original output", C6); shape never replaces an edit result. The diff echo goes in `hookSpecificOutput.additionalContext` (verified, C8) and `reason` holds only the verdict line |
| `PostToolUse` / `Bash` | `edit post --from-tree` | as above for files changed by the command; no output when nothing changed |
| `PostToolUse` / `Read\|Grep\|Glob` | `parse --json` on `tool_response` | rewritable via `updatedToolOutput` (verified on 2.1.259, harness-probes P1: a replacement in the observed shape is what the model sees, the original is gone from the transcript; a plain-string replacement is silently ignored, C7). Observed shapes, which shape reproduces field for field: `Read` `{"type":"text","file":{"filePath","content","numLines","startLine","totalLines"}}` (Claude Code re-renders `file.content` with line numbers); `Grep` `{"mode","numFiles","filenames","content","numLines","totalLines"}` (in `content` mode `numFiles` is 0 and `filenames` empty; the model sees `content` only); `Glob` `{"filenames","durationMs","numFiles","truncated","totalMatches","countIsComplete"}` (the model sees `filenames` joined by newlines). Because a shape drift in a later release is ignored silently, the install probe still runs the round-trip fixture and enables replacement only when it passes; the fixture must pass `--tools "Read,Grep,Glob,Bash"` because `Grep` and `Glob` are absent from the `-p` tool list on 2.1.259 (P1a). Until the fixture passes shape records `result_bytes` for the ledger and emits nothing. The bench measures whether the managed-block advice to prefer `saga shape read` and `saga shape run -- rg` is followed; it is advice, not enforcement (ADR 0002), and is reported as such |

Without `updatedInput` the Bash path degrades to the `updatedToolOutput` result path, and without both to accounting only, and `saga doctor` reports `shape: wrapper unavailable on this Claude Code version`. Hook output strings are capped at 10,000 characters (C24), so a rendering above that goes to the cache and the hook returns the `shape.more` stub.

### 7.2 Gemini CLI (gate-spec §6.2)

| Event | Shape call | Output |
|---|---|---|
| `BeforeTool` (shell tool) | none | `tool_input` rewrite to `saga shape run --harness gemini -- <cmd>` when the version accepts an input update *probe*; otherwise nothing |
| `AfterTool` (shell, read, grep tools) | `parse --json` on `tool_response` | when the parser matched or truncation applied: `{"decision":"block","reason":"<shaped rendering>"}`, which **replaces** the tool result. Emitted only when shaping changed the bytes, so ordinary results are untouched (the gate-spec §6.2 rule). The `block` framing is recorded as `replaced_via: block` in the trace because its effect on model behaviour is unmeasured (§11) |
| `AfterTool` (edit tools) | `edit post` | same shape as Claude Code; the verdict replaces the result on Gemini, so the rendering repeats the harness's own success or error line first |

### 7.3 Codex CLI (gate-spec §6.3)

Verified 2026-09-03 (harness-facts X8, X11): `PreToolUse` accepts `updatedInput` with `permissionDecision: "allow"` and a string `command`, so the Bash rewrite is the same as on Claude Code; `PostToolUse` `decision: block` "replaces the tool result with that feedback", so a shaped rendering can be delivered as the result on the Gemini pattern, emitted only when shaping changed the bytes, and edit verdicts stay feedback via `hookSpecificOutput.additionalContext` (X12) because a `block` would hide the harness's own result. Model-visible hook output is capped near 2,500 tokens (X20). Codex hooks run outside the sandbox (guard-spec §8.3), so `shape run` inside the sandbox must be the manifest binary and the cache directory must be writable from within it (`.saga/shape/` added to the sandbox write list, guard-spec §8.4).

### 7.4 Wrapper mode (every harness, CI, harnesses without hooks)

```
saga shape run [--family f] [--cache serve|advisory|off] [--no-cache] [--timeout s] [--mask] [--json|--raw] -- <cmd...>
```

Runs the command with the harness's shell semantics (`bash -lc` on POSIX, `pwsh -c` on Windows, or argv when `--argv`), streams stdout and stderr through the §2.1 pipeline, prints the rendering on stdout and **propagates the child's exit code and signal unchanged**. `--raw` prints the untruncated masked log (for humans and CI artefacts) while still recording the shaped result. The managed block that index writes into `CLAUDE.md`/`AGENTS.md` (index-spec §7.3) gains two lines naming the wrapper and `shape more`; the bench measures adherence.

### 7.5 What each harness gets

| Capability | Claude Code | Gemini CLI | Codex CLI | Wrapper / CI |
|---|---|---|---|---|
| Shaped Bash results in context | via `updatedInput` rewrite (verified) or `updatedToolOutput` (verified) | via `AfterTool` replace (verified) or `BeforeTool` `tool_input` merge (verified) | via `updatedInput` rewrite (verified) or `PostToolUse` `block` replace (verified) | yes |
| Shaped `Read`/`Grep` results | `updatedToolOutput` once the output shape probe passes; else accounting only | `AfterTool` replace | no (Codex file reads are shell commands, so they take the Bash path) | `shape read`, `shape run -- rg` |
| Edit verification feedback | `reason` + `additionalContext` (verified) | replaces result | `additionalContext` (a `block` would replace the result) | `shape edit post` in CI over the PR diff (parse check only) |
| Cache | yes, inside the wrapper | yes | yes | yes |
| Per-call token accounting | yes (`tool_response` bytes) | yes | yes | yes |

---

## 8. Token accounting

Every shaped call writes, on the trace `tool_result` event, the `shaped` object that trace-spec §2.2 now carries (optional, present only when shape handled the call):

```json
"shaped": {"family": "pytest", "confidence": "exact", "raw_bytes": 38412, "shaped_bytes": 1840,
           "raw_tokens_est": 9603, "shaped_tokens_est": 460, "cache": "miss", "log": "sha256:…",
           "dropped_lines": 1230, "promoted_lines": 2}
```

The ledger row (trace-spec §3.2) carries `tool_output_bytes_raw_turn` next to `tool_output_bytes_turn`, and `attribution.shape` is the share of `context_delta` that shaped results occupy. Shape's feedback messages (§4.2) are its 600-token share of the one per-session injected budget (contracts §7); shaped tool results and diff echoes are tool output, attributed to `shape`, and are not injected text. The report (trace-spec §3.7) prints per session: raw vs shaped tool bytes, cache hits and the wall time they saved, and edits by verdict. Tokens are estimated by `bytes/4` at the hook and reconciled by the bench against harness usage counters, the same discipline as gate-spec §9.

**Honest expected range.** Per-call reductions on parsed families are large (a 40k-character test log, doc 02 item 6, becomes a few hundred tokens), but tool output is a fraction of context and the saving "dilutes at every step" (rtk, doc 04 §2.3). rtk's verdict is single-digit to low-double-digit bill savings; context editing, a different mechanism, showed −84% tokens on a 100-turn vendor eval (doc 05 §5), which is an upper bound for what removing stale results can do, not a shape number. Shape therefore pre-registers a primary outcome of `tokens_per_solved` down by at least 8% with a 95% CI excluding zero, and reports whatever the bench finds. No README number appears except as a `saga bench badge` (bench-spec §7.3).

---

## 9. CLI and MCP surface

### 9.1 CLI

```
saga shape run     [--family f] [--cache serve|advisory|off] [--no-cache] [--timeout s] [--mask] [--harness h] [--json|--raw] -- <cmd...>
saga shape parse   [--family f] [--command "<cmd>"] [--exit n] [--json] [< log | --log <hash>]
saga shape more    <hash-prefix> <range> [--json]
saga shape read    <path|path::qname> [--lines a-b] [--context n] [--json]
saga shape edit    pre  --path p [--tool t]
saga shape edit    post --path p --tool t [--input <json>] [--response <json>] [--from-tree] [--json]
saga shape cache   stats [--json] | clear [--family f] [--older-than 7d] [--all] | verify [--sample n]
saga shape parsers list | test <family> [--corpus dir] [--json]
saga shape install --harness claude|gemini|codex [--shared] | status | uninstall
```

### 9.2 Exit codes (the uniform table of contracts §4)

| Code | Meaning |
|---|---|
| child's code | `run`: the wrapped command's exit code or 128+signal, always propagated; shape's own failures go to stderr and never change it |
| 0 | `parse`/`more`/`read`/`edit`/`cache`: success; `edit post` with verdict `applied` |
| 1 | `parse --strict` with `confidence: none`; `edit post` with a non-`applied` verdict; `cache verify` found a stale hit |
| 2 | usage error, ambiguous hash prefix, unknown family |
| 5 | log or cache entry not found or evicted |
| 6 | environment refusal: unwritable store, symlinked `.saga/shape`, log over 8 MiB inline without a blob store |

Precedence: 6, 2, 5, 1.

### 9.3 JSON envelopes

`run` and `parse` emit `saga.shape.result/1` (§2.4); `edit post` emits `saga.shape.edit/1` (§4.4); `more` and `read` emit:

```json
{"schema": "saga.shape.window/1", "log": "sha256:…", "range": "L800-870", "lines": [[800, "…"], [801, "…"]],
 "total_lines": 1260, "next": "L871-1170", "tokens_est": 610, "file_hash": null}
```

`cache stats` emits `{"schema":"saga.shape.cache/1","entries":n,"bytes":n,"hits":n,"misses":n,"served_ms_saved":n,"by_family":{…},"nondeterministic":[…],"stale_hits":n}`. Every record carries `schema` as `<name>/<major>` and consumers reject unknown majors (gate-spec §7.2 rule). Text fields are control- and bidi-stripped before serialisation.

### 9.4 MCP

`saga shape serve --stdio` exposes two tools, `shape_run` (arguments `command`, `cwd`, `cache`) and `shape_more` (`log`, `range`), with the schemas above. It exists for harnesses where hooks cannot rewrite the shell tool; the index server keeps `read` (index-spec §7.2), so shape does not duplicate it over MCP. Tool descriptions are versioned text and changing one re-runs the canary (index-spec §7.2 rule).

### 9.5 Config

```toml
[shape]
enabled = true
echo = "on-mismatch"            # on-mismatch | always | off
max_failures = 10
max_errors = 20
strip_comments = "off"          # off | added
prefer_structured = false       # experimental, §11
[shape.ceilings]
scale = 1.0                     # 0.5 for harnesses without context editing
test = 2000; compile = 2000; lint = 1500; pkg = 800; search = 1200; unknown = 2000; window = 2000
[shape.cache]
mode = "serve"                  # serve | advisory | off
families = ["test", "compile", "lint", "grep"]
max_bytes = "500MiB"; max_age = "14d"
scratch = ["node_modules/.cache", ".pytest_cache", "target", "build", ".dart_tool", "DerivedData", "__pycache__"]
env = []                        # extra env names in env_fp
[shape.cache.inputs]
"pytest*" = ["tests/**", "src/**", "conftest.py", "pyproject.toml", "pytest.ini"]
[shape.logs]
max_bytes = "200MiB"; max_age = "7d"
```

Config is read from the working tree; unlike gate's guards it is not security-relevant, so the `BASE:` rule of gate-spec §5.4 does not apply.

---

## 10. Test plan and bench ablation

### 10.1 Parser golden corpus

`fixtures/shape/<family>/<nnn>.log`, `.meta.json` (command, exit code, runner version, OS, CRLF, ANSI), `.expected.json` (the `saga.shape.result/1` minus `log.hash` and `tokens`). At least 30 real logs per family in §2.3, collected from public CI logs of permissively licensed repositories and from bench runs (bench-spec archive layout), masked by guard before commit. Each family's set must include: ≥1 all-pass, ≥5 fail, ≥3 `error` (collection or compile failure inside the test build, missing module), ≥2 killed or timed out, ≥3 runner major versions, both newline conventions, ANSI on and off, and ≥2 logs longer than 1 MiB.

| Bar | Threshold |
|---|---|
| `status` accuracy against exit code and meta | 1.0 |
| Failing-test id recall (first `max_failures`) | ≥ 0.98 |
| First-assertion `path:line` match | ≥ 0.95 |
| Compiler error `path:line:col` recall (first 20) | ≥ 0.98 |
| Summary count reconciliation | ≥ 0.97 of logs at `exact` |
| Cross-family fingerprint false positives on the full corpus | 0 |
| Parser wall time per log | < 50 ms per MiB |

`saga shape parsers test <family>` runs the corpus; CI fails on any bar. A family below its bar ships as `heuristic` only.

### 10.2 Truncation and window property tests

Generators: random byte strings with random newline density (0 to 1 per 8 bytes), random line lengths up to 64 KiB, random insertion of error-regex lines at random depths, random CRLF. Properties: the §3.1 invariants; ceiling respected within one line; promoted lines ⊆ error lines in the dropped middle; `more` over all announced ranges reconstructs the input byte-exactly; shaping twice equals shaping once. Negative corpus for the error regex: 2,000 lines of paths, URLs and identifiers containing `error`, `fail`, `panic` as substrings; promotion count on it must be 0.

### 10.3 Cache correctness

| Test | Expectation |
|---|---|
| Same argv, cwd, env, inputs | hit; served result byte-identical to the stored rendering |
| Any byte change in a read-set file | miss |
| Change in a file outside the read set | hit, and `cache verify` (executes anyway, compares) reports `identical` |
| `via = index` read set on 200 recorded bench commands, `--cache advisory` | `stale_hits / would_hits` = 0 on the corpus; the bench reports the rate on live runs and demotes `serve` to `advisory` for `index` sets if it exceeds 1% |
| Every command in §5.4 across the guard incident suite | never stored |
| Third identical invocation with differing output | entry evicted, family excluded, trace `drift` event |
| `extractor_version` bump | all `index` entries gone; `tree` and `declared` entries survive |
| Trace replay of a session with hits | strict replay passes; permissive replay serves from the shape cache when blobs are absent |

### 10.4 Adapter golden logs

Recorded stdin JSON per harness per event (bash, edit, read, grep; with and without `updatedInput`/`additionalContext` support) to expected stdout JSON, in gate-spec §10.1 style; plus a live conformance probe at install that records which fields the installed version honoured (trace-spec §9.3 *probe* discipline). Composition tests: guard deny plus shape rewrite yields deny; shape verdict plus gate finding yields one merged `reason` under 200 est. tokens.

### 10.5 Edit verification fixtures

For each of the seven languages: 20 recorded edit calls covering exact apply, whitespace-normalised apply, 0 occurrences, 2 occurrences, multi-edit partial, CRLF file, BOM file, an edit that breaks syntax, an edit that fixes syntax, and a Bash `sed -i` edit. Expected verdicts are hand-labelled; accuracy bar 1.0 (these are deterministic).

### 10.6 Bench ablation

Ladder position per bench-spec §4.3: `base + gate + guard + index + mem` vs the same plus shape; the cache directory exists only in the treatment arm (bench-spec §3, "no shared caches unless under test"). Paired, K ≥ 5, control arm blocked at the `saga shape` binary and `.saga/shape/`. Pre-registered (the shape row of bench-spec §5.11 names the report keys):

| Metric | Definition | Expectation |
|---|---|---|
| `tokens_per_solved` (primary) | bench-spec §5 | down ≥ 8%, 95% CI excluding 0 |
| `pass@1`, `pass^k` | bench-spec §5.2 | Δ point estimate ≥ 0; a CI entirely below 0 fails the milestone (noise floor 3.5 to 4.5 points, doc 09 §7) |
| `first_error_found` | among runs with ≥1 failing test or compile result: fraction where the next `edit` event after that `tool_result` touches the file of the first reported failure or error, or a file one import hop from it (index) | up; reported per family and per confidence |
| `false_success_after_edit` | edit calls whose `tool_response` reports success and whose verdict ∉ {`applied`, `applied_normalised`}, divided by edit calls; plus its inverse (error reported, file changed) | the rate itself is the finding; shape cannot lower the harness's rate, only the fraction the agent acts on, so the secondary metric is `retry_after_false_success` (an identical edit within 2 turns) |
| `tool_output_bytes_turn` raw vs shaped, `cache_hit_ratio`, `served_ms_saved` | ledger | reported; wall time per solved task expected down on Swift and JVM tasks (doc 02 item 14) |
| Adherence | fraction of shell calls that went through the wrapper, per harness | reported; a harness under 50% adherence has its shape delta labelled `partial exposure` (bench-spec §4.2 `component_unused` discipline) |

Sub-arms: `echo = always` vs `on-mismatch`; `cache = serve` vs `advisory`; `ceilings.scale = 0.5` vs 1.0. Each is a separate paired comparison, never pooled.

---

## 11. Experimental register

| Item | Default | Graduates when |
|---|---|---|
| `via = index` read-set inference | on, `serve` | stale-hit rate 0 on the corpus and < 1% on the bench for two milestones |
| `via = observed` (strace) read set | off | Linux corpus shows it is a superset of `index` sets on 200 commands |
| Comment stripper | off | bench: correctness flat, comment density down, zero parse regressions |
| `prefer_structured` reporter flags | off | trace shows no command-semantics drift and parsers gain `exact` on ≥ 95% of logs |
| Gemini `AfterTool` result replacement via `block` | on where no input rewrite exists | bench shows no behaviour difference against the rewrite path |
| Managed-block advice to use the wrapper | on | adherence measured per harness; advice is never counted as enforcement |
| Learned pruner (SWE-Pruner, Squeez class) as an opt-in tail compressor | off, not shipped | independent replication of doc 05 §5 numbers on the bench, `more` still lossless |
| Token estimate `bytes/4` | on | ratio to harness-measured tokens reported by the bench (shared with gate-spec §11) |
