Reproductions for the 2026-09-06 oracle-integrity finding: a workspace can forge
the oracle's exit code, so the old `Pass = exit == 0` scored a solve for no work.
Run any script from the repository root; each stages a corpus task into a temp
directory, applies one tamper and prints the oracle's own verdict.

These print the raw per-task oracle result, which still exits 0 under the tamper:
that is the finding, not a regression. The grader now refuses such a run through
the integrity probe, which is covered by TestOracleRejectsTamperedPass.
