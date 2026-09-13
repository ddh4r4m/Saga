# Bench results

One directory per run, each with its own `NOTES.md` recording what the
run showed and what was changed because of it. Archives are evidence:
they are never re-graded when a rule changes, and a correction is added
to the notes rather than applied to the numbers.

Two commands produce them, both from any shell once the owner has run
`bash scripts/bench-approve.sh` once from a plain terminal (ADR 0010):

    BUDGET_USD=60 bash scripts/bench-pilot.sh          # tasks 1-20, K=5, Opus 5, pre-registered
    BUDGET_USD=8  bash scripts/bench-dev.sh 21-40      # one pass, Sonnet, for finding defects
    BUDGET_USD=20 bash scripts/bench-explore.sh        # exploration, Haiku 4.5, tier user, never a result

All three refuse before spending anything if the corpus is not approved
for the binary they just built, or if `BUDGET_USD` is below the runner's
own estimate, and all print a provenance head worth pasting whole.

`bench-explore.sh` is not the experiment. The experiment closed on
2026-09-13 (docs/12 §13) and its result is `pilot-2026-09-13/`; an
exploration cell exercises the bench on a cheap model and its numbers
may not be quoted as results. It runs at tier `user`, so the runner's
20 usd cap applies by construction, it writes a `PURPOSE.txt` saying all
of this into its own archive, and its provenance head carries a
`purpose:` line.
