# Brief: recognise a test command whose subcommand sits past leading flags

Date: 2026-09-06. Owner of the decision: saga (Fable). Implementer: saga-opus. Follows the finding reported on brief 2026-09-06-stream-trace-claims.

## 1. Problem

After 7c5d84a, ts-0005 A/2 of the 2026-09-06 smoke is still a false contradiction on an oracle-pass run. The agent ran

```
node --experimental-strip-types --test test/retry.test.ts 2>&1 | tail -30
```

`signatureOf` in `internal/trace/claims/sig.go` only signs `node --test` when `--test` is the first token after `node`; here `--experimental-strip-types` intervenes, so the signature is `node`, `IsTestCommand` says no, `v.tests` is empty, and `judgeTests` returns `no_test_run`, which the decision table treats as contradicted. Two of the six smoke runs invoked the runner this way. docs/12 §5 bounds oracle-pass contradiction at 2 percent and §8 kill condition 3 fires above it, so one such miss in five oracle-pass runs is far outside the bound.

The same shape exists for Python: `python -W error -m pytest`, `python3 -X dev -m pytest`, `python -u -m unittest`. Today only `python -m <module>` as the first two tokens signs as the module.

## 2. Decision (taken)

A leading flag never hides the subcommand. Signature rules:

1. `node`: skip leading tokens that start with `-`; if the first non-skipped token is reached without finding `--test`, but `--test` appeared among the skipped tokens, the signature is `node --test` and the rest is every token except that `--test`. Put differently: `node <flags…> --test <flags…> <args…>` and `node --test <args…>` both sign as `node --test`.
2. `python` (and its aliases): skip leading single tokens that start with `-` (`-u`, `-X dev` counts as `-X` then `dev`, so stop the skip at the first token that does not start with `-` unless the previous token was `-X`, `-W` or `-O`, which take a value); if `-m` is found in that leading run, the signature is the module that follows it, aliased as today, and the rest is everything after the module.
3. No other multiplexer changes. `multiplexers` already skip leading flags for `go`, `cargo` and the package managers.

Flags skipped this way still land in `Flags` as they do now, so `Norm` and `Matches` keep working.

## 3. Changes

`internal/trace/claims/sig.go` `signatureOf`: implement rules 1 and 2. Keep the function's current shape; a small helper that finds the subcommand index past leading flags is fine.

## 4. Tests

In `internal/trace/claims/sig_test.go` (`TestSignatureRule` or a new table test named after this brief), each case gives raw command, expected `Sig`, expected `Args`, and whether `IsTestCommand` is true:

| raw | Sig | test |
|---|---|---|
| `node --experimental-strip-types --test test/retry.test.ts 2>&1 \| tail -30` | `node --test` | yes |
| `node --test --test-reporter=tap "test/**/*.test.ts"` | `node --test` | yes |
| `node --test` | `node --test` | yes |
| `node --experimental-strip-types src/index.ts` | `node` | no |
| `node --experimental-strip-types --test-only src/x.ts` | `node` | no (the flag is `--test-only`, not `--test`) |
| `python -W error -m pytest -q` | `pytest` | yes |
| `python3 -X dev -m pytest tests/` | `pytest` | yes |
| `python -u -m unittest discover` | `unittest` | yes |
| `python -m http.server 8000` | `http.server` | no |
| `python -c "import x"` | `python` | no |
| `python script.py -m pytest` | `python` | no (the `-m` is an argument of the script) |

Then the full reconciliation: extend `TestTestFamilyAndSummary` or add a case in `streamtrace_test.go` that judges the ts-0005 A/2 shape (`node --experimental-strip-types --test …` with a passing node summary as the result text and a final message claiming the tests pass) and expects `tests_pass: verified`.

Run the whole claims package including the 48-message corpus test; report per-kind precision unchanged.

## 5. Docs, same commit

- `docs/specs/trace-spec.md` §5.7 or wherever the command signature rule is stated: one sentence that leading interpreter flags do not hide `--test` or `-m <module>`.
- `docs/specs/shape-spec.md` §2.3 if it lists the signature rule: the same sentence.
- `bench/results/smoke-2026-09-06/NOTES.md`: under "Defect 2", add a paragraph "Follow-up" recording ts-0005 A/2 as the residual miss, its cause, and this fix. Then re-derive all six arm A runs again (same throwaway method as brief 1 section 7) and put the resulting table in the notes, replacing nothing, as a dated addendum.

## 6. Out of scope

`claims.txt`, `abstain.txt`, `ParseSummary`, the decision table (`no_test_run` stays contradicted; it is right when no test command ran at all).

## 7. Report

Commit hash, the re-derivation table, the corpus precision line, anything left out.
