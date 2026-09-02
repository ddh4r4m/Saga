# Review log: `saga gate` spec v0.1 → v0.2

*Fable, 2026-09-02. Reviewed as if implementing next week. Harness facts verified the same day against code.claude.com/docs/en/hooks, geminicli.com/docs/hooks, and the Codex config and hooks reference.*

## Changed, and why

| Area | v0.2 | Reason |
|---|---|---|
| Modes | minimal mode (title, `IN:`, `CHECK`/`EXPECT`) in 12 lines; everything else opt-in | unlazy ceremony scales badly downward (doc 08 §2.6) |
| `REQUEST:`/`FROM:` | required only when `REQUEST:` present | hand-written contracts have no request to hash |
| `EVIDENCE:` | absent ≡ pending; checker writes it | fewer agent lines, same semantics |
| `SIDE-EFFECTS:` | removed | `network`, `write-outside-repo` unobservable from a diff: unenforced text |
| Grammar | blanks between headers; unexpressible rules listed; both examples parse | v0.1 example failed its own EBNF |
| Manual gates | `saga gate attest` writes an `attested` record | no path to met existed |
| `contract_hash` | `EVIDENCE:` lines removed, `[x]` normalised | checker's own write changed the hash it recorded |
| Baseline red | run by `check` while `git diff BASE -- IN` is empty | `init` executed unapproved code; fresh worktree lacks deps |
| Wrong-reason red | exit 126/127, timeout, missing-module patterns rejected | a broken environment "proved" every gate |
| `control` mode | pass iff RED-CHECK exits 0, matches RED-EXPECT, does not match EXPECT | table contradicted example |
| Mutation miss | `unproven (mutation not observed)`, experimental | cannot separate tautology from gate observing another part |
| Regex | linear-time RE2 dialect, no worker | removes ReDoS and a subsystem |
| G-LEDGER | content validation of the store | checker's own writes were in the diff |
| Trust | committed records are a cache; CI `reverify` is the anchor | agent and checker are one OS user |
| Config | read from `BASE:` via `git show` | `scope_exempt = ["**"]` was one edit away |
| CI approvals | `--ci`; secret-less runner is the isolation | committed approval file was agent-writable |
| Claude Code | PostToolUse is feedback only; `Bash` matcher added; Stop cap 8 noted | docs: PostToolUse cannot block; Edit hooks skip Bash rewrites |
| Codex | native hooks with blocking `Stop`; unverified items listed | v0.1 `notify`-only was stale |
| Gemini | `BeforeTool`/`AfterTool`/`AfterAgent`; AfterTool block replaces result | docs |
| Self-approval | PreToolUse denies `approve`/`attest` strings from the agent | closes the obvious path where a hook exists |
| Stop reason | ids and states only | outcome text is repo-controlled |
| Token cap | 1,000 injected/session; diagnostics per call; `bytes/4` estimate | three failing checks exhausted the old cap; needed a measurable unit |
| Exit precedence | 6, 2, 3, 4, 5, 1 | 6 vs 2 was undefined |
| Ablation | `DONE`/`NOT-DONE` last line; 9,000 runs and pilot declared | "reported completion" was unmeasurable in arm A |
| Traceability | §1.4 evidence map, §11 experimental register | every mechanism traces or is flagged |
| Style | no em-dashes | house style |

## Rejected

- Signing local evidence: same principal holds the key.
- `SIDE-EFFECTS:` as declared-not-enforced: ADR 0002 forbids text-only rules.
- Typed zero-value stub operator for all seven languages at M1: needs type inference.
- Indefinite block on parse error: fail-closed kept, loop guard still releases.

## Open risks

1. Baseline is available only before the first edit; late `check` lands on experimental `mutation` or needs `control`. Bench should report baseline-miss rate.
2. Codex PostToolUse prevention and Stop-block cap unverified; adapter treats them as config.
3. G-ASSERT regex-degraded for four of seven languages until M2.
4. Default witnesses cover only path tokens in `CHECK:`; a script not on the command line escapes.
5. CI boundary depends on repo hygiene (no secrets, protected workflow dir).
6. 9,000-run ablation is expensive; pilot may shrink the M1 language set.
