# Cross-spec review log

*2026-09-03. Consistency pass over bench, trace, gate, guard, index, mem, shape and route against doc 09 §3, §5 and ADRs 0001 to 0007. Shared contracts: `00-cross-spec-contracts.md`. Doc 09 bumped to v0.4.*

## Found and resolved

| # | Inconsistency | Resolution |
|---|---|---|
| 1 | route's `route_decision` event and `route` ledger key missing from trace | trace §2.2, §2.8, §3.2; `route_unapplied` folded into `route_decision.outcome` |
| 2 | shape's `shaped` object and `tool_output_bytes_raw_turn` missing from trace | added, trace §2.2, §3.2 |
| 3 | Hook ownership: shape proposed one `saga hook` (guard → shape → gate); guard said "both hooks run, stricter wins"; gate, mem, trace, route each installed their own | one composed entry per event, order and merge rules in contracts §1; all adapter sections defer to it |
| 4 | mem and route both write `PreToolUse` `updatedInput` on `Agent` | field-wise merge; same field twice is exit 2 |
| 5 | Exit codes 3 to 6 meant different things per layer | uniform table, contracts §4; bench 4→6, 6→7; guard 5↔6; route 4→6, 5→4 |
| 6 | Eight schema ids broke `saga.<layer>.<thing>/1` | renamed; registry in contracts §11 |
| 7 | Four snapshot stores (guard refs, trace overlay, shape pre-image copies, gate scratch copy), two id formats | one `saga snapshot` primitive; `tree_hash` shared with gate's `worktree_hash` |
| 8 | Two masking placeholder formats | guard's everywhere |
| 9 | shape wrote blake3 into trace `edit` events that require sha256 | sha256 cross-layer; blake3 opaque ids only |
| 10 | bench `run.json` usage object differed from trace's | bench adopts trace §3.1 |
| 11 | Drift ids hyphenated in bench §5.9, underscored elsewhere | underscores |
| 12 | Layer ablations cited metrics bench §5 never defined | bench §5.11 registers layer-declared metrics |
| 13 | Token caps: trace and route drew on gate's 1,000; mem had its own; guard had none | per-layer shares summed to 3,800, contracts §7.3 |
| 14 | `SIDE-EFFECTS:` removed by gate v0.2 but required by index §8, bench §2.1, doc 09 §3.2 | guard `[net] fetch` policy instead |
| 15 | Guard class names misquoted in route and shape | corrected; `volatile` is shape's own list |
| 16 | Four `init` commands each edited `.gitignore` | `saga init` writes `.saga/.gitignore` |
| 17 | Gate config keys top-level; `[trace.budget]` editable by the agent | `[gate]` table; `[gate]` and `[trace.*]` read from `BASE:` |
| 18 | Agent could run `saga trace ack`, `saga mem confirm`, `saga route policy trust` | shared forbidden list, contracts §8, guard D11 |
| 19 | LSP tier gate named two of route's four tiers | `small`, `local` on; `frontier`, `standard` off |
| 20 | route tiers named models absent from the price table; `open_weight` undefined | table covers all; tag added |
| 21 | index claimed a `PreCompact` stub-rewriter; mem verified `PreCompact` cannot add context | removed |
| 22 | route raised a canary verdict outside trace's closed set | `budget` event instead |
| 23 | Schema repair assigned to guard by doc 09, specified nowhere | guard §12 deferred contract |
| 24 | Turn id defined three ways | one counter in `session-<id>.json` |

## Open risks for a human decision

1. **Resolved 2026-09-03.** Trace owns claim verification: trace-spec §5.5 to §5.9 (claim detection, evidence reconciliation, claim-versus-diff, the `gate` `kind: claim` event, verdict and exit mapping, `claim_contradiction_rate`); gate-spec §6 cites the verdict at Stop (`contradicted` blocks; `unverified` warns in minimal mode, blocks in full mode); contracts §1, §4, §6, §7.3, §11 and bench-spec §5.4, §5.11 amended.
2. **Resolved 2026-09-03 (harness-facts sprint, `harness-facts.md` C4, C5).** `PreToolUse` `additionalContext` is verified on Claude Code 2.1.258; the "debug log only" sentence concerns plain-text stdout, not the JSON field, so the pages never disagreed. `PostToolUse` and `PostToolBatch` `additionalContext` are verified too. M3 stratifies by placement for effect size only.
3. **Resolved 2026-09-03 (harness-facts X8, X10, X11, X14).** Codex `PreToolUse` `updatedInput` is verified (with `permissionDecision: "allow"`); `PostToolUse` `decision: block` replaces the tool result rather than merely annotating it, so gate, guard and shape emit it only on a finding; there is no Codex-side cap on consecutive Stop blocks (verified in `turn.rs`), so `max_blocks` is the only cap. New finding: `permissionDecision: "ask"` is unsupported on Codex `PreToolUse` and the call proceeds, so `ask` collapses to `deny` there.
4. **The 3,800-token injected budget is a sum of priors.** If too small on long sessions, the cap-breach rule fails runs.
5. **Snapshot cost on monorepos** now sits inside every PreToolUse entry through the single primitive; `on_budget = "ask"` reintroduces prompts.
6. **Exit-code renumbering** touches bench and guard fixture tables; cheap now, expensive after code exists.
7. **Resolved 2026-09-03.** Em-dashes removed mechanically from bench-spec.
8. **Resolved 2026-09-03.** Contract header `RISK: impossible` (gate-spec §2.2 grammar, §2.3, §2.6 row 19, §7.2 `risk` and `risk_removed`); route-spec §2.1 and §2.2 read `risk_impossible` from gate status; `RED: control` is a red-proof mode only.
9. **New 2026-09-03: fail-open modes found by the harness-facts sprint.** A timed-out `PreToolUse` command hook allows on Claude Code (C23); non-JSON stdout allows on Gemini (G11); an unsupported `ask` allows on Codex (X10); an untrusted or changed project hook never fires on Codex (X3) and re-prompts on Gemini (G14). Contracts §1.1 now mandates an entry-side deadline with `deny` on overrun, single-JSON-object stdout, and a `saga doctor` canary for hook trust. Also contradicted and fixed: Claude Code `PostToolUse` can replace results (`updatedToolOutput`, C7), `PostCompact` has no context channel (C16), `SubagentStart` can inject context (C17), and hooks run outside the sandbox on Claude Code and Codex (C27, X21).

## Applied fixes (2026-09-03, from docs/11-red-team-review.md §4 and argument 5; factual corrections only, no scope, milestone or architecture change)

- Doc 01 one-sentence: "demonstrably got it right" is now "demonstrably passed its declared oracles" (red-team #1).
- Doc 09 §1 and doc 10: "a tenth of what they claim" is now "between a seventh and a half", with caveman and ponytail's claimed and measured figures split by metric (#2).
- Doc 09 §1: index row adds the agentic-grep comparator (45.3%, p=0.087), "one model, one study", and moves the +3.5 to +37% figure to type-constrained decoding per doc 05 §6 (#3, #10).
- Doc 09 §1: "+8.8 pp" is now "+8.8 pp among intervened runs, population effect unstated (preprint)" (#4).
- Doc 09 §1: "co-equal with the weights" row retitled; Terminal-Bench gap labelled secondary and unverified, SWE-bench Pro gap noted as differing in split and effort (#5).
- Doc 09 §7 item 7, ADR 0006 and contracts §5 now agree: git-tree is the default on every filesystem including APFS, zfs and btrfs are constant-time only for a dedicated dataset or subvolume, APFS volume snapshots are never used; ADR and doc 09 defer to contracts §5. guard-spec §3.1's sentence quoting ADR 0006's old "APFS snapshot" wording was left untouched (another agent owns the specs) and is now a stale reference to fix (#6).
- Doc 01 principle 8, doc 06 C.2 and doc 09 §3.4 now quote one secret-leak figure: GitGuardian State of Secrets Sprawl 2026, Claude-Code-co-authored commits at about 2× baseline, as doc 04 §2.5 states it; the Snyk and dev.to "~40%" figure is dropped. Doc 04's "(3.2%)" parenthetical is ambiguous about whether 3.2% is the AI-assisted rate or the baseline; confirm against the report before quoting externally (#7).
- Doc 01 and doc 09 §1 closing paragraph: "shipped by no harness across more than one product" is now "shipped natively by at most one harness; ledger, canary and post-expansion validation by none" (#8).
- Doc 09 §3.1: the under-$20 user run is now "a directional, unbadged reading" with the `user` tier's K=3 and about 11 pp standard error stated (#9). bench-spec §1.1 line 13 carries the same overstatement and was not edited (outside the allowed spec sections).
- Doc 09 §3.4: package-recurrence figure reconciled with doc 03 §2.7 and doc 06 C.4: 58% recur more than once across 10 runs, 43% on all 10 (red-team §4 footnote).
- Doc 09 §1 "Prompt text decays" row adds the ETH finding that human-written files did not help in Claude Code (red-team §4 footnote).
- Doc 09 §1 gains a "Source grade" column (primary-measured / vendor / secondary / unverified) taken from docs 03 to 07's own labels, plus a legend; 5 of 18 rows are vendor or secondary in whole or part, 1 carries an unverified figure (Khare decay curve).
- bench-spec §4.4 cost column corrected from the spec's own run counts: smoke $5-20, dev $450-1,250 per language, publish $5,500-15,000 for three arms and $13,500-36,400 for the full ladder, with the arithmetic shown under the table; doc 09 §5 M0 row states canary cost ($17 daily, $51 per burst at $0.85; $500 to $5,500 per month) and §5 ends with the cost note. Doc 10 updated to match.
- Doc 10 thesis paragraph now names the variance-decomposition result as one small preprint experiment.
- Not done, by instruction: README wording, scope cut, milestones, and any spec section other than bench-spec §4.4 and contracts §5.
