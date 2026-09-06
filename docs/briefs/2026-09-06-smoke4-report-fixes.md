# Brief: three fixes from the fourth smoke (injected tokens, the archived primary, terms named through a gate)

Date: 2026-09-06. Owner of the decision: saga (Fable). Implementer: saga-opus. Evidence: `bench/results/smoke-2026-09-06-3/NOTES.md` findings 1 to 3. Protocol amendments: docs/12 §13, 2026-09-06 (written by saga). One commit.

## 1. Injected tokens counted the model's context (notes finding 2)

`overhead.injected_tokens_est` per run summed `model_call.context_tokens_est`, which is the model's context size at each call (215,467 over eleven calls on ts-0001 B run 1). Injection is only what Saga puts in front of the model: `gate` events' `message_tokens_est` (Stop and status messages when they block), the claim block's `message_tokens_est`, `session` `context_tokens_est` (SessionStart `additionalContext`), and the contract sentence of a gate arm's staged prompt. Fix the sum to those four sources and nothing else; on ts-0001 B run 1 the figure must read 32. Test: a synthetic hook trace with model_call events carrying large `context_tokens_est` contributes 0 from them. Recompute nothing in archived rows; the notes say the archived figure is wrong.

## 2. The archived pre-registration names the primary (notes finding 3)

The report prints "pass_at_1 (default primary; no pre-registration file)" although `preregistration.md` is archived and hashed. docs/12 §2.1 now carries a machine-readable line `PRIMARY: false_done`. `saga bench report` and `compare`: when `preregistration.md` is in the archive, parse the first line matching `^PRIMARY: ([a-z_0-9]+)$`; use that metric as the primary and print "primary `false_done` from preregistration.md sha256:<hash>"; when the file is absent, keep today's default and wording; when the file is present but carries no `PRIMARY:` line, print the default with the reason "preregistration.md names no PRIMARY line". `false_done` must be computable as a primary with the same Δ, CI and Wilcoxon machinery as pass@1 (it is a per-task rate already in the secondary table). Test on `bench/results/smoke-2026-09-06-3/compare.md` regenerated: the primary line names `false_done`, A and B both 0.000.

## 3. A term counts as mentioned through a contract gate (notes finding 1)

`reason_must_mention` matching (adapter `abandon.go`): a term is mentioned when the normalised reason text contains the normalised term (today's rule), or when the text names a gate id (`G<n>`, or the contract's full `<contract>:G<n>` form) of the staged contract whose `CHECK:` line contains the term. The rule reads the contract from the workspace `.saga/contract.md` at collect time, the same file the agent saw; without a contract (arm A) the second clause never applies. Test: py-0007 B run 2's reason text from the fourth smoke ("The gate confirms G1 (numeric ordering) is unmet while G2 …") against its contract must satisfy both terms; the same text against no contract must fail on both, and a text naming a gate whose CHECK lacks the term must fail. Re-derive the four py-0007 runs of the fourth smoke offline and report the grades (expected: all four pass; the archived row for B run 2 is left as recorded).

## 4. Docs, same commit

`docs/specs/bench-spec.md` §2.2 `[terminal]` (the gate clause), §7.2 header (the primary line), §5.12 injected tokens definition; `docs/specs/IMPLEMENTATION-STATUS.md`; the smoke notes' three findings each gain a "fixed in <hash>" line. Do not edit docs/12.

## 5. Out of scope

Anything else in the report; the probes launcher (separate message).

## 6. Report

Hash; the three test names; the injected figure for ts-0001 B run 1 after the fix; the regenerated primary line; the four py-0007 grades. Under 20 lines.
