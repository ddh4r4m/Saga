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

1. **Claim verification has no owner.** Doc 09 §3.2 and M1 promise it; gate v0.2 omits it; trace reserves `turn.claimed_done` and `gate.kind = claim`. Proposal: trace computes the three deterministic checks, gate's Stop step cites them. Needs a spec section before M1.
2. **Claude Code `PreToolUse` `additionalContext` is contradicted across two vendor pages.** Mem's targeted placement depends on it; M3 must stratify by placement.
3. **Codex `PostToolUse` prevention, Stop cap and `updatedInput` are unverified.** Without them guard masking and shape wrapping fall back to wrapper mode on Codex.
4. **The 3,800-token injected budget is a sum of priors.** If too small on long sessions, the cap-breach rule fails runs.
5. **Snapshot cost on monorepos** now sits inside every PreToolUse entry through the single primitive; `on_budget = "ask"` reintroduces prompts.
6. **Exit-code renumbering** touches bench and guard fixture tables; cheap now, expensive after code exists.
7. **bench-spec keeps 21 em-dashes** against house style; untouched, the pass was semantic.
8. **`RED: control` doubles as route's `impossible_risk` proxy;** should be a tag, not an overloaded field.
