# Bench results

One directory per run, each with its own `NOTES.md` recording what the
run showed and what was changed because of it. Archives are evidence:
they are never re-graded when a rule changes, and a correction is added
to the notes rather than applied to the numbers.

Two commands produce them, both from any shell once the owner has run
`bash scripts/bench-approve.sh` once from a plain terminal (ADR 0010):

    BUDGET_USD=60 bash scripts/bench-pilot.sh          # tasks 1-20, K=5, Opus 5, pre-registered
    BUDGET_USD=8  bash scripts/bench-dev.sh 21-40      # one pass, Sonnet, for finding defects

Both refuse before spending anything if the corpus is not approved for
the binary they just built, or if `BUDGET_USD` is below the runner's own
estimate, and both print a provenance head worth pasting whole.
