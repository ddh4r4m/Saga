# `saga index`: technical specification

*Draft v0.1, 2026-09-02. Implements doc 09 §3.3 and ADR 0005. Evidence base: doc 05 §1 (structural graphs help localization; depth two hurts), §6 (gated API knowledge), §7 (test-impact maps), §9 (recommended shape) and §10 (open problems); doc 04 §2.4 (serena, codegraph, graphify trade-offs, including codegraph's +80% resident-context cost); doc 03 §1.2 (few tools, compact output, descriptions are prompts). Integrates with `gate-spec.md` §2 (contracts) and `bench-spec.md` §4–5 (ablation and metrics). Ships at M2.*

---

## 1. Purpose, non-goals, regime gate

### 1.1 Purpose

`saga index` is a deterministic, content-addressed structural index of a repository, exposed to an agent as four tools. It exists to move two numbers that three independent studies say a def/ref graph moves (doc 05 §1.2): file-level localization accuracy and resolve rate, at equal or lower cost per solved task. It also supplies `saga gate` with the affected-test set for a diff.

| Question the agent asks | Tool | Backing structure |
|---|---|---|
| Where is the thing called X? | `search` | BM25 over split identifiers, symbol table, optional AST-chunk embeddings |
| What is X, who calls it, what does it call? | `symbol` | Tree-sitter node table plus heuristic call and reference edges, depth one |
| Show me the exact source | `read` | The working tree, by path or symbol id |
| If I change these files, what breaks? | `impact` | Reverse import and call edges, test map, last coverage |

### 1.2 Non-goals

| Not this | Because |
|---|---|
| An LLM-extracted knowledge graph | Costs 10⁷–10⁸ tokens per corpus and adds relation errors; AST graphs are cheaper and at least as accurate (doc 05 §2). ADR 0004 forbids narrative memory as facts. |
| Narrative summaries of modules or files | LLM-generated repo notes reduce resolve rate 3% and raise cost 20–23% (doc 05 §3.3). The index stores signatures and edges, never prose about them. |
| A replacement for grep | Grep persists in every major agent for zero setup, all file types, and benign failure (doc 05 §1.4). No adapter removes, wraps, or discourages grep. |
| A compiler-grade index | SCIP, Glean and Kythe need builds and per-language indexers. This index needs a parser. The precision gap is documented (§3) and bridged lazily by LSP (§3.4). |
| Two-hop context | RepoGraph 1-hop 29.67% vs 2-hop 26.00% (doc 05 §1.2). `depth` defaults to 1 and is capped at 2. |
| Always on | Every structural tool in the survey is net negative on small repos (doc 04 §5 item 5; serena 4× cost on a 36k-line Java repo). Below the regime gate the tools are not registered. |
| Effect claims before `saga bench` | ADR 0001. §9.2 is the design that would licence a number. |

### 1.3 Regime gate

The gate is measured by `saga index status --probe`, which is cheap enough to run at every session start (directory walk, no parsing).

| Measure | Definition |
|---|---|
| `source_files` | Files whose extension maps to a supported grammar (§3.1), after `.gitignore`, `.sagaignore` and the generated/vendored rules (§4.6) |
| `source_lines` | Newline count over `source_files` |
| `top_dirs` | Distinct first path segments among `source_files` |
| `cross_dir_ratio` | Fraction of import edges whose source and target are in different `top_dirs`. Computed only once an index exists; `null` before the first build |

| Regime | Condition (default) | Behaviour |
|---|---|---|
| `off` | `source_lines < 20_000` **and** `source_files < 200` | No tools registered, no map, no watcher. `status` prints the regime and the thresholds. |
| `lite` | otherwise, and `source_lines < 80_000` | Tools registered; map capped at 500 tokens; embeddings off regardless of config |
| `full` | `source_lines ≥ 80_000` **or** `source_files ≥ 800` | Tools registered; map up to 1,000 tokens; embeddings on if a model is configured |

Hysteresis: a repo leaves `off` when it exceeds either threshold by 10% and re-enters only when it falls 10% below. `[index] regime = "off" | "lite" | "full" | "auto"` in `.saga/config.toml` overrides. The thresholds are priors, not findings; §9.2 measures the crossover and the defaults are revised from the bench, never from a README.

A second gate applies to the LSP overlay only (§3.4): `[index] lsp = "auto"` enables it for models in the route policy's `small` and `local` tiers and disables it for `frontier` and `standard` (route-spec §3.1 `[tiers]`; the primary's tier when route is not installed is read from `[index] tier`, default `frontier`), because LSP tools saved tokens for Haiku and cost 118% more for Sonnet (doc 05 §1.4). Contracts §10 restates both gates.

---

## 2. Data model

One SQLite database per repository at `.saga/index/index.db` (WAL mode, `.saga/index/` gitignored through `.saga/.gitignore`, written by `saga init`, contracts §2). All derived, all rebuildable, nothing in it is authoritative over the working tree.

### 2.1 Content addressing

| Object | Key |
|---|---|
| File | `file_hash = blake3(bytes)` (32 bytes) |
| Per-file fact set | `file_hash` plus `(lang, grammar_version, extractor_version)`; immutable once written, shared across branches and worktrees |
| Node | `node_id = blake3(file_hash ‖ kind ‖ qname ‖ ordinal)[0:16]`; agent-facing id is `path::qname` (readable ids reduce hallucination, doc 03 §1.2) |
| Index version | `index_version = blake3(concat(sorted (path ‖ 0x00 ‖ file_hash)))` over `file` rows with `kind IN ('source','test','config')`; generated and vendored files are excluded (§4.4) |
| Tool result | `(tool, canonical_args_hash, index_version)`; `read` uses `(path, file_hash, lines)` instead so reads survive unrelated edits |

### 2.2 Schema

```sql
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);
-- keys: schema_version, index_version, extractor_version, regime, built_at, root_hash

CREATE TABLE file (
  path        TEXT PRIMARY KEY,           -- repo-relative, '/' separated
  file_hash   BLOB NOT NULL,
  lang        TEXT NOT NULL,              -- grammar id from §3.1
  kind        TEXT NOT NULL CHECK (kind IN ('source','test','generated','vendored','config')),
  lines       INTEGER NOT NULL,
  bytes       INTEGER NOT NULL,
  parse_ok    INTEGER NOT NULL,           -- 0 if the tree contains ERROR/MISSING nodes
  error_count INTEGER NOT NULL DEFAULT 0,
  mtime_ns    INTEGER NOT NULL
);

CREATE TABLE facts (                      -- immutable per content hash
  file_hash         BLOB NOT NULL,
  lang              TEXT NOT NULL,
  grammar_version   TEXT NOT NULL,
  extractor_version INTEGER NOT NULL,
  nodes             BLOB NOT NULL,        -- msgpack [node...]
  local_edges       BLOB NOT NULL,        -- msgpack [(src, dst, kind, line)] resolved inside the file
  refs              BLOB NOT NULL,        -- msgpack [(src, name, receiver_hint, line)] needing cross-file resolution
  imports           BLOB NOT NULL,        -- msgpack [(raw_spec, resolved_path|null, names[])]
  PRIMARY KEY (file_hash, lang, grammar_version, extractor_version)
);

CREATE TABLE node (                        -- materialised union for the current tree
  node_id     BLOB PRIMARY KEY,
  path        TEXT NOT NULL REFERENCES file(path) ON DELETE CASCADE,
  kind        TEXT NOT NULL CHECK (kind IN ('file','module','class','function','method','variable','test')),
  name        TEXT NOT NULL,
  qname       TEXT NOT NULL,               -- Outer.Inner.method
  sig         TEXT,                        -- one line, ≤ 200 chars, no body
  start_line  INTEGER NOT NULL, end_line INTEGER NOT NULL,
  start_byte  INTEGER NOT NULL, end_byte   INTEGER NOT NULL,
  parent_id   BLOB,
  visibility  TEXT,                        -- 'public','private','internal','export', null
  doc_line    TEXT                         -- first line of the doc comment, ≤ 120 chars
);
CREATE INDEX node_path ON node(path);
CREATE INDEX node_name ON node(name);
CREATE INDEX node_qname ON node(qname);

CREATE TABLE edge (
  src         BLOB NOT NULL,
  dst         BLOB NOT NULL,
  kind        TEXT NOT NULL CHECK (kind IN ('contains','imports','references','calls','inherits','tests')),
  confidence  REAL NOT NULL,               -- 1.0 exact ... 0.2 ambiguous
  resolved_by TEXT NOT NULL,               -- 'scope','same_file','import','receiver','global_unique','ambiguous','lsp'
  line        INTEGER,
  PRIMARY KEY (src, dst, kind)
);
CREATE INDEX edge_dst ON edge(dst, kind);

CREATE TABLE unresolved (                  -- references no rule could bind
  src BLOB NOT NULL, name TEXT NOT NULL, line INTEGER NOT NULL, candidates INTEGER NOT NULL
);

CREATE VIRTUAL TABLE ident_fts USING fts5(
  node_id UNINDEXED, path UNINDEXED, tokens, doc_line,
  tokenize = 'unicode61 remove_diacritics 0'
);
-- tokens = name + camelCase/snake_case/kebab splits + qname parts; BM25 via fts5's bm25()

CREATE TABLE chunk (                       -- optional, cAST-style
  chunk_id    BLOB PRIMARY KEY,            -- blake3(file_hash ‖ start_byte ‖ end_byte)
  path        TEXT NOT NULL,
  file_hash   BLOB NOT NULL,
  start_byte  INTEGER NOT NULL, end_byte INTEGER NOT NULL,
  node_ids    BLOB NOT NULL,               -- msgpack [node_id]
  tokens      INTEGER NOT NULL
);
CREATE TABLE embedding (
  chunk_id BLOB NOT NULL, model TEXT NOT NULL, dim INTEGER NOT NULL,
  vec BLOB NOT NULL,                       -- int8 quantised, dim bytes
  PRIMARY KEY (chunk_id, model)
);

CREATE TABLE coverage_run (
  run_id BLOB PRIMARY KEY, index_version BLOB NOT NULL, format TEXT NOT NULL, imported_at INTEGER NOT NULL
);
CREATE TABLE coverage (                    -- test node -> covered node, from the last imported run
  test_id BLOB NOT NULL, node_id BLOB NOT NULL, run_id BLOB NOT NULL, PRIMARY KEY (test_id, node_id)
);
CREATE TABLE test_map (                    -- materialised code -> test edges
  node_id BLOB NOT NULL, test_id BLOB NOT NULL,
  via TEXT NOT NULL CHECK (via IN ('import','call','coverage','name')),
  weight REAL NOT NULL, PRIMARY KEY (node_id, test_id, via)
);

CREATE TABLE ext_pkg (
  pkg_id BLOB PRIMARY KEY,                 -- blake3(ecosystem ‖ name ‖ version)
  ecosystem TEXT NOT NULL, name TEXT NOT NULL, version TEXT NOT NULL,
  source TEXT NOT NULL,                    -- 'installed','stubs','context7','docs-url'
  lockfile TEXT NOT NULL, fetched_at INTEGER NOT NULL
);
CREATE TABLE ext_sym (
  pkg_id BLOB NOT NULL, qname TEXT NOT NULL, kind TEXT NOT NULL,
  sig TEXT, doc TEXT,                      -- doc ≤ 600 chars
  PRIMARY KEY (pkg_id, qname)
);

CREATE TABLE tool_cache (
  tool TEXT NOT NULL, args_hash BLOB NOT NULL, version_key BLOB NOT NULL,
  result BLOB NOT NULL, tokens INTEGER NOT NULL, created_at INTEGER NOT NULL,
  PRIMARY KEY (tool, args_hash, version_key)
);
```

### 2.3 Node and edge extraction rules

| Node kind | Source | Notes |
|---|---|---|
| `file` | one per indexed path | `sig` is null |
| `module` | Go package clause, Rust `mod`, Python package `__init__`, Dart `library`, Kotlin/Java `package` | One per file where declared |
| `class` | class, struct, enum, interface, trait, protocol, object, extension (Swift/Kotlin, see §3.3), mixin (Dart) | `sig` is the declaration line without body |
| `function` | free functions, arrow functions bound to a top-level `const`, lambdas assigned to a name | anonymous lambdas are not nodes |
| `method` | function whose parent is a `class` | `qname = Class.method` |
| `variable` | top-level or class-level declarations; **not** locals | locals are used for scope resolution only |
| `test` | function or method matched by the language's discovery convention (gate-spec §5.2 table, last column) | also gets the `function`/`method` shape; `kind = 'test'` wins |

| Edge kind | Rule | Confidence |
|---|---|---|
| `contains` | AST parent to child | 1.0 |
| `imports` | import statement resolved to a file node via the language's module resolver (§3.2) | 1.0 resolved, edge omitted if not |
| `references` | identifier use bound to a definition by §3.2 rules, excluding call positions | rule-dependent |
| `calls` | call expression whose callee binds to a `function`/`method` | rule-dependent |
| `inherits` | extends, implements, `: Base`, Rust `impl Trait for`, Dart `with`/`implements` | 1.0 when target resolves |
| `tests` | test node to code node, via the test map (§2.6) | weight from `test_map` |

### 2.4 Lexical index

`ident_fts.tokens` for a node named `getUserSessionID` in `pkg/auth/session.go` is `getUserSessionID get user session id getusersessionid auth session`. Split rules: camel humps, digit boundaries, `_`, `-`, `.`, `::`, path segments of `qname`; acronym runs kept whole and also lowercased. Ranking is fts5 `bm25()` with column weights `tokens:1.0, doc_line:0.3`, plus a multiplicative prior of 0.5 for `kind = 'vendored'` and 0.7 for `kind = 'test'` unless the query names a test.

### 2.5 AST-chunk embeddings (optional)

Off in `lite`; on in `full` only when `[index.embeddings] model` is set. cAST merge rules (doc 05 §1.3): walk top-level nodes; a node whose text fits `max_tokens` (default 512) is a chunk; a larger node is split at its child boundaries recursively; consecutive small siblings are merged while the merged size stays under `max_tokens` and the merge does not cross a `class` boundary. Every chunk records the node ids it covers. Model is pluggable behind one trait (`embed(texts) -> [vec]`); default is a local model, remote providers are opt-in and flagged in §8. Vectors are stored int8; search is brute force over an in-memory matrix (no ANN dependency below 200k chunks).

### 2.6 Test-impact map

`test_map` is materialised from three sources, in ascending weight:

| `via` | Rule | Weight |
|---|---|---|
| `name` | test node name contains a code node name after splitting (`test_refresh_session` ↔ `refresh_session`) | 0.3 |
| `import` | test file imports the file containing the node (transitively to depth 2 through `imports` edges, weight halves per hop) | 0.6 / 0.3 |
| `call` | test node has a `calls` edge to the node, or to any node that calls it (one hop) | 0.9 / 0.5 |
| `coverage` | the last imported coverage run (lcov, Cobertura XML, `go test -coverprofile`, `coverage.py` JSON, Jacoco, `xccov`, `lcov.info` from Flutter) recorded the test executing the node's line range | 1.0 |

Coverage is imported by `saga index coverage <file>` and by the `saga gate check` adapter when a `CHECK:` produced a recognised coverage artefact. Coverage older than the node's file hash is kept with weight × 0.7 and marked `stale` in results.

### 2.7 External API cache

Populated only for packages present in a lockfile the repo actually has (`package-lock.json`, `pnpm-lock.yaml`, `yarn.lock`, `poetry.lock`, `uv.lock`, `requirements*.txt` with pins, `go.sum`, `Cargo.lock`, `gradle.lockfile`, `Package.resolved`, `pubspec.lock`). Sources in priority order: installed package on disk (type stubs, `.d.ts`, docstrings, `go doc`, rustdoc JSON, Swift interface files, Dart `dartdoc` JSON), then Context7 or a configured docs URL, only when `[index.extapi] network = true`. It is consulted by `search` and `symbol` only when the queried name has zero local candidates, which is the gating condition CloudAPIBench showed matters (doc 05 §6). Ecosystems: npm, PyPI, Go modules, crates.io, Maven, SwiftPM/CocoaPods, pub.dev.

### 2.8 Size estimates, 100k-line repository

Assumptions: ~1,000 source files, ~12k symbol nodes (one per 8 lines), ~60k edges (RepoGraph reports ~19 edges per node on SWE-bench repos; heuristics here produce fewer because locals are excluded), ~9k chunks.

| Table | Rows | Bytes/row | Size |
|---|---|---|---|
| `file` | 1,000 | 150 | 0.15 MB |
| `facts` | 1,000 | 6,000 | 6 MB |
| `node` + indexes | 12,000 | 320 | 3.8 MB |
| `edge` + index | 60,000 | 70 | 4.2 MB |
| `ident_fts` | 12,000 | 200 | 2.4 MB |
| `test_map`, `coverage` | 30,000 | 50 | 1.5 MB |
| `chunk` | 9,000 | 120 | 1.1 MB |
| `embedding` (384-dim int8) | 9,000 | 400 | 3.6 MB |
| `ext_sym` (50 packages) | 25,000 | 500 | 12.5 MB |
| **Total without embeddings and ext cache** | | | **≈ 19 MB** |
| **Total, everything on** | | | **≈ 35 MB** |

Linear scaling is expected to 1M lines (≈ 350 MB), which is the stated ceiling for v0.1; the Linux-kernel-sized case (2.1M nodes, doc 05 §1.2) is out of scope until measured.

---

## 3. Language support

### 3.1 Grammar matrix

| Lang id | Extensions | Grammar (pinned by commit in `grammars.lock`) | Module resolver | Test discovery |
|---|---|---|---|---|
| `ts`, `tsx`, `js`, `jsx` | `.ts .tsx .mts .cts .js .jsx .mjs .cjs` | tree-sitter-typescript, tree-sitter-javascript | `tsconfig.json` `paths`/`baseUrl`, package `exports`, relative, index files | gate-spec §5.2 |
| `py` | `.py .pyi` | tree-sitter-python | `sys.path` roots = repo root, `src/`, dirs with `__init__.py`; relative imports | same |
| `go` | `.go` | tree-sitter-go | `go.mod` module path, package directories | same |
| `rust` | `.rs` | tree-sitter-rust | `mod` tree from `lib.rs`/`main.rs`, `crate::`, `super::`, workspace `Cargo.toml` | same |
| `java` | `.java` | tree-sitter-java | package directories under `src/main/java`, `src/test/java`, Gradle source sets | same |
| `kotlin` | `.kt .kts` | tree-sitter-kotlin | package declaration, same-package implicit visibility | same |
| `swift` | `.swift` | tree-sitter-swift | SwiftPM targets (`Package.swift` parsed by tree-sitter), Xcode target directories; module = target | same |
| `dart` | `.dart` | tree-sitter-dart | `package:` via `.dart_tool/package_config.json`, relative, `part`/`part of` (§3.3) | same |
| `c`, `cpp` | `.c .h .cc .cpp .cxx .hpp .hh` | tree-sitter-c, tree-sitter-cpp | `#include` search: file dir, `include/`, `compile_commands.json` `-I` flags when present | ctest/gtest `TEST(` macros, Catch2 `TEST_CASE(` |

Any other file with a tree-sitter grammar Saga bundles is indexed for `file` nodes and the lexical index only; no edges. Files with no grammar are not indexed and remain grep-only.

### 3.2 Name resolution

Resolution runs per reference, first rule that binds wins, later rules never override an earlier binding.

| # | Rule | `resolved_by` | Confidence |
|---|---|---|---|
| 1 | Lexical scope: nearest enclosing definition in the same file (locals, parameters, class members via `self`/`this`) | `scope` | 1.0 |
| 2 | Same-file top-level definition | `same_file` | 0.95 |
| 3 | Explicit import binding: the name (or alias) appears in an `imports` row whose `resolved_path` is known, and that file defines the name | `import` | 0.95 |
| 4 | Receiver type: `x.m()` where `x` has a declared type, constructor call, or `new` in the same function or class, and a class of that name (or an ancestor via `inherits`, depth ≤ 3) defines `m` | `receiver` | 0.8 |
| 5 | Same package or module (Go, Java, Kotlin, Swift target) defines the name exactly once | `import` | 0.85 |
| 6 | Exactly one definition of the name in the whole repo, excluding vendored | `global_unique` | 0.6 |
| 7 | 2–5 definitions: an edge to each, `confidence = 0.5 / n` | `ambiguous` | ≤ 0.25 |
| 8 | More than 5, or zero: row in `unresolved` | | |

Results show confidence < 0.8 with a `~` prefix so the agent can escalate to `read` or to the LSP overlay.

### 3.3 Per-language heuristics and blind spots

| Lang | Specific rules | Known blind spots (documented in `status --explain`) |
|---|---|---|
| TS/JS | `export default` gets `qname = default`; re-exports followed one hop; `require()` treated as import; JSX component tags are `references` | Dynamic `import()` with computed paths; barrel files beyond one hop; DI containers (NestJS, InversifyJS); prototype patching; `this` in callbacks |
| Python | `from x import *` expands to the target's public names; decorators produce `references`; `@property` counts as method; `__init__.py` re-exports followed one hop | Duck typing (rule 4 rarely fires); metaclasses; `getattr`/`importlib`; Django/pytest fixtures by name (partially covered by `via = name`); monkeypatching |
| Go | Package-scoped resolution is rule 5; receiver methods attach to the type node; interface satisfaction is **not** an `inherits` edge | Interface dispatch: a call through an interface binds to the interface method only; `reflect`; code generation (`//go:generate` outputs are `generated` kind) |
| Rust | `impl` blocks attach methods to the type; `impl Trait for T` yields `inherits`; `use` trees expanded; `pub(crate)` recorded as visibility | Macros: `macro_rules!` bodies and derive outputs are opaque, calls inside macro invocations are unresolved; trait method dispatch binds to the trait's declaration; generic bounds |
| Java | Package + explicit + wildcard imports; overloads resolved by arity only; anonymous classes are not nodes | Reflection; Spring/Guice DI (`@Autowired` fields bind to the declared interface, not the bean); annotation processors (Lombok accessors do not exist in the AST) |
| Kotlin | Extension functions: `fun Foo.bar()` becomes `method` with `qname = Foo.bar` and `resolved_by = receiver` on calls; top-level functions are package-scoped; `companion object` members attach to the class | Delegation (`by`), Koin/Hilt DI, `inline`/`reified`, KMP `expect`/`actual` pairs (both indexed, `inherits` edge between them), coroutine builders' lambda receivers |
| Swift | Protocol extensions: `extension P { func f() }` becomes `method` on `P`; a call `x.f()` where `x: T` and `T` conforms to `P` (via `inherits`) binds through rule 4 with depth ≤ 3; `@objc`/`#selector` strings are `references` by name | Protocol default vs conforming-type override ambiguity (both get an edge, confidence 0.5); result builders (SwiftUI bodies parse, but view modifiers chain through generics); `@dynamicMemberLookup`; ObjC bridging headers; `Package.swift` plugins |
| Dart | `part`/`part of` files are merged into one logical library before extraction: nodes keep their own `path`, but scope and rule 2 apply across the library; `*.g.dart`, `*.freezed.dart` are `generated` and indexed for definitions only; mixins yield `inherits` | Flutter widget trees (a `build` method references dozens of constructors, weight-diluted); `GetIt`/Riverpod providers bind by type not name; `dynamic`; `noSuchMethod` |
| C/C++ | Headers and sources are separate nodes; a declaration in `.h` and a definition in `.c` with the same signature get a `references` edge (confidence 0.9); namespaces qualify `qname` | Preprocessor macros (opaque; function-like macros produce `unresolved` calls, the Codebase-Memory 0.58 case); templates; virtual dispatch; `#ifdef` branches are all parsed, so contradictory definitions coexist |

### 3.4 LSP overlay

An LSP client is started **only** for these requests, never for `search`, never at build time, never at session start:

| Request | Trigger | Server |
|---|---|---|
| `symbol(..., precise=true)` | agent asked for exact callers or references | ts: typescript-language-server; py: pyright; go: gopls; rust: rust-analyzer; java: jdtls; kotlin: kotlin-language-server; swift: sourcekit-lsp; dart: `dart language-server`; c/cpp: clangd |
| `impact(..., precise=true)` | gate seeding for a rename or signature change | same |
| `saga index rename` (CLI only) | human or agent renames a symbol | same |

Rules: the server gets 10 s to initialise and 5 s per request, then the heuristic answer is returned with `"precision": "heuristic"` and a one-line reason. LSP answers are merged as edges with `resolved_by = 'lsp'`, `confidence = 1.0`, cached under the current `index_version`, and never persisted into `facts`. The server is killed after 120 s idle. When `lsp = "off"` the `precise` argument is accepted and ignored, so tool schemas do not change between regimes (schema stability matters for the cache, §6).

---

## 4. Update strategy

### 4.1 Pipeline

```
watch ─▶ debounce 150 ms ─▶ hash changed paths ─▶ for each new hash:
        parse (tree-sitter, incremental where the old tree exists) ─▶ extract facts ─▶ store
     ─▶ rematerialise node/edge rows for changed files
     ─▶ reverse-dependency pass (§4.3)
     ─▶ recompute index_version ─▶ invalidate tool_cache rows keyed on the old version
     ─▶ refresh test_map rows touching changed nodes
```

The watcher uses FSEvents, inotify or ReadDirectoryChangesW (the `notify` crate), falls back to a 2 s polling scan when the watch limit is exhausted, and reports which mode it is in via `status`. Edits arriving through a harness tool (Edit, Write) are also visible to the watcher, so no hook is required; a `PostToolUse` adapter that calls `saga index update --paths` exists for harnesses running in containers without a working watcher.

### 4.2 Incremental re-parse by hash

A path whose bytes hash to a `facts` row already present (a revert, a branch switch, a formatter no-op) is re-linked without parsing. Otherwise the file is parsed, the new fact set stored, and the old materialised rows for that path deleted and replaced. `git checkout` across branches therefore costs one hash pass plus parsing of files never seen on either branch.

### 4.3 Reverse-dependency edge rebuild

For a changed file F with old node set N_old and new node set N_new:

1. Delete all `edge` rows with `src ∈ N_old` and all `unresolved` rows for F; recompute F's outgoing edges.
2. Let `D` = names in `(N_old ∪ N_new)` whose definition set changed (added, removed, moved, or re-signed).
3. Reverse dependents = files with an `imports` edge to F, plus files with an `unresolved` row or an `ambiguous` edge naming any name in `D`, plus files with any edge whose `dst ∈ N_old`.
4. For each reverse dependent, re-run resolution (§3.2) over its stored `refs` blob. No re-parse.
5. Files two hops away are not touched; a two-hop change can only affect confidence through rule 4 ancestors, and those are recomputed lazily on `symbol` queries against the stale flag.

### 4.4 Versioning and cache invalidation

| Change | `index_version` | `tool_cache` effect |
|---|---|---|
| Any source file content | changes | `search`, `symbol`, `impact` rows for the old version are dropped at the next compaction; replays against the old version still hit if the row survived |
| Generated or vendored file | unchanged (excluded from the hash) | none |
| Coverage import | unchanged | `impact` rows dropped |
| Grammar or extractor version bump | full rebuild; `meta.extractor_version` changes | all rows dropped |
| `.saga/config.toml` `[index]` change | rebuild if excludes or languages changed | all rows dropped |

`saga trace` records `index_version` on every tool call, so a replay is exact when the version matches and is marked `divergent` when it does not (doc 05 §4.2).

### 4.5 Time budgets

Measured on a 4-core laptop, warm page cache, and enforced by the latency suite (§9.1).

| Operation | Budget |
|---|---|
| Cold build, 100k lines | ≤ 10 s wall (Codebase-Memory indexes Django in ~6 s) |
| Cold build, 1M lines | ≤ 120 s |
| Single-file edit to queryable | p50 ≤ 300 ms, p95 ≤ 800 ms, p99 ≤ 1.5 s including the reverse-dependency pass |
| Branch switch touching 200 files | ≤ 3 s |
| `search` / `symbol` / `impact` query | p95 ≤ 50 ms from SQLite; embeddings add ≤ 150 ms |
| `status --probe` | ≤ 200 ms on 10k files |
| Embedding a changed chunk | asynchronous; results served without the vector until it lands |

A build that exceeds twice its budget prints a warning naming the slowest 5 files and continues; it never blocks a session start.

### 4.6 Parse errors, generated and vendored files

| Case | Behaviour |
|---|---|
| Tree contains `ERROR` or `MISSING` | File stored with `parse_ok = 0`; nodes inside `ERROR` subtrees dropped; nodes outside kept; `status` lists the file with its first error line. The file is not excluded, because a half-edited file is exactly what the agent is working on |
| File > 2 MiB or a line > 5,000 chars | Indexed as a `file` node only, flagged `minified` |
| Binary or invalid UTF-8 | Skipped |
| Generated: `@generated` or `DO NOT EDIT` in the first 20 lines, `*.pb.go`, `*_pb2.py`, `*.g.dart`, `*.freezed.dart`, `*.generated.swift`, `R.java`, `build/`, `dist/`, `.dart_tool/`, `target/`, `DerivedData/` | `kind = 'generated'`: definitions indexed so imports resolve; excluded from `search` by default, from the map, and from `impact` suggestions |
| Vendored: `node_modules/`, `vendor/`, `Pods/`, `Carthage/`, `.build/`, `third_party/`, `*.min.js` | `kind = 'vendored'`: same treatment as generated; excluded from `index_version` |
| User overrides | `[index] include = [...]`, `exclude = [...]`, `generated = [...]` globs; `.sagaignore` uses gitignore syntax |

---

## 5. Agent-facing tools

Four tools, fixed names, fixed schemas across regimes and languages. Defaults are chosen from the evidence: depth 1 (RepoGraph), signatures not bodies (10× token reduction in Codebase-Memory), `k = 10`, every hit carries `path:line` so the model can escalate to `read`.

### 5.1 Result envelope and rendering

Every tool returns a JSON object with `index_version`, `truncated` (bool), `precision` (`exact | heuristic | mixed`) and the payload. The MCP text rendering is a compact line format; JSON is returned only when `format = "json"` is requested. Ceilings are enforced by truncating the lowest-ranked items and setting `truncated = true` with a hint line `… N more; narrow the query or raise k`.

| Tool | Default ceiling | Hard ceiling |
|---|---|---|
| `search` | 1,200 tokens | 3,000 |
| `symbol` | 1,500 tokens | 4,000 |
| `read` | 4,000 tokens (200 lines) | 12,000 (600 lines) |
| `impact` | 1,500 tokens | 4,000 |

### 5.2 `search`

```json
{
  "name": "search",
  "description": "Find code by identifier, path or short phrase. Returns path:line, symbol and a snippet. Use grep for regexes and non-code files.",
  "input_schema": {
    "type": "object",
    "properties": {
      "query": {"type": "string", "minLength": 1, "maxLength": 300},
      "mode":  {"type": "string", "enum": ["auto", "symbol", "lexical", "semantic", "path"], "default": "auto"},
      "k":     {"type": "integer", "minimum": 1, "maximum": 50, "default": 10},
      "lang":  {"type": "string"},
      "path_prefix": {"type": "string"},
      "kinds": {"type": "array", "items": {"type": "string", "enum": ["class","function","method","variable","test","file"]}},
      "include_generated": {"type": "boolean", "default": false},
      "snippet_lines": {"type": "integer", "minimum": 0, "maximum": 12, "default": 3},
      "format": {"type": "string", "enum": ["text", "json"], "default": "text"}
    },
    "required": ["query"]
  }
}
```

Result item: `{path, line, end_line, symbol_id, kind, sig, score, snippet, source: "symbol|lexical|semantic|ext"}`. Text rendering, one hit per line group:

```
src/auth/session.py:41  method  SessionStore.refresh(self, token: str) -> Session   [0.92]
    41 | def refresh(self, token: str) -> Session:
    42 |     s = self._load(token)
```

Auto-mode routing (first match wins; two modes run and merge by reciprocal rank fusion where stated):

| Query shape | Route |
|---|---|
| Contains `/` or ends in a known extension | `path` (prefix and fuzzy path match), then `lexical` |
| Single token matching `^[A-Za-z_][A-Za-z0-9_.:#]*$` | `symbol` (exact and prefix on `name`/`qname`) fused with `lexical` |
| Quoted string | `lexical` exact phrase |
| Contains regex metacharacters (`\ ^ $ * + ? [ ] ( ) |`) outside quotes | `lexical` over the literal parts, plus the hint line `regex detected: grep is exact for this` |
| Three or more words with a stopword, or ends in `?` | `semantic` fused with `lexical` when embeddings are on; `lexical` over split identifiers and `doc_line` otherwise |
| Otherwise | `lexical` |

If every local route returns zero hits and the query is a single token, the external API cache (§2.7) is consulted and hits are marked `source: "ext"`.

### 5.3 `symbol`

```json
{
  "name": "symbol",
  "description": "Definition and depth-1 neighbourhood of a symbol: signature, callers, callees, imports, subclasses, tests. Signatures only; use read for bodies.",
  "input_schema": {
    "type": "object",
    "properties": {
      "id":    {"type": "string", "description": "path::qname from search, or a bare name"},
      "edges": {"type": "array", "items": {"type": "string", "enum": ["callers","callees","references","imports","imported_by","inherits","subclasses","tests","contains"]},
                "default": ["callers","callees","inherits","tests"]},
      "depth": {"type": "integer", "minimum": 1, "maximum": 2, "default": 1},
      "limit_per_edge": {"type": "integer", "minimum": 1, "maximum": 50, "default": 10},
      "precise": {"type": "boolean", "default": false, "description": "Confirm callers/references with a language server if enabled"},
      "format": {"type": "string", "enum": ["text", "json"], "default": "text"}
    },
    "required": ["id"]
  }
}
```

A bare name with several definitions returns the candidate list (≤ 10, with `path:line`) instead of a neighbourhood. `depth = 2` is permitted for `contains` and `inherits` only; for `callers`/`callees` it is clamped to 1 with a note, because the two-hop result is the one the evidence says hurts. Text rendering:

```
src/auth/session.py:38-71  class  SessionStore(BaseStore)      "Persists sessions in Redis."
  contains  refresh(self, token) -> Session :41   _load(self, token) :55   purge(self) :63
  callers   ~api/routes/login.py:88 login_handler   api/middleware.py:30 SessionMiddleware.__call__
  callees   store/redis.py:12 RedisClient.get   store/redis.py:20 RedisClient.set
  inherits  store/base.py:9 BaseStore
  tests     tests/auth/test_session.py:14 test_refresh_rotates_token [coverage]
precision: heuristic (2 of 5 callers marked ~; add precise=true to confirm)
```

### 5.4 `read`

```json
{
  "name": "read",
  "description": "Exact source by path and line range, or by symbol id (returns the symbol's body). The only tool that returns bodies.",
  "input_schema": {
    "type": "object",
    "properties": {
      "target": {"type": "string", "description": "repo-relative path, or path::qname"},
      "lines":  {"type": "string", "pattern": "^[0-9]+(-[0-9]+)?$", "description": "start or start-end; default: whole symbol, or first 200 lines of a file"},
      "context": {"type": "integer", "minimum": 0, "maximum": 30, "default": 0, "description": "extra lines around a symbol"},
      "format": {"type": "string", "enum": ["text", "json"], "default": "text"}
    },
    "required": ["target"]
  }
}
```

Output is numbered lines, never reformatted, with `file_hash` in the JSON form so an edit tool can detect staleness. A `read` of the same path later in a session supersedes the earlier result for context-editing purposes (§6.3). `read` never returns generated or vendored content beyond 60 lines unless `lines` is explicit.

### 5.5 `impact`

```json
{
  "name": "impact",
  "description": "For a diff or a set of paths: affected symbols, dependent files, tests to run, and the test command. Feeds saga gate.",
  "input_schema": {
    "type": "object",
    "properties": {
      "paths": {"type": "array", "items": {"type": "string"}},
      "diff":  {"type": "string", "description": "unified diff; default: git diff against the contract BASE or HEAD"},
      "depth": {"type": "integer", "minimum": 1, "maximum": 2, "default": 1},
      "precise": {"type": "boolean", "default": false},
      "max_tests": {"type": "integer", "minimum": 1, "maximum": 200, "default": 30},
      "format": {"type": "string", "enum": ["text", "json"], "default": "text"}
    }
  }
}
```

Result: `{changed_symbols[], dependents[{path, via, confidence}], tests[{test_id, path, via, weight, stale}], suggested_test_commands[{runner, command, covers}], unresolved_count}`. Changed symbols are computed by mapping diff hunks to node line ranges of the pre-edit and post-edit files; a hunk outside any symbol maps to the `file` node. Tests are ranked by summed weight and cut at `max_tests`; the command list groups selected tests per runner using the bench §2.6 runner table (`pytest path::name`, `vitest -t`, `go test -run`, `cargo test name`, `gradle test --tests`, `swift test --filter`, `flutter test path --name`). If the selected set is empty, the suggestion is the runner's whole-suite command with `covers: "all (no impact edges)"`, never silence.

### 5.6 Repo map

The map is the only index content in the cached prefix. It is generated by `saga index map` and embedded by the adapter (§7.3).

| Property | Rule |
|---|---|
| Budget | ≤ 1,000 tokens in `full`, ≤ 500 in `lite`, 0 in `off`; measured with the harness's tokenizer where exposed, else cl100k as a proxy, then multiplied by 1.1 |
| Ranking | Personalised PageRank over `node` with `calls`, `references`, `imports`, `inherits` edges weighted by confidence (aider's construction); personalisation vector = uniform 0.3, plus 0.4 spread over nodes in files matching the active contract's `IN:` globs if a `.saga/contract.md` exists, plus 0.3 over files changed in the last 20 commits |
| Rendering | Directory tree collapsed to directories containing ranked nodes; under each file, up to 4 signatures in rank order; tests and generated files omitted; `…` marks omissions |
| Change policy | Regenerated only when `index_version` changes **and** a session is not active; an active session keeps the map it started with (a rewritten prefix costs a full cache write, claude-code #91514). `saga index map --refresh` forces it and prints the estimated cache-write cost from the trace ledger |
| Content hash | `map_hash` recorded in `meta` and in `saga trace` so a bench run can prove which map the agent saw |

---

## 6. Prompt-cache discipline

The ordering `tools → system → messages` means a mutated tool list or system text invalidates the entire cache (doc 05 §4.2). The index therefore has exactly one stable artefact and everything else is a late message.

### 6.1 Stable prefix versus injected result

| Content | Where | Stability |
|---|---|---|
| Four tool definitions (§5), ≤ 120 tokens of description each, ≤ 550 tokens total | tools block | Fixed for the life of the release; regime changes do not alter schemas (§3.4 rule) |
| Repo map (§5.6) | system or instructions file, adapter-specific | Fixed for the session |
| One-line usage rule: "search returns path:line; symbol returns signatures; read returns bodies; grep remains available" | system | Fixed |
| Every `search`/`symbol`/`read`/`impact` result | tool result message | Injected, expiring |
| Regime, index version, LSP availability | **never** in the prefix; available via `status` | |

Adapters must not re-register tools, rewrite descriptions, or append to the system prompt mid-session. `saga doctor` checks the harness config hash before and after a session and flags any drift as an index-layer failure.

### 6.2 Avoiding resident-context bloat

codegraph's own benchmark reports 62% fewer tokens processed but 80% more tokens resident at session end (67k vs 18k) because dense answers stay in view (doc 04 §2.4). The index treats resident tokens as a first-class cost:

| Mechanism | Rule |
|---|---|
| Result ceilings | §5.1, enforced in the core, not the adapter |
| Signatures only | `symbol` never returns bodies; `search` snippets default to 3 lines |
| Supersession | A later `read` of the same path or a later `symbol` of the same id marks the earlier result `stale`; adapters that support context editing clear stale results first |
| Expiry | Each result carries `expires_after_turns` (default 12 for `search`, 20 for `symbol` and `impact`, 30 for `read`). Where the harness exposes context editing (Anthropic `context_management` `clear_tool_uses`, Claude Code's tool-result clearing), the adapter passes the expiry; where it does not, the result footer says `expires in N turns; re-run if needed` and the ledger records the tokens as resident |
| Ledger | `saga trace` attributes every result's tokens to `component = index` and reports `resident_tokens_index` at each turn and at session end |
| Bench cost | `resident_context_tokens_end` is a mandatory reported metric (§9.2), with a pre-registered ceiling of 1.5× the control arm |

### 6.3 Context-editing hooks by harness

| Harness | Mechanism | Fallback |
|---|---|---|
| Claude Code, Anthropic API | `context_management.edits[clear_tool_uses]` with `keep` and `trigger` set from the expiry table. The stub form `search("x") → 8 hits, top: path:line` is what the mem state block's `FILES` line and the trace import context (trace-spec §8.2) carry after a compaction; `PreCompact` itself cannot modify the transcript (mem-spec §4.4, verified), so no PreCompact stub-rewriter exists | Result footer plus ledger |
| Codex CLI | No context editing; `notify` cannot edit history | Footer, ledger; ceilings halved (`[index.ceilings] scale = 0.5`) when `harness = codex` |
| Gemini CLI | Version-dependent; adapter probes and records | Footer, ledger |
| `bare` bench adapter | Implements `clear_tool_uses` directly in its 300-line loop so the bench can ablate expiry | |

---

## 7. Surfaces

Per ADR 0003: one core binary, a CLI, an MCP server, and translation-only adapters.

### 7.1 CLI

```
saga index build     [--root <dir>] [--regime auto|lite|full] [--jobs <n>] [--embeddings] [--json]
saga index update    [--paths <p>...] [--watch] [--json]
saga index query     search|symbol|read|impact <args as --key value or --json '<obj>'> [--format text|json]
saga index map       [--budget <tokens>] [--refresh] [--contract <file>] [--json]
saga index impact    [--diff <file|->] [--paths <p>...] [--base <rev>] [--precise] [--json]
saga index status    [--probe] [--explain] [--json]
saga index coverage  <file> [--format lcov|cobertura|go|coverage.py|jacoco|xccov]
saga index rename    <path::qname> <new_name> [--dry-run]
saga index serve     [--stdio] [--socket <path>]          # MCP server
saga index clean     [--cache-only]
```

| Subcommand | Reads tree | Writes index | Starts LSP |
|---|---|---|---|
| `build`, `update` | yes | yes | no |
| `query`, `map`, `impact` | no (except `read`) | cache rows only | only with `--precise` |
| `status` | probe only | no | no |
| `coverage` | the artefact | `coverage`, `test_map` | no |
| `rename` | yes | yes, after applying | yes |
| `serve` | on demand | cache rows, watcher updates | on demand |

Exit codes follow the uniform table of contracts §4: 0 ok, 1 result empty or regime `off` (`query` and `map` print why), 2 usage or schema error, 3 budget refusal (`map --budget` unsatisfiable, cold build over 2× budget with `--strict`), 5 integrity: index stale relative to the tree when `--require-fresh` is set, 6 environment refusal (index directory symlinked outside the repo, unreadable, or not owner-private).

`saga index status --json`:

```json
{
  "schema": "saga.index.status/1",
  "regime": "full", "regime_reason": "source_lines=143210 >= 80000",
  "source_files": 1381, "source_lines": 143210, "top_dirs": 9, "cross_dir_ratio": 0.31,
  "index_version": "blake3:…", "built_at": "2026-09-02T10:14:03Z", "extractor_version": 3,
  "languages": {"ts": 812, "py": 402, "go": 167},
  "parse_errors": 4, "generated": 61, "vendored": 2210,
  "embeddings": {"enabled": true, "model": "local:bge-small-1.5", "pending": 0},
  "lsp": {"policy": "auto", "tier": "frontier", "enabled": false},
  "coverage": {"runs": 1, "newest": "2026-09-01T18:02:11Z", "stale_nodes": 12},
  "watcher": {"mode": "fsevents", "healthy": true},
  "map": {"tokens": 962, "hash": "blake3:…"},
  "budgets": {"cold_build_s": 7.9, "incremental_p95_ms": 412}
}
```

### 7.2 MCP server

`saga index serve --stdio` exposes exactly the four tools with the schemas of §5. Tool names are namespaced by the harness prefix (`mcp__saga__search`); the core names never change. `tools/list` returns the same bytes for the life of a release (schema hash recorded in `status`), and the server refuses to start in regime `off` with a one-line stderr reason so a harness does not register empty capability.

```json
{
  "tools": [
    {"name": "search", "description": "Find code by identifier, path or short phrase. Returns path:line, symbol and a snippet. Use grep for regexes and non-code files.", "inputSchema": {"$ref": "#/schemas/search"}},
    {"name": "symbol", "description": "Definition and depth-1 neighbourhood of a symbol: signature, callers, callees, imports, subclasses, tests. Signatures only; use read for bodies.", "inputSchema": {"$ref": "#/schemas/symbol"}},
    {"name": "read",   "description": "Exact source by path and line range, or by symbol id. The only tool that returns bodies.", "inputSchema": {"$ref": "#/schemas/read"}},
    {"name": "impact", "description": "For a diff or paths: affected symbols, dependent files, tests to run, and the test command.", "inputSchema": {"$ref": "#/schemas/impact"}}
  ]
}
```

Descriptions are prompts (doc 03 §1.2); they are versioned text under `core/tools/descriptions/` and changing one is a release, with the bench canary re-run.

### 7.3 Adapter registration

| Harness | Registration | Map placement |
|---|---|---|
| Claude Code | `.mcp.json` entry `{"saga": {"command": "saga", "args": ["index", "serve", "--stdio"]}}`; no hooks required; the optional `PostToolUse` `saga index update --paths` step (§4.1) runs inside the composed `saga hook` entry (contracts §1) when enabled | Managed block in `CLAUDE.md` between `<!-- saga:map -->` markers, written once per session start by `saga index map`; never rewritten mid-session |
| Codex CLI | `~/.codex/config.toml` `[mcp_servers.saga] command = "saga" args = ["index","serve","--stdio"]` | Managed block in `AGENTS.md` |
| Gemini CLI | `settings.json` `mcpServers.saga` | Managed block in `GEMINI.md` or its successor file |
| OpenCode | `opencode.json` `mcp.saga` | Managed block in `AGENTS.md` |
| CI / no MCP | CLI only | none |

Adapters are under 150 lines, contain no ranking or resolution logic, and pass the recorded-fixture suite (§9.1). The managed block carries `map_hash` so `saga doctor` can verify the file the agent read is the map the index produced.

### 7.4 Feeding `saga gate`

`saga gate init --from-diff` calls `saga index impact --json --base <BASE>` and drafts one runnable gate per suggested command:

```markdown
- [ ] T1: tests affected by the change pass
    CHECK: pytest tests/auth/test_session.py::test_refresh_rotates_token tests/api/test_login.py -q
    EXPECT: /passed/
    FROM: R1 "…"
    RED: baseline
    WITNESS: src/auth/session.py
    EVIDENCE: pending
```

`WITNESS:` is seeded from `changed_symbols` paths. When `unresolved_count > 0` or any selected test has `stale = true`, the drafted gate gets a comment line `# impact precision: heuristic; N unresolved references` and `saga gate lint` warns if the whole-suite command was substituted. The gate checker also re-runs `impact` at `check` time and reports, without blocking, tests that became affected after the contract was drafted.

---

## 8. Privacy

| Rule | Mechanism |
|---|---|
| The index never leaves the machine | `.saga/index/` is gitignored by `build`; no network call exists in `build`, `update`, `query`, `map`, `impact`, `status`. `serve` binds stdio or a Unix socket, never TCP |
| External API cache is opt-in per network use | `[index.extapi] network = false` default; when true, only package names and pinned versions from the lockfile are sent, never repo paths or code; an agent-triggered fetch is a `network_fetch` segment under guard's `[net] fetch` policy (guard-spec §2.5, §2.6), since the contract has no side-effect field (gate-spec §1.3) |
| Embeddings are local by default | A remote embedding provider requires `[index.embeddings] remote = true`; `status` then prints `code text leaves the machine for embedding` and the bench disclosure block records it |
| Masking on every result | Results pass through `saga guard mask` before rendering: gitleaks-class secret rules, configured PII patterns, `.env` values, absolute paths outside the repo root rewritten to `<outside-repo>`; masking is reversible only inside `saga guard`'s placeholder store, never in the index |
| Snippets from denied files | Files matching `saga guard`'s credential deny-list (`.env*`, `*.pem`, `id_rsa*`, keychains) are indexed as `file` nodes only; `read` on them is refused with the guard's reason |
| Index directory hygiene | Opened no-follow; must be a real, owner-private directory inside the canonical repo root; otherwise exit 6 (gate-spec §8 rules) |
| Retention caveat | Masking reduces what a model provider retains; it does not change retention policy (doc 09 §3.5). `status --explain` says so |

---

## 9. Test plan and ablation

### 9.1 Tests for the layer

| Suite | Content | Pass bar |
|---|---|---|
| **Grammar fixtures** | Per language in §3.1, ≥ 30 fixture files covering every node kind, every edge kind, every module-resolver path, and every blind-spot row (which must produce the documented `unresolved` or low-confidence result, not a wrong edge). Golden `facts` JSON per fixture; grammar version bumps re-run the suite | 100% golden match; a bump that changes a golden requires a reviewed diff |
| **Name-resolution corpus** | 3 repos per language (10k–150k lines) with SCIP or LSP ground truth for `calls` and `references`. Report precision and recall per rule and per language | Typed languages: precision ≥ 0.90, recall ≥ 0.75 on `calls`; Python/JS: precision ≥ 0.85, recall ≥ 0.60; no language ships without its row in `status --explain` |
| **Incremental correctness** | Property test: from a seed repo apply 200 random edit sequences (insert, delete, rename symbol, move file, revert, branch switch, formatter run); after each, `incremental index == cold build` on `node`, `edge`, `unresolved`, `test_map` and `index_version` | Byte-identical, 100% of sequences |
| **Latency** | The §4.5 table on the reference machine, recorded per commit | Every budget met; regression > 20% fails CI |
| **Token ceilings** | Adversarial repos (10k-symbol single file, 500-caller function, 40 KB signature) | No result exceeds its hard ceiling; `truncated` set correctly |
| **Routing** | 200 labelled queries (identifiers, paths, phrases, regexes) | Route agrees with the label ≥ 0.95; regex queries always carry the grep hint |
| **Map** | Budget met with three tokenizers; personalisation shifts with `IN:`; map unchanged across a session despite edits | all |
| **Cache** | Replay of a recorded session against the same `index_version` returns identical bytes; against a different version marks `divergent` | all |
| **Adapters** | Recorded config fixtures per harness version; `tools/list` byte-stable across 50 restarts; managed block idempotent; `doctor` detects a mutated description | all |
| **Privacy** | Canary secret placed in a fixture file must never appear in any result, map, or log; network sandbox proves zero egress during build/query with `extapi.network = false` | canary never found; zero packets |
| **LSP overlay** | Server missing, slow (> 10 s), crashing mid-request; each yields the heuristic answer with `precision: heuristic` | all |
| **Self-index** | Saga's own repo is indexed in CI; `impact` on each PR must include the tests CI subsequently runs and fails | recall ≥ 0.9 on 50 historical PRs |

These validate implementation behaviour only. Whether the layer changes what a model does is §9.2's job, and until it runs no number about `saga index` appears in the README.

### 9.2 Ablation on `saga bench`

Position in the stacking ladder (bench-spec §4.3): `gate → guard → index`. The headline number is the delta against `base + gate + guard`.

| Arm | Contents |
|---|---|
| C | base + gate + guard, index blocked: `saga` PATH shim exits 127 for `index`, MCP server not registered and its socket path denied, `.saga/index/` replaced by a read-denied sentinel, no map block (bench-spec §4.2) |
| D1 | C + four tools, map, expiry, regime `auto` |
| D2 | D1 without the map (tools only) |
| D3 | D1 with `lsp = "on"` regardless of tier |
| D4 | D1 with embeddings on (`full` regime tasks only) |

Only D1 vs C is funded at `dev` tier; D2–D4 are `publish`-tier or pre-registered follow-ups.

Report keys are the index row of bench-spec §5.11 (`localization_acc5`, `resident_context_tokens_end`, `tool_exposure`, `grep_calls`, `line_recall`); resolve rate is bench-spec §5.1 and §5.2.

| Metric | Definition | Role |
|---|---|---|
| **Localization acc@5** | Fraction of runs in which at least one gold-diff file is among the first 5 distinct files the agent reads or receives as a `search`/`symbol` hit, from `trace.jsonl` | Primary |
| **Resolve rate** | pass@1 and pass^k on the hidden oracle (bench-spec §5.1–5.2) | Primary |
| **Resident context tokens at session end** | Input tokens of the final model call, minus the cached prefix, from the ledger; reported as median and as the ratio treatment/control | **Mandatory cost; pre-registered ceiling 1.5×** |
| Tokens and cost per solved task | bench-spec §5.6 | Secondary |
| Tool exposure | `search`/`symbol`/`read`/`impact` call counts; a treatment run with zero index calls is `component_unused` | Validity |
| Grep displacement | grep calls per run in each arm; the index is not meant to reduce this to zero | Descriptive |
| Regime crossover | Tasks binned by `source_lines`; delta reported per bin to revise §1.3 thresholds | Design input |
| Line-level recall | Fraction of gold-diff hunks whose line range the agent read before its first edit | Honest limit (expected ≈ 0.15, doc 05 §10 item 1) |

Design: 30 tasks per language over the M0 set (TypeScript, Python, Go), k = 5 at `dev`, k = 10 and two model families at `publish`; Wilcoxon on per-task medians, bootstrap CIs over tasks. A `size ≥ L` stratum is required, since the evidence says the effect concentrates there. The pre-registration names the equivalence bound for "does not hurt cost per solved" and the ceiling for resident tokens; breaching the ceiling is reported as a failure of the layer even when resolve rate rises. A component that does not move localization acc@5 at k = 5 with the control arm blocked is cut.

---

## 10. Open problems carried from doc 05 §10

| # | Problem | Experiment that would settle it |
|---|---|---|
| 1 | **Line-level recall (~15%) is the bottleneck no index fixes.** | Add arm D5: `search` results re-ranked by a small learned reranker trained on PR data (CORE-Bench SFT recovered 20.3→32.8 NDCG). Metric: line-level recall from §9.2 and resolve rate. If recall does not move ≥ 10 points at k = 10, the reranker is out and the README keeps the 15% number |
| 2 | **Name resolution without a build.** | The §9.1 corpus gives per-rule precision and recall against SCIP. Then arm D3 (LSP on) vs D1 per model tier on the bench: if LSP raises resolve rate for `tier = small` and not for `frontier`, the `lsp = "auto"` rule stands; if it helps neither, the overlay is cut; if it helps both, it becomes default |
| 3 | **One leak-audited index ablation exists; it needs replication.** | §9.2 at `publish` tier is that replication: two model families, control arm blocked and instrumented, tasks post-cutoff, manifest published. Success is a delta in the same direction with CIs excluding zero on both families |
| 5 | **Determinism versus prompt caching: injected knowledge vs stable prefix.** | Arm D2 (no map) vs D1 measures the map's value; the ledger's `cache_read`/`cache_write` per turn measures its cost, including the mid-session refresh case (`map --refresh` forced at turn 10 in a sub-arm). Decision rule: keep the map only if resolve rate or acc@5 rises and cache-write tokens per solved task rise less than the pre-registered bound |
| 6 | **No principled model of when a richer tool pays for its tokens.** | Fit `Δresolve = f(model_tier, source_lines, tool_precision)` over the D1/D3 cells binned by regime; publish the fitted crossover with CIs. Until then the §1.3 thresholds are labelled priors in `status` |
| 7 | **External API grounding without independent evidence.** | Task subset with fast-moving SDKs (Swift 6, iOS 26, doc 06 A.9): arm with `extapi` off, arm with gated cache (§2.7 rule), arm with ungated retrieval on every query. Metrics: hallucinated-API rate from compiler errors in the trace, resolve rate. CloudAPIBench predicts gated > off > ungated; if gated does not beat off, the cache is cut |

Item 4 (memory that helps) is `saga mem`'s problem and item 8 (test oracles) is `saga gate`'s; neither is carried here.
