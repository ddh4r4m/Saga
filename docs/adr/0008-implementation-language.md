# ADR 0008: Implement `saga` as one Go binary with a single cgo boundary for tree-sitter

**Status:** proposed, 2026-09-03

## Context

The specs fix the binary's shape before any code exists. Guard-spec §2.2 parses and expands bash, zsh, PowerShell and cmd without executing them and names mvdan/sh; §2.7 and §11.3 demand zero escapes on I-01 to I-18 and a 100k-seed differential fuzz where a resolved argv that differs from the real shell is a P0. Guard-spec §3 snapshots through git plumbing plus clonefile reflinks. Guard-spec §7 requires one signed static binary per platform (five targets), cosign, `SHA256SUMS`, minisign, no script hooks, no self-update. Index-spec §2 to §4 needs tree-sitter grammars for ten languages, SQLite with FTS5, blake3, msgpack, a watcher, a 100k-line cold build ≤ 10 s and edit-to-queryable p95 ≤ 800 ms. Trace-spec §2 is hash-chained JSONL with 64 MiB rotation; §11.1 bounds hook overhead at p50 ≤ 5 ms and p99 ≤ 20 ms per invocation. Contracts §1.1 makes the entry run its own deadline because a timed-out `PreToolUse` allows on Claude Code and any non-JSON byte allows on Gemini; Codex runs hooks with a cleared environment. Bench-spec §3 spawns containers, worktrees and harness CLIs. Mem-spec stores TOML records named by ULID; shape-spec §2.2 loads a TOML parser registry.

A hook binary is spawned once per tool call, so process start is a per-call tax. Measured here on an Apple M1 Pro (60 spawns each, JSON in on stdin, JSON out, after warm-up): Go 1.26.3 p50 4.7 ms, p95 6.1 ms; Rust 1.88 p50 3.7 ms, p95 4.1 ms; Node p50 41.5 ms, p95 45.4 ms (`node -e 0` alone is 34.7 ms). Node's floor is eight times the trace-spec p50 budget before any work is done. Doc 08 Part 2 records that unlazy's zero-dependency Node core needed 5,700 lines of hardening for symlink, FIFO, bidi and Windows process-tree cases that a systems standard library covers; doc 04 §2.3 records context-mode's better-sqlite3 SIGSEGV class and rtk shipping as a single Rust binary. The owner's own work is Swift and Flutter, so no candidate is a home language.

### Evaluation matrix

| Requirement (spec) | Go | Rust | TypeScript / Node | Hybrid (Rust index, rest Go or TS) |
|---|---|---|---|---|
| Static single binary, 5 targets (guard §7) | Yes; fully static without cgo, musl or `-extldflags -static` with it; macOS links libSystem only | Yes; `*-linux-musl` fully static; smallest binaries (stub 454 KB vs Go 3.1 MB) | No; Node SEA is experimental, bun `--compile` ships a 50 MB+ runtime, pkg is deprecated | Yes, with two toolchains and an FFI seam |
| Signing and notarization (guard §7) | goreleaser: `codesign`, `notarize.macos`, cosign keyless, minisign, `SHA256SUMS`, Windows `signtool` hook | cargo-dist for the matrix; `rcodesign` notarizes from Linux; cosign and minisign as CI steps | Sign whatever wrapper produced the executable; SEA injection breaks signatures until re-signed | Two pipelines or one custom one |
| tree-sitter binding, grammar packaging (index §3.1) | Official `go-tree-sitter`, cgo; grammar C sources vendored and compiled in; per-node cgo crossings are the cost, cut by running `.scm` queries in C | First class: tree-sitter's own CLI is Rust; grammars as crates via `cc` | `web-tree-sitter` (WASM, 2 to 3× slower) or `node-tree-sitter` (native addon per Node ABI, the better-sqlite3 failure class) | Rust wins this row; it is the whole case for a hybrid |
| SQLite with FTS5, no cgo pain (index §2.2) | `modernc.org/sqlite` (pure Go, FTS5 on); `mattn/go-sqlite3` behind `database/sql` if the latency suite needs it | `rusqlite` with `bundled` and `fts5`, no system dependency | `better-sqlite3` (SIGSEGV class); `node:sqlite` still experimental on 24; WASM SQLite has no real WAL | As Rust |
| Shell parse plus expansion (guard §2.2) | `mvdan.cc/sh/v3`: parser and `expand` package, behind shfmt since 2016, differential-tested; zsh via the spec's flag table | No equivalent: `yash-syntax` parses POSIX only, `conch-parser` is unmaintained, `brush` expands only inside its interpreter, `tree-sitter-bash` parses without expanding. Expansion would be reimplemented, and expansion is where every ADR 0006 incident lives | `mvdan-sh` on npm is a GopherJS build of the syntax package only; `bash-parser` is unmaintained | Rust still needs the expander, so guard lands in Go anyway |
| PowerShell AST, cmd tokenizer, git plumbing, reflink (guard §2.2, §3) | Shell out to `pwsh -NoProfile`, `git`, `cp -c`, `zfs`, `btrfs`; hand-written cmd tokenizer; `clonefile(2)` via `x/sys` | Same | Same, minus syscall access | Same |
| Hook cold start (trace §11.1) | 4.7 ms p50 measured; inside budget, so nothing beyond the standard library on the hook path | 3.7 ms p50 measured; best | 41.5 ms p50 measured; fails by 8× before doing any work | Meets it |
| Watcher (index §4.1) | `fsnotify` (kqueue on macOS, so the polling fallback is real) plus `fsevents` on darwin | `notify` crate, the one the spec names | `chokidar`; recursive watch uneven | Rust wins slightly |
| Cross-compilation | Trivial without cgo; with cgo, zig as `CC` or native runners, which notarization needs anyway | `cross` or `cargo-zigbuild`; windows-msvc from Linux is awkward | None, but a runtime must exist on the target, which Codex's cleared environment does not guarantee | Two matrices |
| Contributor pool for an OSS CLI | Largest for this shape of tool (gh, terraform, hugo); gentle curve | Strongest in the Codex and rtk ecosystems; steeper curve; compile times slow the fuzz loop | Largest overall; the harnesses themselves are TS | Splits reviewers |
| Test tooling | `go test`, native fuzzing for §11.3, `-race`, `testscript` golden CLI tests | `cargo test`, `insta`, `proptest`, `cargo-fuzz` on nightly | `vitest`, third-party fuzzing | Both |
| Long-term maintenance | Backward-compatible language; stdlib covers HTTP, JSON, crypto, exec | Stronger compile-time checks; deep dependency trees | Runtime and addon ABI drift; `node_modules` supply chain (doc 06 C.3) | Two ecosystems drift independently |

## Decision

1. **Language: Go.** One module, one binary, `cmd/saga`. The deciding rows are shell expansion (mvdan/sh is the only production-grade bash expander available as a library, and guard's zero-escape suite is an expansion suite), cold start inside the trace budget, and a release pipeline that exists as a tool rather than a project. Rust wins tree-sitter, the watcher, binary size and one millisecond of start; none of those is a spec bar Go misses. TypeScript fails the static-binary and cold-start rows outright. A hybrid buys Rust's tree-sitter row for two toolchains, and contracts §1 removes the only place TS would have belonged: hooks are `saga hook <harness> <event>`, not scripts.
2. **One cgo boundary.** cgo is permitted only in `internal/index/ts` (tree-sitter runtime plus vendored grammars). Everything else builds with `CGO_ENABLED=0`; a `nots` build tag yields a lexical-only index regime that `saga doctor` reports. Extraction is written as tree-sitter `.scm` queries per language so the tree walk stays in C and cgo crossings are per capture, not per node.
3. **Repository layout.**

| Path | Contents |
|---|---|
| `cmd/saga/` | `main.go`: version stamp, calls `internal/cli` |
| `internal/cli/` | subcommand tree, `--json`, exit-code precedence 6, 7, 2, 3, 4, 5, 1 (contracts §4) |
| `internal/hook/` | the composed entry: layer order, merge rules, `hook.deadline_ms`, one JSON object on stdout (contracts §1) |
| `internal/harness/{claude,codex,gemini}/` | translation only, ≤ 150 lines each (gate-spec §6, ADR 0003) |
| `internal/trace/` | events, `prev` chain, rotation, ledger, pins, watchdog, claims, canary, `doctor` |
| `internal/gate/` | contract grammar, `check`, `reverify`, red proof, evidence, diff guards |
| `internal/guard/` | parse and expand (mvdan), segment, classify, policy, mask, deps, MCP gateway |
| `internal/snapshot/` | `git-tree`, `zfs`, `btrfs`, `reflink`, `undo`; owned by guard, called by trace, shape, gate (contracts §5) |
| `internal/index/` | schema, facts, resolver, tools, MCP server; `index/ts/` is the cgo package |
| `internal/mem/`, `shape/`, `route/`, `bench/` | one package per layer, milestone-gated |
| `internal/canon/`, `internal/store/`, `internal/schema/` | canonical JSON, hash ids, ULID, path and text normalisation (contracts §3); SQLite open and hostile-shape checks (gate-spec §8); embedded schemas and strict validator (contracts §11) |
| `schema/<layer>/<major>/` | the schema files, named per contracts §11 (`schemas/` was proposed; the contracts file wins) |
| `adapters/` | settings fragments written by `saga install`, the CI workflow template (gate-spec §6.4), the M6 OpenCode plugin shim |
| `bench/tasks/<lang>/<id>/`, `bench/images/` | task directories and OCI image definitions (bench-spec §2, §3) |
| `fixtures/` | incident rows and benign controls, adapter golden logs per harness version, masking corpora, shape golden logs |
| `grammars/` | vendored tree-sitter C sources and `grammars.lock` commit pins (index-spec §3.1) |
| `scripts/install-saga.sh` | pinned download; verifies `SHA256SUMS` and minisign before extracting |

4. **Dependency policy.** Allow-list, one PR per addition, `go.sum` and `govulncheck` in CI. Standard library first (`log/slog`, `net/http`, `crypto/sha256`, `encoding/json`, `os/exec`).

| Module | Reason |
|---|---|
| `mvdan.cc/sh/v3` | bash and POSIX parse and expand (guard-spec §2.2) |
| `github.com/tree-sitter/go-tree-sitter` | official binding; the only cgo import |
| `modernc.org/sqlite` | pure Go SQLite with FTS5; `mattn/go-sqlite3` is the documented swap behind `database/sql` |
| `github.com/zeebo/blake3` | blake3 with SIMD |
| `github.com/fsnotify/fsnotify`, `fsnotify/fsevents` (darwin) | watcher; polling fallback stays |
| `github.com/oklog/ulid/v2` | ULIDs (mem-spec, trace session ids) |
| `github.com/BurntSushi/toml` | TOML with line and column errors for policy and records |
| `github.com/vmihailenco/msgpack/v5` | facts blobs (index-spec §2.2) |
| `github.com/santhosh-tekuri/jsonschema/v6` | strict schema validation |
| `github.com/spf13/cobra` | CLI tree; no viper |
| `github.com/modelcontextprotocol/go-sdk` | MCP server for index, mem, gate tools |
| `github.com/jedisct1/go-minisign` | `saga doctor` verifies the binary against the embedded public key |
| `golang.org/x/sys`, `golang.org/x/text` | `clonefile`, `Nlink`, FIFO detection; bidi and control stripping |
| `rogpeppe/go-internal/testscript`, `google/go-cmp` | test only |

Denied: go-git (git is exec'd, guard-spec §3.1), logging frameworks, HTTP wrappers, a second SQLite or regex engine. Go's RE2 regexp is linear time, which matters because repo-controlled text reaches the masker; gitleaks rules are vendored as data, and PCRE-only patterns are rewritten or dropped with a fixture.

5. **Build and release.** goreleaser v2 on native GitHub runners: macos-14 builds both darwin arches with cgo, ubuntu builds linux amd64 and arm64 with `zig cc` targeting musl (fully static), windows-latest builds `windows-x86_64`. Flags `-trimpath -ldflags "-s -w -X main.version=..."`, toolchain pinned in `go.mod`, `GOFLAGS=-mod=readonly`. Steps: build; macOS `codesign` with the owner's Developer ID and `notarize.macos`; Windows Authenticode when a certificate exists (until then the release notes say so); `SHA256SUMS`; cosign keyless `sign-blob` under GitHub OIDC; minisign with an offline key whose public half is embedded; SLSA provenance; GitHub release, Homebrew tap, Scoop bucket. Hashes follow contracts §3, bench-spec §8.1 records `binary_sha256` per manifest, guard-spec §7 is the signing contract. `saga.exe` is invoked directly, so the Windows `.cmd` shim and its `doctor` hash row disappear.

6. **First three milestones**, mapped to spec sections and the doc 09 §5 exit criteria.

| Milestone | Delivers | Spec sections | Language-risk checkpoint |
|---|---|---|---|
| M0 Measure | `init`, `install`, composed `hook`, `doctor`; trace events, chain, ledger, pins, watchdog; bench runner and archive; Claude Code adapter | contracts §1, §2, §9; trace-spec §2, §3, §7, §11.1 (overhead p50 ≤ 5 ms, p99 ≤ 20 ms); bench-spec §3 (isolation, run directory), §8 (manifest, `verify`, `replay`) | Hook overhead on the 50k-file repo across all five targets |
| M1 Gate, guard, compaction survival | contract grammar, `check`, red proof, diff guards, Stop adapters (Claude Code, Gemini, Codex, CI); guard parse, classify, policy, `snapshot`, `undo`, masking, deps; claims; mem state block; Windows green | gate-spec §2 to §7 (§7.1 exit codes); guard-spec §2 (§2.7 zero escapes), §3 (§3.3 budgets), §4, §5, §7, §11.3 fuzz, §11.4 latency bars; trace-spec §5.5 to §5.9; mem-spec §4.2 | mvdan/sh against the 100k-seed differential fuzz per shell; unresolvable rate reported (doc 09 §7 item 6) |
| M2 Index | schema, ten grammars, extraction queries, resolver, incremental update, four tools, MCP server, regime gate | index-spec §2 to §4 (§4.5: 100k lines ≤ 10 s cold, edit p95 ≤ 800 ms, query p95 ≤ 50 ms), §5, §7, §9; contracts §10 | go-tree-sitter and modernc SQLite against §4.5; fallbacks in order: `mattn/go-sqlite3`, then a Rust index sidecar |

7. **Reversal conditions.** Reopened by a new ADR when measured, not argued: (a) mvdan/sh cannot reach the guard-spec §11.3 bar and a Rust expander demonstrably can; (b) the M2 cold build exceeds 20 s on the 100k-line reference with queries already in C, and a Rust prototype of `index/ts` meets 10 s; (c) hook p99 exceeds 20 ms on any target for reasons traced to the Go runtime rather than Saga's own work; (d) the cgo boundary blocks a signed static build on any target after two release cycles. Contributor-pool arguments alone do not reopen it.

## Consequences

Easier: one toolchain, formatter and test runner; native fuzzing for the parser suite; a release pipeline that is configuration; hook processes inside the trace budget; ADR 0006's expansion semantics ride on a library already differential-tested against bash.

Harder: `internal/index/ts` is a permanent cgo island, so builds need native runners or zig and compile ten grammars (the Swift grammar's `parser.c` alone is tens of megabytes of source); every tree-sitter call outside the query layer is a performance review item; `modernc.org/sqlite` may be swapped at M2; FSEvents needs the cgo package or polling; zsh stays a flag table over the bash grammar.

Given up: Rust's smaller binary, native tree-sitter ergonomics and the `notify` crate; TypeScript's overlap with the harness codebases; any in-process integration except the M6 OpenCode shim, which calls the binary and holds no logic.
