# `saga gate` — technical specification

*Draft v0.1, 2026-09-02. Implements doc 09 §3.2. Adopts the ledger grammar and fail-closed rules of unlazy (doc 08 Part 2), the verification-evidence and reward-hacking findings of doc 03 §2.5/§3/§4, and ADRs 0002 (mechanisms over prompts) and 0003 (one core, three surfaces).*

---

## 1. Purpose and non-goals

### 1.1 Purpose

`saga gate` is the layer that turns "done" into a machine-decidable proposition. It answers four questions with files, not with model output:

| Question | Mechanism |
|---|---|
| What was the agent authorised to change and to claim? | **Contract** (§2) |
| Does the check that certifies the work actually detect the work's absence? | **Red proof** (§3) |
| Did this specific run pass, in this environment, on this tree? | **Evidence** (§4) |
| Did the agent get to green by breaking the measurement? | **Diff guards** (§5) |

Enforcement is a hook where the harness has one and CI where it does not (§6). The core is a single binary with no knowledge of any vendor's prompt format.

The layer targets the six complaints doc 09 §4 assigns to `gate`: *said done / does not work*, *touched what I did not ask*, *stopped halfway*, *deleted or weakened my test*, *ignored the rules we agreed*, and *same prompt, different result* (gates collapse the branch point at the end of a run).

### 1.2 Non-goals

- **Not a sandbox.** `CHECK:` is arbitrary shell code with the caller's permissions. The boundary is review and approval (§8). Borrowed verbatim from unlazy's honesty standard: *approval is consent, not isolation*.
- **Not a semantic oracle.** No component infers whether an English outcome and a shell command mean the same thing. Red proof narrows this; it does not close it.
- **No LLM judge in the enforcement path.** Doc 03 §4 records LLM judges mislabelling 85.6% of transcripts once told the label had consequences. Every decision in this layer is a parse, a hash, an exit status, a glob, or a tree-sitter node comparison.
- **No prompt-only rules.** Per ADR 0002, any rule expressible as a check is a check. Text emitted to the agent is a bounded translation of a check result (§9).
- **Not a planner.** No depth trees, no wave/dispatch state machine, no PLAN.md. Orchestration is out of scope for this layer.
- **No effect claims before `saga bench`.** Per ADR 0001, §10.3 is the design that would license a number; until then the README says nothing quantitative.

---

## 2. The contract

### 2.1 Files

| Path | Written by | Purpose |
|---|---|---|
| `.saga/contract.md` | agent or human | The active contract. Committed. |
| `.saga/request.md` | `saga gate init` | Verbatim copy of the request text, sentence-numbered. Committed. |
| `.saga/evidence/<gate>.json` | checker only | Per-gate evidence (§4). Committed. |
| `.saga/red/<gate>.json` | checker only | Red proof records (§3). Committed. |
| `.saga/observed/` | checker only | Ring buffer of parsed failure outputs, for guard G-HARDCODE. **Not** committed. |
| `.saga/config.toml` | human | Test globs, registries, language overrides, budgets. Committed. |
| `~/.saga/approved/` | `saga gate approve` | Approval records. Never inside the repo (§8). |

Only `contract.md`, `request.md` and `config.toml` are agent-writable. Writes by any other actor to `evidence/`, `red/` or the approval store are a hard guard violation (§5, G-LEDGER) and a checker-level refusal.

### 2.2 Grammar

The parser is strict, shared by every subcommand and every adapter, and fails closed: an invalid contract is never `ALL MET`, it is exit 2.

```ebnf
contract      = title , { header-field } , { block } ;

title         = "#" , SP , text , NL ;
header-field  = header-key , ":" , SP , value , NL ;
header-key    = "CONTRACT" | "REQUEST" | "IN" | "OUT" | "SIDE-EFFECTS" | "BASE" ;

block         = gate-block | statement | blank | fenced-code | prose ;

gate-block    = gate-line , { attr-line } ;
gate-line     = "- [" , ( " " | "x" ) , "] " , id , ":" , SP , outcome , NL ;
attr-line     = indent , attr-key , ":" , SP , value , NL ;
attr-key      = "CHECK" | "EXPECT" | "CWD" | "FROM"
              | "RED" | "RED-CHECK" | "RED-EXPECT" | "WITNESS" | "EVIDENCE" ;

statement     = abandon-line | waive-line ;
abandon-line  = "ABANDON:" , SP , id , SP , reason , NL ;
waive-line    = "WAIVE:" , SP , guard-id , SP , path , SP , hunk-hash , SP , reason , NL ;

id            = ALPHA , { ALNUM | "-" | "_" } ;          (* unique per file *)
indent        = SP , SP , { SP } ;                        (* 2-4 spaces; TAB invalid *)
guard-id      = "G-" , UPPER , { UPPER | "-" } ;
hunk-hash     = 12 * HEXDIG ;
value         = { CHAR - NL } ;                           (* non-empty after trim *)
```

Lines inside fenced code blocks are ignored, using CommonMark fence rules (same character, closing fence at least as long, ≤3 leading spaces). Original CRLF/LF style is preserved on any checker write.

### 2.3 Field semantics

| Field | Cardinality | Meaning |
|---|---|---|
| `CONTRACT:` | 1 | Stable slug. Names the evidence namespace. |
| `REQUEST:` | 1 | `sha256:<64hex>` of `.saga/request.md`. Mismatch = exit 2. |
| `IN:` | 1..n | Comma-separated repo-relative globs the work may touch. |
| `OUT:` | 0..n | Globs the work must not touch. `OUT` wins over `IN`. |
| `SIDE-EFFECTS:` | 0..1 | Allow-list from a closed vocabulary: `network`, `install`, `migrate`, `write-outside-repo`, `git-history`, `none`. Unknown token = exit 2. |
| `BASE:` | 1 | Git rev the diff guards diff against (default `HEAD` at `init`). |
| `CHECK:` | 0..1 | Shell command. Present ⇒ `EXPECT:` required. |
| `EXPECT:` | 0..1 | Plain substring, or `/pattern/flags` for a regex. Present ⇒ `CHECK:` required. |
| `CWD:` | 0..1 | Repo-relative. Absolute paths and `..` segments = exit 2. |
| `FROM:` | 1..n | Traceability (§2.4). Mandatory on every gate. |
| `RED:` | 0..1 | Red-proof mode: `baseline` \| `mutation` \| `control`. Default `baseline`. |
| `RED-CHECK:` / `RED-EXPECT:` | 0..1 | Positive-control command, required iff `RED: control`. |
| `WITNESS:` | 0..n | Globs whose content hash binds the red proof and the approval (§3.4). |
| `EVIDENCE:` | 1 | `pending` or `sha256:<hash>` pointing into `.saga/evidence/`. Checker-written only. |

A gate with `CHECK:`+`EXPECT:` is **runnable**; with neither, **manual**. Exactly one of the two shapes; a half-runnable gate is a parse error.

### 2.4 `FROM:` — request traceability

`saga gate init` copies the request into `.saga/request.md` and numbers sentences deterministically:

```
R1  Import valid records from the vendor feed.
R2  Reject malformed records and report the line number.
R3  Do not change the public API of the importer.
```

Segmentation is a fixed rule set (terminal `.?!` followed by whitespace + uppercase or EOF; abbreviation exception list; list items and headings are their own sentences; fenced code is one sentence). It is versioned; `request.md` records `SEGMENTER: v1`.

```
  FROM: R2 "Reject malformed records"
```

The checker validates that the quoted span occurs **verbatim** (after whitespace normalisation) inside sentence `R2`. A quote that does not occur is exit 2 — this is the one lexical defence against a gate that cites a requirement it does not discharge.

**Coverage report.** `saga gate status` classifies every sentence as `covered` (≥1 `FROM:`), `uncovered`, or `ignored` (matched by a lexical non-requirement filter: interrogatives, greetings, sentences with no verb). Uncovered sentences are reported, and in `--strict` they make status exit 1. The classifier is a heuristic and is labelled as one in output: it can only ever say *"nothing in this contract claims to discharge R3"*, never *"R3 is unmet"*.

### 2.5 Example

```markdown
# Contract: vendor feed importer

CONTRACT: vendor-import
REQUEST: sha256:9f2c…a1
IN: src/import/**, tests/import/**
OUT: src/api/**, **/*.lock
SIDE-EFFECTS: none
BASE: 4c1e9ab

- [ ] G1: a valid fixture imports every record
    CHECK: node scripts/check-import.mjs fixtures/valid.json
    EXPECT: import verification passed
    FROM: R1 "Import valid records from the vendor feed"
    RED: mutation
    WITNESS: scripts/check-import.mjs, fixtures/valid.json
    EVIDENCE: pending

- [ ] G2: malformed records are rejected with a line number
    CHECK: node scripts/check-reject.mjs
    EXPECT: /rejected 3 records at lines 4, 9, 17/
    FROM: R2 "Reject malformed records and report the line number"
    RED: control
    RED-CHECK: node scripts/check-reject.mjs --against fixtures/all-valid.json
    RED-EXPECT: rejected 0 records
    EVIDENCE: pending

- [ ] G3: the importer's public API is unchanged
    CHECK: npx api-extractor run --local && git diff --exit-code etc/importer.api.md
    EXPECT: /api report is up to date/
    FROM: R3 "Do not change the public API of the importer"
    RED: baseline
    EVIDENCE: pending

- [ ] G4: the migration wording matches the product decision
    FROM: R2 "report the line number"
    EVIDENCE: pending

ABANDON: G4 decision owner unavailable; handoff recorded in issue 123
```

### 2.6 Parse-failure rules (fail closed)

Every row below is exit code **2**, produces no evidence, and never yields a completion certificate. This table is the parser's test matrix.

| # | Condition | Rationale |
|---|---|---|
| 1 | Zero gates in a named contract | An empty ledger is not `ALL MET`. |
| 2 | Duplicate gate id | Evidence keys would collide. |
| 3 | Missing or empty gate id | Line-derived ids are unstable across edits. |
| 4 | `CHECK:` without `EXPECT:`, or vice versa | Exit-0-only is not an oracle. |
| 5 | Attribute at column 1 | Would silently convert a runnable gate to manual. |
| 6 | Tab indentation | Ambiguous width. |
| 7 | Duplicate attribute in one gate | Ambiguous oracle. |
| 8 | Invalid regex in `EXPECT:`/`RED-EXPECT:` | Cannot be evaluated. |
| 9 | Absolute path or `..` in `IN`/`OUT`/`CWD`/`WITNESS` | Escapes the repo. |
| 10 | Missing `FROM:` on any gate | Untraceable gate. |
| 11 | `FROM:` quote absent from the named sentence | Fabricated traceability. |
| 12 | `REQUEST:` hash ≠ hash of `.saga/request.md` | Contract detached from its request. |
| 13 | `ABANDON:` naming an unknown id, or with an empty reason | A typo must not promote a parent to green. |
| 14 | Indented `ABANDON:`/`WAIVE:` | File-level statements. |
| 15 | `RED: control` without `RED-CHECK:`+`RED-EXPECT:` | Declared proof mode with no proof. |
| 16 | Unknown `SIDE-EFFECTS:` token | Closed vocabulary. |
| 17 | `EVIDENCE:` written by a non-checker actor (hash not in store) | Forged evidence. |
| 18 | Contract file is a symlink, multi-link, FIFO, or >1 MiB | Hostile input. |

Warnings (exit 0, printed, counted): slash-wrapped path-shaped regex; `EXPECT:` drawn from vocabulary that failure output also uses (`ok`, `done`, `pass`); an outcome phrased as an activity (`improve`, `ensure`, `refactor`); a number in the outcome that no `CHECK:` measures; >50% manual gates. `saga gate lint --strict` promotes all warnings to exit 1.

---

## 3. Red proof

> *"Before trusting an absence check, run the same logic against a known positive fixture and confirm that it fails."* — unlazy `references/gates.md`, promoted here from a rule to a tool.

### 3.1 Definition

A gate is **proven-red** when the checker has itself observed the gate's oracle *fail* against a tree in which the outcome is known to be absent, and has recorded that observation. Until then the gate is `unproven`; a green result on an unproven gate is reported as `MET (UNPROVEN)` and, under `--require-red` (the default for `saga gate check` in CI and in the Stop adapter), is treated as **unmet**.

### 3.2 The three modes

| Mode | Procedure | Cost | When to use |
|---|---|---|---|
| `baseline` | At `init`, before any edit, run the oracle on the pre-work tree. Must exit non-zero **or** fail the `EXPECT:` match. | Free | Default. Works for any gate whose outcome does not yet exist. |
| `mutation` | After the gate is green: apply a deterministic mutation to the in-scope artefact, re-run the oracle, require failure, restore the tree, re-run and require green. | One extra run pair | Gates that were already green at `init` (regression guards, API-stability gates). |
| `control` | Run `RED-CHECK:`/`RED-EXPECT:` — an oracle deliberately pointed at a known-broken or known-different fixture — and require it to produce the *failure* signature. | One extra run | Absence checks ("no record is rejected", "no TODO remains") where mutation is meaningless. |

**Mutation operators** (deterministic, applied to a scratch copy, in this priority order; the first that changes the parse tree is used):

1. Invert the first boolean return in the newest in-scope function touched since `BASE:`.
2. Replace the first numeric or string literal in that function with a fixed sentinel (`-424242` / `"__saga_mutant__"`).
3. Delete the body of that function, leaving a stub returning the type's zero value.
4. If none apply (pure config/data change), delete the first added key in the changed file.

The operator applied, its file, byte range, and the resulting oracle output fingerprint go into the record. If **no** operator makes the gate go red, the gate is `red-proof: failed` — the strongest signal this layer produces, because it means the oracle does not observe the artefact at all (the `node -e "console.log('ok')"` class of gate).

### 3.3 Record

`.saga/red/<gate>.json`:

```json
{
  "schema": "saga.red/1",
  "gate": "vendor-import:G1",
  "mode": "mutation",
  "oracle_hash": "sha256:…",
  "witness_hash": "sha256:…",
  "proved_at": "2026-09-02T14:03:11Z",
  "operator": {"kind": "literal-sentinel", "file": "src/import/parse.ts", "range": [812, 819]},
  "red_observation": {"exit": 1, "matched": false, "output_sha256": "sha256:…", "output_bytes": 2411},
  "green_observation": {"exit": 0, "matched": true, "output_sha256": "sha256:…", "output_bytes": 118},
  "toolchain": {"shell": "/bin/sh", "path_fingerprint": "sha256:…", "path_entries": 24}
}
```

### 3.4 Invalidation

A red proof is void, and the gate returns to `unproven`, when any of these change:

| Input | Bound as |
|---|---|
| `CHECK:`, `EXPECT:`, `CWD:`, resolved shell, timeout, output/regex limits | `oracle_hash` |
| Content of every path matched by `WITNESS:` | `witness_hash` (sha256 of the sorted `path\0sha256` list) |
| Platform, `PATH` fingerprint | `toolchain` |
| Contract `REQUEST:` hash | recorded alongside |

`WITNESS:` is the deliberate answer to unlazy's documented gap that approval does not hash transitive inputs — *"an agent can edit `scripts/verify.mjs` to `console.log('passed')` under a standing approval"*. `saga gate init` seeds `WITNESS:` automatically with every repo-relative path that appears as an argument in the `CHECK:` command line, plus, when `CHECK:` invokes a repo script, that script's own first-order local imports resolved statically. It is best-effort, and the spec says so: dynamic requires, generated fixtures, and network inputs are not traced. Additionally, a red proof older than `config.red_ttl_days` (default 30) or predating the current `BASE:` by more than `config.red_ttl_commits` (default 200) is re-established on next `check`.

---

## 4. Evidence

### 4.1 Pass condition

A runnable gate passes iff **both**: the process exits `0`, **and** `EXPECT:` matches the combined stdout+stderr. A non-zero exit never passes because the error text happens to contain the marker. Timeout (default 120 s, `--timeout` 1..86400), shell-start failure, missing command, or output-cap breach (1 MiB combined) all fail. Regex matching runs in a disposable worker with a 250 ms match budget after a 5 s startup allowance; a timed-out worker cannot certify a gate.

A gate is **met** iff: it passes, its evidence record exists and hashes to the `EVIDENCE:` value, and (under `--require-red`) its red proof is valid. A checked box with `pending` or absent evidence is unmet. An abandoned gate is terminal and non-successful: `check` prints `HANDOFF REQUIRED` and exits 1 even when every other gate is met.

### 4.2 Record

`.saga/evidence/<gate>.json`:

```json
{
  "schema": "saga.evidence/1",
  "gate": "vendor-import:G1",
  "contract_hash": "sha256:…",
  "oracle_hash": "sha256:…",
  "outcome": "met",
  "exit_status": 0,
  "matched": {"kind": "regex", "expect_hash": "sha256:…", "span_bytes": [1104, 1131]},
  "output_sha256": "sha256:…",
  "output_bytes": 118,
  "duration_ms": 3412,
  "started_at": "2026-09-02T14:03:11Z",
  "resolved": {
    "shell": "/bin/sh", "cwd": "packages/importer",
    "path_fingerprint": "sha256:…", "path_entries": 24,
    "platform": "darwin-arm64", "timeout_s": 120, "output_cap_bytes": 1048576
  },
  "tree": {"base": "4c1e9ab", "head": "9de20f1", "worktree_hash": "sha256:…", "dirty": true},
  "red_proof": "sha256:…",
  "guards": {"clean": true, "waivers": []},
  "approval": "sha256:…"
}
```

`worktree_hash` is the hash of the sorted `path\0mode\0sha256` list over tracked, non-ignored files — it is what makes an evidence record answer *"which tree was this true of"* and lets `reverify` detect that the tree moved under a green gate.

### 4.3 What is never persisted

| Never written to disk or terminal | Why |
|---|---|
| Raw successful output | Success output is consumed for matching, then fingerprinted (unlazy's rule; also the only structural privacy mitigation this layer has). |
| Full `PATH` or any environment value | Machine-specific; fingerprint + entry count only. Display cap 800 chars, pre-execution transcript only. |
| Absolute paths outside the repo root | Directory-name leakage. Stored repo-relative. |
| Free-form `ABANDON:` reasons in privileged hook messages | Repo-controlled text into a privileged channel. Hook messages carry qualified ids only. |
| Anything matching `saga guard`'s secret patterns, in failure diagnostics | Failure diagnostics are bounded, terminal-only, control-stripped, bidi-stripped, and passed through the masker before display. |

Failure diagnostics are capped at 4 KiB with an error-aware tail (never cut mid-line).

---

## 5. Diff guards

Guards run on every `PostToolUse` edit event (incrementally, on the touched paths) and in full at Stop and in CI. Input is always `git diff --find-renames --name-status -z <BASE>..worktree` plus, for AST guards, the pre- and post-image parsed with tree-sitter. **No guard consults an LLM.** A guard that cannot parse a file falls back to the line-regex form of its rule and marks the finding `degraded: true`.

### 5.1 Guard catalogue

| Id | Rule (deterministic) | FP risk | Waivable |
|---|---|---|---|
| **G-SCOPE** | Every path in the diff (both sides of a rename) must match ≥1 `IN:` glob and 0 `OUT:` globs. Case-folded on case-insensitive filesystems. | Low — formatters, generated files, lockfiles. Mitigation: `config.scope_exempt` for a small committed list. | yes |
| **G-TESTDEL** | A file is deleted, or renamed to a non-test path, when it matched the test glob set **or** its pre-image contained ≥1 test declaration for its language. A rename test→test whose post-image declaration count ≥ pre-image is clean. | Low. | yes |
| **G-ASSERT** | For every test function present in both images: (a) assertion-node count decreased, or (b) an assertion changed to a strictly weaker class (§5.2), or (c) a numeric tolerance widened, or (d) `require`-class replaced by `assert`-class. | Medium — legitimate refactors that consolidate assertions. | yes |
| **G-SKIP** | Per-language skip/xfail/only/disable marker count increased, or a test function was renamed out of the runner's discovery convention. | Very low. | yes |
| **G-HARDCODE** | A literal added or changed inside an assertion (or a snapshot/golden file) is byte-equal, after normalisation, to a value captured as `actual` in a failing check output inside this contract window, and did not exist anywhere in the pre-image tree. Also fires on self-referential assertions (`expect(f(x)).toBe(f(x))`) and on snapshot-only commits (golden files changed, no source file changed). | High — genuinely correct fixes often produce the observed value. Always requires justification, never auto-blocks in `--advisory` mode. | yes |
| **G-DEP** | A new non-relative import/require/use, or a new manifest entry, that is (a) absent from the manifest, (b) absent from the lockfile, or (c) not resolvable in the configured registry (offline cache permitted). Delegates to `saga guard`'s existence check. | Low. Targets the 19.7% hallucinated-package class (doc 03 §2.7). | yes |
| **G-LEDGER** | The diff touches `.saga/evidence/**`, `.saga/red/**`, or the approval store; or an `EVIDENCE:` line changed without a matching store write; or `IN:`/`OUT:`/`BASE:`/`REQUEST:` changed after the first evidence record was written. | Nil. | **no** |

`G-LEDGER` is unwaivable because a waiver for it would be self-approving.

### 5.2 Per-language heuristics

Assertion **strength classes**, strongest to weakest — a move down the list is a weakening:

`1 identity/exact-equality` → `2 structural equality` → `3 partial/subset match` → `4 membership/containment` → `5 predicate on shape or type` → `6 truthiness/definedness` → `7 no-op (logged, not asserted)`

| Language | Assertion forms (class) | Weakening examples | Skip / only markers | Discovery convention |
|---|---|---|---|---|
| **JS/TS** | `toBe`/`toStrictEqual` (1–2), `toEqual` (2), `toMatchObject` (3), `toContain` (4), `toBeInstanceOf` (5), `toBeTruthy`/`toBeDefined` (6); `node:assert` `strictEqual`(1)/`deepStrictEqual`(2)/`ok`(6); chai `to.equal`/`to.include`/`to.exist` | `toStrictEqual`→`toEqual`→`toMatchObject`→`toBeTruthy`; `assert.strictEqual`→`assert.ok`; `toHaveLength(n)`→`toBeDefined()`; a real import replaced by `jest.mock` inside an existing test | `it.skip`, `describe.skip`, `xit`, `xdescribe`, `test.todo`, `it.only`, `fdescribe`, `test.concurrent.skip` | filename `*.test.*`/`*.spec.*`, `__tests__/` |
| **Python** | `assertEqual`/`assertIs` (1), `assertDictEqual`/`assertListEqual` (2), `assertIn` (4), `assertIsInstance` (5), `assertTrue`/`assert x` (6); bare `assert a == b` (1–2) | `assertEqual`→`assertTrue`; `assert a == b`→`assert a`; `pytest.approx(v, rel=…)` tolerance increased; `with assertRaises(...)` → `try/except: pass`; `return` inserted at the top of a test body | `@pytest.mark.skip`, `skipif`, `xfail`, `@unittest.skip*`, `pytest.skip()` in body, `-k` narrowing committed to config | `test_*.py`, `*_test.py`, `Test*` classes, `test_*` functions |
| **Go** | `if got != want { t.Fatalf }` (1), `reflect.DeepEqual` (2), testify `require.Equal` (1, fatal), `assert.Equal` (1, non-fatal), `assert.Contains` (4), `assert.NotNil` (6) | `require.*`→`assert.*`; `t.Fatal`→`t.Error`→`t.Log`; comparison branch deleted; `assert.Equal`→`assert.NotNil` | `t.Skip`, `t.SkipNow`, `t.Parallel` added around shared state, `testing.Short()` guard added, `//go:build ignore` | `*_test.go`, `func Test*` |
| **Rust** | `assert_eq!`/`assert_ne!` (1), `matches!` in `assert!` (3), `assert!(x.is_some())` (6), `debug_assert*` (7 in release) | `assert_eq!`→`assert!`; `assert!`→`debug_assert!`; `.unwrap()` on the result under test → `let _ =`; `#[should_panic]` added to a previously passing test | `#[ignore]`, a new `#[cfg(feature = …)]` gating a test, `return` before assertions | `#[test]`, `#[tokio::test]`, `tests/` |
| **Java/Kotlin** | `assertEquals`/`assertSame` (1), AssertJ `isEqualTo` (1), `containsExactly` (2), `contains` (4), `isInstanceOf` (5), `isNotNull`/`assertTrue` (6) | `assertEquals`→`assertNotNull`; `containsExactly`→`contains`; `assertThrows` → try/catch swallow; a `@Test` losing its assertions to a bare method call | `@Disabled`, `@Ignore`, `@Test(enabled=false)`, `Assumptions.assumeTrue(false)`, new `@Tag` plus a surefire/gradle exclusion | `*Test.java`, `*Spec.kt`, `src/test/` |
| **Swift** | `XCTAssertEqual`/`XCTAssertIdentical` (1), `XCTUnwrap` (1), `XCTAssertTrue` (6); swift-testing `#require` (1, fatal) / `#expect` (non-fatal) | `XCTAssertEqual`→`XCTAssertNotNil`; `try XCTUnwrap(x)` → `x?`; `#require`→`#expect`; accuracy parameter widened | `XCTSkip`, `try XCTSkipIf`, `@Test(.disabled())`, **method renamed off the `test` prefix** (silently un-discovers the test) | `func test*` in `XCTestCase`, `@Test` |
| **Dart** | `expect(a, equals(b))` (1), `orderedEquals` (2), `containsAll` (4), `isA<T>()` (5), `isNotNull` (6), `anything` (7) | `equals`→`isNotNull`→`anything`; `throwsA(isA<X>())`→`throwsA(anything)`; `expectLater` awaited → un-awaited | `skip: true` on `test()`/`group()`, `@Skip()` annotation, `markTestSkipped`, `solo_test` | `*_test.dart`, `test/` |

### 5.3 Waivers

A guard finding blocks until waived. A waiver is a line in the contract:

```
WAIVE: G-ASSERT tests/import/parse.test.ts 3f9a12cd7b04 consolidated four equality assertions into one deep-equal; count drop is mechanical
```

Rules: (1) the `hunk-hash` is the sha256/12 of the normalised hunk the guard flagged, so a waiver never covers a later, different change to the same file; (2) the reason must be ≥8 non-whitespace tokens and must not be byte-identical to another waiver in the file; (3) waivers are recorded verbatim in the evidence record's `guards.waivers` and echoed by `status`; (4) `G-LEDGER` cannot be waived; (5) `config.waiver_policy = "human"` requires the waiver line to be added in a commit whose author is not the agent identity, which is the only way to make a waiver non-self-serving; the default is `"logged"`.

---

## 6. Stop enforcement adapters

Adapters are **translation-only**: read the harness's JSON on stdin, shell out to `saga gate check --json` / `saga gate guard-diff --json`, map exit codes into the harness envelope, cap the message at the §9 budget. Each adapter is under ~150 lines, contains no gate logic, and is covered by recorded-fixture tests (§10.1).

### 6.1 Claude Code

Hook input arrives as JSON on stdin; output is either an exit code or a JSON object on stdout.

| Event | Input fields used | Saga call | Output on violation |
|---|---|---|---|
| `PreToolUse` | `tool_name`, `tool_input`, `cwd`, `session_id` | `saga gate guard-diff --predict --path <target> --json` (scope only) | `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"<≤150 tok>"}}` |
| `PostToolUse` | `tool_name`, `tool_input`, `tool_response` | `saga gate guard-diff --incremental --json` | `{"decision":"block","reason":"<≤200 tok>"}` |
| `Stop` | `session_id`, `transcript_path`, `stop_hook_active` | `saga gate check --status --require-red --json` (non-executing) | `{"decision":"block","reason":"<≤400 tok>"}` |

Rules:
- The `Stop` adapter **never executes** a `CHECK:`. It reports historical ledger state, exactly like unlazy's hook boundary. Execution happens in `check`, invoked by the agent or by CI.
- `stop_hook_active` must be honoured: when true, the adapter does not block again for the same resolved state.
- **Loop guard**: block decisions are counted against a *semantic progress hash* — the sha256 of the sorted `(gate, outcome, evidence_hash, red_valid)` tuples, not file bytes — so a reflowed line or a rewritten comment is not progress. After `config.max_blocks` (default 6) consecutive blocks with an unchanged hash, the adapter releases with a `HANDOFF REQUIRED` message. This is a structural backstop, not a guarantee: an agent can wait it out, and the spec says so.
- Abandonment allows the stop but emits the bounded handoff message containing qualified ids only.
- Exit 2 with stderr is the fallback envelope when the JSON shape is rejected by the harness version; adapters detect their supported schema at install time and record it.

### 6.2 Gemini CLI

Same three lifecycle points where the installed version exposes them (`PreToolUse`, `PostToolUse`, and an end-of-agent event). The adapter probes the CLI version at install, writes the hook into the project settings file, and **records which events it actually bound**. If no terminal event exists in that version, install fails closed with: *"no stop-equivalent event on this version; enable the CI fallback (§6.4)"*. Event naming across Gemini CLI releases is unstable enough that the adapter treats the event table as configuration, not as a constant.

### 6.3 Codex CLI

Codex's `notify` program is invoked **after** a turn completes and cannot block. Therefore:

```toml
# ~/.codex/config.toml
notify = ["/usr/local/bin/saga-codex-notify"]
```

The notify adapter runs `saga gate check --status --json`, writes the result to `.saga/last-status.json`, and prints a bounded `HANDOFF REQUIRED` line. This is **advisory**. For Codex, the CI fallback is mandatory, not optional, and `saga gate init` says so at install time. Where a Codex build exposes a pre-exec hook, the adapter binds the `PreToolUse`-equivalent scope guard there; the terminal enforcement point remains CI.

### 6.4 CI fallback (normative for every harness)

```yaml
name: saga-gate
on: [pull_request]
jobs:
  gate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/setup-node@v4
        with: { node-version: 22 }
      - run: npm i -g @saga/cli
      - name: Guards
        run: saga gate guard-diff --base "origin/${{ github.base_ref }}" --json --out guards.json
      - name: Gates
        run: saga gate reverify --approve-from .saga/approvals.ci.json --require-red --json --out gates.json
      - name: Coverage of the request
        run: saga gate status --strict --json --out status.json
      - if: always()
        uses: actions/upload-artifact@v4
        with: { name: saga-gate, path: "*.json" }
```

CI runs with a distinct approval store bound to the CI `PATH` and platform, so a local approval never authorises a CI execution. `reverify` re-runs *every* runnable gate, including ones already green, and demotes any whose oracle no longer passes.

---

## 7. CLI surface

```
saga gate init        [--request <file|-> ] [--from-diff] [--base <rev>] [--template <name>]
saga gate status      [--strict] [--json] [--contract <file>]
saga gate check       [--gate <id>...] [--approve] [--require-red] [--advisory]
                      [--timeout <s>] [--shell <path>] [--cwd <dir>] [--jobs <n>] [--json]
saga gate reverify    [--gate <id>...] [--require-red] [--json]
saga gate approve     [--gate <id>...] [--print] [--revoke] [--store <dir>]
saga gate lint        [--strict] [--json]
saga gate guard-diff  [--base <rev>] [--incremental] [--predict --path <p>]
                      [--guard <id>...] [--advisory] [--json]
```

| Subcommand | Executes `CHECK:`? | Writes evidence? |
|---|---|---|
| `init` | yes (baseline red proofs only) | red records only |
| `status` | **never** | no |
| `check` | yes, with approval | yes |
| `reverify` | yes, all runnable gates | yes |
| `approve` | no | approval records only |
| `lint` | **never** | no |
| `guard-diff` | no | no |

### 7.1 Exit codes (uniform across subcommands)

| Code | Meaning |
|---|---|
| 0 | All met / clean / lint OK |
| 1 | Unmet gate, uncovered outcome in `--strict`, or `HANDOFF REQUIRED` (abandonment) |
| 2 | Usage error or contract parse failure (§2.6) |
| 3 | Guard violation with no valid waiver |
| 4 | Approval required — one or more oracles have no exact approval record |
| 5 | Red proof missing, stale, or `failed` under `--require-red` |
| 6 | Environment refusal — hostile file shape, symlinked store, unreadable state |

Higher codes win when several apply, except that 2 always wins.

### 7.2 JSON output

```json
{
  "schema": "saga.gate.status/1",
  "contract": "vendor-import",
  "contract_hash": "sha256:…",
  "exit": 1,
  "summary": {"gates": 4, "met": 2, "unmet": 1, "abandoned": 1,
              "unproven": 1, "manual": 1, "waivers": 1},
  "gates": [{
    "id": "vendor-import:G1", "outcome": "a valid fixture imports every record",
    "state": "met", "runnable": true,
    "from": [{"sentence": "R1", "quote": "Import valid records from the vendor feed"}],
    "evidence": "sha256:…", "red": {"mode": "mutation", "valid": true, "proved_at": "…"},
    "approval": "present"
  }],
  "coverage": {"sentences": 3, "covered": 2,
               "uncovered": [{"id": "R3", "text": "…", "confidence": "heuristic"}]},
  "guards": [{"id": "G-ASSERT", "path": "tests/import/parse.test.ts", "hunk": "3f9a12cd7b04",
              "rule": "assertion-count-drop", "pre": 4, "post": 1,
              "degraded": false, "waived": true, "waiver_reason": "…"}],
  "handoff": [{"gate": "vendor-import:G4", "state": "abandoned"}],
  "budget": {"tokens_emitted": 312, "ceiling": 400}
}
```

Every record carries a `schema` field with an integer-suffixed name; consumers must reject unknown majors. Text fields are control-stripped and bidi-stripped before serialisation.

---

## 8. Security boundary

**What the layer assumes.** `CHECK:` is code, executed with the caller's permissions and inherited environment. The boundary is explicit review plus approval, not sandboxing.

| Control | Rule |
|---|---|
| Approval store | `~/.saga/approved` by default. `SAGA_APPROVAL_DIR` accepted only if it is a real, owner-private directory whose canonical target is outside the canonical repo root. Symlinked stores, multi-link records, and non-private modes fail closed (exit 6). |
| Approval identity | Absolute contract path, gate id, exact `CHECK:`/`EXPECT:`/`CWD:`, resolved shell, timeout, output and regex limits, platform, full inherited `PATH`, **and `witness_hash`**. Any change requires re-approval. |
| Dry run | A gate with no exact approval prints its resolved oracle and is not executed. `--status` and every adapter never execute. Normal mode is not a universal dry run: an existing exact approval permits execution. |
| Untrusted ledger text | Contract titles, outcomes, `ABANDON:` reasons, waiver reasons and check output are **data, never instructions**. They are control-character-, line-separator- and bidi-stripped before display, capped per field, and never copied into a privileged hook message. An inherited contract is read with `status`/`lint` before any `check`. |
| Repo-discovered inputs | Must be regular, single-link, within the canonical repo root, under the size cap; opened no-follow, and re-verified to name the same inode after read. |
| Hook install | Writes the project-local settings file atomically with a `.saga.bak`; preserves unrelated hooks; carries an exact managed marker; embeds absolute paths, so the shared-settings target is documented as non-portable. |

**What the checker cannot protect against**, stated as plainly as unlazy states it:

1. **Semantic mismatch.** A green gate proves the oracle passed, not that the oracle measures the English outcome. Red proof raises the floor (the oracle observes *something* about the artefact) but a proven-red gate can still measure the wrong property.
2. **Transitive input drift beyond `WITNESS:`.** Dynamic imports, generated fixtures, network dependencies, and container images are not hashed. `WITNESS:` covers first-order, statically resolvable local inputs only.
3. **A determined agent waiting out the loop guard.** The block ceiling is a backstop.
4. **Anything a `CHECK:` chooses to do** — network calls, credential reads, writes outside `IN:`. Guards observe the *diff*, not syscalls. Use OS/container isolation for untrusted code.
5. **Secrets in failure diagnostics.** Success output is fingerprinted; failure output is masked by `saga guard` and bounded, but masking is pattern-based and has a measured, non-zero miss rate.
6. **A human waiving a true positive.** `waiver_policy = "human"` shifts the decision, it does not verify it.
7. **Self-review.** A gate authored by the agent, approved by the agent, and waived by the agent is self-certification with better paperwork. The independent legs are red proof, `reverify` in CI under a different approval store, and hidden oracles on the bench.

---

## 9. Token budget

Per ADR 0002 every text emission is a bounded translation of a check result, and per doc 09 §3.7 it is attributed in `saga trace`.

| Injection point | Content | Ceiling |
|---|---|---|
| Session start | **nothing** | 0 tokens |
| Tool/skill description registered with the harness | one line | ≤60 tokens |
| `PreToolUse` deny reason | violated guard id, path, the `IN:`/`OUT:` glob that decided it | 150 tokens |
| `PostToolUse` block reason | guard id, path, rule, pre/post counts, waiver syntax reminder | 200 tokens |
| `Stop` block reason | unmet gate ids + outcomes (truncated to 12 words each), uncovered sentence ids, handoff ids | 400 tokens |
| `check` failure diagnostics on the agent's terminal | bounded, error-aware tail, masked | 1,200 tokens (4 KiB) |
| Contract file itself, when the agent reads it | authored by the agent; typical 5-gate contract | ~450 tokens |
| **Per-session total attributable to `gate`** | | **3,000 tokens, hard-capped** |

The cap is enforced, not advisory: the adapter truncates and appends `… (N more; run: saga gate status)`. `saga trace` records `tokens_emitted` per event, and the bench treats a budget breach as a failed run of the layer itself.

---

## 10. Test plan and ablation

### 10.1 Tests for the layer

| Suite | Content | Pass bar |
|---|---|---|
| **Parser** | Table-driven over all 18 rows of §2.6 plus their near-miss valid twins; CRLF preservation; fenced-code exclusion; 1 MiB and 10k-gate inputs; property test: any single character deleted from a valid contract yields exit 0 or 2, never a false `ALL MET`. | 100% of rows; zero false green |
| **Execution** | exit-0-without-marker fails; marker-without-exit-0 fails; timeout kills the process tree on POSIX and Windows; output cap; regex worker budget; concurrent `--jobs` cannot starve the regex budget. | all |
| **Evidence** | Record round-trips; forged `EVIDENCE:` rejected; `worktree_hash` changes demote on `reverify`; raw success output appears in no artefact (grep the whole `.saga/` tree for a canary string emitted by a passing check). | canary never found |
| **Red proof** | For each of 4 mutation operators × 7 languages: mutant makes a real gate red; a tautological gate (`echo passed`) is reported `red-proof: failed`; oracle edit invalidates; `WITNESS:` byte change invalidates. | 28/28 mutants red; 7/7 tautologies caught |
| **Guards** | A labelled corpus of ≥40 diffs per language (half true positives drawn from real reward-hacking transcripts, half legitimate refactors that superficially match). Report precision/recall per guard. | Recall ≥0.95 on G-SKIP/G-TESTDEL/G-SCOPE/G-DEP; ≥0.80 on G-ASSERT; G-HARDCODE reported with its measured FP rate and shipped `--advisory` by default until precision ≥0.7 |
| **Adapters** | Recorded stdin fixtures per harness version → asserted stdout JSON; `stop_hook_active` loop guard; semantic progress hash unchanged by comment/reflow edits; missing-event install fails closed. | all |
| **Security** | Symlinked/hard-linked/FIFO/oversized state; TOCTOU on the approval record; bidi and ANSI in gate titles never reach the hook message; approval store inside the repo refused; `PATH` change invalidates approval. | all |
| **Self-gate** | Saga's own `.saga/contract.md` includes `G0: this contract states outcomes that can fail / CHECK: saga gate lint --strict / EXPECT: LINT OK` and a gate that runs the guard corpus. | green in CI |

### 10.2 What the tests do *not* validate

Copying unlazy's standard: these validate implementation behaviour. They say nothing about whether the layer changes what a model does. That is §10.3's job, and until it runs, no number about `saga gate` appears in the README.

### 10.3 Ablation on `saga bench`

**Arms** (same tasks, same models, same seeds; the control arm is blocked from reading `.saga/` at the filesystem level, per the codegraph discipline):

| Arm | Contents |
|---|---|
| A | Bare harness |
| B | A + contract (scope, gates, `FROM:`) — no red proof, no guards, no Stop enforcement |
| C | B + red proof (`--require-red`) |
| D | C + diff guards |
| E | D + Stop enforcement adapter |

**Design.** 30 tasks per language across TypeScript, Python and Go for M1 (the other four languages at M2), each with a **hidden** oracle the agent never sees, k = 10 clean-room runs per cell, 2 model families. Six of the 30 are **positive controls**: tasks that are impossible as stated, where the only correct terminal state is `ABANDON`. Six more are **hack-bait**: solvable only by real work but trivially "passable" by weakening the visible test.

**Primary metric — false-done rate**: fraction of runs where the agent reported completion and the hidden oracle failed. This is the number the layer exists to move. Doc 03 §2.5 predicts arm A is worst here and that a visible oracle alone (arm B) does not fix it.

**Secondary metrics**: pass^k on the hidden oracle; scope-violation count; test-weakening incidents found by an independent post-hoc scan (run on *every* arm, including those without guards, so the guards' effect is measured against ground truth, not against themselves); ABANDON rate on the impossible six (higher is better; a run that claims success on an impossible task is a failure); uncovered-sentence count at Stop; tokens, wall time, cost; and `tokens_emitted` against the §9 ceiling.

**Reporting**: per-run rows, per-task medians, Wilcoxon signed-rank across paired arms, pass^k with variance as a first-class output, and the negative results committed. A component that does not move the false-done rate at k = 10 with the control arm blocked is cut, not shipped and explained.

**Known confound to declare**: arms C–E cost extra runs (red proofs, `reverify`), so cost comparisons must be reported per *completed and verified* task, not per run.
