# Retention

Snapshots are taken nightly per volume and named `bk-<volume>-<date>`. The scheduler runs `selectForDeletion` once a day and deletes what it returns, so the function is the only thing standing between a policy typo and data loss; it must be conservative and deterministic.

Both rules count distinct calendar units, newest first, and the current unit counts: with `keepWeekly: 4` on a Wednesday, this week and the three before it each keep their newest snapshot. Daily and weekly are independent; a snapshot can be kept by both.
